// Epistemic risk dimensions (R2-9 through R2-12).
//
// All functions are pure: they take immutable inputs and return a *Dimension.
// Returned Sufficient flag is the gate the aggregator uses; the underlying
// score is reported regardless so observability is good even when we can't
// rely on it.

package risk

import (
	"math"
	"time"
)

// === Input shapes ===
// These are deliberately minimal so the calculators don't depend on the full
// praxon types. engine.go (T6) builds these from upstream stores.

// PraxonRecord summarizes one published praxon's relevant features for risk.
type PraxonRecord struct {
	ID        string
	IssuedAt  time.Time
	Domain    string
	// Methodology tag distribution as published in the praxon's methodology field.
	PriorSourceTag    string // e.g. "observation" | "tool" | "peer" | "literature"
	InferenceMethodTag string // e.g. "deductive" | "abductive" | "statistical"

	// Verification outcome, if known. nil means "not yet verified".
	GroundTruthMatched *bool
	GroundingTags      []string

	HasMethodology   bool
	HasGrounding     bool
	HasFalsifiability bool
}

// ChallengeRecord summarizes one challenge against this agent's claim.
type ChallengeRecord struct {
	PraxonID  string
	ChallengedAt time.Time
	Survived  bool // true = claim stood after challenge round
}

// RetractionRecord summarizes one retraction event for this agent.
type RetractionRecord struct {
	PraxonID         string
	GroundTruthAt    time.Time // when reverse-evidence appeared
	RetractedAt      time.Time // when agent retracted (or epoch if never)
	Retracted        bool
}

// === R2-9: ground_truth_alignment ===
//
// Score = 1 - hits / verified  (i.e. 0 = perfect alignment, 1 = always wrong).
// Sufficient if verified >= MinPraxonsForGroundTruth.
func GroundTruthAlignment(praxons []PraxonRecord) *Dimension {
	verified := 0
	hits := 0
	for _, p := range praxons {
		if p.GroundTruthMatched != nil {
			verified++
			if *p.GroundTruthMatched {
				hits++
			}
		}
	}
	if verified == 0 {
		return &Dimension{Score: 0.5, SampleSize: 0, Sufficient: false, Confidence: 0}
	}
	score := 1.0 - float64(hits)/float64(verified)
	return &Dimension{
		Score:      clamp01(score),
		SampleSize: verified,
		Sufficient: verified >= MinPraxonsForGroundTruth,
		Confidence: confidenceFromCount(verified, MinPraxonsForGroundTruth),
	}
}

// === R2-10: methodology_stability ===
//
// We score the *consistency* of methodology declarations: a healthy agent uses
// a recognizable methodology mix (entropy in a target band, e.g. 0.3..0.85
// of max entropy for the observed tag set). Too low → mechanical / overfit,
// too high → inconsistent / undisciplined. Score is the absolute deviation
// from the band centre, normalized to [0, 1].
//
// Tagged-as-empty praxons (no methodology field) count as "incomplete" and
// add a flat penalty to the score (clamped at 1).
func MethodologyStability(praxons []PraxonRecord) *Dimension {
	n := len(praxons)
	if n == 0 {
		return &Dimension{Score: 0.5, SampleSize: 0, Sufficient: false, Confidence: 0}
	}

	// Joint distribution over (prior_source_tag, inference_method_tag)
	dist := map[string]int{}
	missing := 0
	total := 0
	for _, p := range praxons {
		if !p.HasMethodology {
			missing++
			continue
		}
		key := p.PriorSourceTag + "|" + p.InferenceMethodTag
		dist[key]++
		total++
	}
	if total == 0 {
		// All praxons missing methodology — maximum risk.
		return &Dimension{
			Score: 1.0, SampleSize: n,
			Sufficient: n >= MinPraxonsForMethodology,
			Confidence: confidenceFromCount(n, MinPraxonsForMethodology),
		}
	}

	entropy := shannonNats(dist, total)
	maxEntropy := math.Log(float64(len(dist)))
	if maxEntropy == 0 {
		// only one tag combo — perfectly mechanical
		return scoreMethodology(1.0, missing, n)
	}
	normEntropy := entropy / maxEntropy // [0, 1]

	// Target band: 0.3..0.85. Distance from band centre 0.575 → score.
	const bandCentre = 0.575
	const bandHalf = 0.275
	deviation := math.Abs(normEntropy - bandCentre)
	deviationScore := math.Min(1.0, math.Max(0, deviation)/bandHalf)

	return scoreMethodology(deviationScore, missing, n)
}

