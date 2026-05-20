// Package consensus implements the adaptive convergence algorithm for
// knowledge collision resolution.
//
// When multiple agents propose different versions of the same knowledge,
// this engine determines which version "wins" through a mathematically
// rigorous process that:
//
//   1. Weights votes by agent reputation (earned, not assigned)
//   2. Penalizes conformity clusters (anti-echo-chamber)
//   3. Decays old votes over time (freshness bias)
//   4. Adapts the acceptance threshold to network size
//   5. Detects convergence via entropy minimization
//
// Mathematical basis:
//   - Weighted majority with diversity discount (Theorem: converges to
//     ground truth faster than simple majority when agent errors are
//     partially independent)
//   - Shannon entropy as convergence metric (H → 0 means consensus reached)
//   - Exponential time decay (half-life = 24h)
//   - Anti-conformity penalty from HP-02 (collective hallucination defense)
//
// This is the implementation of Charter §3.5 (Adversarial Consensus) and
// the structural defense against HP-02 (Collective Hallucination).
package consensus

import (
	"math"
	"time"

	"github.com/anthropic-cav/cav-gateway/internal/social/reputation"
)

// Vote represents a single agent's position on a proposal.
type Vote struct {
	AgentFingerprint string    `json:"agent"`
	Position         Position  `json:"position"`   // endorse / reject / abstain
	Confidence       float64   `json:"confidence"` // [0,1] how sure the voter is
	Reputation       float64   `json:"reputation"` // legacy scalar fallback
	Timestamp        time.Time `json:"timestamp"`
	Tags             []string  `json:"tags"` // agent's expertise areas

	// Social-trust extensions. When ReputationVector and Domain are both set,
	// the engine uses ReputationVector.EffectiveScore(Domain) instead of the
	// scalar Reputation field. This is the migration path off citizen.Level.
	ReputationVector *reputation.Vector `json:"-"`     // pointer to caller's vector; not serialized
	Domain           string             `json:"domain,omitempty"` // domain key for EffectiveScore
}

// effectiveReputation returns the score the engine uses for this vote.
// Falls back to the legacy scalar field when no vector is attached.
func (v Vote) effectiveReputation() float64 {
	if v.ReputationVector != nil && v.Domain != "" {
		score := v.ReputationVector.EffectiveScore(v.Domain)
		if score > 0 {
			return score
		}
		// Domain unknown to the vector → fall back to legacy scalar so an
		// agent with seeded scalar reputation isn't silently zeroed when a
		// new domain arrives.
	}
	return v.Reputation
}

// Position is the voter's stance.
type Position string

const (
	Endorse Position = "endorse"
	Reject  Position = "reject"
	Abstain Position = "abstain"
)

// ConvergenceResult is the output of the convergence engine.
type ConvergenceResult struct {
	Decision       Decision `json:"decision"`        // accepted / rejected / pending
	Confidence     float64  `json:"confidence"`      // [0,1] how confident the decision is
	Entropy        float64  `json:"entropy"`         // current belief entropy (0 = full consensus)
	WeightedScore  float64  `json:"weighted_score"`  // net weighted endorsement score
	Threshold      float64  `json:"threshold"`       // adaptive acceptance threshold
	DiversityScore float64  `json:"diversity_score"` // how diverse the voters are [0,1]
	TotalWeight    float64  `json:"total_weight"`    // sum of all vote weights
	VoteCount      int      `json:"vote_count"`
	Reason         string   `json:"reason"`
}

type Decision string

const (
	Accepted Decision = "accepted"
	Rejected Decision = "rejected"
	Pending  Decision = "pending"
)

// Config holds tunable parameters for the convergence engine.
type Config struct {
	// Time decay half-life (votes lose half their weight after this duration)
	DecayHalfLife time.Duration

	// Minimum votes required before any decision can be made
	MinVotes int

	// Base acceptance threshold (fraction of weighted votes needed)
	// Adaptive: actual threshold = BaseThreshold * AdaptiveFactor(networkSize)
	BaseThreshold float64

	// Entropy threshold below which convergence is declared
	// H < EntropyThreshold → decision is final
	EntropyThreshold float64

	// Anti-conformity penalty: how much to discount votes that cluster
	// with high-reputation agents (0 = no penalty, 1 = full penalty)
	ConformityPenalty float64

	// Maximum time a proposal can stay pending before auto-resolution
	MaxPendingDuration time.Duration
}

