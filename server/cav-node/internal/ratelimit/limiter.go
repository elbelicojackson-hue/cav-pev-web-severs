// Package ratelimit provides per-issuer rate limiting for Praxon publish.
package ratelimit

import (
	"sync"
	"time"
)

// Limiter enforces a maximum number of operations per issuer per second.
type Limiter struct {
	maxPerSecond int
	mu           sync.Mutex
	windows      map[string]*window
}

type window struct {
	count    int
	resetAt  time.Time
}

// New creates a rate limiter with the given max operations per second per key.
func New(maxPerSecond int) *Limiter {
	return &Limiter{
		maxPerSecond: maxPerSecond,
		windows:      make(map[string]*window),
	}
}

// Allow checks if the given key (issuer DID) is within rate limits.
// Returns true if allowed, false if rate-limited.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	w, exists := l.windows[key]

	if !exists || now.After(w.resetAt) {
		l.windows[key] = &window{count: 1, resetAt: now.Add(time.Second)}
		return true
	}

	if w.count >= l.maxPerSecond {
		return false
	}

	w.count++
	return true
}

// Cleanup removes expired windows. Call periodically to prevent memory leak.
func (l *Limiter) Cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	for key, w := range l.windows {
		if now.After(w.resetAt) {
			delete(l.windows, key)
		}
	}
}
