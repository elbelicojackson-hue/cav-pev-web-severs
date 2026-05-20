package consensus

import (
	"math"
	"testing"
	"time"

	"github.com/anthropic-cav/cav-gateway/internal/social/reputation"
)

func TestEngineLegacyPathUnchanged(t *testing.T) {
	// Sanity: with no vectors attached, the engine produces the same
	// evaluation it always did. We exercise the path with two scenarios:
	//
	//  1. Strong unanimous endorsement → should accept.
	//  2. Mixed votes → behavior unchanged (we just confirm no panic / no
	//     crash from the new effectiveReputation indirection).
	cfg := DefaultConfig()
	cfg.MinVotes = 3
	cfg.ConformityPenalty = 0 // isolate from penalty effects in scenario 1
	eng := NewEngine(cfg)

	now := time.Now()

	// Scenario 1: 4 endorsers with high reputation, no rejections.
	// Should accept clearly via entropy convergence.
	unanimous := []Vote{
		{AgentFingerprint: "a", Position: Endorse, Confidence: 0.95, Reputation: 0.9, Timestamp: now, Tags: []string{"crypto"}},
		{AgentFingerprint: "b", Position: Endorse, Confidence: 0.95, Reputation: 0.9, Timestamp: now, Tags: []string{"ml"}},
		{AgentFingerprint: "c", Position: Endorse, Confidence: 0.95, Reputation: 0.9, Timestamp: now, Tags: []string{"forensics"}},
		{AgentFingerprint: "d", Position: Endorse, Confidence: 0.95, Reputation: 0.9, Timestamp: now, Tags: []string{"net"}},
	}
	res := eng.Evaluate(unanimous, 10, now.Add(-time.Hour))
	if res.Decision != Accepted {
		t.Errorf("legacy-path unanimous endorsement: expected Accepted, got %s (reason=%q)",
			res.Decision, res.Reason)
	}

	// Scenario 2: mixed — exercises the path without panicking.
	mixed := []Vote{
		{AgentFingerprint: "a", Position: Endorse, Confidence: 0.9, Reputation: 0.8, Timestamp: now, Tags: []string{"crypto"}},
		{AgentFingerprint: "b", Position: Reject, Confidence: 0.6, Reputation: 0.4, Timestamp: now, Tags: []string{"forensics"}},
		{AgentFingerprint: "c", Position: Abstain, Confidence: 0.5, Reputation: 0.5, Timestamp: now, Tags: []string{"net"}},
	}
	res2 := eng.Evaluate(mixed, 10, now.Add(-time.Hour))
	if res2.VoteCount != 3 {
		t.Errorf("expected VoteCount=3, got %d", res2.VoteCount)
	}
	// Determinism check: same input twice → same result.
	res2b := eng.Evaluate(mixed, 10, now.Add(-time.Hour))
	if res2.Decision != res2b.Decision || math.Abs(res2.WeightedScore-res2b.WeightedScore) > 1e-12 {
		t.Errorf("non-deterministic legacy path: %+v vs %+v", res2, res2b)
	}
}

func TestEngineVectorOverridesScalar(t *testing.T) {
	// When a ReputationVector is attached, the EffectiveScore for the
	// matching domain must beat the scalar Reputation field.
	cfg := DefaultConfig()
	cfg.MinVotes = 2
	cfg.ConformityPenalty = 0 // isolate the weighting effect
	eng := NewEngine(cfg)

	now := time.Now()
	highVec := reputation.NewVector("did:high", now)
	highVec.Operational.Domains["crypto"] = reputation.DomainScore{
		Score: 0.9, Confidence: 1.0, SampleSize: 30, LastUpdated: now,
	}
	lowVec := reputation.NewVector("did:low", now)
	lowVec.Operational.Domains["crypto"] = reputation.DomainScore{
		Score: 0.1, Confidence: 1.0, SampleSize: 30, LastUpdated: now,
	}

	// Two votes with identical scalar Reputation but very different vectors.
	// Endorse-side has high vector, reject-side has low vector → endorse wins.
	votes := []Vote{
		{
			AgentFingerprint: "high", Position: Endorse, Confidence: 1.0,
			Reputation: 0.5, Timestamp: now,
			ReputationVector: highVec, Domain: "crypto",
			Tags: []string{"crypto", "ml"},
		},
		{
			AgentFingerprint: "low", Position: Reject, Confidence: 1.0,
			Reputation: 0.5, Timestamp: now,
			ReputationVector: lowVec, Domain: "crypto",
			Tags: []string{"forensics"},
		},
	}
	res := eng.Evaluate(votes, 10, now.Add(-time.Hour))
	if res.WeightedScore <= 0 {
		t.Errorf("expected endorse to win on vector strength, got score=%v reason=%q",
			res.WeightedScore, res.Reason)
	}
}

