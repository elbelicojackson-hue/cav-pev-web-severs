// Compute a BehavioralDigest from a citizen's signal/vote history.
//
// The compute path is deliberately small: it takes already-projected counts
// rather than raw signals so the same logic can run at the agent (with its
// full local context) or at the gateway (during bootstrap when an agent
// hasn't started signing yet).

package digest

import (
	"math"
	"time"
)

// ComputeInput is the projection of one citizen's activity over a single
// digest period. Provided by the caller; this package does no IO.
type ComputeInput struct {
	DID         string
	PeriodStart time.Time
	PeriodEnd   time.Time

	SignalCount int
	VoteCount   int

	// VotesAlignedWithMajority: how many of `VoteCount` agreed with the
	// dominant high-rep position at vote time. -1 means "majority signal
	// unavailable for some votes" — the alignment factor is computed over
	// votes where it WAS available; this field is the tally that counts.
	VotesAlignedWithMajority   int
	VotesWithMajorityAvailable int

	// DomainCounts: how many signals per domain, used for entropy &
	// uniqueness counts.
	DomainCounts map[string]int
}

// Compute builds a (yet-unsigned) BehavioralDigest from the input projection.
// Returns an error only if PeriodEnd <= PeriodStart.
func Compute(in ComputeInput) (*BehavioralDigest, error) {
	if !in.PeriodEnd.After(in.PeriodStart) {
		return nil, ErrInvalidPeriod
	}
	d := &BehavioralDigest{
		DID:           in.DID,
		PeriodStart:   in.PeriodStart,
		PeriodEnd:     in.PeriodEnd,
		SchemaVersion: SchemaVersion,
		SignalCount:   in.SignalCount,
		VoteCount:     in.VoteCount,
	}

	// Vote alignment: fraction in [0, 1]. If no comparable votes, report 0.5
	// — neither aligned nor counter-aligned, which is the calibration signal
	// the anti-conformity engine prefers.
	if in.VotesWithMajorityAvailable > 0 {
		d.VoteAlignmentWithMajority = clamp01(
			float64(in.VotesAlignedWithMajority) / float64(in.VotesWithMajorityAvailable),
		)
	} else {
		d.VoteAlignmentWithMajority = 0.5
	}

	// Unique active domains.
	d.UniqueDomainsActive = len(in.DomainCounts)

	// Signal diversity entropy: Shannon entropy in nats over domain shares,
	// normalized by log(N) so the result lives in [0, 1].
	d.SignalDiversityEntropy = normalizedEntropy(in.DomainCounts)

	return d, nil
}

func normalizedEntropy(counts map[string]int) float64 {
	total := 0
	for _, c := range counts {
		if c > 0 {
			total += c
		}
	}
	if total == 0 || len(counts) <= 1 {
		return 0
	}
	var h float64
	for _, c := range counts {
		if c <= 0 {
			continue
		}
		p := float64(c) / float64(total)
		h -= p * math.Log(p)
	}
	max := math.Log(float64(len(counts)))
	if max == 0 {
		return 0
	}
	return clamp01(h / max)
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
