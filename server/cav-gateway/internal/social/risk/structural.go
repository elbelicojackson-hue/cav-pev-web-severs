// Structural risk dimensions (R2-16, R2-17).
//
// These are computed in real time relative to the requester's current trust
// graph, which is why they can't be cached against the subject alone.

package risk

import "math"

// DomainActivityVector is a probability distribution over domain activity.
// Keys are domain names (e.g. "crypto", "ml"); values sum to 1 over the
// agent's recent praxon publication record.
type DomainActivityVector map[string]float64

// === R2-16: diversity_impact ===
//
// "If I add this trust edge, does my graph become *less* domain-diverse?"
//
// We compute Shannon entropy over the requester's existing per-domain trust
// distribution, then over the union with the subject's domain activity. If
// entropy drops, score = magnitude of the drop, normalized by the original.
// If entropy rises (or stays flat), score = 0.
func DiversityImpact(requesterDomains, subjectDomains DomainActivityVector) *Dimension {
	// This is a real-time computation; "sufficient" means we have at least
	// one observed domain on each side.
	if len(requesterDomains) == 0 {
		// First trust ever — diversity impact is zero by convention (no
		// existing graph to disturb).
		return &Dimension{Score: 0, SampleSize: 0, Sufficient: true, Confidence: 1}
	}
	if len(subjectDomains) == 0 {
		// Subject has no observable domain activity — we can't say.
		return &Dimension{Score: 0.5, SampleSize: 0, Sufficient: false, Confidence: 0}
	}

	hOld := shannonOverDistribution(requesterDomains)

	// Merge: average of the two distributions, weighted by their sums.
	merged := make(DomainActivityVector)
	for d, p := range requesterDomains {
		merged[d] = p
	}
	for d, p := range subjectDomains {
		merged[d] += p
	}
	// Normalize back to probability distribution
	var total float64
	for _, v := range merged {
		total += v
	}
	for d, v := range merged {
		merged[d] = v / total
	}
	hNew := shannonOverDistribution(merged)

	if hNew >= hOld {
		return &Dimension{Score: 0, SampleSize: 1, Sufficient: true, Confidence: 1}
	}

	drop := hOld - hNew
	score := drop / hOld // hOld > 0 here because len > 0 and probabilities sum to 1
	return &Dimension{
		Score:      clamp01(score),
		SampleSize: 1, Sufficient: true, Confidence: 1,
	}
}

// === R2-17: echo_chamber_delta ===
//
// "Will adding this edge amplify echo-chamber pressure on me?"
//
// Two signals are combined and we take the max:
//   1. The subject's own conformity_index — adding a follower amplifies your
//      own conformity drift.
//   2. The maximum behavioral correlation between the subject and any agent
//      already trusted by the requester. If your existing graph already
//      contains someone who acts like the subject, this edge is redundant in
//      the worst possible way.
//
// `existingCorrelations` is an array of behavioral correlation coefficients
// (precomputed by the engine) between the subject and every agent the
// requester already trusts.
func EchoChamberDelta(subjectConformity float64, existingCorrelations []float64) *Dimension {
	if math.IsNaN(subjectConformity) {
		subjectConformity = 0.5
	}
	max := subjectConformity
	for _, c := range existingCorrelations {
		if c > max {
			max = c
		}
	}
	// Sufficient as long as we had a conformity reading at all.
	return &Dimension{
		Score:      clamp01(max),
		SampleSize: 1 + len(existingCorrelations),
		Sufficient: true, Confidence: 1,
	}
}

// shannonOverDistribution computes Shannon entropy in nats over a probability
// distribution stored as a domain→probability map. Assumes p sums to ~1.
func shannonOverDistribution(d DomainActivityVector) float64 {
	if len(d) == 0 {
		return 0
	}
	var h float64
	for _, p := range d {
		if p > 0 {
			h -= p * math.Log(p)
		}
	}
	return h
}
