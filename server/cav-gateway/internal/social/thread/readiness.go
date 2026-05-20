// Readiness factor calculations (cav-social-trust §5.2).
//
// The five factors operate on a SignalSnapshot — a minimal projection of the
// signals that belong to one thread. We avoid pulling the full
// signal.EntropicSignal type so this package is testable in isolation and so
// the upstream wire format can evolve without touching the algorithm.

package thread

import (
	"math"
	"time"
)

// SignalSnapshot is the per-signal information needed to compute readiness.
// One snapshot per signal in the thread.
type SignalSnapshot struct {
	ID          string
	From        string  // sender fingerprint
	Position    string  // "endorse" | "reject" | "abstain"
	Confidence  float64 // [0, 1]
	Reputation  float64 // sender's effective reputation in this thread's domain
	IssuedAt    time.Time
	Tags        []string
}

// ComputeReadiness produces a full ReadinessScore from a thread's signals.
//
//   networkSize — count of active citizens, used to pick the participation
//                 target and maturation period (small networks crystallize
//                 with fewer participants and faster, see §R5-10)
//   threadStart — anchors the maturity factor
//   now         — wall-clock at evaluation time
func ComputeReadiness(snaps []SignalSnapshot, networkSize int, threadStart, now time.Time) ReadinessScore {
	consensus, dominantPosition, dominantConfidence := computeConsensus(snaps)
	diversity := computeDiversity(snaps)
	participation := computeParticipation(snaps, networkSize)
	confidence := dominantConfidence
	maturity := computeMaturity(threadStart, now, networkSize)

	total := WeightConsensus*consensus +
		WeightDiversity*diversity +
		WeightParticipation*participation +
		WeightConfidence*confidence +
		WeightMaturity*maturity

	_ = dominantPosition // exposed via DominantPosition() helper for crystallize.go

	return ReadinessScore{
		Total:              clamp01(total),
		ConsensusScore:     consensus,
		DiversityScore:     diversity,
		ParticipationScore: participation,
		ConfidenceScore:    confidence,
		MaturityScore:      maturity,
		ComputedAt:         now,
	}
}

// DominantPosition returns the position with the largest weighted vote share
// across the snapshots, plus its weighted-average confidence. Used by
// crystallize.go to set the Praxon's conclusion direction.
func DominantPosition(snaps []SignalSnapshot) (position string, confidence float64) {
	_, position, confidence = computeConsensus(snaps)
	return
}

// === Factor calculators ===

// computeConsensus returns:
//   consensus_score  = 1 - H / log2(3)   where H is Shannon entropy over the
//                                        weighted (endorse, reject, abstain) shares
//   dominantPosition = the position with the largest weighted share
//   dominantConfidence = weighted-avg confidence among signals that agreed
//                        with dominantPosition
//
// Weights = reputation × confidence so a low-rep agent with weak signal
// barely shifts the entropy, while a high-rep agent with strong conviction
// dominates.
func computeConsensus(snaps []SignalSnapshot) (consensus float64, dominantPosition string, dominantConfidence float64) {
	var endorseW, rejectW, abstainW float64
	var endorseConfSum, rejectConfSum, abstainConfSum float64
	var endorseW2, rejectW2, abstainW2 float64

	for _, s := range snaps {
		w := s.Reputation * s.Confidence
		if w == 0 {
			// Heartbeats / unsigned signals still count for participation but
			// shouldn't shift the entropy. Skip the weight bucket.
			continue
		}
		switch s.Position {
		case "endorse":
			endorseW += w
			endorseConfSum += w * s.Confidence
			endorseW2 += w
		case "reject":
			rejectW += w
			rejectConfSum += w * s.Confidence
			rejectW2 += w
		default:
			abstainW += w
			abstainConfSum += w * s.Confidence
			abstainW2 += w
		}
	}
	totalW := endorseW + rejectW + abstainW
	if totalW == 0 {
		return 0, "", 0
	}
	pE := endorseW / totalW
	pR := rejectW / totalW
	pA := abstainW / totalW
	h := shannon3(pE, pR, pA)
	maxH := math.Log2(3)
	consensus = clamp01(1 - h/maxH)

	// Dominant position
	switch {
	case endorseW >= rejectW && endorseW >= abstainW:
		dominantPosition = "endorse"
		if endorseW2 > 0 {
			dominantConfidence = endorseConfSum / endorseW2
		}
	case rejectW >= abstainW:
		dominantPosition = "reject"
		if rejectW2 > 0 {
			dominantConfidence = rejectConfSum / rejectW2
		}
	default:
		dominantPosition = "abstain"
		if abstainW2 > 0 {
			dominantConfidence = abstainConfSum / abstainW2
		}
	}
	return clamp01(consensus), dominantPosition, clamp01(dominantConfidence)
}

// computeDiversity is the average pairwise Jaccard distance across
// participant tag sets. Mirrors consensus.computeDiversity to keep behavior
// consistent.
func computeDiversity(snaps []SignalSnapshot) float64 {
	if len(snaps) <= 1 {
		return 0
	}
	// Build per-participant tag union.
	byParticipant := map[string]map[string]struct{}{}
	for _, s := range snaps {
		set, ok := byParticipant[s.From]
		if !ok {
			set = map[string]struct{}{}
			byParticipant[s.From] = set
		}
		for _, tag := range s.Tags {
			set[tag] = struct{}{}
		}
	}
	if len(byParticipant) <= 1 {
		return 0
	}
	keys := make([]string, 0, len(byParticipant))
	for k := range byParticipant {
		keys = append(keys, k)
	}
	totalDistance := 0.0
	pairs := 0
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			totalDistance += jaccardDist(byParticipant[keys[i]], byParticipant[keys[j]])
			pairs++
		}
	}
	if pairs == 0 {
		return 0
	}
	return clamp01(totalDistance / float64(pairs))
}

// computeParticipation is min(1, distinct_participants / target).
func computeParticipation(snaps []SignalSnapshot, networkSize int) float64 {
	uniq := map[string]struct{}{}
	for _, s := range snaps {
		if s.From != "" {
			uniq[s.From] = struct{}{}
		}
	}
	target := TargetParticipants(networkSize)
	return clamp01(float64(len(uniq)) / float64(target))
}

// computeMaturity is min(1, age / maturation_period).
func computeMaturity(threadStart, now time.Time, networkSize int) float64 {
	if threadStart.IsZero() || !now.After(threadStart) {
		return 0
	}
	age := now.Sub(threadStart)
	period := MaturationPeriod(networkSize)
	if period == 0 {
		return 1
	}
	return clamp01(float64(age) / float64(period))
}

// === Helpers ===

func shannon3(p1, p2, p3 float64) float64 {
	var h float64
	for _, p := range [3]float64{p1, p2, p3} {
		if p > 0 && p < 1 {
			h -= p * math.Log2(p)
		}
	}
	if h < 0 {
		return 0
	}
	return h
}

func jaccardDist(a, b map[string]struct{}) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	inter := 0
	for k := range a {
		if _, ok := b[k]; ok {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return 1.0 - float64(inter)/float64(union)
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
