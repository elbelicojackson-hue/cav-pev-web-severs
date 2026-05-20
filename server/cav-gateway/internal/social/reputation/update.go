// Reputation update logic.
//
// All mutations to a Vector flow through Apply(Event). This is invariant I3
// from cav-social-trust/design.md §11: "Reputation mutates only via
// reputation.Event records." The store's putVectorLocked is package-private
// and is callable only from Apply (or from migration helpers like Bootstrap,
// which logs an Event of trigger=bootstrap before writing).

package reputation

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// Trigger values. Stored on Event for audit + retrospective replay.
const (
	TriggerGroundTruth        = "ground_truth_verified"
	TriggerChallengeSurvived  = "challenge_survived"
	TriggerChallengeFailed    = "challenge_failed"
	TriggerCanaryCompleted    = "canary_completed"
	TriggerRetroactive        = "retroactive"
	TriggerBootstrap          = "bootstrap"
)

// EpisodeRole describes how an agent participated in a consensus episode that
// later receives ground truth. Independent reasoners (own observation or tool
// result) get full credit/blame; followers (agreement only) get attenuated
// credit and amplified blame — invariant of design §5.3.
type EpisodeRole string

const (
	RoleIndependent EpisodeRole = "independent" // grounding via observation/tool_result
	RoleFollower    EpisodeRole = "follower"    // endorsement of someone else's claim
)

// EpisodeParticipation is one agent's role + confidence inside an episode.
// Used by ProcessGroundTruth.
type EpisodeParticipation struct {
	DID        string
	Domain     string
	Role       EpisodeRole
	Confidence float64
}

// Apply mutates the citizen's vector by the delta on `ev` and records the
// event in the append-only log. This is the ONLY public write path.
//
// Behavior:
//   - The event is timestamped with OccurredAt; if zero, set to now.
//   - The vector is created lazily if no record exists yet (e.g. canary seed).
//   - Score is clamped to [0, 1].
//   - Confidence grows toward 1 with each event (1 - 0.5^(SampleSize/10) law).
//   - SampleSize increments by 1 unless ev.Trigger is bootstrap (which seeds).
//   - LastUpdated and Vector.LastUpdatedAt are bumped.
func (s *Store) Apply(ev Event) error {
	if err := validateEvent(ev); err != nil {
		return err
	}
	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = time.Now()
	}
	if ev.ID == "" {
		ev.ID = newEventID(ev.OccurredAt)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	v, ok := s.cache[ev.DID]
	if !ok {
		v = NewVector(ev.DID, ev.OccurredAt)
		s.cache[ev.DID] = v
	}

	tier := pickTier(v, ev.Tier)
	current, exists := tier.Domains[ev.Domain]
	if !exists {
		current = DomainScore{LastUpdated: ev.OccurredAt}
	}

	// Apply decay from current.LastUpdated → ev.OccurredAt before adding the
	// delta. This gives an "effective accumulation" path so events that are
	// far apart in time don't compound at full strength.
	hl := halfLifeFor(ev.Tier)
	current.Score = decayedScore(current.Score, current.LastUpdated, ev.OccurredAt, hl)

	current.Score += ev.Delta
	if current.Score < 0 {
		current.Score = 0
	}
	if current.Score > 1 {
		current.Score = 1
	}

	if ev.Trigger != TriggerBootstrap {
		current.SampleSize++
	}
	// Confidence asymptote: 1 - 0.5^(n/10). After ~30 events, confidence ≈ 0.875.
	current.Confidence = 1 - math.Pow(0.5, float64(current.SampleSize)/10.0)
	if current.Confidence < 0 {
		current.Confidence = 0
	}
	if current.Confidence > 1 {
		current.Confidence = 1
	}
	current.LastUpdated = ev.OccurredAt

	tier.Domains[ev.Domain] = current
	v.LastUpdatedAt = ev.OccurredAt
	v.SchemaVersion = SchemaVersion

	if err := s.recordEventLocked(ev); err != nil {
		return fmt.Errorf("record event: %w", err)
	}
	return s.persistLocked(v)
}

// ApplyBatch applies a sequence of events. Best-effort: stops on first error
// and returns it together with the count of events successfully applied.
func (s *Store) ApplyBatch(events []Event) (int, error) {
	for i, ev := range events {
		if err := s.Apply(ev); err != nil {
			return i, err
		}
	}
	return len(events), nil
}

// ProcessGroundTruth applies retrospective updates to all participants of a
// consensus episode once ground truth is published.
//
// Math (design §5.3):
//   correct  & independent → +0.05 × confidence
//   correct  & follower    → +0.05 × confidence × 0.3
//   wrong    & independent → -0.10 × confidence
//   wrong    & follower    → -0.10 × confidence × 1.5  (follower-penalty amplified)
//
// `episodeID` is recorded as AnchorID on each generated Event; `now` is used
// as OccurredAt so the retrospective is anchored on truth-publication time,
// not original participation time.
func (s *Store) ProcessGroundTruth(episodeID string, correct bool, participants []EpisodeParticipation, now time.Time) error {
	for _, p := range participants {
		var base float64
		if correct {
			base = +0.05 * p.Confidence
			if p.Role == RoleFollower {
				base *= 0.3
			}
		} else {
			base = -0.10 * p.Confidence
			if p.Role == RoleFollower {
				base *= 1.5
			}
		}
		ev := Event{
			DID:        p.DID,
			Domain:     p.Domain,
			Tier:       TierOperational,
			Trigger:    TriggerRetroactive,
			Delta:      base,
			Reason:     fmt.Sprintf("ground_truth=%v role=%s", correct, p.Role),
			AnchorID:   episodeID,
			OccurredAt: now,
		}
		if err := s.Apply(ev); err != nil {
			return err
		}
	}
	return nil
}

// pickTier returns a pointer to the tier on v matching `tier`. Defaults to
// operational on unknown values rather than failing — over-cautious here is
// safer than refusing a legitimate event.
func pickTier(v *Vector, tier string) *TierVector {
	switch tier {
	case TierDeliberation:
		return &v.Deliberation
	default:
		return &v.Operational
	}
}

func halfLifeFor(tier string) time.Duration {
	switch tier {
	case TierDeliberation:
		return HalfLifeDeliberation
	default:
		return HalfLifeOperational
	}
}

// validateEvent checks an Event before mutation. Anything that would corrupt
// the audit log gets rejected early.
func validateEvent(ev Event) error {
	if ev.DID == "" {
		return errors.New("reputation: event missing DID")
	}
	if ev.Domain == "" {
		return errors.New("reputation: event missing Domain")
	}
	if ev.Tier == "" {
		return errors.New("reputation: event missing Tier")
	}
	if ev.Tier != TierOperational && ev.Tier != TierDeliberation {
		return fmt.Errorf("reputation: invalid Tier %q", ev.Tier)
	}
	if ev.Trigger == "" {
		return errors.New("reputation: event missing Trigger")
	}
	if math.IsNaN(ev.Delta) || math.IsInf(ev.Delta, 0) {
		return errors.New("reputation: event Delta is not finite")
	}
	return nil
}

// newEventID generates a deterministic-ish event ID anchored on the timestamp.
// Not a security primitive — uniqueness within a DID + occurred_at ms is enough
// because the storage key is `rep:event:<did>:<ms>:<id>`.
func newEventID(t time.Time) string {
	return fmt.Sprintf("ev_%d", t.UnixNano())
}
