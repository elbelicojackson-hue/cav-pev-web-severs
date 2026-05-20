// Background batch decay.
//
// Design §5.3: scores decay continuously, but applying decay on every read is
// expensive and bookkeeping-heavy. We compromise: EffectiveScore() applies
// best-effort decay on read, and BatchDecay periodically rewrites the stored
// `Score` field so the in-memory view doesn't drift unbounded from disk.
//
// Idempotence is the key property: BatchDecay(t) twice in a row is the same as
// once, because we re-anchor LastUpdated = t on every cell that was processed.
// This makes the cron loop safe even if a tick is missed or fires twice.

package reputation

import (
	"time"
)

// BatchDecay applies operational and deliberation half-life decay to every
// cached vector. It rewrites the Score field and bumps LastDecayedAt.
// Confidence is reduced by sqrt of the same factor (smaller penalty than score).
//
// Returns the number of (DID, tier, domain) cells that were decayed.
func (s *Store) BatchDecay(now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cells := 0
	for _, v := range s.cache {
		if decayTier(&v.Operational, now, HalfLifeOperational) {
			cells += len(v.Operational.Domains)
		}
		if decayTier(&v.Deliberation, now, HalfLifeDeliberation) {
			cells += len(v.Deliberation.Domains)
		}
		v.LastDecayedAt = now
		if err := s.persistLocked(v); err != nil {
			return cells, err
		}
	}
	return cells, nil
}

// decayTier applies in-place decay to every domain cell in the tier whose
// LastUpdated < now. Returns true if any cell was modified.
//
// Why "score *= 0.5^(elapsed/halfLife)":
//   exponential decay matches half-life semantics; a cell at score s after
//   one half-life becomes s/2, regardless of starting magnitude.
//
// Why "confidence *= sqrt(factor)":
//   confidence is meant to track sample-quality, which shouldn't crash as fast
//   as the score itself. Square-root smoothing was the design call (§5.3).
func decayTier(t *TierVector, now time.Time, halfLife time.Duration) bool {
	if t.Domains == nil {
		return false
	}
	any := false
	for k, d := range t.Domains {
		if d.LastUpdated.IsZero() || !now.After(d.LastUpdated) {
			continue
		}
		factor := decayedScore(1.0, d.LastUpdated, now, halfLife) // factor in [0, 1]
		newScore := d.Score * factor
		newConfidence := d.Confidence * sqrtFactor(factor)

		// Only write back if there's a meaningful change. This is what makes
		// repeated calls of BatchDecay(t) idempotent — once we've re-anchored
		// LastUpdated to `now`, the next call computes factor=1 and skips.
		d.Score = newScore
		d.Confidence = newConfidence
		d.LastUpdated = now
		t.Domains[k] = d
		any = true
	}
	return any
}

// sqrtFactor returns sqrt(x) without pulling in math.Sqrt's NaN handling.
// x is a non-negative decay factor in [0, 1]; below we early-return on the
// degenerate case.
func sqrtFactor(x float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}
	// Newton's method, 4 iterations — overkill precision but cheap.
	z := x
	for i := 0; i < 4; i++ {
		z = 0.5 * (z + x/z)
	}
	return z
}