func TestEngineVectorEqualsScalarWhenNil(t *testing.T) {
	// Vote with nil vector but scalar set → must produce the same weight as
	// vote with vector set to a single-domain copy of the scalar.
	cfg := DefaultConfig()
	cfg.MinVotes = 2
	cfg.ConformityPenalty = 0
	eng := NewEngine(cfg)

	now := time.Now()
	scalarOnly := []Vote{
		{AgentFingerprint: "a", Position: Endorse, Confidence: 1.0, Reputation: 0.6, Timestamp: now, Tags: []string{"crypto"}},
		{AgentFingerprint: "b", Position: Endorse, Confidence: 1.0, Reputation: 0.6, Timestamp: now, Tags: []string{"ml"}},
	}
	resScalar := eng.Evaluate(scalarOnly, 10, now.Add(-time.Hour))

	// Same agents, attach vectors with identical effective score.
	v := reputation.NewVector("did:a", now)
	v.Operational.Domains["crypto"] = reputation.DomainScore{
		Score: 0.6, Confidence: 1.0, SampleSize: 5, LastUpdated: now,
	}
	v.Operational.Domains["ml"] = reputation.DomainScore{
		Score: 0.6, Confidence: 1.0, SampleSize: 5, LastUpdated: now,
	}
	withVec := []Vote{
		{AgentFingerprint: "a", Position: Endorse, Confidence: 1.0, Reputation: 0.6, ReputationVector: v, Domain: "crypto", Timestamp: now, Tags: []string{"crypto"}},
		{AgentFingerprint: "b", Position: Endorse, Confidence: 1.0, Reputation: 0.6, ReputationVector: v, Domain: "ml", Timestamp: now, Tags: []string{"ml"}},
	}
	resVec := eng.Evaluate(withVec, 10, now.Add(-time.Hour))

	if math.Abs(resScalar.WeightedScore-resVec.WeightedScore) > 1e-9 {
		t.Errorf("scalar(%v) and equivalent-vector(%v) paths must produce the same weighted score",
			resScalar.WeightedScore, resVec.WeightedScore)
	}
}

func TestEngineVectorUnknownDomainFallsBackToScalar(t *testing.T) {
	// If the vote names a domain that the vector has no record of, we fall
	// back to scalar Reputation rather than zero-out the agent. This protects
	// citizens whose seed Vector hasn't been populated for a brand-new domain.
	cfg := DefaultConfig()
	cfg.MinVotes = 2
	cfg.ConformityPenalty = 0
	eng := NewEngine(cfg)

	now := time.Now()
	v := reputation.NewVector("did:a", now)
	// Note: vector has no "crypto" domain.

	votes := []Vote{
		{AgentFingerprint: "a", Position: Endorse, Confidence: 1.0, Reputation: 0.7, ReputationVector: v, Domain: "crypto", Timestamp: now},
		{AgentFingerprint: "b", Position: Endorse, Confidence: 1.0, Reputation: 0.7, Timestamp: now},
	}
	res := eng.Evaluate(votes, 10, now.Add(-time.Hour))
	if res.TotalWeight <= 0 {
		t.Errorf("expected non-zero weight via scalar fallback, got %v", res.TotalWeight)
	}
}

func TestEngineMinVotesGuard(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MinVotes = 3
	eng := NewEngine(cfg)
	now := time.Now()
	res := eng.Evaluate([]Vote{
		{Position: Endorse, Confidence: 1, Reputation: 1, Timestamp: now},
	}, 10, now.Add(-time.Hour))
	if res.Decision != Pending {
		t.Errorf("with too few votes expected Pending, got %s", res.Decision)
	}
}

func TestEngineVectorParticipatesInConformityPenalty(t *testing.T) {
	// The conformity penalty's "high reputation" detection must also use the
	// effective score, not the legacy scalar field. Otherwise the penalty
	// mistargets agents.
	cfg := DefaultConfig()
	cfg.MinVotes = 3
	cfg.ConformityPenalty = 0.5
	eng := NewEngine(cfg)

	now := time.Now()
	highVec := reputation.NewVector("did:hi", now)
	highVec.Operational.Domains["crypto"] = reputation.DomainScore{
		Score: 0.9, Confidence: 1.0, SampleSize: 30, LastUpdated: now,
	}

	// Three high-vector endorsers (scalar=0 to confirm vector path is used)
	// and one low-vector dissenter — the penalty should kick in based on the
	// vector path.
	dissentVec := reputation.NewVector("did:lo", now)
	dissentVec.Operational.Domains["crypto"] = reputation.DomainScore{
		Score: 0.1, Confidence: 1.0, SampleSize: 30, LastUpdated: now,
	}
	votes := []Vote{
		{AgentFingerprint: "h1", Position: Endorse, Confidence: 1, Reputation: 0, ReputationVector: highVec, Domain: "crypto", Timestamp: now},
		{AgentFingerprint: "h2", Position: Endorse, Confidence: 1, Reputation: 0, ReputationVector: highVec, Domain: "crypto", Timestamp: now},
		{AgentFingerprint: "h3", Position: Endorse, Confidence: 1, Reputation: 0, ReputationVector: highVec, Domain: "crypto", Timestamp: now},
		{AgentFingerprint: "d1", Position: Reject, Confidence: 1, Reputation: 0, ReputationVector: dissentVec, Domain: "crypto", Timestamp: now},
	}
	res := eng.Evaluate(votes, 10, now.Add(-time.Hour))
	// Endorse should still win (3 high-vector endorsers vs 1 low-vector reject)
	// but the conformity penalty should attenuate the margin compared to the
	// no-penalty case.
	if res.WeightedScore <= 0 {
		t.Errorf("expected endorse to win, got score=%v", res.WeightedScore)
	}

	// Re-run with no penalty, confirm the score is higher (penalty attenuates).
	cfg2 := DefaultConfig()
	cfg2.MinVotes = 3
	cfg2.ConformityPenalty = 0
	eng2 := NewEngine(cfg2)
	resNoPenalty := eng2.Evaluate(votes, 10, now.Add(-time.Hour))
	if resNoPenalty.WeightedScore <= res.WeightedScore {
		t.Errorf("conformity penalty should attenuate; got penalty=%v noPenalty=%v",
			res.WeightedScore, resNoPenalty.WeightedScore)
	}
}
