package client

import (
	"context"
	"net/http"
	"time"
)

// AuthTransport is an http.RoundTripper that injects JWT Authorization headers
// and retries once on 401 (triggering re-authentication).
type AuthTransport struct {
	auth *AuthClient
	base http.RoundTripper
}

// NewAuthTransport creates a transport that wraps the given base transport
// (or http.DefaultTransport if nil) with JWT injection.
func NewAuthTransport(auth *AuthClient, base http.RoundTripper) *AuthTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &AuthTransport{auth: auth, base: base}
}

// RoundTrip implements http.RoundTripper.
// It injects the Authorization header and retries once on 401.
func (t *AuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Get current token
	ctx := req.Context()
	token, err := t.auth.Token(ctx)
	if err != nil {
		return nil, err
	}

	// Clone request to avoid mutating the original
	req2 := req.Clone(ctx)
	req2.Header.Set("Authorization", "Bearer "+token)

	resp, err := t.base.RoundTrip(req2)
	if err != nil {
		return nil, err
	}

	// If 401, try re-authenticating once
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()

		// Force re-authentication
		newToken, err := t.auth.authenticate(ctx)
		if err != nil {
			// Return the original 401 response concept as error
			return nil, err
		}

		// Retry with new token
		req3 := req.Clone(ctx)
		req3.Header.Set("Authorization", "Bearer "+newToken)
		return t.base.RoundTrip(req3)
	}

	return resp, nil
}

// NewAuthenticatedClient creates an *http.Client with JWT injection and 401 retry.
// This is the standard client all NPC outbound calls should use.
func NewAuthenticatedClient(auth *AuthClient) *http.Client {
	return &http.Client{
		Transport: NewAuthTransport(auth, nil),
		Timeout:   30 * time.Second,
	}
}

// GatewayClient bundles the authenticated HTTP client with the gateway base URL.
// It provides convenience methods for common gateway API calls.
type GatewayClient struct {
	BaseURL string
	HTTP    *http.Client
	Auth    *AuthClient
}

// NewGatewayClient creates a fully configured gateway client.
func NewGatewayClient(gatewayURL string, auth *AuthClient) *GatewayClient {
	return &GatewayClient{
		BaseURL: gatewayURL,
		HTTP:    NewAuthenticatedClient(auth),
		Auth:    auth,
	}
}

// Do executes an HTTP request against the gateway with authentication.
func (g *GatewayClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	req = req.WithContext(ctx)
	return g.HTTP.Do(req)
}
