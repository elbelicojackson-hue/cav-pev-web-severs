package llm

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"net"
	"time"
)

// retryProvider wraps a Provider with exponential backoff retry logic.
// Retry conditions (R4.3): HTTP 408, 429, 500, 502, 503, 504, network errors.
// Non-retryable (R4.4): HTTP 400, 401, 403, 404, 422.
// Backoff: 1s → 4s → 12s with ±20% jitter, max 3 retries.
type retryProvider struct {
	inner      Provider
	maxRetries int
}

// WithRetry wraps a Provider with retry logic per design §3.2.
func WithRetry(p Provider) Provider {
	return &retryProvider{inner: p, maxRetries: 3}
}

func (r *retryProvider) Name() string { return r.inner.Name() }

func (r *retryProvider) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if attempt > 0 {
			delay := r.backoff(attempt)
			select {
			case <-ctx.Done():
				return CompletionResponse{}, ctx.Err()
			case <-time.After(delay):
			}
		}

		resp, err := r.inner.Complete(ctx, req)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		if !isRetryable(err) {
			return CompletionResponse{}, err
		}

		// Context cancelled — don't retry
		if ctx.Err() != nil {
			return CompletionResponse{}, ctx.Err()
		}
	}

	return CompletionResponse{}, lastErr
}

// backoff computes delay for the given attempt (1-indexed).
// Formula: base * 2^(attempt-1) * jitter, where base=1s, jitter ∈ [0.8, 1.2]
// Sequence: ~1s, ~4s, ~12s (capped implicitly by maxRetries=3)
func (r *retryProvider) backoff(attempt int) time.Duration {
	// 1s, 4s, 12s (geometric with factor ~3-4)
	bases := []float64{1.0, 4.0, 12.0}
	var base float64
	if attempt-1 < len(bases) {
		base = bases[attempt-1]
	} else {
		base = math.Min(30.0, 12.0*math.Pow(2, float64(attempt-3)))
	}
	jitter := 0.8 + rand.Float64()*0.4 // [0.8, 1.2]
	return time.Duration(base*jitter*1000) * time.Millisecond
}

// isRetryable determines if an error should trigger a retry.
func isRetryable(err error) bool {
	// Check for our HTTPError type
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.IsRetryable()
	}

	// Network errors are retryable
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	// Context errors are not retryable
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Unknown errors — don't retry (conservative)
	return false
}
