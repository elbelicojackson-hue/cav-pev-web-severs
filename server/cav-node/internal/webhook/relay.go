// Package webhook provides announcement relay to registered subscribers.
package webhook

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/anthropic-cav/cav-node/internal/praxon"
)

// Relay manages webhook subscribers and forwards announcements.
type Relay struct {
	mu          sync.RWMutex
	subscribers []string // webhook URLs
	client      *http.Client
}

// NewRelay creates a webhook relay with default HTTP client settings.
func NewRelay() *Relay {
	return &Relay{
		subscribers: make([]string, 0),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Subscribe adds a webhook URL to the relay.
func (r *Relay) Subscribe(url string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Deduplicate
	for _, existing := range r.subscribers {
		if existing == url {
			return
		}
	}
	r.subscribers = append(r.subscribers, url)
}

// Unsubscribe removes a webhook URL from the relay.
func (r *Relay) Unsubscribe(url string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, existing := range r.subscribers {
		if existing == url {
			r.subscribers = append(r.subscribers[:i], r.subscribers[i+1:]...)
			return
		}
	}
}

// Subscribers returns the current list of subscriber URLs.
func (r *Relay) Subscribers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.subscribers))
	copy(out, r.subscribers)
	return out
}

// Broadcast sends an announcement to all subscribers asynchronously.
// Failures are logged but do not block the caller.
func (r *Relay) Broadcast(ann praxon.Announcement) {
	r.mu.RLock()
	subs := make([]string, len(r.subscribers))
	copy(subs, r.subscribers)
	r.mu.RUnlock()

	if len(subs) == 0 {
		return
	}

	payload, err := json.Marshal(ann)
	if err != nil {
		log.Printf("[webhook] marshal error: %v", err)
		return
	}

	for _, url := range subs {
		go r.send(url, payload)
	}
}

func (r *Relay) send(url string, payload []byte) {
	resp, err := r.client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		log.Printf("[webhook] failed to relay to %s: %v", url, err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Printf("[webhook] relay to %s returned %d", url, resp.StatusCode)
	}
}
