// Package client provides HTTP and WebSocket clients for communicating
// with the CAV gateway. All outbound requests are authenticated via JWT.
package client

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/anthropic-cav/cav-npc/internal/identity"
)

// AuthClient manages JWT authentication with the gateway.
// It handles the challenge-verify flow and proactive token refresh.
type AuthClient struct {
	base string // gateway base URL (e.g. "http://localhost:8421")
	http *http.Client
	kp   *identity.KeyPair

	mu        sync.RWMutex
	token     string
	expiresAt time.Time
	refreshAt time.Time // = expiresAt - 1h

	// refreshOnce ensures only one concurrent refresh happens
	refreshMu sync.Mutex
}

// NewAuthClient creates an AuthClient for the given gateway and key pair.
func NewAuthClient(gatewayURL string, kp *identity.KeyPair) *AuthClient {
	return &AuthClient{
		base: gatewayURL,
		http: &http.Client{Timeout: 30 * time.Second},
		kp:   kp,
	}
}

// Token returns a valid JWT, performing authentication if needed.
// It blocks until a token is available or the context is cancelled.
func (a *AuthClient) Token(ctx context.Context) (string, error) {
	// Fast path: check if current token is still valid
	a.mu.RLock()
	if a.token != "" && time.Now().Before(a.expiresAt) {
		tok := a.token
		a.mu.RUnlock()
		return tok, nil
	}
	a.mu.RUnlock()

	// Need to authenticate
	return a.authenticate(ctx)
}

// StartRefreshLoop runs a background goroutine that proactively refreshes
// the JWT before it expires (R2.4: within 1 hour of expiry).
// It stops when ctx is cancelled.
func (a *AuthClient) StartRefreshLoop(ctx context.Context) {
	go func() {
		for {
			a.mu.RLock()
			refreshAt := a.refreshAt
			a.mu.RUnlock()

			if refreshAt.IsZero() {
				// No token yet, wait a bit and retry
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
					continue
				}
			}

			// Wait until refresh time
			waitDur := time.Until(refreshAt)
			if waitDur <= 0 {
				waitDur = time.Second // Don't spin
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(waitDur):
			}

			// Proactive refresh — don't clear old token on failure
			if _, err := a.authenticate(ctx); err != nil {
				slog.Warn("proactive JWT refresh failed (old token still valid until expiry)",
					"error", err,
					"npc_did", a.kp.DID,
				)
			}
		}
	}()
}

// authenticate performs the challenge-verify flow.
// Only one authentication attempt runs at a time.
func (a *AuthClient) authenticate(ctx context.Context) (string, error) {
	a.refreshMu.Lock()
	defer a.refreshMu.Unlock()

	// Double-check: another goroutine may have refreshed while we waited
	a.mu.RLock()
	if a.token != "" && time.Now().Before(a.expiresAt) {
		tok := a.token
		a.mu.RUnlock()
		return tok, nil
	}
	a.mu.RUnlock()

	// Step 1: POST /v1/auth/challenge
	challengeResp, err := a.postChallenge(ctx)
	if err != nil {
		return "", fmt.Errorf("auth challenge: %w", err)
	}

	// Step 2: Sign the nonce
	nonceBytes, err := base64.RawURLEncoding.DecodeString(challengeResp.Nonce)
	if err != nil {
		return "", fmt.Errorf("auth: decode nonce: %w", err)
	}
	signature := ed25519.Sign(a.kp.PrivateKey, nonceBytes)
	signatureB64 := base64.RawURLEncoding.EncodeToString(signature)

	// Step 3: POST /v1/auth/verify
	verifyResp, err := a.postVerify(ctx, challengeResp.SessionID, signatureB64)
	if err != nil {
		return "", fmt.Errorf("auth verify: %w", err)
	}

	// Parse expiry
	expiresAt, err := time.Parse(time.RFC3339, verifyResp.ExpiresAt)
	if err != nil {
		// Fallback: assume 24h from now
		expiresAt = time.Now().Add(24 * time.Hour)
	}

	// Store token
	a.mu.Lock()
	a.token = verifyResp.Token
	a.expiresAt = expiresAt
	a.refreshAt = expiresAt.Add(-1 * time.Hour) // Refresh 1h before expiry (R2.4)
	a.mu.Unlock()

	slog.Info("authenticated with gateway",
		"npc_did", a.kp.DID,
		"expires_at", expiresAt.Format(time.RFC3339),
	)

	return verifyResp.Token, nil
}

// --- Gateway auth API types ---

type challengeRequest struct {
	PublicKey string `json:"public_key"`
	DID      string `json:"did"`
}

type challengeResponse struct {
	Nonce     string `json:"nonce"`      // base64url encoded
	SessionID string `json:"session_id"` // opaque session identifier
}

type verifyRequest struct {
	SessionID   string `json:"session_id"`
	NonceSigned string `json:"nonce_signed"` // base64url Ed25519 signature
}

type verifyResponse struct {
	Token     string `json:"token"`      // JWT
	ExpiresAt string `json:"expires_at"` // RFC3339
}

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (a *AuthClient) postChallenge(ctx context.Context) (*challengeResponse, error) {
	pubB64 := base64.RawURLEncoding.EncodeToString(a.kp.PublicKey)
	body, _ := json.Marshal(challengeRequest{
		PublicKey: pubB64,
		DID:      a.kp.DID,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", a.base+"/v1/auth/challenge", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, parseHTTPError(resp)
	}

	var result challengeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode challenge response: %w", err)
	}
	return &result, nil
}

func (a *AuthClient) postVerify(ctx context.Context, sessionID, signatureB64 string) (*verifyResponse, error) {
	body, _ := json.Marshal(verifyRequest{
		SessionID:   sessionID,
		NonceSigned: signatureB64,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", a.base+"/v1/auth/verify", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, parseHTTPError(resp)
	}

	var result verifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode verify response: %w", err)
	}
	return &result, nil
}

// parseHTTPError reads an error response body and returns a structured error.
func parseHTTPError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))

	var env errorEnvelope
	if json.Unmarshal(body, &env) == nil && env.Error.Code != "" {
		return fmt.Errorf("HTTP %d: %s — %s", resp.StatusCode, env.Error.Code, env.Error.Message)
	}

	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
}
