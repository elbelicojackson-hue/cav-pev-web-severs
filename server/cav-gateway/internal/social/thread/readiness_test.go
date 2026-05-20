package thread

import (
	"math"
	"testing"
	"time"
)

func snap(id, from, position string, conf, rep float64, tags ...string) SignalSnapshot {
	return SignalSnapshot{
		ID: id, From: from, Position: position,
		Confidence: conf, Reputation: rep,
		IssuedAt: time.Now(), Tags: tags,
	}
}

func TestComputeReadinessEmpty(t *testing.T) {
	r := ComputeReadiness(nil, 100, time.Now(), time.Now())
	if r.Total != 0 {
		t.Errorf("empty thread: expected 0 total, got %v", r.Total)
	}
}

func TestConsensusUnanimousScores1(t *testing.T) {
	snaps := []SignalSnapshot{
		snap("1", "a", "endorse", 1.0, 0.9, "x"),
		snap("2", "b", "endorse", 1.0, 0.9, "y"),
		snap("3", "c", "endorse", 1.0, 0.9, "z"),
	}
	consensus, pos, conf := computeConsensus(snaps)
	if consensus < 0.99 {
		t.Errorf("unanimous endorse → consensus ~1, got %v", consensus)
	}
	if pos != "endorse" {
		t.Errorf("expected endorse dominant, got %q", pos)
	}
	if conf < 0.99 {
		t.Errorf("expected confidence ~1, got %v", conf)
	}
}

func TestConsensusEvenSplitNearZero(t *testing.T) {
	snaps := []SignalSnapshot{
		snap("1", "a", "endorse", 1.0, 0.9),
		snap("2", "b", "reject", 1.0, 0.9),
	}
	consensus, _, _ := computeConsensus(snaps)
	// 50/50 endorse/reject → entropy ≈ 1, normalized by log2(3) ≈ 0.63
	// → consensus ≈ 0.37
	if consensus > 0.5 {
		t.Errorf("split vote should produce low consensus, got %v", consensus)
	}
}

func TestConsensusReputationWeighting(t *testing.T) {
	// One high-rep endorse should dominate three low-rep rejects.
	snaps := []SignalSnapshot{
		snap("1", "a", "endorse", 1.0, 0.9),
		snap("2", "b", "reject", 1.0, 0.05),
		snap("3", "c", "reject", 1.0, 0.05),
		snap("4", "d", "reject", 1.0, 0.05),
	}
	_, pos, _ := computeConsensus(snaps)
	if pos != "endorse" {
		t.Errorf("high-rep should dominate, got %q", pos)
	}
}

func TestDiversityAcrossDistinctTags(t *testing.T) {
	snaps := []SignalSnapshot{
		snap("1", "a", "endorse", 1.0, 0.9, "crypto"),
		snap("2", "b", "endorse", 1.0, 0.9, "ml"),
		snap("3", "c", "endorse", 1.0, 0.9, "forensics"),
	}
	d := computeDiversity(snaps)
	if d != 1.0 {
		t.Errorf("disjoint tags → diversity 1, got %v", d)
	}
}

func TestDiversityIdenticalTagsIsZero(t *testing.T) {
	snaps := []SignalSnapshot{
		snap("1", "a", "endorse", 1.0, 0.9, "crypto"),
		snap("2", "b", "endorse", 1.0, 0.9, "crypto"),
	}
	d := computeDiversity(snaps)
	if d != 0 {
		t.Errorf("identical tags → 0, got %v", d)
	}
}

func TestParticipationCapAtTarget(t *testing.T) {
	snaps := []SignalSnapshot{
		snap("1", "a", "endorse", 1.0, 0.5),
		snap("2", "b", "endorse", 1.0, 0.5),
		snap("3", "c", "endorse", 1.0, 0.5),
		snap("4", "d", "endorse", 1.0, 0.5),
		snap("5", "e", "endorse", 1.0, 0.5),
		snap("6", "f", "endorse", 1.0, 0.5),
	}
	// network 100 → target 5. 6 participants → score capped at 1.
	p := computeParticipation(snaps, 100)
	if p != 1.0 {
		t.Errorf("over-target should cap at 1, got %v", p)
	}
}

