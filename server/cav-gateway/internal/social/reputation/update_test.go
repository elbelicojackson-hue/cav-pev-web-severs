package reputation

import (
	"math"
	"math/rand"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "rep")
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestApplyValidationRejectsBadEvents(t *testing.T) {
	s := newTestStore(t)
	cases := []struct {
		name string
		ev   Event
	}{
		{"missing DID", Event{Domain: "crypto", Tier: TierOperational, Trigger: "x"}},
		{"missing Domain", Event{DID: "d", Tier: TierOperational, Trigger: "x"}},
		{"missing Tier", Event{DID: "d", Domain: "crypto", Trigger: "x"}},
		{"bad Tier", Event{DID: "d", Domain: "crypto", Tier: "weird", Trigger: "x"}},
		{"missing Trigger", Event{DID: "d", Domain: "crypto", Tier: TierOperational}},
		{"NaN Delta", Event{DID: "d", Domain: "crypto", Tier: TierOperational, Trigger: "x", Delta: math.NaN()}},
		{"Inf Delta", Event{DID: "d", Domain: "crypto", Tier: TierOperational, Trigger: "x", Delta: math.Inf(1)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := s.Apply(tc.ev); err == nil {
				t.Errorf("Apply should reject %s", tc.name)
			}
		})
	}
}

func TestApplyCreatesVectorOnDemand(t *testing.T) {
	s := newTestStore(t)
	ev := Event{
		DID: "did:cav:fresh", Domain: "crypto", Tier: TierOperational,
		Trigger: TriggerCanaryCompleted, Delta: 0.4,
	}
	if err := s.Apply(ev); err != nil {
		t.Fatalf("apply: %v", err)
	}
	v := s.Get("did:cav:fresh")
	d, ok := v.Operational.Domains["crypto"]
	if !ok {
		t.Fatal("expected domain to be created")
	}
	if math.Abs(d.Score-0.4) > 1e-9 {
		t.Errorf("score=%v, want 0.4", d.Score)
	}
	if d.SampleSize != 1 {
		t.Errorf("sample size should be 1, got %d", d.SampleSize)
	}
}

func TestApplyClampsScoreToUnitInterval(t *testing.T) {
	s := newTestStore(t)
	// Push above 1
	for i := 0; i < 50; i++ {
		s.Apply(Event{
			DID: "d", Domain: "crypto", Tier: TierOperational,
			Trigger: TriggerChallengeSurvived, Delta: 0.5,
			OccurredAt: time.Now().Add(time.Duration(i) * time.Millisecond),
		})
	}
	if s.Get("d").Operational.Domains["crypto"].Score > 1.0 {
		t.Error("score must clamp to 1")
	}
	// Push below 0
	for i := 0; i < 50; i++ {
		s.Apply(Event{
			DID: "d", Domain: "crypto", Tier: TierOperational,
			Trigger: TriggerChallengeFailed, Delta: -0.5,
			OccurredAt: time.Now().Add(time.Duration(i) * time.Millisecond),
		})
	}
	if s.Get("d").Operational.Domains["crypto"].Score < 0 {
		t.Error("score must clamp to 0")
	}
}

func TestApplyConfidenceGrowsWithSampleSize(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 30; i++ {
		s.Apply(Event{
			DID: "d", Domain: "crypto", Tier: TierOperational,
			Trigger: TriggerCanaryCompleted, Delta: 0.01,
			OccurredAt: time.Now().Add(time.Duration(i) * time.Millisecond),
		})
	}
	d := s.Get("d").Operational.Domains["crypto"]
	if d.Confidence < 0.85 || d.Confidence > 1.0 {
		t.Errorf("after 30 events, confidence should be ~0.875, got %v", d.Confidence)
	}
	if d.SampleSize != 30 {
		t.Errorf("sample size should be 30, got %d", d.SampleSize)
	}
}

func TestApplyEventLoggedToAuditTrail(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 3; i++ {
		s.Apply(Event{
			DID: "d", Domain: "crypto", Tier: TierOperational,
			Trigger: TriggerCanaryCompleted, Delta: 0.1,
			OccurredAt: time.Unix(int64(i+1), 0).UTC(),
		})
	}
	evs, err := s.Events("d")
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(evs) != 3 {
		t.Fatalf("expected 3 events, got %d", len(evs))
	}
	for i, ev := range evs {
		if ev.Trigger != TriggerCanaryCompleted {
			t.Errorf("event %d: trigger=%q", i, ev.Trigger)
		}
	}
}

