// Crystallization state machine — turns thread level changes into Praxon
// emissions and retraction signals.
//
// Transition table (cav-social-trust §R5-3..R5-9):
//
//   none        → draft         emit Draft Praxon (provenance only, no rep impact)
//   draft       → provisional   upgrade Praxon, reputation weight ×0.5
//   provisional → canonical     upgrade Praxon to full reputation weight
//
//   canonical   → provisional   downgrade (challenge succeeded)
//   provisional → draft         downgrade
//   draft       → none          retract Praxon
//
// Always-Challengeable invariant (Axiom 1, design §11 I7): a Canonical
// Praxon may still be challenged and demoted. We never write a "sealed"
// flag — the level field on the Praxon record is always the current truth.
//
// issuer = `did:cav:thread:<thread_id>` (design §10 OQ-3 decision)

package thread

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// CrystallizedPraxon is the minimal record we hand to the upstream Praxon
// store / webhook relay. We don't import the full cav-node praxon types
// here — the gateway publishes via webhook to cav-node, which validates
// (T14 widens its issuer whitelist to include did:cav:thread:*).
type CrystallizedPraxon struct {
	ID          string               `json:"id"`
	Issuer      string               `json:"issuer"`        // did:cav:thread:<thread_id>
	Level       CrystallizationLevel `json:"level"`
	Conclusion  string               `json:"conclusion"`     // dominant position label
	Confidence  float64              `json:"confidence"`     // dominant-position weighted-avg
	Provenance  PraxonProvenance     `json:"provenance"`
	IssuedAt    time.Time            `json:"issued_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
}

// PraxonProvenance points back at the thread that produced the Praxon.
type PraxonProvenance struct {
	DerivedFrom      []string `json:"derived_from"`      // signal IDs
	ConsensusEpisode string   `json:"consensus_episode"` // thread ID
}

// PraxonRelay is the side-effect interface — the gateway implementation
// writes through to webhook/relay so cav-node persists the Praxon.
//
// In tests / Phase 1 boot this can be a noop or in-memory accumulator.
type PraxonRelay interface {
	Publish(ctx context.Context, p *CrystallizedPraxon) error
}

// RetractionEmitter is invoked when a level downgrades — used to broadcast
// a SignalRetraction on the entropic channel. Optional; nil disables.
type RetractionEmitter interface {
	EmitRetraction(ctx context.Context, threadID, praxonID string, fromLevel, toLevel CrystallizationLevel) error
}

// Crystallizer wires the tracker's level-change events to Praxon emission.
type Crystallizer struct {
	relay     PraxonRelay
	retractor RetractionEmitter
	tracker   *Tracker
}

// NewCrystallizer attaches a level-change subscriber to `tracker`.
// The returned Crystallizer can be discarded — the subscription is the
// active component.
func NewCrystallizer(tracker *Tracker, relay PraxonRelay, retractor RetractionEmitter) (*Crystallizer, error) {
	if tracker == nil {
		return nil, errors.New("crystallizer: nil tracker")
	}
	if relay == nil {
		return nil, errors.New("crystallizer: nil relay")
	}
	c := &Crystallizer{relay: relay, retractor: retractor, tracker: tracker}
	tracker.Subscribe(c.onLevelChange)
	return c, nil
}

// onLevelChange reacts to a tracker-emitted LevelChange.
func (c *Crystallizer) onLevelChange(change LevelChange) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Direction: did we move up or down?
	fromRank := levelRank(change.From)
	toRank := levelRank(change.To)
	upgraded := toRank > fromRank
	downgraded := toRank < fromRank
	draftRank := levelRank(LevelDraft)

	switch {
	case upgraded && toRank >= draftRank:
		c.emitOrUpgrade(ctx, change)
	case downgraded && toRank >= draftRank:
		// Still crystallized but at a lower level — relay an updated record.
		c.emitOrUpgrade(ctx, change)
		// And emit a retraction-flavored notice so subscribers know.
		if c.retractor != nil {
			th := c.tracker.Get(change.ThreadID)
			if th != nil && th.CrystallizedPraxonID != "" {
				_ = c.retractor.EmitRetraction(ctx, change.ThreadID, th.CrystallizedPraxonID, change.From, change.To)
			}
		}
	case downgraded && change.To == LevelNone:
		// Full retraction — Praxon is no longer valid.
		if c.retractor != nil {
			th := c.tracker.Get(change.ThreadID)
			if th != nil && th.CrystallizedPraxonID != "" {
				_ = c.retractor.EmitRetraction(ctx, change.ThreadID, th.CrystallizedPraxonID, change.From, change.To)
			}
		}
	}
}

// emitOrUpgrade publishes (or republishes-as-upgrade) the Praxon for this
// thread at the new level.
//
// Gate: we require at least 2 derived_from signals — single-signal "threads"
// shouldn't crystallize (they'd fail cav-node's structural validator at T14
// anyway, and conceptually a thread of one isn't a discussion).
func (c *Crystallizer) emitOrUpgrade(ctx context.Context, change LevelChange) {
	th := c.tracker.Get(change.ThreadID)
	if th == nil {
		return
	}
	if len(th.SignalIDs) < 2 {
		return
	}
	p := buildPraxon(th, change)
	if err := c.relay.Publish(ctx, p); err != nil {
		// Best-effort: failures are surfaced via logging at the relay layer.
		// We don't bubble — the readiness snapshot already records the level
		// change for audit.
		return
	}
	// Stamp the praxon ID back onto the thread so future upgrades can refer
	// to it and downgrades can target the right record.
	_ = c.tracker.SetCrystallizedPraxonID(th.ID, p.ID)
}

// buildPraxon constructs the wire-shaped CrystallizedPraxon from a thread +
// level change.
func buildPraxon(th *Thread, change LevelChange) *CrystallizedPraxon {
	now := time.Now()
	id := th.CrystallizedPraxonID
	if id == "" {
		// First emission — synthesize a deterministic ID from thread + time.
		id = fmt.Sprintf("praxon_thread_%s_%d", th.ID, now.UnixNano())
	}
	return &CrystallizedPraxon{
		ID:         id,
		Issuer:     ThreadIssuerDID(th.ID),
		Level:      change.To,
		Conclusion: change.DominantPosition,
		Confidence: change.DominantConfidence,
		Provenance: PraxonProvenance{
			DerivedFrom:      append([]string{}, th.SignalIDs...),
			ConsensusEpisode: th.ID,
		},
		IssuedAt:  th.StartedAt,
		UpdatedAt: now,
	}
}

// ThreadIssuerDID returns the synthetic Praxon issuer used for crystallized
// thread Praxons. Must remain stable; cav-node's praxon validator (T14)
// whitelists this prefix.
func ThreadIssuerDID(threadID string) string {
	return "did:cav:thread:" + threadID
}

// === Helpers ===

func levelRank(l CrystallizationLevel) int {
	switch l {
	case LevelCanonical:
		return 3
	case LevelProvisional:
		return 2
	case LevelDraft:
		return 1
	default:
		return 0
	}
}
