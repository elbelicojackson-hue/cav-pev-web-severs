// Package recommend implements the diversity recommendation engine
// (cav-social-trust §R6 / §5.4).
//
// Goal: for each active citizen, surface a small list of high-value
// candidate trustees that *increase* their cognitive diversity rather than
// reinforce existing patterns. The score is
//
//   score = methodology_distance × domain_overlap × (1 - aggregate_risk)
//
// We classify into bands:
//
//   strong       — methodology_distance > 0.8 AND domain_overlap > 0.5
//   moderate     — methodology_distance > 0.5 AND domain_overlap > 0.3
//   exploratory  — anything else worth surfacing
//
// Each recommendation carries the precomputed TrustRiskVector (so the user
// can act without a separate /preview round trip) and a unique ID for
// feedback tracking.

package recommend

import "time"

// SchemaVersion identifies stored payload format.
const SchemaVersion = "1.0"

// Tier labels.
const (
	TierStrong      = "strong"
	TierModerate    = "moderate"
	TierExploratory = "exploratory"
)

// Recommendation is one suggested trustee for one requester.
type Recommendation struct {
	ID                 string    `json:"id"` // unique; used as bandit feedback key
	Requester          string    `json:"requester"`
	Subject            string    `json:"subject"`
	Tier               string    `json:"tier"`
	Score              float64   `json:"score"`
	MethodologyDistance float64  `json:"methodology_distance"`
	DomainOverlap      float64   `json:"domain_overlap"`
	RiskAggregate      float64   `json:"risk_aggregate"`
	RiskClass          string    `json:"risk_class"`
	Strategy           string    `json:"strategy"` // bandit arm key
	GeneratedAt        time.Time `json:"generated_at"`
	ExpiresAt          time.Time `json:"expires_at"`

	// VectorHash points back to the audit log so the requester can fetch
	// the full TrustRiskVector if they want to inspect the dimensions.
	VectorHash string `json:"vector_hash,omitempty"`
}

// SourceProfile is the per-agent input the engine consumes. Callers project
// this from their stores (signal store + reputation behavioral subvector +
// trust graph DomainsOfCognitiveTrust).
type SourceProfile struct {
	DID string

	// MethodologyDistribution: joint (prior_source_tag, inference_method_tag)
	// frequencies across the agent's published Praxons. The map's keys are
	// "<prior>|<inference>"; values are non-negative counts.
	MethodologyDistribution map[string]float64

	// Domains: distinct domain labels the agent is meaningfully active in.
	// Used for set-overlap with other agents.
	Domains map[string]struct{}
}

// Default expiry — recommendations are regenerated weekly per spec §R6-1, so
// 7 days is a sensible TTL on each batch.
var DefaultTTL = 7 * 24 * time.Hour