// DefaultConfig returns production-ready convergence parameters.
func DefaultConfig() Config {
	return Config{
		DecayHalfLife:      24 * time.Hour,
		MinVotes:           3,
		BaseThreshold:      0.6,
		EntropyThreshold:   0.3,
		ConformityPenalty:  0.4,
		MaxPendingDuration: 7 * 24 * time.Hour, // 1 week
	}
}

// Engine computes convergence decisions.
type Engine struct {
	config Config
}

// NewEngine creates a convergence engine with the given config.
func NewEngine(config Config) *Engine {
	return &Engine{config: config}
}

// Evaluate computes the current convergence state for a set of votes.
func (e *Engine) Evaluate(votes []Vote, networkSize int, proposalCreatedAt time.Time) ConvergenceResult {
	now := time.Now()

	if len(votes) < e.config.MinVotes {
		return ConvergenceResult{
			Decision:  Pending,
			VoteCount: len(votes),
			Reason:    "insufficient votes",
		}
	}

	// Step 1: Compute time-decayed weights
	weights := make([]float64, len(votes))
	for i, v := range votes {
		age := now.Sub(v.Timestamp)
		timeDecay := math.Pow(0.5, age.Hours()/e.config.DecayHalfLife.Hours())
		weights[i] = v.effectiveReputation() * v.Confidence * timeDecay
	}

	// Step 2: Apply anti-conformity penalty
	// Votes that cluster with high-reputation agents get discounted
	weights = e.applyConformityPenalty(votes, weights)

	// Step 3: Compute weighted endorsement score
	var endorseWeight, rejectWeight, totalWeight float64
	for i, v := range votes {
		totalWeight += weights[i]
		switch v.Position {
		case Endorse:
			endorseWeight += weights[i]
		case Reject:
			rejectWeight += weights[i]
		}
	}

	if totalWeight == 0 {
		return ConvergenceResult{Decision: Pending, Reason: "zero total weight"}
	}

	// Normalized score: +1 = full endorse, -1 = full reject
	netScore := (endorseWeight - rejectWeight) / totalWeight

	// Step 4: Compute belief entropy
	pEndorse := endorseWeight / totalWeight
	pReject := rejectWeight / totalWeight
	pAbstain := 1.0 - pEndorse - pReject
	entropy := shannonEntropy([]float64{pEndorse, pReject, pAbstain})

	// Step 5: Adaptive threshold based on network size
	threshold := e.adaptiveThreshold(networkSize)

	// Step 6: Diversity score (how different are the voters)
	diversity := e.computeDiversity(votes)

	// Step 7: Decision logic
	result := ConvergenceResult{
		WeightedScore:  netScore,
		Entropy:        entropy,
		Threshold:      threshold,
		DiversityScore: diversity,
		TotalWeight:    totalWeight,
		VoteCount:      len(votes),
	}

	// Convergence check: entropy below threshold = decision is clear
	if entropy < e.config.EntropyThreshold {
		if netScore > 0 {
			result.Decision = Accepted
			result.Confidence = math.Min(1.0, netScore*diversity)
			result.Reason = "entropy converged (endorse dominant)"
		} else {
			result.Decision = Rejected
			result.Confidence = math.Min(1.0, -netScore*diversity)
			result.Reason = "entropy converged (reject dominant)"
		}
		return result
	}

	// Threshold check: strong enough signal even without full convergence
	if netScore > threshold && diversity > 0.5 {
		result.Decision = Accepted
		result.Confidence = netScore * diversity
		result.Reason = "threshold exceeded with sufficient diversity"
		return result
	}
	if netScore < -threshold && diversity > 0.5 {
		result.Decision = Rejected
		result.Confidence = -netScore * diversity
		result.Reason = "rejection threshold exceeded with sufficient diversity"
		return result
	}

	// Timeout check: if pending too long, use current majority
	if now.Sub(proposalCreatedAt) > e.config.MaxPendingDuration {
		if netScore > 0 {
			result.Decision = Accepted
			result.Confidence = math.Abs(netScore) * 0.7 // lower confidence for timeout
			result.Reason = "timeout: accepted by current majority"
		} else {
			result.Decision = Rejected
			result.Confidence = math.Abs(netScore) * 0.7
			result.Reason = "timeout: rejected by current majority"
		}
		return result
	}

	result.Decision = Pending
	result.Confidence = 0
	result.Reason = "awaiting more votes or convergence"
	return result
}