// TestProcessGroundTruthFollowerPenaltyAmplified verifies the design §5.3
// invariant: for a wrong outcome, follower's blame is 1.5× independent's blame.
func TestProcessGroundTruthFollowerPenaltyAmplified(t *testing.T) {
	s := newTestStore(t)

	// Two participants, same confidence. One independent, one follower.
	now := time.Now()
	parts := []EpisodeParticipation{
		{DID: "did:indep", Domain: "crypto", Role: RoleIndependent, Confidence: 0.8},
		{DID: "did:follow", Domain: "crypto", Role: RoleFollower, Confidence: 0.8},
	}
	if err := s.ProcessGroundTruth("ep1", false, parts, now); err != nil {
		t.Fatalf("process: %v", err)
	}

	indepEv, _ := s.Events("did:indep")
	followEv, _ := s.Events("did:follow")
	if len(indepEv) != 1 || len(followEv) != 1 {
		t.Fatalf("expected 1 event each; got %d/%d", len(indepEv), len(followEv))
	}
	indepDelta := indepEv[0].Delta
	followDelta := followEv[0].Delta

	// Both should be negative (wrong outcome).
	if indepDelta >= 0 || followDelta >= 0 {
		t.Fatalf("both deltas should be negative, got indep=%v follow=%v", indepDelta, followDelta)
	}
	ratio := followDelta / indepDelta // both negative → positive ratio
	if math.Abs(ratio-1.5) > 1e-9 {
		t.Errorf("follower penalty should be 1.5× independent, got ratio=%v (indep=%v, follow=%v)",
			ratio, indepDelta, followDelta)
	}
}

// TestProcessGroundTruthFollowerCreditAttenuated verifies the symmetric rule
// for correct outcomes: follower credit = 0.3 × independent credit.
func TestProcessGroundTruthFollowerCreditAttenuated(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()
	parts := []EpisodeParticipation{
		{DID: "did:indep", Domain: "crypto", Role: RoleIndependent, Confidence: 1.0},
		{DID: "did:follow", Domain: "crypto", Role: RoleFollower, Confidence: 1.0},
	}
	if err := s.ProcessGroundTruth("ep_correct", true, parts, now); err != nil {
		t.Fatalf("process: %v", err)
	}
	indepEv, _ := s.Events("did:indep")
	followEv, _ := s.Events("did:follow")
	if indepEv[0].Delta <= 0 || followEv[0].Delta <= 0 {
		t.Fatal("correct outcome should yield positive deltas")
	}
	ratio := followEv[0].Delta / indepEv[0].Delta
	if math.Abs(ratio-0.3) > 1e-9 {
		t.Errorf("follower credit should be 0.3× independent, got ratio=%v", ratio)
	}
}

func TestProcessGroundTruthAnchorIDRecorded(t *testing.T) {
	s := newTestStore(t)
	parts := []EpisodeParticipation{
		{DID: "d", Domain: "crypto", Role: RoleIndependent, Confidence: 0.9},
	}
	s.ProcessGroundTruth("episode_xyz", true, parts, time.Now())
	evs, _ := s.Events("d")
	if len(evs) == 0 || evs[0].AnchorID != "episode_xyz" {
		t.Errorf("AnchorID should be episodeID, got %+v", evs)
	}
}

// === Decay tests ===

func TestBatchDecayHalfLife(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()
	pastEvent := Event{
		DID: "d", Domain: "crypto", Tier: TierOperational,
		Trigger: TriggerCanaryCompleted, Delta: 0.8,
		OccurredAt: now.Add(-HalfLifeOperational), // exactly one half-life ago
	}
	if err := s.Apply(pastEvent); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, err := s.BatchDecay(now); err != nil {
		t.Fatalf("decay: %v", err)
	}
	d := s.Get("d").Operational.Domains["crypto"]
	if math.Abs(d.Score-0.4) > 0.01 {
		t.Errorf("after one half-life, score 0.8 → ~0.4; got %v", d.Score)
	}
}

