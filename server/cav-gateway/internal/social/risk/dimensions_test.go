package risk

import (
	"math"
	"math/rand"
	"testing"
	"time"
)

func boolPtr(b bool) *bool { return &b }

// === GroundTruthAlignment ===

func TestGroundTruthAlignmentInsufficientWhenFew(t *testing.T) {
	d := GroundTruthAlignment(nil)
	if d.Sufficient {
		t.Error("nil input must be insufficient")
	}
	if d.SampleSize != 0 {
		t.Errorf("expected sample 0, got %d", d.SampleSize)
	}
}

func TestGroundTruthAlignmentExtremes(t *testing.T) {
	mkN := func(n int, hit bool) []PraxonRecord {
		out := make([]PraxonRecord, n)
		for i := range out {
			out[i] = PraxonRecord{ID: "p", GroundTruthMatched: boolPtr(hit)}
		}
		return out
	}
	allHit := GroundTruthAlignment(mkN(5, true))
	if allHit.Score != 0 {
		t.Errorf("all hits → score 0, got %v", allHit.Score)
	}
	if !allHit.Sufficient {
		t.Errorf("5 verified must be sufficient")
	}
	allMiss := GroundTruthAlignment(mkN(5, false))
	if allMiss.Score != 1 {
		t.Errorf("all miss → score 1, got %v", allMiss.Score)
	}
}

func TestGroundTruthAlignmentExcludesUnverified(t *testing.T) {
	rs := []PraxonRecord{
		{GroundTruthMatched: boolPtr(true)},
		{GroundTruthMatched: boolPtr(true)},
		{}, // unverified — should be ignored
		{GroundTruthMatched: boolPtr(false)},
	}
	d := GroundTruthAlignment(rs)
	// 2 hits / 3 verified = 0.67 → score = 1 - 0.67 = 0.33
	if math.Abs(d.Score-(1.0/3.0)) > 1e-9 {
		t.Errorf("expected ~0.33, got %v", d.Score)
	}
	if d.SampleSize != 3 {
		t.Errorf("expected sample=3 (verified only), got %d", d.SampleSize)
	}
}

// === MethodologyStability ===

func TestMethodologyStabilityAllMissing(t *testing.T) {
	rs := make([]PraxonRecord, 12)
	d := MethodologyStability(rs)
	if d.Score != 1.0 {
		t.Errorf("all missing methodology → score 1, got %v", d.Score)
	}
	if !d.Sufficient {
		t.Error("12 ≥ MinPraxonsForMethodology should be sufficient")
	}
}

func TestMethodologyStabilityHealthyMix(t *testing.T) {
	// Band centre maps to normalized entropy 0.575. For two equally weighted
	// methods that's an 85/15 split (H ≈ 0.42 nats, norm ≈ 0.61, deviation ≈
	// 0.035). Should give a low score.
	rs := make([]PraxonRecord, 0, 25)
	for i := 0; i < 17; i++ {
		rs = append(rs, PraxonRecord{
			PriorSourceTag: "tool", InferenceMethodTag: "deductive",
			HasMethodology: true,
		})
	}
	for i := 0; i < 3; i++ {
		rs = append(rs, PraxonRecord{
			PriorSourceTag: "observation", InferenceMethodTag: "abductive",
			HasMethodology: true,
		})
	}
	d := MethodologyStability(rs)
	if d.Score >= 0.4 {
		t.Errorf("85/15 split between two methods should be near band centre (low risk), got %v", d.Score)
	}
	if !d.Sufficient {
		t.Error("20 praxons must be sufficient (≥ 10)")
	}
}

func TestMethodologyStabilityTooScattered(t *testing.T) {
	// 5 distinct methods used with equal frequency → max entropy → high risk
	tags := []struct{ p, i string }{
		{"observation", "deductive"},
		{"observation", "abductive"},
		{"tool", "statistical"},
		{"tool", "deductive"},
		{"peer", "abductive"},
	}
	rs := make([]PraxonRecord, 0, 25)
	for i := 0; i < 5; i++ {
		for _, tg := range tags {
			rs = append(rs, PraxonRecord{
				PriorSourceTag: tg.p, InferenceMethodTag: tg.i,
				HasMethodology: true,
			})
		}
	}
	d := MethodologyStability(rs)
	// Maximum entropy with 5 equal-weight categories is far above the band
	// centre, so the calculator should flag this as high risk (undisciplined).
	if d.Score < 0.7 {
		t.Errorf("5-way uniform usage should be high risk, got %v", d.Score)
	}
}

func TestMethodologyStabilityMechanical(t *testing.T) {
	// All same tag → entropy = 0 → far below band → high score
	rs := make([]PraxonRecord, 15)
	for i := range rs {
		rs[i] = PraxonRecord{
			PriorSourceTag: "observation", InferenceMethodTag: "deductive",
			HasMethodology: true,
		}
	}
	d := MethodologyStability(rs)
	if d.Score < 0.9 {
		t.Errorf("single-tag methodology should be near max risk, got %v", d.Score)
	}
}

