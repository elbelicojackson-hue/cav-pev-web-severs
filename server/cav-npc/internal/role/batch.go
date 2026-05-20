package role

import (
	"sync"
	"time"

	"github.com/anthropic-cav/cav-npc/internal/signal"
)

// Batcher accumulates signals over a time interval before flushing them
// as a batch for processing (R6.3).
type Batcher struct {
	interval time.Duration
	maxSize  int
	out      chan []*signal.EntropicSignal

	mu  sync.Mutex
	buf []*signal.EntropicSignal

	done chan struct{}
}

// NewBatcher creates a Batcher that flushes every interval or when maxSize is reached.
// The out channel receives batches of signals ready for processing.
func NewBatcher(interval time.Duration, maxSize int) *Batcher {
	if maxSize <= 0 {
		maxSize = 100
	}
	return &Batcher{
		interval: interval,
		maxSize:  maxSize,
		out:      make(chan []*signal.EntropicSignal, 4),
		done:     make(chan struct{}),
	}
}

// Out returns the channel that receives flushed batches.
func (b *Batcher) Out() <-chan []*signal.EntropicSignal {
	return b.out
}

// Add appends a signal to the current batch.
// If the batch reaches maxSize, it is flushed immediately.
func (b *Batcher) Add(sig *signal.EntropicSignal) {
	b.mu.Lock()
	b.buf = append(b.buf, sig)
	shouldFlush := len(b.buf) >= b.maxSize
	b.mu.Unlock()

	if shouldFlush {
		b.flush()
	}
}

// Run starts the periodic flush ticker. Blocks until Close is called.
func (b *Batcher) Run() {
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.flush()
		case <-b.done:
			// Final flush on close
			b.flush()
			close(b.out)
			return
		}
	}
}

// Close stops the batcher and flushes any remaining signals.
func (b *Batcher) Close() {
	close(b.done)
}

// flush sends the current buffer to the output channel and resets it.
func (b *Batcher) flush() {
	b.mu.Lock()
	if len(b.buf) == 0 {
		b.mu.Unlock()
		return
	}
	batch := b.buf
	b.buf = nil
	b.mu.Unlock()

	// Non-blocking send — if out is full, we drop (shouldn't happen with cap=4)
	select {
	case b.out <- batch:
	default:
	}
}
