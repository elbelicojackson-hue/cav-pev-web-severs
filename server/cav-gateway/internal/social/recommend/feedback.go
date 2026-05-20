// Feedback bookkeeping + early-warning triggers (R6-5, R6-6, R6-7).
//
// Two responsibilities:
//
//   1. ScheduleObservation — when a recommendation is accepted, snapshot
//      the requester's current behavioral baseline and stash a row keyed
//      by (recommendation_id, observe_at). The cron in T20 runs at the
//      observation time, computes the deltas, and feeds the bandit.
//
//   2. ShouldTriggerExtraRecommendation — the early-warning guard. If a
//      citizen's conformity_index has been climbing for ≥ 3 consecutive
//      digest periods, or their domain entropy has been falling, generate
//      an unscheduled recommendation batch.

package recommend

import (
	"errors"
	"sync"
	"time"
)

// FeedbackRecord ties a recommendation to its observation horizon plus the
// pre-acceptance behavioral baseline. The cron walks the in-memory queue and
// computes the bandit reward.
type FeedbackRecord struct {
	RecommendationID string    `json:"recommendation_id"`
	Strategy         string    `json:"strategy"`
	Requester        string    `json:"requester"`
	Subject          string    `json:"subject"`
	BaselineAt       time.Time `json:"baseline_at"`
	ObserveAt        time.Time `json:"observe_at"`

	BaselineConformity   float64 `json:"baseline_conformity"`
	BaselineDiversity    float64 `json:"baseline_diversity"`
	BaselineChallengeWin float64 `json:"baseline_challenge_win"`
}

// ObservationWindow is the wait between acceptance and reward computation
// (R6-7: 30 days).
var ObservationWindow = 30 * 24 * time.Hour

// FeedbackStore is the in-memory bookkeeping for pending feedback. The
// production wiring layer can serialize to BadgerDB if persistence across
// restarts is required; for the MVP we keep this in-process so the bandit
// learning curve is observable without extra moving parts.
type FeedbackStore struct {
	mu      sync.Mutex
	pending map[string]FeedbackRecord
}

// NewFeedbackStore constructs an empty store.
func NewFeedbackStore() *FeedbackStore {
	return &FeedbackStore{pending: map[string]FeedbackRecord{}}
}

// ScheduleObservation enqueues a record for later reward computation.
// If a record with the same ID already exists, it's overwritten — accepted
// the same recommendation twice is treated as a single observation.
func (s *FeedbackStore) ScheduleObservation(rec FeedbackRecord) error {
	if rec.RecommendationID == "" {
		return errors.New("feedback: recommendation_id required")
	}
	if rec.Strategy == "" {
		return errors.New("feedback: strategy required")
	}
	if rec.ObserveAt.Before(rec.BaselineAt) {
		return errors.New("feedback: observe_at must be after baseline_at")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[rec.RecommendationID] = rec
	return nil
}

// Due returns the records whose observation horizon has elapsed at `now`.
// They are removed from the queue once returned so the cron only processes
// each record once.
func (s *FeedbackStore) Due(now time.Time) []FeedbackRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []FeedbackRecord
	for id, rec := range s.pending {
		if !now.Before(rec.ObserveAt) {
			out = append(out, rec)
			delete(s.pending, id)
		}
	}
	return out
}

// PendingCount is exposed for ops/observability.
func (s *FeedbackStore) PendingCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pending)
}

// === Early warning ===

// ConformityHistory is the recent window of conformity_index values for a
// citizen, oldest-first. Pulled by the warning detector from the digest
// store or behavioral subvector trail.
type ConformityHistory struct {
	DID    string
	Values []float64 // chronologically oldest-first
}

// DiversityHistory is the analogous trail for signal_diversity_entropy.
type DiversityHistory struct {
	DID    string
	Values []float64
}

// ShouldTriggerExtraRecommendation returns true when at least one of the
// two warning conditions fires:
//
//   1. conformity_index has risen monotonically for ≥ minRising periods
//   2. signal_diversity_entropy has fallen monotonically for ≥ minRising periods
//
// `minRising` defaults to 3 per the spec. Pass 0 to use the default.
func ShouldTriggerExtraRecommendation(c ConformityHistory, d DiversityHistory, minRising int) bool {
	if minRising <= 0 {
		minRising = 3
	}
	if isMonotoneRising(c.Values, minRising) {
		return true
	}
	if isMonotoneFalling(d.Values, minRising) {
		return true
	}
	return false
}

// isMonotoneRising reports whether the last `n` values are strictly
// non-decreasing AND show a net positive change.
func isMonotoneRising(xs []float64, n int) bool {
	if len(xs) < n {
		return false
	}
	tail := xs[len(xs)-n:]
	for i := 1; i < len(tail); i++ {
		if tail[i] < tail[i-1] {
			return false
		}
	}
	return tail[len(tail)-1] > tail[0]
}

func isMonotoneFalling(xs []float64, n int) bool {
	if len(xs) < n {
		return false
	}
	tail := xs[len(xs)-n:]
	for i := 1; i < len(tail); i++ {
		if tail[i] > tail[i-1] {
			return false
		}
	}
	return tail[len(tail)-1] < tail[0]
}
