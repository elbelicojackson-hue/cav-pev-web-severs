// Behavioral risk dimensions (R2-13 through R2-15).

package risk

import (
	"math"
	"time"
)

// === Input shapes ===

// VoteRecord summarizes one of the subject's votes for conformity computation.
//   At                = vote timestamp
//   Position          = "endorse" | "reject" | "abstain" (or whatever upstream uses)
//   MajorityPosition  = the high-rep majority's position at vote time on the same proposal
//
// MajorityPosition = "" means "no clear majority existed" — that vote is excluded
// from the Pearson correlation but still counts toward sample size.
type VoteRecord struct {
	At               time.Time
	Position         string
	MajorityPosition string // "" = no majority signal
}

// FingerprintFeatures is the behavioral-fingerprint feature vector used for
// sybil similarity. The same shape is computed for the subject and for every
// already-known agent in the network. Cosine similarity between vectors
// indicates likely sybil pairing.
type FingerprintFeatures struct {
	// SignalIntervals: average and stddev of seconds-between-signals
	IntervalMean   float64
	IntervalStddev float64

	// Hour-of-day distribution (24 buckets, each in [0, 1] proportion)
	HourBuckets [24]float64

	// Tag usage distribution (sparse)
	TagUsage map[string]float64
}

// ActivitySample is one period's signal count for the subject; used for
// activity-anomaly KL divergence.
type ActivitySample struct {
	WindowStart time.Time
	Count       int
}

// === R2-13: conformity_index ===
//
// Pearson correlation of (subject_position, majority_position) across votes,
// mapped to [0, 1] via (1 + r)/2. r ≈ 1 → perfect follower → score 1 (high
// risk). r ≈ -1 → counter-majority → score 0. r ≈ 0 → independent → 0.5.
//
// Positions are encoded numerically: endorse=+1, reject=-1, abstain=0.
func ConformityIndex(votes []VoteRecord) *Dimension {
	n := len(votes)
	if n == 0 {
		return &Dimension{Score: 0.5, SampleSize: 0, Sufficient: false, Confidence: 0}
	}

	// Filter to votes with a known majority signal
	var subj, maj []float64
	for _, v := range votes {
		if v.MajorityPosition == "" {
			continue
		}
		subj = append(subj, encodePosition(v.Position))
		maj = append(maj, encodePosition(v.MajorityPosition))
	}
	if len(subj) < 2 {
		return &Dimension{
			Score: 0.5, SampleSize: n,
			Sufficient: n >= MinVotesForConformity,
			Confidence: confidenceFromCount(n, MinVotesForConformity),
		}
	}

	r := pearson(subj, maj)
	score := (1.0 + r) / 2.0
	return &Dimension{
		Score:      clamp01(score),
		SampleSize: n,
		Sufficient: n >= MinVotesForConformity,
		Confidence: confidenceFromCount(n, MinVotesForConformity),
	}
}

func encodePosition(p string) float64 {
	switch p {
	case "endorse":
		return 1
	case "reject":
		return -1
	default:
		return 0
	}
}

func pearson(x, y []float64) float64 {
	n := float64(len(x))
	if n == 0 || len(x) != len(y) {
		return 0
	}
	var sumX, sumY, sumXY, sumXX, sumYY float64
	for i := range x {
		sumX += x[i]
		sumY += y[i]
		sumXY += x[i] * y[i]
		sumXX += x[i] * x[i]
		sumYY += y[i] * y[i]
	}
	num := n*sumXY - sumX*sumY
	den := math.Sqrt((n*sumXX - sumX*sumX) * (n*sumYY - sumY*sumY))
	if den == 0 {
		return 0
	}
	return num / den
}

