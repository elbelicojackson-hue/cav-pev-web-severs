// Package llm provides a unified interface for LLM providers.
// All three supported providers (DeepSeek, Volcengine, DashScope) use
// the OpenAI Chat Completions compatible API format.
package llm

import "context"

// Provider is the interface for LLM completion backends.
type Provider interface {
	// Name returns the provider identifier (e.g. "deepseek", "volcengine", "dashscope").
	Name() string

	// Complete sends a chat completion request and returns the response.
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
}

// CompletionRequest is the input to an LLM completion call.
type CompletionRequest struct {
	System      string  // system message content
	User        string  // user message content
	MaxTokens   int     // max tokens to generate
	Temperature float64 // sampling temperature
	JSONMode    bool    // force JSON output (response_format: json_object)
}

// CompletionResponse is the output from an LLM completion call.
type CompletionResponse struct {
	Content          string // generated text
	PromptTokens     int    // tokens in the prompt
	CompletionTokens int    // tokens generated
	LatencyMs        int64  // round-trip latency in milliseconds
	Model            string // model that was used
}
