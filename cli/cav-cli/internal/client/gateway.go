// Package client provides HTTP client for the CAV Gateway API.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is the Gateway HTTP client.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

// New creates a new Gateway client.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL:    baseURL,
		Token:      token,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// ChallengeResponse from POST /v1/auth/challenge.
type ChallengeResponse struct {
	Nonce     string `json:"nonce"`
	ExpiresAt string `json:"expires_at"`
}

// VerifyResponse from POST /v1/auth/verify.
type VerifyResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
	Citizen   struct {
		DID   string `json:"did"`
		Level int    `json:"level"`
	} `json:"citizen"`
}

// GetChallenge requests a nonce for authentication.
func (c *Client) GetChallenge(did string) (*ChallengeResponse, error) {
	body, _ := json.Marshal(map[string]string{"did": did})
	resp, err := c.HTTPClient.Post(c.BaseURL+"/v1/auth/challenge", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("challenge failed (%d): %s", resp.StatusCode, string(b))
	}

	var result ChallengeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SubmitVerify submits the signed nonce to get a JWT.
func (c *Client) SubmitVerify(did, nonce, signature string) (*VerifyResponse, error) {
	body, _ := json.Marshal(map[string]string{
		"did":       did,
		"nonce":     nonce,
		"signature": signature,
	})
	resp, err := c.HTTPClient.Post(c.BaseURL+"/v1/auth/verify", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("verify failed (%d): %s", resp.StatusCode, string(b))
	}

	var result VerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// PublishPraxon publishes a Praxon to the network.
func (c *Client) PublishPraxon(praxonJSON []byte) (map[string]interface{}, error) {
	req, _ := http.NewRequest("POST", c.BaseURL+"/v1/praxon", bytes.NewReader(praxonJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if resp.StatusCode >= 400 {
		return result, fmt.Errorf("publish failed (%d)", resp.StatusCode)
	}
	return result, nil
}

// FetchPraxon fetches a Praxon by ID.
func (c *Client) FetchPraxon(id string) (map[string]interface{}, error) {
	resp, err := c.HTTPClient.Get(c.BaseURL + "/v1/praxon/" + id)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if resp.StatusCode >= 400 {
		return result, fmt.Errorf("fetch failed (%d)", resp.StatusCode)
	}
	return result, nil
}

// GetCitizens lists active citizens.
func (c *Client) GetCitizens() (map[string]interface{}, error) {
	resp, err := c.HTTPClient.Get(c.BaseURL + "/v1/citizens")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}

// GetStatus gets current identity info.
func (c *Client) GetStatus() (map[string]interface{}, error) {
	req, _ := http.NewRequest("GET", c.BaseURL+"/v1/auth/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}

// SubmitChallenge challenges a Praxon.
func (c *Client) SubmitChallenge(praxonID, reason string) (map[string]interface{}, error) {
	body, _ := json.Marshal(map[string]string{"reason": reason})
	req, _ := http.NewRequest("POST", c.BaseURL+"/v1/praxon/"+praxonID+"/challenge", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}