// === ChallengeSurvivalRate ===

func TestChallengeSurvivalSufficiency(t *testing.T) {
	if d := ChallengeSurvivalRate(nil); d.Sufficient {
		t.Error("nil challenges insufficient")
	}
	if d := ChallengeSurvivalRate([]ChallengeRecord{{Survived: true}}); d.Sufficient {
		t.Error("1 < min should not be sufficient")
	}
}

func TestChallengeSurvivalAllSurvive(t *testing.T) {
	cs := []ChallengeRecord{
		{Survived: true}, {Survived: true}, {Survived: true},
	}
	d := ChallengeSurvivalRate(cs)
	if d.Score != 0 {
		t.Errorf("all survive → score 0, got %v", d.Score)
	}
	if !d.Sufficient {
		t.Errorf("3 ≥ min should be sufficient")
	}
}

func TestChallengeSurvivalAllFail(t *testing.T) {
	cs := []ChallengeRecord{
		{Survived: false}, {Survived: false}, {Survived: false},
	}
	d := ChallengeSurvivalRate(cs)
	if d.Score != 1 {
		t.Errorf("all fail → score 1, got %v", d.Score)
	}
}

// === RetractionResponsiveness ===

func TestRetractionResponsivenessFastIsLowRisk(t *testing.T) {
	now := time.Now()
	rs := []RetractionRecord{
		{Retracted: true, GroundTruthAt: now, RetractedAt: now.Add(1 * time.Hour)},
		{Retracted: true, GroundTruthAt: now, RetractedAt: now.Add(2 * time.Hour)},
	}
	d := RetractionResponsiveness(rs)
	if d.Score >= 0.5 {
		t.Errorf("fast retractions should be < 0.5, got %v", d.Score)
	}
	if !d.Sufficient {
		t.Error("2 ≥ min should be sufficient")
	}
}

func TestRetractionResponsivenessSlowIsHighRisk(t *testing.T) {
	now := time.Now()
	rs := []RetractionRecord{
		{Retracted: true, GroundTruthAt: now, RetractedAt: now.Add(7 * 24 * time.Hour)},
		{Retracted: false, GroundTruthAt: now}, // never retracted
	}
	d := RetractionResponsiveness(rs)
	if d.Score < 0.9 {
		t.Errorf("slow / never retract should be near 1, got %v", d.Score)
	}
}

// === ConformityIndex ===

func TestConformityIndexPerfectFollower(t *testing.T) {
	now := time.Now()
	vs := make([]VoteRecord, 25)
	for i := range vs {
		pos := "endorse"
		if i%3 == 0 {
			pos = "reject"
		}
		vs[i] = VoteRecord{At: now, Position: pos, MajorityPosition: pos}
	}
	d := ConformityIndex(vs)
	if d.Score < 0.95 {
		t.Errorf("perfect follower should approach 1, got %v", d.Score)
	}
	if !d.Sufficient {
		t.Error("25 votes should be sufficient")
	}
}

func TestConformityIndexCounterMajority(t *testing.T) {
	now := time.Now()
	vs := make([]VoteRecord, 25)
	for i := range vs {
		major := "endorse"
		opp := "reject"
		if i%2 == 0 {
			major, opp = opp, major
		}
		vs[i] = VoteRecord{At: now, Position: opp, MajorityPosition: major}
	}
	d := ConformityIndex(vs)
	if d.Score > 0.05 {
		t.Errorf("counter-majority should approach 0, got %v", d.Score)
	}
}

// PBT: conformity_index is monotonic in fraction-of-agreement.
func TestConformityIndexMonotonicityPBT(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for iter := 0; iter < 100; iter++ {
		n := 30
		// Generate two agreement fractions, keep the one that should produce
		// a strictly higher score.
		fA := rng.Float64()
		fB := rng.Float64()
		if math.Abs(fA-fB) < 0.1 {
			continue
		}
		votesA := genVotesAtFraction(rng, n, fA)
		votesB := genVotesAtFraction(rng, n, fB)
		dA := ConformityIndex(votesA).Score
		dB := ConformityIndex(votesB).Score
		// Higher agreement fraction → higher (or roughly equal) score.
		if fA > fB && dA < dB-0.15 {
			t.Errorf("iter %d: fA=%v dA=%v but fB=%v dB=%v (expected dA ≥ dB)",
				iter, fA, dA, fB, dB)
		}
	}
}

func genVotesAtFraction(rng *rand.Rand, n int, agreeFraction float64) []VoteRecord {
	out := make([]VoteRecord, n)
	now := time.Now()
	for i := range out {
		major := "endorse"
		if rng.Float64() < 0.5 {
			major = "reject"
		}
		var pos string
		if rng.Float64() < agreeFraction {
			pos = major
		} else if major == "endorse" {
			pos = "reject"
		} else {
			pos = "endorse"
		}
		out[i] = VoteRecord{At: now, Position: pos, MajorityPosition: major}
	}
	return out
}

// === SybilSimilarityMax ===

