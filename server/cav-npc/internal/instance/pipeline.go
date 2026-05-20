package instance

import (
	"context"
	"log/slog"

	"github.com/anthropic-cav/cav-npc/internal/llm"
	"github.com/anthropic-cav/cav-npc/internal/role"
	"github.com/anthropic-cav/cav-npc/internal/signal"
)

// Pipeline processes incoming signals through the role's LLM and publishes results.
type Pipeline struct {
	role    role.Role
	llm     llm.Provider
	pub     *Publisher
	bidder  *Bidder
	npcDID  string
	npcRep  float64
}

// NewPipeline creates a signal processing pipeline.
func NewPipeline(r role.Role, provider llm.Provider, pub *Publisher, bidder *Bidder, npcDID string, npcRep float64) *Pipeline {
	return &Pipeline{
		role:   r,
		llm:    provider,
		pub:    pub,
		bidder: bidder,
		npcDID: npcDID,
		npcRep: npcRep,
	}
}

// Process handles a single incoming signal through the full pipeline:
// bid check → prompt build → LLM call → parse → publish.
func (p *Pipeline) Process(ctx context.Context, sig *signal.EntropicSignal, bidInbox <-chan *signal.EntropicSignal) {
	// Task bidding check (R8)
	if isTaskSignal(sig) {
		result := p.bidder.ShouldBid(ctx, sig, bidInbox)
		if result != BidWon {
			slog.Debug("skipping task (not won bid or not capable)",
				"npc_did", p.npcDID,
				"signal_id", sig.ID,
				"result", result,
			)
			return
		}
	}

	// Build prompt
	promptCtx := role.PromptContext{
		Signal: sig,
		NPCDID: p.npcDID,
		NPCRep: p.npcRep,
	}

	system, user := p.role.BuildPrompt(promptCtx)

	// LLM call
	resp, err := p.llm.Complete(ctx, llm.CompletionRequest{
		System:      system,
		User:        user,
		MaxTokens:   4096,
		Temperature: 0.3,
		JSONMode:    true,
	})
	if err != nil {
		slog.Warn("LLM call failed",
			"npc_did", p.npcDID,
			"signal_id", sig.ID,
			"error", err,
		)
		return
	}

	slog.Debug("LLM call completed",
		"npc_did", p.npcDID,
		"prompt_tokens", resp.PromptTokens,
		"completion_tokens", resp.CompletionTokens,
		"latency_ms", resp.LatencyMs,
	)

	// Parse output
	outputs, err := p.role.ParseOutput(resp.Content, promptCtx)
	if err != nil {
		slog.Warn("LLM output parse failed",
			"npc_did", p.npcDID,
			"signal_id", sig.ID,
			"error", err,
		)
		return
	}

	if len(outputs) == 0 {
		slog.Debug("role produced no output signals",
			"npc_did", p.npcDID,
			"signal_id", sig.ID,
		)
		return
	}

	// Publish each output signal
	for _, out := range outputs {
		if err := p.pub.Publish(ctx, out, sig.ID); err != nil {
			slog.Warn("publish failed",
				"npc_did", p.npcDID,
				"error", err,
			)
			// Continue with remaining outputs
		}
	}
}

// ProcessBatch handles a batch of signals (for batch-mode roles).
func (p *Pipeline) ProcessBatch(ctx context.Context, batch []*signal.EntropicSignal) {
	promptCtx := role.PromptContext{
		Batch:  batch,
		NPCDID: p.npcDID,
		NPCRep: p.npcRep,
	}

	system, user := p.role.BuildPrompt(promptCtx)

	resp, err := p.llm.Complete(ctx, llm.CompletionRequest{
		System:      system,
		User:        user,
		MaxTokens:   8192, // larger for batch
		Temperature: 0.3,
		JSONMode:    true,
	})
	if err != nil {
		slog.Warn("batch LLM call failed",
			"npc_did", p.npcDID,
			"batch_size", len(batch),
			"error", err,
		)
		return
	}

	outputs, err := p.role.ParseOutput(resp.Content, promptCtx)
	if err != nil {
		slog.Warn("batch LLM output parse failed",
			"npc_did", p.npcDID,
			"batch_size", len(batch),
			"error", err,
		)
		return
	}

	for _, out := range outputs {
		if err := p.pub.Publish(ctx, out, ""); err != nil {
			slog.Warn("batch publish failed", "npc_did", p.npcDID, "error", err)
		}
	}
}
