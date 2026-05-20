// Package thread implements thread crystallization (cav-social-trust §R5).
//
// A "thread" is a chain of EntropicSignals linked by InReplyTo. Each signal
// arriving on the entropic channel either starts a new thread (its root) or
// continues an existing one (the InReplyTo chain leads back to a root signal).
//
// Threads accumulate a `readiness` score in [0, 1] computed from five
// weighted factors:
//
//   consensus     0.30 — 1 - shannon_entropy / max_entropy across positions
//   diversity     0.25 — Jaccard distance over participant tag sets
//   participation 0.15 — min(1, n_participants / target)
//   confidence    0.20 — weighted-avg confidence of dominant-position signals
//   maturity      0.10 — min(1, age / maturation_period)
//
// At readiness ≥ 0.9 → Canonical Praxon
// At 0.7–0.9        → Provisional Praxon
// At 0.5–0.7        → Draft Praxon
// At < 0.5          → no crystallization
//
// State machine + Praxon emission live in crystallize.go (T13).

package thread

import "time"

// CrystallizationLevel labels the four readiness bands.
type CrystallizationLevel string

const (
	LevelNone        CrystallizationLevel = "none"
	LevelDraft       CrystallizationLevel = "draft"
	LevelProvisional CrystallizationLevel = "provisional"
	LevelCanonical   CrystallizationLevel = "canonical"
)

// Readiness band thresholds (R5-3..R5-6).
const (
	ThresholdDraft       = 0.50
	ThresholdProvisional = 0.70
	ThresholdCanonical   = 0.90
)

// Readiness factor weights (R5-2).
const (
	WeightConsensus     = 0.30
	WeightDiversity     = 0.25
	WeightParticipation = 0.15
	WeightConfidence    = 0.20
	WeightMaturity      = 0.10
)

// SmallNetworkThreshold is the active-citizen count at or below which the
// readiness algorithm uses the smaller participation target and shorter
// maturation period (R5-10).
const SmallNetworkThreshold = 20

// TargetParticipants returns the participation target for the given network
// size. 5 normally, dropping to 3 in small networks so threads can still
// crystallize when the network has few active agents.
func TargetParticipants(networkSize int) int {
	if networkSize <= SmallNetworkThreshold {
		return 3
	}
	return 5
}

// MaturationPeriod returns the time a thread must exist before its maturity
// factor saturates. Smaller in small networks where slow, deliberate
// discussion isn't realistic.
func MaturationPeriod(networkSize int) time.Duration {
	if networkSize <= SmallNetworkThreshold {
		return 1 * time.Hour
	}
	return 6 * time.Hour
}

// Thread is the persisted state of one discussion chain.
type Thread struct {
	ID           string    `json:"id"` // = root signal ID
	StartedAt    time.Time `json:"started_at"`
	LastActivity time.Time `json:"last_activity"`

	SignalIDs    []string `json:"signal_ids"`
	Participants []string `json:"participants"` // distinct fingerprints, dedup-stable order

	Readiness    ReadinessScore       `json:"readiness"`
	CurrentLevel CrystallizationLevel `json:"current_level"`

	// CrystallizedPraxonID is set once the thread first reaches Draft and is
	// updated when the level changes (Provisional / Canonical) — see T13.
	CrystallizedPraxonID string `json:"crystallized_praxon_id,omitempty"`

	// History of readiness snapshots, capped to the most recent N entries.
	ReadinessHistory []ReadinessSnapshot `json:"readiness_history"`
}

// ReadinessScore captures the per-factor breakdown alongside the aggregate.
// Stored on the Thread so callers can introspect why a level changed.
type ReadinessScore struct {
	Total              float64   `json:"total"`
	ConsensusScore     float64   `json:"consensus_score"`
	DiversityScore     float64   `json:"diversity_score"`
	ParticipationScore float64   `json:"participation_score"`
	ConfidenceScore    float64   `json:"confidence_score"`
	MaturityScore      float64   `json:"maturity_score"`
	ComputedAt         time.Time `json:"computed_at"`
}

// ReadinessSnapshot is one historical readiness point with the level it
// implied. Drives the audit trail for crystallization decisions.
type ReadinessSnapshot struct {
	At    time.Time            `json:"at"`
	Score ReadinessScore       `json:"score"`
	Level CrystallizationLevel `json:"level"`
}

// MaxHistoryEntries caps ReadinessHistory length on each Thread to keep the
// stored JSON bounded. Older entries are dropped (history is observability,
// not the audit trail of the crystallized Praxon — that lives on the Praxon
// itself).
const MaxHistoryEntries = 64

// ClassifyReadiness maps a Total score to its crystallization level.
func ClassifyReadiness(total float64) CrystallizationLevel {
	switch {
	case total >= ThresholdCanonical:
		return LevelCanonical
	case total >= ThresholdProvisional:
		return LevelProvisional
	case total >= ThresholdDraft:
		return LevelDraft
	default:
		return LevelNone
	}
}
