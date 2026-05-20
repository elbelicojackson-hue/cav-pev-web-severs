package recommend

import (
	"context"
	"math"
	"testing"
	"time"
)

// === Distance ===

func TestMethodologyDistanceIdenticalIsZero(t *testing.T) {
	d := MethodologyDistance(
		map[string]float64{"tool|deductive": 5, "obs|abductive": 2},
		map[string]float64{"tool|deductive": 5, "obs|abductive": 2},
	)
	if d > 1e-9 {
		t.Errorf("identical distributions → 0, got %v", d)
	}
}

func TestMethodologyDistanceDisjointIsHigh(t *testing.T) {
	d := MethodologyDistance(
		map[string]float64{"tool|deductive": 1},
		map[string]float64{"obs|abductive": 1},
	)
	if d < 0.5 {
		t.Errorf("disjoint distributions → high distance, got %v", d)
	}
}

func TestDomainOverlapIdenticalIsOne(t *testing.T) {
	a := map[string]struct{}{"crypto": {}, "ml": {}}
	b := map[string]struct{}{"crypto": {}, "ml": {}}
	if got := DomainOverlap(a, b); math.Abs(got-1) > 1e-9 {
		t.Errorf("identical domains → 1, got %v", got)
	}
}

func TestDomainOverlapDisjointIsZero(t *testing.T) {
	a := map[string]struct{}{"crypto": {}}
	b := map[string]struct{}{"ml": {}}
	if got := DomainOverlap(a, b); got != 0 {
		t.Errorf("disjoint domains → 0, got %v", got)
	}
}

func TestDomainOverlapPartial(t *testing.T) {
	a := map[string]struct{}{"crypto": {}, "ml": {}}
	b := map[string]struct{}{"ml": {}, "forensics": {}}
	// |inter|=1, |union|=3 → 1/3
	if got := DomainOverlap(a, b); math.Abs(got-1.0/3.0) > 1e-9 {
		t.Errorf("partial overlap should be 1/3, got %v", got)
	}
}

// === Engine ===

type fakeProfiles struct {
	profiles map[string]SourceProfile
	trusted  map[string]struct{}
}

func (f *fakeProfiles) List(ctx context.Context) ([]SourceProfile, error) {
	out := make([]SourceProfile, 0, len(f.profiles))
	for _, p := range f.profiles {
		out = append(out, p)
	}
	return out, nil
}

func (f *fakeProfiles) Get(ctx context.Context, did string) (SourceProfile, error) {
	if p, ok := f.profiles[did]; ok {
		return p, nil
	}
	return SourceProfile{DID: did, MethodologyDistribution: map[string]float64{}, Domains: map[string]struct{}{}}, nil
}

func (f *fakeProfiles) AlreadyTrusted(ctx context.Context, requester string) (map[string]struct{}, error) {
	return f.trusted, nil
}

func TestEngineExcludesSelfAndTrusted(t *testing.T) {
	prof := &fakeProfiles{
		profiles: map[string]SourceProfile{
			"requester": {DID: "requester", Domains: map[string]struct{}{"crypto": {}}, MethodologyDistribution: map[string]float64{"tool|deductive": 1}},
			"trusted":   {DID: "trusted", Domains: map[string]struct{}{"ml": {}}, MethodologyDistribution: map[string]float64{"obs|abductive": 1}},
			"candidate": {DID: "candidate", Domains: map[string]struct{}{"crypto": {}, "ml": {}}, MethodologyDistribution: map[string]float64{"obs|abductive": 1}},
		},
		trusted: map[string]struct{}{"trusted": {}},
	}
	e, _ := NewEngine(prof, nil, nil)
	recs, err := e.Generate(context.Background(), "requester", 5)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recs {
		if r.Subject == "requester" || r.Subject == "trusted" {
			t.Errorf("should exclude %q", r.Subject)
		}
	}
}

func TestEngineRespectsLimit(t *testing.T) {
	profiles := map[string]SourceProfile{
		"requester": {DID: "requester", Domains: map[string]struct{}{"crypto": {}}, MethodologyDistribution: map[string]float64{"tool|deductive": 1}},
	}
	for i := 0; i < 20; i++ {
		id := "cand" + string(rune('a'+i%26))
		profiles[id] = SourceProfile{
			DID:                     id,
			Domains:                 map[string]struct{}{"crypto": {}, "ml": {}},
			MethodologyDistribution: map[string]float64{"obs|abductive": 1},
		}
	}
	prof := &fakeProfiles{profiles: profiles}
	e, _ := NewEngine(prof, nil, nil)
	recs, _ := e.Generate(context.Background(), "requester", 3)
	if len(recs) > 3 {
		t.Errorf("limit respected: got %d > 3", len(recs))
	}
}