// applyConformityPenalty reduces the weight of votes that cluster
// with high-reputation agents. This is the HP-02 defense.
//
// Algorithm: for each vote, check if it agrees with the majority of
// high-reputation voters. If so, discount its weight by ConformityPenalty.
// This ensures that independent thinkers have more influence than followers.
func (e *Engine) applyConformityPenalty(votes []Vote, weights []float64) []float64 {
	if e.config.ConformityPenalty == 0 {
		return weights
	}

	// Find the dominant position among top-reputation agents
	var topEndorse, topReject float64
	for _, v := range votes {
		if v.effectiveReputation() > 0.7 { // "high reputation" threshold
			if v.Position == Endorse {
				topEndorse++
			} else if v.Position == Reject {
				topReject++
			}
		}
	}

	var dominantPosition Position
	if topEndorse > topReject {
		dominantPosition = Endorse
	} else if topReject > topEndorse {
		dominantPosition = Reject
	} else {
		return weights // no clear dominant position, no penalty
	}

	// Penalize votes that agree with the dominant high-rep position
	result := make([]float64, len(weights))
	for i, v := range votes {
		if v.Position == dominantPosition {
			// Conformist: reduce weight
			result[i] = weights[i] * (1.0 - e.config.ConformityPenalty)
		} else {
			// Dissenter: keep full weight (or even boost slightly)
			result[i] = weights[i] * (1.0 + e.config.ConformityPenalty*0.2)
		}
	}
	return result
}

// adaptiveThreshold computes the acceptance threshold based on network size.
// Larger networks need lower thresholds (harder to get everyone to agree).
// Smaller networks need higher thresholds (need near-unanimity).
//
// Formula: threshold = BaseThreshold * (1 + log2(3) / log2(max(3, networkSize)))
// At N=3: threshold = BaseThreshold * 2.0 (need strong majority)
// At N=100: threshold = BaseThreshold * 1.24 (lower bar)
// At N=10000: threshold = BaseThreshold * 1.12 (even lower)
func (e *Engine) adaptiveThreshold(networkSize int) float64 {
	n := math.Max(3, float64(networkSize))
	factor := 1.0 + math.Log2(3)/math.Log2(n)
	return math.Min(0.95, e.config.BaseThreshold*factor)
}

// computeDiversity measures how diverse the voter set is.
// Based on tag overlap: if all voters have the same expertise tags,
// diversity is low. If they come from different domains, diversity is high.
//
// Returns [0, 1] where 1 = maximally diverse.
func (e *Engine) computeDiversity(votes []Vote) float64 {
	if len(votes) <= 1 {
		return 0
	}

	// Count unique tags across all voters
	allTags := make(map[string]int)
	voterTagSets := make([]map[string]bool, len(votes))

	for i, v := range votes {
		voterTagSets[i] = make(map[string]bool)
		for _, tag := range v.Tags {
			allTags[tag]++
			voterTagSets[i][tag] = true
		}
	}

	if len(allTags) == 0 {
		return 0.5 // no tags = assume moderate diversity
	}

	// Compute average pairwise Jaccard distance
	var totalDistance float64
	pairs := 0
	for i := 0; i < len(votes); i++ {
		for j := i + 1; j < len(votes); j++ {
			distance := jaccardDistance(voterTagSets[i], voterTagSets[j])
			totalDistance += distance
			pairs++
		}
	}

	if pairs == 0 {
		return 0.5
	}

	return totalDistance / float64(pairs)
}

// jaccardDistance = 1 - |A∩B| / |A∪B|
func jaccardDistance(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}

	intersection := 0
	union := make(map[string]bool)
	for k := range a {
		union[k] = true
		if b[k] {
			intersection++
		}
	}
	for k := range b {
		union[k] = true
	}

	if len(union) == 0 {
		return 0
	}

	return 1.0 - float64(intersection)/float64(len(union))
}

// shannonEntropy computes H(p1, p2, ..., pk) = -Σ pi * log2(pi)
func shannonEntropy(probs []float64) float64 {
	var h float64
	for _, p := range probs {
		if p > 0 && p < 1 {
			h -= p * math.Log2(p)
		}
	}
	return math.Max(0, h)
}