func scoreMethodology(deviationScore float64, missing, n int) *Dimension {
	missingPenalty := float64(missing) / float64(n) * 0.3
	score := clamp01(deviationScore + missingPenalty)
	return &Dimension{
		Score:      score,
		SampleSize: n,
		Sufficient: n >= MinPraxonsForMethodology,
		Confidence: confidenceFromCount(n, MinPraxonsForMethodology),
	}
}

// === R2-11: challenge_survival_rate ===
//
// Score = 1 - survived / total_challenges. Sufficient if total_challenges >= MinChallengesForSurvival.
func ChallengeSurvivalRate(challenges []ChallengeRecord) *Dimension {
	n := len(challenges)
	if n == 0 {
		return &Dimension{Score: 0.5, SampleSize: 0, Sufficient: false, Confidence: 0}
	}
	survived := 0
	for _, c := range challenges {
		if c.Survived {
			survived++
		}
	}
	score := 1.0 - float64(survived)/float64(n)
	return &Dimension{
		Score:      clamp01(score),
		SampleSize: n,
		Sufficient: n >= MinChallengesForSurvival,
		Confidence: confidenceFromCount(n, MinChallengesForSurvival),
	}
}

// === R2-12: retraction_responsiveness ===
//
// Median time-to-retract after ground truth contradicts a claim. Long lag = high
// risk. We use a sigmoid centred at 24h with scale 24h: score ≈ 0.5 at 24h,
// ~0.88 at 48h, ~0.99 at 96h, ~0.12 at 0h.
//
// Agents that never retract (Retracted=false) are treated as if they took
// 30 days — a high but bounded penalty.
func RetractionResponsiveness(events []RetractionRecord) *Dimension {
	n := len(events)
	if n == 0 {
		return &Dimension{Score: 0.5, SampleSize: 0, Sufficient: false, Confidence: 0}
	}
	const targetHours = 24.0
	const scaleHours = 24.0
	lags := make([]float64, 0, n)
	for _, e := range events {
		var lag float64
		if !e.Retracted {
			lag = 30 * 24 // 30 days
		} else if e.RetractedAt.Before(e.GroundTruthAt) {
			lag = 0
		} else {
			lag = e.RetractedAt.Sub(e.GroundTruthAt).Hours()
		}
		lags = append(lags, lag)
	}
	median := medianHours(lags)
	score := sigmoid((median - targetHours) / scaleHours)

	return &Dimension{
		Score:      clamp01(score),
		SampleSize: n,
		Sufficient: n >= MinRetractionsForResponsive,
		Confidence: confidenceFromCount(n, MinRetractionsForResponsive),
	}
}

// === Helpers ===

// shannonNats computes Shannon entropy in nats over a count distribution.
func shannonNats(counts map[string]int, total int) float64 {
	if total == 0 {
		return 0
	}
	var h float64
	for _, c := range counts {
		if c == 0 {
			continue
		}
		p := float64(c) / float64(total)
		h -= p * math.Log(p)
	}
	return h
}

// confidenceFromCount returns a self-confidence in [0, 1] for a dimension
// based on how its sample size compares to the minimum. Asymptote 1, half
// at min/2.
func confidenceFromCount(count, min int) float64 {
	if count <= 0 || min <= 0 {
		return 0
	}
	x := float64(count) / float64(min)
	return clamp01(x / (1.0 + x))
}

// medianHours returns the median of a slice of hour values.
func medianHours(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sorted := append([]float64(nil), xs...)
	insertionSort(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return 0.5 * (sorted[mid-1] + sorted[mid])
	}
	return sorted[mid]
}

func insertionSort(a []float64) {
	for i := 1; i < len(a); i++ {
		v := a[i]
		j := i
		for j > 0 && a[j-1] > v {
			a[j] = a[j-1]
			j--
		}
		a[j] = v
	}
}

func sigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}