func TestEngineSortsByScoreDescending(t *testing.T) {
	prof := &fakeProfiles{
		profiles: map[string]SourceProfile{
			"requester": {DID: "requester", Domains: map[string]struct{}{"crypto": {}}, MethodologyDistribution: map[string]float64{"tool|deductive": 1}},
			// "near" has slightly different methodology
			"near": {DID: "near", Domains: map[string]struct{}{"crypto": {}}, MethodologyDistribution: map[string]float64{"tool|deductive": 7, "obs|abductive": 3}},
			// "distant" is fully different methodology
			"distant": {DID: "distant", Domains: map[string]struct{}{"crypto": {}}, MethodologyDistribution: map[string]float64{"obs|abductive": 1}},
		},
	}
	// Use no exploration so we exercise pure exploitation ordering.
	e := &Engine{profiles: prof, strategy: fixedStrategy{Strategy{Key: "no-explore", WeightMethodology: 1, WeightDomainOverlap: 1, ExplorationBias: 0}}}
	recs, _ := e.Generate(context.Background(), "requester", 5)
	if len(recs) < 2 {
		t.Fatalf("expected at least 2 recommendations, got %v", subjects(recs))
	}
	// The first one should be "distant" (the closer match scores lower).
	if recs[0].Subject != "distant" {
		t.Errorf("expected distant first, got order: %+v", subjects(recs))
	}
}

type fixedStrategy struct{ s Strategy }

func (f fixedStrategy) Pick(string) Strategy        { return f.s }
func (f fixedStrategy) Record(string, float64)     {}

func subjects(rs []Recommendation) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Subject
	}
	return out
}

func TestClassifyTier(t *testing.T) {
	if got := classifyTier(0.9, 0.6); got != TierStrong {
		t.Errorf("strong: got %s", got)
	}
	if got := classifyTier(0.6, 0.4); got != TierModerate {
		t.Errorf("moderate: got %s", got)
	}
	if got := classifyTier(0.2, 0.1); got != TierExploratory {
		t.Errorf("exploratory: got %s", got)
	}
}

// === Bandit ===

func TestBanditStartsExplorative(t *testing.T) {
	b := NewBandit()
	picks := map[string]int{}
	for i := 0; i < 200; i++ {
		picks[b.Pick("req").Key]++
	}
	// With no rewards recorded, all arms have mean=0; ε=0.20 means we should
	// see a spread across arms (no single arm dominates).
	if len(picks) < 2 {
		t.Errorf("with cold start, bandit should explore multiple arms; saw %v", picks)
	}
}

func TestBanditLearnsFromRewards(t *testing.T) {
	b := NewBandit()
	// Pick once to know an arm.
	first := b.Pick("req").Key
	for i := 0; i < 100; i++ {
		b.Record(first, 1.0) // strongly reward this arm
	}
	// Now most picks should land on `first` (1-ε exploitation).
	hits := 0
	for i := 0; i < 200; i++ {
		if b.Pick("req").Key == first {
			hits++
		}
	}
	if hits < 100 {
		t.Errorf("bandit should exploit rewarded arm; hits=%d/200", hits)
	}
}

func TestBanditClampsRewards(t *testing.T) {
	b := NewBandit()
	first := b.Pick("req").Key
	b.Record(first, 999) // way out of range
	snap := b.Snapshot()
	if snap[first].Mean > 1 {
		t.Errorf("reward should clamp to ≤1, got %v", snap[first].Mean)
	}
}

func TestBanditUnknownArmIgnored(t *testing.T) {
	b := NewBandit()
	b.Record("nonexistent_arm", 1.0)
	snap := b.Snapshot()
	for _, s := range snap {
		if s.Count != 0 {
			t.Errorf("unknown arm shouldn't have updated any other; got count=%d", s.Count)
		}
	}
}

func TestBanditSnapshotRestore(t *testing.T) {
	b := NewBandit()
	first := b.Pick("req").Key
	b.Record(first, 0.8)
	snap := b.Snapshot()

	b2 := NewBandit()
	b2.Restore(snap)
	snap2 := b2.Snapshot()
	if snap2[first].Mean != snap[first].Mean {
		t.Errorf("restore lost mean: %v vs %v", snap[first].Mean, snap2[first].Mean)
	}
}

