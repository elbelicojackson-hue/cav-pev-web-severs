package instance

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/anthropic-cav/cav-npc/internal/client"
	"github.com/anthropic-cav/cav-npc/internal/llm"
)

// HeartbeatLoop sends heartbeats every 60 seconds (R7.1).
// It determines status from recent activity and budget state.
type HeartbeatLoop struct {
	pub         *client.Publisher
	budget      *llm.Budget
	npcDID      string
	roleName    string
	operatorTag string

	lastProcessed atomic.Int64 // unix timestamp of last signal processed
	currentNote   atomic.Value // string: current task description
}

// NewHeartbeatLoop creates a heartbeat loop.
func NewHeartbeatLoop(pub *client.Publisher, budget *llm.Budget, npcDID, roleName, operatorTag string) *HeartbeatLoop {
	h := &HeartbeatLoop{
		pub:         pub,
		budget:      budget,
		npcDID:      npcDID,
		roleName:    roleName,
		operatorTag: operatorTag,
	}
	h.currentNote.Store("")
	return h
}

// Run starts the heartbeat ticker. Blocks until ctx is cancelled.
func (h *HeartbeatLoop) Run(ctx context.Context) error {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// Send initial heartbeat immediately
	h.sendHeartbeat(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			h.sendHeartbeat(ctx)
		}
	}
}

// MarkProcessing records that a signal is being processed (for status determination).
func (h *HeartbeatLoop) MarkProcessing(note string) {
	h.lastProcessed.Store(time.Now().Unix())
	h.currentNote.Store(note)
}

// MarkIdle clears the current processing note.
func (h *HeartbeatLoop) MarkIdle() {
	h.currentNote.Store("")
}

func (h *HeartbeatLoop) sendHeartbeat(ctx context.Context) {
	status := h.determineStatus()
	note := ""
	if status == "working" {
		if n, ok := h.currentNote.Load().(string); ok {
			note = n
		}
	}

	// Truncate note to 256 chars (gateway limit)
	if len(note) > 256 {
		note = note[:256]
	}

	hb := client.HeartbeatBody{
		Status: status,
		Note:   note,
		Capabilities: &client.Capabilities{
			Nickname:    h.roleName,
			Description: "NPC role: " + h.roleName + " | operator: " + h.operatorTag,
			Tools:       []string{"llm_" + h.roleName},
		},
	}

	resp, err := h.pub.Heartbeat(ctx, hb)
	if err != nil {
		slog.Warn("heartbeat failed",
			"npc_did", h.npcDID,
			"error", err,
		)
		return
	}

	slog.Debug("heartbeat sent",
		"npc_did", h.npcDID,
		"status", status,
		"state", resp.State,
		"peers", resp.PeersOnline,
	)
}

// determineStatus returns the heartbeat status based on current state.
// Priority: blocked > working > idle (design §3.7)
func (h *HeartbeatLoop) determineStatus() string {
	// Budget paused → blocked
	if h.budget != nil && h.budget.IsPaused() {
		return "blocked"
	}

	// Processed signal in last 10 minutes → working (R7.5-6)
	lastProc := h.lastProcessed.Load()
	if lastProc > 0 {
		elapsed := time.Since(time.Unix(lastProc, 0))
		if elapsed < 10*time.Minute {
			return "working"
		}
	}

	return "idle"
}
