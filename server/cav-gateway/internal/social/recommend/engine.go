// Recommendation engine — produces a ranked list of candidate trustees for
// each requester.
//
// The engine is stateless w.r.t. its own outputs: callers persist the
// recommendations and feedback (T18 wires bandit + cron + storage). This
// file defines the algorithm and the data plumbing it expects.

package recommend

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/anthropic-cav/cav-gateway/internal/social/risk"
)

// Engine produces Recommendations. It depends on:
//   - a Profiles source (one SourceProfile per active citizen)
//   - the risk engine (to attach risk vectors)
//   - a Strategy provider (the bandit; T18 wires the real one)
type Engine struct {
	profiles ProfilesProvider
	risk     *risk.Engine
	strategy StrategyProvider
}

// ProfilesProvider returns SourceProfile for every citizen the engine should
// consider as a candidate (typically: all active citizens minus probation /
// inactive). Implementations live in the wiring layer.
type ProfilesProvider interface {
	List(ctx context.Context) ([]SourceProfile, error)
	Get(ctx context.Context, did string) (SourceProfile, error)
	AlreadyTrusted(ctx context.Context, requester string) (map[string]struct{}, error)
}

// StrategyProvider returns the current bandit-selected weighting strategy
// for one citizen and accepts feedback.
type StrategyProvider interface {
	Pick(requester string) Strategy
	Record(strategy string, outcome float64)
}

// Strategy parameterizes the score function. Default values are the spec
// baseline; bandit variants tweak weights and exploration bias.
type Strategy struct {
	Key                 string  // identifier for feedback bookkeeping
	WeightMethodology   float64 // 0..1 — how much methodology distance counts
	WeightDomainOverlap float64 // 0..1
	ExplorationBias     float64 // 0..1 — fraction of slots given to exploratory tier
}

// DefaultStrategy is the spec baseline (§5.4).
var DefaultStrategy = Strategy{
	Key:                 "baseline",
	WeightMethodology:   1.0,
	WeightDomainOverlap: 1.0,
	ExplorationBias:     0.20,
}

// NewEngine builds an Engine. `strategy` may be nil — defaults to
// DefaultStrategy on every call.
func NewEngine(profiles ProfilesProvider, riskEngine *risk.Engine, strategy StrategyProvider) (*Engine, error) {
	if profiles == nil {
		return nil, errors.New("recommend: nil ProfilesProvider")
	}
	return &Engine{profiles: profiles, risk: riskEngine, strategy: strategy}, nil
}

// Generate produces up to `limit` recommendations for `requester`. The
// engine excludes the requester itself + anyone they already trust.
//
// Risk vectors are attached when `e.risk` is non-nil; otherwise risk fields
// stay zero (low-info phase 1 wiring).
func (e *Engine) Generate(ctx context.Context, requester string, limit int) ([]Recommendation, error) {
	if requester == "" {
		return nil, errors.New("recommend: requester required")
	}
	if limit <= 0 {
		limit = 5
	}

	requesterProfile, err := e.profiles.Get(ctx, requester)
	if err != nil {
		return nil, fmt.Errorf("requester profile: %w", err)
	}
	excluded, err := e.profiles.AlreadyTrusted(ctx, requester)
	if err != nil {
		return nil, fmt.Errorf("trusted set: %w", err)
	}
	if excluded == nil {
		excluded = map[string]struct{}{}
	}
	excluded[requester] = struct{}{}

	candidates, err := e.profiles.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("profiles list: %w", err)
	}

	strategy := DefaultStrategy
	if e.strategy != nil {
		strategy = e.strategy.Pick(requester)
	}

	now := time.Now()
	scored := make([]Recommendation, 0, len(candidates))

	for _, c := range candidates {
		if _, skip := excluded[c.DID]; skip {
			continue
		}
		mDist := MethodologyDistance(requesterProfile.MethodologyDistribution, c.MethodologyDistribution)
		dOverlap := DomainOverlap(requesterProfile.Domains, c.Domains)

		// Risk attachment is best-effort — failures don't drop the candidate.
		var riskAgg float64
		var riskClass string
		var riskHash string
		if e.risk != nil {
			v, rerr := e.risk.Compute(ctx, requester, c.DID)
			if rerr == nil && v != nil {
				riskAgg = v.AggregateScore
				riskClass = v.RiskClass
				riskHash = v.VectorHash
			}
		}

		score := combinedScore(mDist, dOverlap, riskAgg, strategy)
		if score <= 0 {
			continue
		}

		scored = append(scored, Recommendation{
			ID:                  fmt.Sprintf("rec_%s_%s_%d", requester, c.DID, now.UnixNano()),
			Requester:           requester,
			Subject:             c.DID,
			Tier:                classifyTier(mDist, dOverlap),
			Score:               score,
			MethodologyDistance: mDist,
			DomainOverlap:       dOverlap,
			RiskAggregate:       riskAgg,
			RiskClass:           riskClass,
			Strategy:            strategy.Key,
			GeneratedAt:         now,
			ExpiresAt:           now.Add(DefaultTTL),
			VectorHash:          riskHash,
		})
	}

	// Sort by score desc.
	sort.Slice(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })

	// Apply exploration bias: keep most slots filled by top-scoring
	// (exploitation) and reserve a fraction for exploratory tier picks.
	final := pickWithExploration(scored, limit, strategy.ExplorationBias)
	return final, nil
}