func TestBatchDecayConfidenceSlower(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()
	// Build up some confidence first
	for i := 0; i < 20; i++ {
		s.Apply(Event{
			DID: "d", Domain: "crypto", Tier: TierOperational,
			Trigger: TriggerCanaryCompleted, Delta: 0.01,
			OccurredAt: now.Add(time.Duration(i) * time.Millisecond),
		})
	}
	preDecay := s.Get("d").Operational.Domains["crypto"]
	preScore := preDecay.Score
	preConfidence := preDecay.Confidence

	// Now apply decay one half-life later.
	if _, err := s.BatchDecay(now.Add(HalfLifeOperational)); err != nil {
		t.Fatalf("decay: %v", err)
	}
	d := s.Get("d").Operational.Domains["crypto"]

	// Score should be ~halved; confidence should be * sqrt(0.5) ≈ 0.707
	if math.Abs(d.Score-preScore*0.5) > 0.01 {
		t.Errorf("score decay wrong: pre=%v post=%v", preScore, d.Score)
	}
	expectedConfidence := preConfidence * math.Sqrt(0.5)
	if math.Abs(d.Confidence-expectedConfidence) > 0.01 {
		t.Errorf("confidence decay wrong: pre=%v post=%v want=%v",
			preConfidence, d.Confidence, expectedConfidence)
	}
}

func TestBatchDecayIdempotent(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()
	s.Apply(Event{
		DID: "d", Domain: "crypto", Tier: TierOperational,
		Trigger: TriggerCanaryCompleted, Delta: 0.6,
		OccurredAt: now.Add(-30 * 24 * time.Hour),
	})

	at := now
	if _, err := s.BatchDecay(at); err != nil {
		t.Fatalf("decay 1: %v", err)
	}
	first := s.Get("d").Operational.Domains["crypto"]

	if _, err := s.BatchDecay(at); err != nil {
		t.Fatalf("decay 2: %v", err)
	}
	second := s.Get("d").Operational.Domains["crypto"]

	if first.Score != second.Score || first.Confidence != second.Confidence {
		t.Errorf("BatchDecay not idempotent: first=%+v second=%+v", first, second)
	}
}

func TestBatchDecayMonotoneNonIncreasing_PBT(t *testing.T) {
	// One shared store; each iteration uses a fresh DID. Avoids opening 200
	// BadgerDB instances which exhausts disk on Windows.
	s := newTestStore(t)
	rng := rand.New(rand.NewSource(1234))
	now := time.Now()
	for iter := 0; iter < 200; iter++ {
		did := "did:cav:pbt_" + iterStr(iter)
		s.Apply(Event{
			DID: did, Domain: "crypto", Tier: TierOperational,
			Trigger:    TriggerCanaryCompleted,
			Delta:      rng.Float64()*0.9 + 0.05,
			OccurredAt: now.Add(-time.Duration(rng.Intn(10000)) * time.Hour),
		})
		preScore := s.Get(did).Operational.Domains["crypto"].Score

		if _, err := s.BatchDecay(now); err != nil {
			t.Fatalf("decay: %v", err)
		}
		postScore := s.Get(did).Operational.Domains["crypto"].Score

		if postScore > preScore+1e-9 {
			t.Fatalf("iter %d: decay must never increase score (pre=%v post=%v)",
				iter, preScore, postScore)
		}
	}
}

func iterStr(i int) string {
	const digits = "0123456789abcdefghij"
	return string(digits[i%len(digits)]) + string(digits[(i/len(digits))%len(digits)]) + string(digits[(i/(len(digits)*len(digits)))%len(digits)])
}

// TestBootstrapTriggerDoesNotCountAsSample verifies that `bootstrap` events
// seed initial values without inflating SampleSize (they are migration, not
// genuine evidence).
func TestBootstrapTriggerDoesNotCountAsSample(t *testing.T) {
	s := newTestStore(t)
	s.Apply(Event{
		DID: "d", Domain: "crypto", Tier: TierOperational,
		Trigger: TriggerBootstrap, Delta: 0.5,
	})
	d := s.Get("d").Operational.Domains["crypto"]
	if d.SampleSize != 0 {
		t.Errorf("bootstrap must not count as a sample, got SampleSize=%d", d.SampleSize)
	}
}

func TestApplyAcrossTiersIndependent(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()
	s.Apply(Event{
		DID: "d", Domain: "crypto", Tier: TierOperational,
		Trigger: TriggerCanaryCompleted, Delta: 0.5, OccurredAt: now,
	})
	s.Apply(Event{
		DID: "d", Domain: "crypto", Tier: TierDeliberation,
		Trigger: TriggerChallengeSurvived, Delta: 0.3, OccurredAt: now,
	})

	v := s.Get("d")
	if v.Operational.Domains["crypto"].Score != 0.5 {
		t.Errorf("operational tier corrupted: %v", v.Operational.Domains["crypto"])
	}
	if v.Deliberation.Domains["crypto"].Score != 0.3 {
		t.Errorf("deliberation tier corrupted: %v", v.Deliberation.Domains["crypto"])
	}
}