func TestSybilNoOthers(t *testing.T) {
	d := SybilSimilarityMax(FingerprintFeatures{}, 100, nil)
	if d.Score != 0 {
		t.Errorf("no others → 0 sim, got %v", d.Score)
	}
}

func TestSybilIdenticalFingerprintsHighScore(t *testing.T) {
	fp := FingerprintFeatures{
		IntervalMean: 30, IntervalStddev: 5,
		HourBuckets: [24]float64{0: 0.1, 12: 0.5, 18: 0.4},
		TagUsage:    map[string]float64{"crypto": 0.5, "ml": 0.5},
	}
	others := map[string]FingerprintFeatures{"sibling": fp}
	d := SybilSimilarityMax(fp, 100, others)
	if d.Score < 0.99 {
		t.Errorf("identical fingerprints should give cosine ≈ 1, got %v", d.Score)
	}
	if !d.Sufficient {
		t.Error("100h > min should be sufficient")
	}
}

func TestSybilOrthogonalFingerprintsLowScore(t *testing.T) {
	a := FingerprintFeatures{
		IntervalMean: 30, IntervalStddev: 5,
		TagUsage: map[string]float64{"crypto": 1},
	}
	b := FingerprintFeatures{
		IntervalMean: 0, IntervalStddev: 0,
		TagUsage: map[string]float64{"forensics": 1},
	}
	d := SybilSimilarityMax(a, 100, map[string]FingerprintFeatures{"x": b})
	// Some shared structure (interval scalars in a are nonzero; b is zero).
	// Expect low cosine.
	if d.Score >= 0.5 {
		t.Errorf("orthogonal fingerprints should give low sim, got %v", d.Score)
	}
}

func TestSybilInsufficientYoungAgent(t *testing.T) {
	fp := FingerprintFeatures{IntervalMean: 30}
	d := SybilSimilarityMax(fp, 5, map[string]FingerprintFeatures{"x": fp})
	if d.Sufficient {
		t.Error("agent age below min should not be sufficient")
	}
}

// === ActivityAnomalyScore ===

func TestActivityAnomalyMatchingDistributionLowRisk(t *testing.T) {
	d := ActivityAnomalyScore(
		[]float64{10, 10, 10, 10, 10, 10, 10},
		[]float64{10, 10, 10, 10, 10, 10, 10},
		7,
	)
	if d.Score >= 0.5 {
		t.Errorf("matching distribution should be near-zero risk; got %v", d.Score)
	}
	if !d.Sufficient {
		t.Error("7 days should be sufficient")
	}
}

func TestActivityAnomalyMismatchHigherRisk(t *testing.T) {
	d := ActivityAnomalyScore(
		[]float64{0, 0, 0, 0, 0, 0, 100},
		[]float64{10, 10, 10, 10, 10, 10, 10},
		7,
	)
	if d.Score <= 0.5 {
		t.Errorf("burst-only behavior should exceed baseline anomaly threshold; got %v", d.Score)
	}
}

// === DiversityImpact ===

func TestDiversityImpactZeroOnFirstEdge(t *testing.T) {
	subj := DomainActivityVector{"crypto": 1.0}
	d := DiversityImpact(nil, subj)
	if d.Score != 0 {
		t.Errorf("first edge should be 0 risk, got %v", d.Score)
	}
}

func TestDiversityImpactReducesDiversity(t *testing.T) {
	// requester is balanced across two domains; subject is 100% in one of them
	// → adding subject moves the distribution toward one side.
	requester := DomainActivityVector{"crypto": 0.5, "ml": 0.5}
	subject := DomainActivityVector{"crypto": 1.0}
	d := DiversityImpact(requester, subject)
	if d.Score == 0 {
		t.Errorf("expected non-zero diversity loss, got %v", d.Score)
	}
}

func TestDiversityImpactBroadensCoverage(t *testing.T) {
	// Subject brings a domain the requester doesn't have → no loss
	requester := DomainActivityVector{"crypto": 1.0}
	subject := DomainActivityVector{"forensics": 1.0}
	d := DiversityImpact(requester, subject)
	if d.Score != 0 {
		t.Errorf("expanding into new domain should be 0 risk, got %v", d.Score)
	}
}

// === EchoChamberDelta ===

func TestEchoChamberDeltaMaxDominates(t *testing.T) {
	d := EchoChamberDelta(0.4, []float64{0.85, 0.3, 0.2})
	if d.Score != 0.85 {
		t.Errorf("max should dominate, got %v", d.Score)
	}
}

func TestEchoChamberDeltaSubjectConformityWhenNoExisting(t *testing.T) {
	d := EchoChamberDelta(0.7, nil)
	if d.Score != 0.7 {
		t.Errorf("expected subject conformity, got %v", d.Score)
	}
}

// === Sample-size confidence helpers ===

func TestConfidenceMonotonicInSampleSize(t *testing.T) {
	last := 0.0
	for n := 1; n <= 50; n++ {
		c := confidenceFromCount(n, 10)
		if c < last {
			t.Errorf("confidence non-monotonic at n=%d (%v < %v)", n, c, last)
		}
		last = c
	}
}
