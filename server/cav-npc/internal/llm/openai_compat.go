package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// openaiCompat implements Provider using the OpenAI Chat Completions API format.
// All three supported providers (DeepSeek, Volcengine/Ark, DashScope) are compatible.
type openaiCompat struct {
	name     string // provider name for metrics
	endpoint string // base URL (e.g. "https://api.deepseek.com/v1")
	apiKey   string
	model    string
	client   *http.Client
}

// NewOpenAICompat creates a Provider that speaks the OpenAI Chat Completions protocol.
func NewOpenAICompat(name, endpoint, apiKey, model string) Provider {
	return &openaiCompat{
		name:     name,
		endpoint: endpoint,
		apiKey:   apiKey,
		model:    model,
		client:   &http.Client{Timeout: 120 * time.Second},
	}
}

func (o *openaiCompat) Name() string { return o.name }

func (o *openaiCompat) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	start := time.Now()

	// Build request body
	messages := []chatMessage{
		{Role: "system", Content: req.System},
		{Role: "user", Content: req.User},
	}

	body := chatRequest{
		Model:       o.model,
		Messages:    messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}
	if req.JSONMode {
		body.ResponseFormat = &responseFormat{Type: "json_object"}
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("llm: marshal request: %w", err)
	}

	// Build HTTP request
	url := o.endpoint + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("llm: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)

	// Execute
	resp, err := o.client.Do(httpReq)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("llm: request failed: %w", err)
	}
	defer resp.Body.Close()

	latency := time.Since(start).Milliseconds()

	// Read response body (cap at 1MB for safety)
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("llm: read response: %w", err)
	}

	// Non-2xx → return error with status and partial body
	if resp.StatusCode != http.StatusOK {
		snippet := string(respBody)
		if len(snippet) > 512 {
			snippet = snippet[:512]
		}
		return CompletionResponse{}, &HTTPError{
			StatusCode: resp.StatusCode,
			Body:       snippet,
		}
	}

	// Parse response
	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return CompletionResponse{}, fmt.Errorf("llm: decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return CompletionResponse{}, fmt.Errorf("llm: no choices in response")
	}

	return CompletionResponse{
		Content:          chatResp.Choices[0].Message.Content,
		PromptTokens:     chatResp.Usage.PromptTokens,
		CompletionTokens: chatResp.Usage.CompletionTokens,
		LatencyMs:        latency,
		Model:            chatResp.Model,
	}, nil
}

// HTTPError represents a non-2xx response from the LLM provider.
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("llm: HTTP %d: %s", e.StatusCode, e.Body)
}

// IsRetryable returns true if the error is a retryable HTTP status.
func (e *HTTPError) IsRetryable() bool {
	switch e.StatusCode {
	case 408, 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

// --- OpenAI Chat Completions wire types ---

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"` // "json_object"
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	Temperature    float64         `json:"temperature"`
	ResponseFormat *responseFormat  `json:"response_format,omitempty"`
}

type chatResponse struct {
	ID      string         `json:"id"`
	Model   string         `json:"model"`
	Choices []chatChoice   `json:"choices"`
	Usage   chatUsage      `json:"usage"`
}

type chatChoice struct {
	Index   int         `json:"index"`
	Message chatMessage `json:"message"`
}

type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
