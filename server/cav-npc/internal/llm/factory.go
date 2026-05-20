package llm

import (
	"context"
	"fmt"
	"os"

	"github.com/anthropic-cav/cav-npc/internal/config"
)

// Build constructs a fully-decorated Provider from NPC configuration.
// The returned Provider has retry logic and budget enforcement applied.
//
// Decoration chain: openaiCompat → WithRetry → WithBudget
func Build(cfg config.LLMConfig, budget *Budget) (Provider, error) {
	// Resolve API key from environment
	apiKey := os.Getenv(cfg.APIKeyEnv)
	if apiKey == "" {
		return nil, fmt.Errorf("llm: environment variable %q is not set", cfg.APIKeyEnv)
	}

	// Create base provider (all three use OpenAI-compatible API)
	base := NewOpenAICompat(cfg.Provider, cfg.Endpoint, apiKey, cfg.Model)

	// Apply retry decorator
	withRetry := WithRetry(base)

	// Apply budget decorator
	withBudget := WithBudgetCheck(withRetry, budget)

	return withBudget, nil
}

// budgetProvider wraps a Provider with budget acquisition and recording.
type budgetProvider struct {
	inner  Provider
	budget *Budget
}

// WithBudgetCheck wraps a Provider to enforce budget limits before each call
// and record token usage after each successful call.
func WithBudgetCheck(p Provider, b *Budget) Provider {
	return &budgetProvider{inner: p, budget: b}
}

func (bp *budgetProvider) Name() string { return bp.inner.Name() }

func (bp *budgetProvider) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	// Acquire budget slot (blocks on rate limiter, fails if paused)
	if err := bp.budget.Acquire(ctx); err != nil {
		return CompletionResponse{}, err
	}

	// Execute LLM call
	resp, err := bp.inner.Complete(ctx, req)
	if err != nil {
		return resp, err
	}

	// Record actual token usage
	bp.budget.Record(resp.PromptTokens + resp.CompletionTokens)

	return resp, nil
}