func TestParticipationSmallNetworkLowerTarget(t *testing.T) {
	snaps := []SignalSnapshot{
		snap("1", "a", "endorse", 1.0, 0.5),
		snap("2", "b", "endorse", 1.0, 0.5),
		snap("3", "c", "endorse", 1.0, 0.5),
	}
	// network ≤20 → target 3. 3 participants → 1.0
	p := computeParticipation(snaps, 10)
	if p != 1.0 {
		t.Errorf("small-network 3-participant target → 1, got %v", p)
	}
	// Same with large network → 3/5 = 0.6
	pBig := computeParticipation(snaps, 100)
	if math.Abs(pBig-0.6) > 1e-9 {
		t.Errorf("large network: 3/5=0.6 expected, got %v", pBig)
	}
}

func TestMaturityFromAge(t *testing.T) {
	now := time.Now()
	// large network, period 6h
	if got := computeMaturity(now.Add(-6*time.Hour), now, 100); got != 1.0 {
		t.Errorf("at one period → 1, got %v", got)
	}
	if got := computeMaturity(now.Add(-3*time.Hour), now, 100); math.Abs(got-0.5) > 1e-9 {
		t.Errorf("half period → 0.5, got %v", got)
	}
	if got := computeMaturity(now, now, 100); got != 0 {
		t.Errorf("zero age → 0, got %v", got)
	}
}

func TestClassifyReadiness(t *testing.T) {
	if got := ClassifyReadiness(0.95); got != LevelCanonical {
		t.Errorf("0.95 → canonical, got %v", got)
	}
	if got := ClassifyReadiness(0.80); got != LevelProvisional {
		t.Errorf("0.80 → provisional, got %v", got)
	}
	if got := ClassifyReadiness(0.60); got != LevelDraft {
		t.Errorf("0.60 → draft, got %v", got)
	}
	if got := ClassifyReadiness(0.40); got != LevelNone {
		t.Errorf("0.40 → none, got %v", got)
	}
}

func TestComputeReadinessFullPath(t *testing.T) {
	now := time.Now()
	snaps := []SignalSnapshot{
		snap("1", "a", "endorse", 0.9, 0.8, "crypto"),
		snap("2", "b", "endorse", 0.9, 0.8, "ml"),
		snap("3", "c", "endorse", 0.9, 0.8, "forensics"),
	}
	r := ComputeReadiness(snaps, 10, now.Add(-2*time.Hour), now)
	// Small network (10) → target 3, period 1h. 3 participants, 2h age → all factors high.
	if r.Total < 0.6 {
		t.Errorf("expected high readiness, got %+v", r)
	}
	// Each factor must be in [0, 1]
	for name, val := range map[string]float64{
		"consensus":     r.ConsensusScore,
		"diversity":     r.DiversityScore,
		"participation": r.ParticipationScore,
		"confidence":    r.ConfidenceScore,
		"maturity":      r.MaturityScore,
		"total":         r.Total,
	} {
		if val < 0 || val > 1 {
			t.Errorf("%s out of [0,1]: %v", name, val)
		}
	}
}

func TestDominantPositionMatchesConsensus(t *testing.T) {
	snaps := []SignalSnapshot{
		snap("1", "a", "endorse", 0.9, 0.5),
		snap("2", "b", "endorse", 0.9, 0.5),
		snap("3", "c", "reject", 0.5, 0.5),
	}
	pos, conf := DominantPosition(snaps)
	if pos != "endorse" {
		t.Errorf("expected endorse, got %q", pos)
	}
	if conf <= 0 || conf > 1 {
		t.Errorf("confidence out of range: %v", conf)
	}
}
