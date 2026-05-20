// Package reputation implements the multi-dimensional reputation vector that
// replaces the legacy scalar citizen.Level in the convergence engine.
//
// Reputation has two independent tiers:
//   - operational  : per-domain competence; half-life 90 days
//   - deliberation : governance/parameter-revision standing; half-life 2 years
//
// Each tier holds a per-domain map of {score, confidence, sample_size, last_updated}.
// In addition, a public behavioral subvector (conformity_index, challenge success
// rate, diversity contribution) is computed from public records and is part of
// every agent's reputation snapshot.
//
// Invariants enforced elsewhere (cav-social-trust §11):
//   I3. Reputation may only mutate via reputation.Event records.
//       Direct setters are package-private; tests live in the same package.
//
// See cav-social-trust/design.md §2.3 for the full data model.
package reputation

import (
	"math"
	"time"
)

// SchemaVersion identifies the on-disk format. Bump when changing field semantics
// in a way that requires migration.
const SchemaVersion = "1.0"

// Tier names. Stored as strings inside Event records so they survive schema
// upgrades without enum re-encoding.
const (
	TierOperational  = "operational"
	TierDeliberation = "deliberation"
)

// Half-life constants for time decay.
var (
	HalfLifeOperational  = 90 * 24 * time.Hour       // 90 days
	HalfLifeDeliberation = 2 * 365 * 24 * time.Hour  // ~2 years
)

// Vector is the full reputation snapshot for a single citizen.
type Vector struct {
	DID string `json:"did"`

	Operational  TierVector `json:"operational"`
	Deliberation TierVector `json:"deliberation"`

	// Behavioral subvector — strictly public, computed from public records.
	Behavioral BehavioralSubvector `json:"behavioral"`

	LastUpdatedAt time.Time `json:"last_updated_at"`
	LastDecayedAt time.Time `json:"last_decayed_at"`
	SchemaVersion string    `json:"schema_version"`

	// Inactive marker — set by the digest subsystem when no behavioral
	// digest has been received in N days. Halves EffectiveScore output.
	Inactive bool `json:"inactive,omitempty"`
}

// TierVector holds per-domain scores within one tier (operational | deliberation).
type TierVector struct {
	Domains map[string]DomainScore `json:"domains"`
}

// DomainScore is the score for one (tier, domain) cell.
type DomainScore struct {
	Score       float64   `json:"score"`        // [0, 1]
	Confidence  float64   `json:"confidence"`   // [0, 1]
	SampleSize  int       `json:"sample_size"`  // count of verifiable events feeding this score
	LastUpdated time.Time `json:"last_updated"` // anchor for decay; not the wall clock of last read
}

// BehavioralSubvector mirrors design.md §2.3. All values are [0, 1] and derived
// from the citizen's own published signals + votes; no private state.
type BehavioralSubvector struct {
	ConformityIndex       float64 `json:"conformity_index"`
	ChallengeSuccessRate  float64 `json:"challenge_success_rate"`
	DiversityContribution float64 `json:"diversity_contribution"`
	SampleSize            int     `json:"sample_size"`
}

// NewVector builds a fresh empty Vector for a DID. Used by Store.Get when a
// citizen has no record yet (e.g. brand-new probation registrant before the
// canary subsystem seeds anything).
func NewVector(did string, now time.Time) *Vector {
	return &Vector{
		DID:           did,
		Operational:   TierVector{Domains: map[string]DomainScore{}},
		Deliberation:  TierVector{Domains: map[string]DomainScore{}},
		Behavioral:    BehavioralSubvector{},
		LastUpdatedAt: now,
		LastDecayedAt: now,
		SchemaVersion: SchemaVersion,
	}
}

// EffectiveScore returns the time-decayed operational score for `domain`,
// gated by confidence and the inactive flag. This is the value the convergence
// engine consumes (replacing the legacy scalar citizen.Level mapping).
//
// If the (tier, domain) cell does not exist, returns 0 (the citizen has no
// established standing in this domain — neutral, not negative).
func (v *Vector) EffectiveScore(domain string) float64 {
	if v == nil {
		return 0
	}
	d, ok := v.Operational.Domains[domain]
	if !ok {
		return 0
	}
	// Apply current time decay in addition to whatever batch decay has already
	// run; this guards against unbounded staleness between batch runs.
	decayed := decayedScore(d.Score, d.LastUpdated, time.Now(), HalfLifeOperational)
	score := decayed * d.Confidence
	if v.Inactive {
		score *= 0.5
	}
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

// decayedScore applies exponential decay with the given half-life from
// `lastUpdated` to `now`. If lastUpdated is in the future or zero, returns
// `score` unchanged (we never amplify on read).
func decayedScore(score float64, lastUpdated, now time.Time, halfLife time.Duration) float64 {
	if lastUpdated.IsZero() || !now.After(lastUpdated) {
		return score
	}
	elapsed := now.Sub(lastUpdated)
	factor := math.Pow(0.5, float64(elapsed)/float64(halfLife))
	return score * factor
}

// LegacyLevelToVector maps the deprecated citizen.Level (1, 2, 3) to a default
// Vector seeded across all of the citizen's capability domains. Used during
// migration and in tests; once a citizen has any reputation Event, this seed is
// replaced by accumulated values.
//
// Mapping (cav-social-trust design.md §6.1):
//   Level 1 → score=0.20, confidence=0.30
//   Level 2 → score=0.50, confidence=0.50
//   Level 3 → score=0.80, confidence=0.70
//
// `domains` is the list of domain names (typically the citizen's declared
// capability hypothesis_kinds). If empty, the resulting Vector has empty tier
// maps and EffectiveScore returns 0 for any domain.
func LegacyLevelToVector(did string, level int, domains []string, now time.Time) *Vector {
	v := NewVector(did, now)
	score, confidence := legacyLevelMapping(level)
	if score == 0 {
		return v
	}
	for _, d := range domains {
		v.Operational.Domains[d] = DomainScore{
			Score:       score,
			Confidence:  confidence,
			SampleSize:  0, // explicitly: no real evidence has been observed yet
			LastUpdated: now,
		}
	}
	return v
}

func legacyLevelMapping(level int) (score, confidence float64) {
	switch level {
	case 1:
		return 0.20, 0.30
	case 2:
		return 0.50, 0.50
	case 3:
		return 0.80, 0.70
	default:
		return 0, 0
	}
}

// Event is the immutable record of a single reputation mutation. The full
// Apply() logic lives in update.go (added by T2) — the type lives here because
// the Store needs to persist it.
//
// Trigger values (string for forward compatibility):
//   ground_truth_verified  — the ground truth for an episode was published
//   challenge_survived     — claim survived a challenge round
//   challenge_failed       — claim was retracted under challenge
//   canary_completed       — canary task graded; positive or negative
//   retroactive            — retrospective adjustment after ground truth
//   bootstrap              — initial seed from legacy citizen.Level
type Event struct {
	ID         string    `json:"id"`
	DID        string    `json:"did"`
	Domain     string    `json:"domain"`
	Tier       string    `json:"tier"` // TierOperational | TierDeliberation
	Trigger    string    `json:"trigger"`
	Delta      float64   `json:"delta"` // signed delta applied to score
	Reason     string    `json:"reason"`
	AnchorID   string    `json:"anchor_id"` // praxon_id / canary_task_id / episode_id
	OccurredAt time.Time `json:"occurred_at"`
}

