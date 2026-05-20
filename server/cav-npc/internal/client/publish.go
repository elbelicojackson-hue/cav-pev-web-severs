package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/anthropic-cav/cav-npc/internal/signal"
)

// Publisher handles outbound API calls to the gateway for broadcasting signals,
// sending heartbeats, and submitting behavioral digests.
type Publisher struct {
	gw *GatewayClient
}

// NewPublisher creates a Publisher using the given gateway client.
func NewPublisher(gw *GatewayClient) *Publisher {
	return &Publisher{gw: gw}
}

// Broadcast publishes an EntropicSignal to the gateway via POST /v1/broadcast.
func (p *Publisher) Broadcast(ctx context.Context, sig *signal.EntropicSignal) error {
	body, err := json.Marshal(sig)
	if err != nil {
		return fmt.Errorf("publish: marshal signal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.gw.BaseURL+"/v1/broadcast", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.gw.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("publish: broadcast request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return parseHTTPError(resp)
	}
	return nil
}

// HeartbeatBody is the request body for POST /v1/agent/heartbeat.
// Matches cav-gateway/internal/agent.HeartbeatRequest schema.
type HeartbeatBody struct {
	Capabilities *Capabilities `json:"capabilities,omitempty"`
	Status       string        `json:"status,omitempty"` // "idle" | "working" | "blocked"
	Note         string        `json:"note,omitempty"`   // ≤256 chars
}

// Capabilities declares the NPC's capabilities to the gateway.
type Capabilities struct {
	HypothesisKinds []string `json:"hypothesis_kinds,omitempty"`
	Tools           []string `json:"tools,omitempty"`
	Languages       []string `json:"languages,omitempty"`
	Description     string   `json:"description,omitempty"`
	Nickname        string   `json:"nickname,omitempty"`
}

// HeartbeatResponse is the gateway's heartbeat acknowledgement.
type HeartbeatResponse struct {
	OK          bool   `json:"ok"`
	ServerTime  string `json:"server_time"`
	State       string `json:"state"`
	PeersOnline int    `json:"peers_online"`
	InboxCount  int    `json:"inbox_count"`
}

// Heartbeat sends a heartbeat to the gateway via POST /v1/agent/heartbeat.
func (p *Publisher) Heartbeat(ctx context.Context, hb HeartbeatBody) (*HeartbeatResponse, error) {
	body, err := json.Marshal(hb)
	if err != nil {
		return nil, fmt.Errorf("heartbeat: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.gw.BaseURL+"/v1/agent/heartbeat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.gw.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("heartbeat: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, parseHTTPError(resp)
	}

	var result HeartbeatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("heartbeat: decode response: %w", err)
	}
	return &result, nil
}

// BehavioralDigest is the signed hourly behavioral summary.
type BehavioralDigest struct {
	DID                       string  `json:"did"`
	PeriodStart               string  `json:"period_start"`
	PeriodEnd                 string  `json:"period_end"`
	VoteAlignmentWithMajority float64 `json:"vote_alignment_with_majority"`
	UniqueDomainsActive       int     `json:"unique_domains_active"`
	SignalDiversityEntropy    float64 `json:"signal_diversity_entropy"`
	SignalCount               int     `json:"signal_count"`
	VoteCount                 int     `json:"vote_count"`
	Signature                 string  `json:"signature"`
	PublicKey                 string  `json:"public_key"`
}

// SubmitDigest posts a signed behavioral digest to POST /v1/social/digest.
func (p *Publisher) SubmitDigest(ctx context.Context, digest *BehavioralDigest) error {
	body, err := json.Marshal(digest)
	if err != nil {
		return fmt.Errorf("digest: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.gw.BaseURL+"/v1/social/digest", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.gw.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("digest: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return parseHTTPError(resp)
	}
	return nil
}

// FetchContext calls GET /v1/agent/context to get the bootstrap snapshot.
// Returns the effective state of the citizen (used to determine probation/active).
type ContextResponse struct {
	ServerTime  string `json:"server_time"`
	Citizen     json.RawMessage `json:"citizen"`
	PeersOnline int    `json:"peers_online"`
	Heartbeat   int    `json:"heartbeat_interval_seconds"`
}

// FetchContext retrieves the agent context from the gateway.
func (p *Publisher) FetchContext(ctx context.Context) (*ContextResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.gw.BaseURL+"/v1/agent/context", nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.gw.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("context: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, parseHTTPError(resp)
	}

	var result ContextResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("context: decode: %w", err)
	}
	return &result, nil
}
