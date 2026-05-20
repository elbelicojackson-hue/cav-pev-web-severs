package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/anthropic-cav/cav-npc/internal/signal"
	"github.com/gorilla/websocket"
)

// Filter specifies which signals an NPC subscribes to.
type Filter struct {
	Types []string `json:"types,omitempty"`
	Tags  []string `json:"tags,omitempty"`
}

// StreamClient manages a WebSocket connection to the gateway's /v1/stream endpoint.
// It handles reconnection with exponential backoff and gap detection.
type StreamClient struct {
	base   string // gateway base URL
	auth   *AuthClient
	filter Filter
	out    chan<- *signal.EntropicSignal

	mu   sync.Mutex
	seqs map[string]uint64 // sender → last seen sequence number
}

// NewStreamClient creates a stream client that pushes received signals to out.
func NewStreamClient(gatewayURL string, auth *AuthClient, filter Filter, out chan<- *signal.EntropicSignal) *StreamClient {
	return &StreamClient{
		base:   gatewayURL,
		auth:   auth,
		filter: filter,
		out:    out,
		seqs:   make(map[string]uint64),
	}
}

// Run connects to the gateway WebSocket and reads signals until ctx is cancelled.
// On disconnect, it reconnects with exponential backoff (1s..60s, ±20% jitter).
// This method blocks until ctx is done.
func (s *StreamClient) Run(ctx context.Context) error {
	attempt := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		err := s.connectAndRead(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Connection dropped — reconnect with backoff
		attempt++
		delay := s.backoffDelay(attempt)
		slog.Warn("websocket disconnected, reconnecting",
			"error", err,
			"attempt", attempt,
			"delay", delay,
		)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

// connectAndRead establishes a single WebSocket connection and reads until error.
func (s *StreamClient) connectAndRead(ctx context.Context) error {
	token, err := s.auth.Token(ctx)
	if err != nil {
		return fmt.Errorf("get token for stream: %w", err)
	}

	// Build WebSocket URL
	wsURL := s.wsURL(token)

	// Dial
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("websocket dial: %w", err)
	}
	defer conn.Close()

	// Set pong handler for keepalive (R3.4: respond within 10s)
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Send filter message (R3.2)
	filterMsg := map[string]any{
		"action": "filter",
		"types":  s.filter.Types,
		"tags":   s.filter.Tags,
	}
	if err := conn.WriteJSON(filterMsg); err != nil {
		return fmt.Errorf("send filter: %w", err)
	}

	// Start ping ticker
	pingDone := make(chan struct{})
	go func() {
		defer close(pingDone)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Connection successful — reset backoff will happen in caller
	slog.Info("websocket connected", "url", s.base+"/v1/stream")

	// Read loop
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read message: %w", err)
		}

		// Parse signal
		var sig signal.EntropicSignal
		if err := json.Unmarshal(message, &sig); err != nil {
			slog.Debug("ignoring unparseable ws message", "error", err)
			continue
		}

		// Gap detection (R3.5-6)
		s.checkGap(ctx, &sig)

		// Push to output channel
		select {
		case s.out <- &sig:
		default:
			// Channel full — drop oldest semantics handled by caller's channel
			slog.Warn("stream output channel full, dropping signal", "signal_id", sig.ID)
		}
	}
}

// checkGap detects sequence gaps and triggers recovery.
func (s *StreamClient) checkGap(ctx context.Context, sig *signal.EntropicSignal) {
	if sig.From == "" || sig.Sequence == 0 {
		return
	}

	s.mu.Lock()
	lastSeq, known := s.seqs[sig.From]
	s.seqs[sig.From] = sig.Sequence
	s.mu.Unlock()

	if known && sig.Sequence > lastSeq+1 {
		// Gap detected — recover asynchronously
		gap := sig.Sequence - lastSeq - 1
		slog.Warn("signal gap detected",
			"sender", sig.From,
			"expected_seq", lastSeq+1,
			"got_seq", sig.Sequence,
			"gap_size", gap,
		)
		go s.recoverGap(ctx, sig.From, lastSeq)
	}
}

// recoverGap fetches missed signals via GET /v1/signals (R3.5).
func (s *StreamClient) recoverGap(ctx context.Context, sender string, sinceSeq uint64) {
	token, err := s.auth.Token(ctx)
	if err != nil {
		slog.Error("gap recovery: get token failed", "error", err)
		return
	}

	url := fmt.Sprintf("%s/v1/signals?sender=%s&since_seq=%d", s.base, sender, sinceSeq)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("gap recovery: request failed", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error("gap recovery: non-200 response", "status", resp.StatusCode)
		return
	}

	var signals []*signal.EntropicSignal
	if err := json.NewDecoder(resp.Body).Decode(&signals); err != nil {
		slog.Error("gap recovery: decode failed", "error", err)
		return
	}

	for _, sig := range signals {
		select {
		case s.out <- sig:
		case <-ctx.Done():
			return
		}
	}

	slog.Info("gap recovery complete", "sender", sender, "recovered", len(signals))
}

// wsURL builds the WebSocket URL from the gateway base URL.
func (s *StreamClient) wsURL(token string) string {
	base := s.base
	base = strings.Replace(base, "https://", "wss://", 1)
	base = strings.Replace(base, "http://", "ws://", 1)
	return base + "/v1/stream?token=" + token
}

// backoffDelay computes exponential backoff with ±20% jitter.
// Formula: min(60s, 1s * 2^attempt) * (1 + rand(-0.2, +0.2))
func (s *StreamClient) backoffDelay(attempt int) time.Duration {
	base := math.Min(60.0, math.Pow(2, float64(attempt-1)))
	jitter := 1.0 + (rand.Float64()*0.4 - 0.2) // [0.8, 1.2]
	return time.Duration(base*jitter*1000) * time.Millisecond
}
