// Trust edge weight decay.
//
// Cognitive and social trust both fade unless reinforced by ongoing
// interaction. We don't have a "reinforcement" event yet (that's a later
// spec), so this implementation is the half-life-only version: every edge
// loses weight on a schedule and never gains it back automatically. Edges
// whose weight falls below a small floor are NOT auto-revoked — that's a
// product decision and we keep them around for audit, just at near-zero
// influence.
//
// Like reputation.BatchDecay, this is idempotent: BatchDecay(t) twice in a
// row equals once, because we re-anchor LastDecayAt = t on every cell that
// was processed.

package trust

import (
	"math"
	"time"
)

// Default half-lives for trust weight decay. Cognitive trust is more durable
// than social trust because it's anchored on demonstrated competence; social
// trust is more about ongoing coordination and fades faster without contact.
var (
	HalfLifeCognitive = 180 * 24 * time.Hour // ~6 months
	HalfLifeSocial    = 60 * 24 * time.Hour  // ~2 months
)

// BatchDecay applies half-life decay to all non-revoked edges and persists
// the new weights. Revoked edges are skipped (they're frozen audit records).
//
// Returns the number of edges that were modified.
func (s *Store) BatchDecay(now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	modified := 0
	for _, e := range s.cache {
		if e.IsRevoked() {
			continue
		}
		if e.LastDecayAt.IsZero() || !now.After(e.LastDecayAt) {
			continue
		}
		halfLife := halfLifeFor(e.Kind)
		factor := math.Pow(0.5, float64(now.Sub(e.LastDecayAt))/float64(halfLife))
		newWeight := e.Weight * factor
		if newWeight < 0 {
			newWeight = 0
		}
		e.Weight = newWeight
		e.LastDecayAt = now
		if err := s.persistLocked(e); err != nil {
			return modified, err
		}
		modified++
	}
	return modified, nil
}

func halfLifeFor(kind TrustKind) time.Duration {
	switch kind {
	case Social:
		return HalfLifeSocial
	default:
		return HalfLifeCognitive
	}
}