// === R2-14: sybil_similarity_max ===
//
// Score = max cosine similarity between the subject's fingerprint and every
// already-known agent's fingerprint. Higher means "looks too much like X".
//
// Sufficient gate is wall-clock based (the agent must have been in the network
// long enough for their fingerprint to be statistically meaningful), expressed
// as `agentAgeHours >= MinHoursForSybilDetection`.
func SybilSimilarityMax(subject FingerprintFeatures, agentAgeHours float64, others map[string]FingerprintFeatures) *Dimension {
	if len(others) == 0 {
		return &Dimension{
			Score: 0, SampleSize: 0,
			Sufficient: agentAgeHours >= MinHoursForSybilDetection,
			Confidence: confidenceFromHours(agentAgeHours),
		}
	}
	maxSim := 0.0
	for _, other := range others {
		sim := cosine(subject, other)
		if sim > maxSim {
			maxSim = sim
		}
	}
	return &Dimension{
		Score:      clamp01(maxSim),
		SampleSize: len(others),
		Sufficient: agentAgeHours >= MinHoursForSybilDetection,
		Confidence: confidenceFromHours(agentAgeHours),
	}
}

func confidenceFromHours(hours float64) float64 {
	if hours <= 0 {
		return 0
	}
	x := hours / float64(MinHoursForSybilDetection)
	return clamp01(x / (1.0 + x))
}

func cosine(a, b FingerprintFeatures) float64 {
	// Build dense representation: 2 scalars + 24 hour buckets + sparse tag union.
	keys := map[string]struct{}{}
	for k := range a.TagUsage {
		keys[k] = struct{}{}
	}
	for k := range b.TagUsage {
		keys[k] = struct{}{}
	}

	var dot, na2, nb2 float64
	add := func(av, bv float64) {
		dot += av * bv
		na2 += av * av
		nb2 += bv * bv
	}
	add(a.IntervalMean, b.IntervalMean)
	add(a.IntervalStddev, b.IntervalStddev)
	for i := 0; i < 24; i++ {
		add(a.HourBuckets[i], b.HourBuckets[i])
	}
	for k := range keys {
		add(a.TagUsage[k], b.TagUsage[k])
	}
	if na2 == 0 || nb2 == 0 {
		return 0
	}
	return dot / (math.Sqrt(na2) * math.Sqrt(nb2))
}

// === R2-15: activity_anomaly_score ===
//
// KL divergence of the subject's recent-activity distribution from the
// network's baseline distribution. Both are normalized count vectors over the
// last N days. Score = sigmoid(KL - threshold).
//
// `subjectDays` is the number of days of subject history we have; the gate
// requires MinDaysForActivityAnomaly.
func ActivityAnomalyScore(subject, baseline []float64, subjectDays int) *Dimension {
	if len(subject) == 0 || len(baseline) == 0 {
		return &Dimension{Score: 0.5, SampleSize: subjectDays, Sufficient: false, Confidence: 0}
	}
	pa := normalizeProbabilities(subject)
	pb := normalizeProbabilities(baseline)

	// Align lengths by truncating to min length (engine should pre-bin to same shape).
	n := len(pa)
	if len(pb) < n {
		n = len(pb)
	}
	const eps = 1e-9
	var kl float64
	for i := 0; i < n; i++ {
		p := pa[i] + eps
		q := pb[i] + eps
		kl += p * math.Log(p/q)
	}

	const klThreshold = 0.5
	score := sigmoid(kl - klThreshold)
	return &Dimension{
		Score:      clamp01(score),
		SampleSize: subjectDays,
		Sufficient: subjectDays >= MinDaysForActivityAnomaly,
		Confidence: confidenceFromCount(subjectDays, MinDaysForActivityAnomaly),
	}
}

func normalizeProbabilities(xs []float64) []float64 {
	var s float64
	for _, x := range xs {
		if x > 0 {
			s += x
		}
	}
	if s == 0 {
		out := make([]float64, len(xs))
		uniform := 1.0 / float64(len(xs))
		for i := range out {
			out[i] = uniform
		}
		return out
	}
	out := make([]float64, len(xs))
	for i, x := range xs {
		if x < 0 {
			x = 0
		}
		out[i] = x / s
	}
	return out
}
