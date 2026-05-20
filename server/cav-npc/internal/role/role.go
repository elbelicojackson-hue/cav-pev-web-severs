// Package role defines the Role interface and global registry for NPC behaviors.
// Each role determines what signals an NPC subscribes to, how it processes them,
// and what output signals it produces.
package role

import (
	"fmt"
	"sync"
	"time"

	"github.com/anthropic-cav/cav-npc/internal/client"
	"github.com/anthropic-cav/cav-npc/internal/signal"
)

// Role defines the behavior of an NPC instance.
type Role interface {
	// Name returns the role identifier (e.g. "signal_sentinel").
	Name() string

	// Filter returns the signal subscription filter for this role.
	Filter() client.Filter

	// BatchMode returns whether this role accumulates signals before processing,
	// and if so, the batch interval.
	BatchMode() (enabled bool, interval time.Duration)

	// Capable returns true if this role can handle the given signal
	// (used by the bidder to decide whether to bid on task_request signals).
	Capable(sig *signal.EntropicSignal) bool

	// BuildPrompt constructs the system and user prompts for the LLM call.
	BuildPrompt(ctx PromptContext) (system, user string)

	// ParseOutput interprets the raw LLM response into output signals.
	// Most roles delegate to signal.ParseLLMOutput, but some (like format_validator)
	// have custom logic.
	ParseOutput(raw string, ctx PromptContext) ([]*signal.OutSignal, error)
}

// PromptContext provides all the data a role needs to build its prompt.
type PromptContext struct {
	Signal      *signal.EntropicSignal   // single signal (non-batch mode)
	Batch       []*signal.EntropicSignal // batch of signals (batch mode)
	NPCDID      string                   // this NPC's DID
	NPCRep      float64                  // this NPC's reputation in relevant domain
	ContextSigs []*signal.EntropicSignal // recent related signals for context
}

// --- Global Registry ---

var (
	registryMu sync.RWMutex
	registry   = make(map[string]Factory)
)

// Factory creates a Role instance from configuration parameters.
type Factory func(params map[string]any) (Role, error)

// Register adds a role factory to the global registry.
// Panics if the name is already registered (programming error).
func Register(name string, factory Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()

	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("role: duplicate registration for %q", name))
	}
	registry[name] = factory
}

// Lookup returns the factory for the named role, or an error if not found.
func Lookup(name string) (Factory, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	f, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("role: unknown role %q", name)
	}
	return f, nil
}

// List returns all registered role names.
func List() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	names := make([]string, 0, len(registry))
	for k := range registry {
		names = append(names, k)
	}
	return names
}
