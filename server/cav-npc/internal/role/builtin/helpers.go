package builtin

import (
	"encoding/json"

	"github.com/anthropic-cav/cav-npc/internal/signal"
)

// formatSignalForPrompt converts a signal to a readable JSON string for LLM prompts.
func formatSignalForPrompt(sig *signal.EntropicSignal) string {
	if sig == nil {
		return "{}"
	}
	b, err := json.MarshalIndent(sig, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}

// formatBatchForPrompt converts a batch of signals to JSON for LLM prompts.
func formatBatchForPrompt(sigs []*signal.EntropicSignal) string {
	if len(sigs) == 0 {
		return "[]"
	}
	b, err := json.MarshalIndent(sigs, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(b)
}