// combinedScore fuses methodology distance, domain overlap, and risk
// according to the active strategy. The risk discount is built into the
// product so a high-risk candidate gets demoted regardless of how
// methodology-distant they are.
func combinedScore(mDist, dOverlap, riskAgg float64, s Strategy) float64 {
	mPart := mDist * s.WeightMethodology
	dPart := dOverlap * s.WeightDomainOverlap
	totalW := s.WeightMethodology + s.WeightDomainOverlap
	if totalW == 0 {
		totalW = 1
	}
	combined := (mPart + dPart) / totalW * mDist * dOverlap
	combined *= 1.0 - clamp01(riskAgg)
	return clamp01(combined)
}

// classifyTier picks one of strong / moderate / exploratory.
func classifyTier(mDist, dOverlap float64) string {
	switch {
	case mDist > 0.8 && dOverlap > 0.5:
		return TierStrong
	case mDist > 0.5 && dOverlap > 0.3:
		return TierModerate
	default:
		return TierExploratory
	}
}

// pickWithExploration takes a sorted list and returns up to `limit`
// recommendations, biasing `explorationBias` of the slots toward the
// exploratory tier (drawn from the tail).
func pickWithExploration(sorted []Recommendation, limit int, explorationBias float64) []Recommendation {
	if len(sorted) == 0 {
		return nil
	}
	if limit > len(sorted) {
		limit = len(sorted)
	}
	if explorationBias <= 0 {
		return append([]Recommendation(nil), sorted[:limit]...)
	}

	exploreSlots := int(float64(limit)*clamp01(explorationBias) + 0.5)
	exploitSlots := limit - exploreSlots
	if exploitSlots < 0 {
		exploitSlots = 0
	}

	out := make([]Recommendation, 0, limit)
	chosen := map[string]bool{}
	// Top exploitation
	for _, r := range sorted[:exploitSlots] {
		out = append(out, r)
		chosen[r.ID] = true
	}
	// Exploratory: walk from the bottom of the sorted list looking for
	// exploratory-tier candidates that scored above 0.
	for i := len(sorted) - 1; i >= 0 && len(out) < limit; i-- {
		r := sorted[i]
		if chosen[r.ID] {
			continue
		}
		if r.Tier != TierExploratory {
			continue
		}
		out = append(out, r)
		chosen[r.ID] = true
	}
	// If we still don't have enough, fill from the remaining top of the list.
	for i := exploitSlots; i < len(sorted) && len(out) < limit; i++ {
		r := sorted[i]
		if chosen[r.ID] {
			continue
		}
		out = append(out, r)
		chosen[r.ID] = true
	}
	return out
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}