func TestRewardFromDeltas(t *testing.T) {
	// Conformity drop is good (negative delta → positive reward)
	r := RewardFromDeltas(-0.3, 0, 0)
	if r <= 0 {
		t.Errorf("conformity drop should produce positive reward, got %v", r)
	}
	// Diversity gain is good
	r = RewardFromDeltas(0, 0.3, 0)
	if r <= 0 {
		t.Errorf("diversity gain should produce positive reward, got %v", r)
	}
	// All bad
	r = RewardFromDeltas(0.5, -0.5, -0.5)
	if r >= 0 {
		t.Errorf("all-bad deltas should produce negative reward, got %v", r)
	}
	// Reward stays in [-1, 1]
	r = RewardFromDeltas(-100, 100, 100)
	if r > 1 || r < -1 {
		t.Errorf("reward must clamp to [-1, 1], got %v", r)
	}
}

// === Feedback ===

func TestFeedbackScheduleAndDue(t *testing.T) {
	fs := NewFeedbackStore()
	now := someTime()
	rec := FeedbackRecord{
		RecommendationID: "rec1",
		Strategy:         "s1",
		Requester:        "req",
		Subject:          "subj",
		BaselineAt:       now,
		ObserveAt:        now.Add(time.Hour),
	}
	if err := fs.ScheduleObservation(rec); err != nil {
		t.Fatal(err)
	}
	if fs.PendingCount() != 1 {
		t.Errorf("expected 1 pending, got %d", fs.PendingCount())
	}
	// Not due yet
	if due := fs.Due(now.Add(30 * time.Minute)); len(due) != 0 {
		t.Errorf("not due, got %d", len(due))
	}
	// Due
	due := fs.Due(now.Add(2 * time.Hour))
	if len(due) != 1 {
		t.Errorf("expected 1 due, got %d", len(due))
	}
	// Removed once returned
	if fs.PendingCount() != 0 {
		t.Errorf("expected 0 pending after Due, got %d", fs.PendingCount())
	}
}

func TestFeedbackRequiresFields(t *testing.T) {
	fs := NewFeedbackStore()
	now := someTime()
	if err := fs.ScheduleObservation(FeedbackRecord{Strategy: "s", BaselineAt: now, ObserveAt: now.Add(time.Hour)}); err == nil {
		t.Error("missing recommendation_id should fail")
	}
	if err := fs.ScheduleObservation(FeedbackRecord{RecommendationID: "r", BaselineAt: now, ObserveAt: now.Add(time.Hour)}); err == nil {
		t.Error("missing strategy should fail")
	}
	if err := fs.ScheduleObservation(FeedbackRecord{
		RecommendationID: "r", Strategy: "s",
		BaselineAt: now, ObserveAt: now.Add(-time.Hour),
	}); err == nil {
		t.Error("observe before baseline should fail")
	}
}

func TestShouldTriggerExtraConformityRising(t *testing.T) {
	c := ConformityHistory{Values: []float64{0.3, 0.4, 0.5, 0.6}}
	d := DiversityHistory{}
	if !ShouldTriggerExtraRecommendation(c, d, 0) {
		t.Error("rising conformity should trigger")
	}
}

func TestShouldTriggerExtraDiversityFalling(t *testing.T) {
	c := ConformityHistory{}
	d := DiversityHistory{Values: []float64{0.7, 0.5, 0.3}}
	if !ShouldTriggerExtraRecommendation(c, d, 0) {
		t.Error("falling diversity should trigger")
	}
}

func TestShouldNotTriggerOnNoise(t *testing.T) {
	c := ConformityHistory{Values: []float64{0.3, 0.4, 0.3, 0.4}}
	d := DiversityHistory{Values: []float64{0.5, 0.5, 0.5}}
	if ShouldTriggerExtraRecommendation(c, d, 3) {
		t.Error("non-monotone signal should not trigger")
	}
}

func TestShouldNotTriggerWithFewSamples(t *testing.T) {
	c := ConformityHistory{Values: []float64{0.3, 0.4}} // only 2 samples
	d := DiversityHistory{}
	if ShouldTriggerExtraRecommendation(c, d, 3) {
		t.Error("too few samples should not trigger")
	}
}

// helper — this avoids importing time in places that don't already have it
func someTime() time.Time {
	return time.Now().Truncate(time.Second)
}
