// Package risk implements the trust-add-time TrustRiskVector engine.
//
// Three dimension classes (cav-social-trust/design.md §2.2):
//
//   Epistemic  : claim-quality risk
//                ground_truth_alignment, methodology_stability,
//                challenge_survival_rate, retraction_responsiveness
//
//   Behavioral : interaction-pattern risk
//                conformity_index, sybil_similarity_max, activity_anomaly_score
//
//   Structural : graph-shape risk (real-time relative to the requester)
//                diversity_impact, echo_chamber_delta
//
// Aggregation:
//   AggregateScore = Σ(weight_class × score_class) / Σ(weight_class, sufficient)
//   weights: epistemic 0.45 / behavioral 0.35 / structural 0.20
//   sufficient_count < 2 → recommendation = defer (regardless of score)
//
// All dimension calculators are pure functions on Go inputs. The IO-heavy
// data-fetching path lives in engine.go (T6).
package risk

import "time"

// SchemaVersion is stamped onto every vector for forward-compat audit.
const SchemaVersion = "1.0"

// === Top-level vector ===

// TrustRiskVector is the trust-add-time risk evaluation result.
type TrustRiskVector struct {
	Subject       string    `json:"subject"`
	Requester     string    `json:"requester"`
	ComputedAt    time.Time `json:"computed_at"`
	SchemaVersion string    `json:"schema_version"`

	Epistemic  *EpistemicRisk  `json:"epistemic,omitempty"`
	Behavioral *BehavioralRisk `json:"behavioral,omitempty"`
	Structural *StructuralRisk `json:"structural,omitempty"`

	AggregateScore         float64          `json:"aggregate_score"`
	RiskClass              string           `json:"risk_class"`
	Recommendation         string           `json:"recommendation"`
	DominantFactors        []DominantFactor `json:"dominant_factors"`
	InsufficientDimensions []string         `json:"insufficient_dimensions"`
	Reasoning              string           `json:"reasoning"`

	// VectorHash is set by the engine after the vector has been persisted to
	// the audit store. It is excluded from canonicalHash to avoid the
	// chicken-and-egg problem of "the hash includes the hash". Callers that
	// want to verify the audit pointer should re-compute via HashOnly on a
	// copy with VectorHash blanked.
	VectorHash string `json:"vector_hash,omitempty"`
}

// Dimension is a single risk axis with its provenance metadata.
//
// Score is in [0, 1] where higher = more risky. Sufficient means we had
// enough samples to trust this dimension; if false, the dimension is reported
// for transparency but excluded from aggregation.
type Dimension struct {
	Score      float64 `json:"score"`
	SampleSize int     `json:"sample_size"`
	Sufficient bool    `json:"sufficient"`
	Confidence float64 `json:"confidence"` // [0, 1] dimension's own self-confidence
}

// EpistemicRisk groups the four claim-quality dimensions.
// Pointer fields so missing dimensions can be omitted from JSON.
type EpistemicRisk struct {
	GroundTruthAlignment     *Dimension `json:"ground_truth_alignment,omitempty"`
	MethodologyStability     *Dimension `json:"methodology_stability,omitempty"`
	ChallengeSurvivalRate    *Dimension `json:"challenge_survival_rate,omitempty"`
	RetractionResponsiveness *Dimension `json:"retraction_responsiveness,omitempty"`
}

// BehavioralRisk groups the three interaction-pattern dimensions.
type BehavioralRisk struct {
	ConformityIndex      *Dimension `json:"conformity_index,omitempty"`
	SybilSimilarityMax   *Dimension `json:"sybil_similarity_max,omitempty"`
	ActivityAnomalyScore *Dimension `json:"activity_anomaly_score,omitempty"`
}

// StructuralRisk groups the two graph-shape dimensions.
type StructuralRisk struct {
	DiversityImpact  *Dimension `json:"diversity_impact,omitempty"`
	EchoChamberDelta *Dimension `json:"echo_chamber_delta,omitempty"`
}

// DominantFactor names a high-scoring dimension for the recommendation UI.
type DominantFactor struct {
	Name  string  `json:"name"`
	Score float64 `json:"score"`
	Note  string  `json:"note,omitempty"`
}

// === Risk classification ===

// Risk class labels (§5.1).
const (
	ClassLow      = "low"
	ClassModerate = "moderate"
	ClassElevated = "elevated"
	ClassHigh     = "high"
	ClassCritical = "critical"
)

// Recommendation labels (§5.1).
const (
	RecProceed            = "proceed"
	RecProceedWithCaution = "proceed_with_caution"
	RecDefer              = "defer"
	RecReject             = "reject"
)

// ClassifyAggregate maps an aggregate score in [0,1] to a (class, recommendation).
// Assumes the caller has already gated on sufficient_count.
func ClassifyAggregate(score float64) (class, recommendation string) {
	switch {
	case score < 0.20:
		return ClassLow, RecProceed
	case score < 0.40:
		return ClassModerate, RecProceed
	case score < 0.60:
		return ClassElevated, RecProceedWithCaution
	case score < 0.80:
		return ClassHigh, RecDefer
	default:
		return ClassCritical, RecReject
	}
}

// === Minimum sample-size constants (design §R2-9..R2-15) ===

const (
	MinPraxonsForGroundTruth      = 5
	MinPraxonsForMethodology      = 10
	MinChallengesForSurvival      = 3
	MinRetractionsForResponsive   = 2
	MinVotesForConformity         = 20
	MinHoursForSybilDetection     = 48
	MinDaysForActivityAnomaly     = 7
)

// === Aggregation weights (design §5.1) ===

const (
	WeightEpistemic  = 0.45
	WeightBehavioral = 0.35
	WeightStructural = 0.20
)

// === Helpers ===

// clamp01 keeps a score in the valid [0, 1] range.
func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}
