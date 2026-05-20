package llm

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// Budget errors
var (
	ErrPaused         = errors.New("llm: budget exhausted, calls paused")
	ErrRateLimited    = errors.New("llm: rate limit exceeded")
)

// BudgetStats exposes current budget state for the health endpoint.
type BudgetStats struct {
	HourlyUsed  int64 `json:"hourly_used"`
	DailyUsed   int64 `json:"daily_used"`
	MaxHourly   int64 `json:"max_hourly"`
	MaxDaily    int64 `json:"max_daily"`
	Paused      bool  `json:"paused"`
}

// Budget enforces per-NPC rate limiting and token budget caps.
// It tracks hourly and daily token usage and pauses calls when budgets are exhausted.
type Budget struct {
	perMinute *rate.Limiter

	hourlyTokens atomic.Int64
	dailyTokens  atomic.Int64
	maxHourly    int64
	maxDaily     int64
	paused       atomic.Bool

	// Reset channels (closed by background goroutine to signal reset)
	done chan struct{}
}

// NewBudget creates a Budget with the given limits.
// perMinute: max LLM calls per minute (R4.5, default 10)
// maxHourly/maxDaily: token caps (R4.7)
func NewBudget(perMinute int, maxHourly, maxDaily int64) *Budget {
	b := &Budget{
		perMinute: rate.NewLimiter(rate.Every(time.Minute/time.Duration(perMinute)), 1),
		maxHourly: maxHourly,
		maxDaily:  maxDaily,
		done:      make(chan struct{}),
	}
	go b.resetLoop()
	return b
}

// Acquire must be called before each LLM call.
// It blocks until the rate limiter allows, or returns an error if:
//   - Budget is paused (ErrPaused)
//   - Context is cancelled
func (b *Budget) Acquire(ctx context.Context) error {
	if b.paused.Load() {
		return ErrPaused
	}

	// Rate limit (calls per minute)
	if err := b.perMinute.Wait(ctx); err != nil {
		return ErrRateLimited
	}

	// Check if we'd exceed hourly budget
	if b.hourlyTokens.Load() >= b.maxHourly {
		b.paused.Store(true)
		return ErrPaused
	}

	return nil
}

// Record adds actual token usage after a successful LLM call.
// If this pushes over the hourly or daily limit, paused is set to true.
func (b *Budget) Record(tokens int) {
	newHourly := b.hourlyTokens.Add(int64(tokens))
	newDaily := b.dailyTokens.Add(int64(tokens))

	if newHourly >= b.maxHourly || newDaily >= b.maxDaily {
		b.paused.Store(true)
	}
}

// Stats returns current budget state for observability.
func (b *Budget) Stats() BudgetStats {
	return BudgetStats{
		HourlyUsed: b.hourlyTokens.Load(),
		DailyUsed:  b.dailyTokens.Load(),
		MaxHourly:  b.maxHourly,
		MaxDaily:   b.maxDaily,
		Paused:     b.paused.Load(),
	}
}

// IsPaused returns whether the budget is currently exhausted.
func (b *Budget) IsPaused() bool {
	return b.paused.Load()
}

// Close stops the background reset goroutine.
func (b *Budget) Close() {
	close(b.done)
}

// resetLoop runs in the background, resetting hourly and daily counters.
func (b *Budget) resetLoop() {
	hourTicker := time.NewTicker(1 * time.Hour)
	dayTicker := time.NewTicker(24 * time.Hour)
	defer hourTicker.Stop()
	defer dayTicker.Stop()

	for {
		select {
		case <-b.done:
			return
		case <-hourTicker.C:
			b.hourlyTokens.Store(0)
			// Unpause if daily budget still has room
			if b.dailyTokens.Load() < b.maxDaily {
				b.paused.Store(false)
			}
		case <-dayTicker.C:
			b.dailyTokens.Store(0)
			b.hourlyTokens.Store(0)
			b.paused.Store(false)
		}
	}
}
