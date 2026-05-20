// Multi-armed bandit for recommendation strategy selection (R6-7, R6-8).
//
// We use ε-greedy with a small discrete strategy space:
//   weight_methodology    ∈ {0.7, 1.0, 1.3}
//   weight_domain_overlap ∈ {0.7, 1.0, 1.3}
//   exploration_bias      ∈ {0.10, 0.20, 0.40}
//
// 27 arms total. Each `Pick` returns the current best arm with probability
// (1 − ε) and a uniform random arm with probability ε. Arms accumulate
// (count, mean_reward); rewards are reported asynchronously via Record once
// the recommendation outcome is observed (T18 schedule_observation hook).
//
// Reward signal (R6-7): for an accepted recommendation, observe the
// requester's behavioral metrics 30 days later and combine:
//   Δconformity_index    (lower is better → flip sign)
//   Δsignal_diversity    (higher is better)
//   Δchallenge_success   (higher is better)
// reward = -dConformity + dDiversity + dChallenge (clamped)
//
// The reward computation lives at the call site (handler / cron); this file
// only manages the bandit state.

package recommend

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"sort"
	"sync"
)

// Bandit is the in-memory ε-greedy strategy selector.
//
// Persistence: callers serialize Snapshot / Restore at startup/shutdown.
// We don't bind to BadgerDB here so the bandit stays test-friendly.
type Bandit struct {
	mu      sync.Mutex
	epsilon float64
	arms    map[string]*armStats
	keys    []string // stable iteration order for tests + persistence
}

type armStats struct {
	Strategy Strategy `json:"strategy"`
	Count    int      `json:"count"`
	Mean     float64  `json:"mean_reward"`
}

// NewBandit constructs the bandit with the spec's discrete grid and ε=0.20
// (matching the 80/20 exploitation/exploration split).
func NewBandit() *Bandit {
	b := &Bandit{epsilon: 0.20, arms: map[string]*armStats{}}
	wmGrid := []float64{0.7, 1.0, 1.3}
	wdGrid := []float64{0.7, 1.0, 1.3}
	expGrid := []float64{0.10, 0.20, 0.40}
	for _, wm := range wmGrid {
		for _, wd := range wdGrid {
			for _, ex := range expGrid {
				key := fmt.Sprintf("wm=%.1f|wd=%.1f|ex=%.2f", wm, wd, ex)
				b.arms[key] = &armStats{
					Strategy: Strategy{
						Key:                 key,
						WeightMethodology:   wm,
						WeightDomainOverlap: wd,
						ExplorationBias:     ex,
					},
				}
				b.keys = append(b.keys, key)
			}
		}
	}
	sort.Strings(b.keys)
	return b
}

// Pick returns the strategy for `requester`. Per-requester randomization
// makes recommendations reproducible within a session (we don't seed by
// requester here — the same agent calling twice gets independent ε rolls,
// which is fine because Generate() is itself called periodically not on
// every request). The `requester` argument is kept on the interface for
// future per-citizen state.
func (b *Bandit) Pick(requester string) Strategy {
	b.mu.Lock()
	defer b.mu.Unlock()

	if randFloat() < b.epsilon {
		// Explore: uniform random arm
		idx := int(randUint64() % uint64(len(b.keys)))
		return b.arms[b.keys[idx]].Strategy
	}
	// Exploit: best mean reward; ties broken by count desc (more confidence).
	best := b.keys[0]
	bestMean := b.arms[best].Mean
	bestCount := b.arms[best].Count
	for _, k := range b.keys[1:] {
		s := b.arms[k]
		if s.Mean > bestMean || (s.Mean == bestMean && s.Count > bestCount) {
			best = k
			bestMean = s.Mean
			bestCount = s.Count
		}
	}
	return b.arms[best].Strategy
}

// Record updates the running mean for `strategy` with a new outcome.
// `outcome` is expected in roughly [-1, 1]; values outside that range are
// clamped before being incorporated.
func (b *Bandit) Record(strategy string, outcome float64) {
	if outcome < -1 {
		outcome = -1
	}
	if outcome > 1 {
		outcome = 1
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	s, ok := b.arms[strategy]
	if !ok {
		return // unknown arm — silently ignore (caller may have stale state)
	}
	s.Count++
	// running mean: m_n = m_{n-1} + (x_n - m_{n-1}) / n
	s.Mean += (outcome - s.Mean) / float64(s.Count)
}

// Snapshot returns a deep copy of the bandit state for persistence.
func (b *Bandit) Snapshot() map[string]armStats {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make(map[string]armStats, len(b.arms))
	for k, v := range b.arms {
		out[k] = *v
	}
	return out
}

// Restore overwrites the bandit state from a previous Snapshot. Unknown
// arm keys are ignored so the grid can evolve without breaking restart.
func (b *Bandit) Restore(snap map[string]armStats) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for k, v := range snap {
		if _, ok := b.arms[k]; ok {
			cp := v
			b.arms[k] = &cp
		}
	}
}

// === Reward computation helper ===

// RewardFromDeltas combines the three behavioral deltas into a single
// reward signal in [-1, 1]. Conformity drop is good (negate), diversity
// gain is good, challenge-success gain is good. Each component is clamped
// before averaging so a single outlier can't dominate.
func RewardFromDeltas(dConformity, dDiversity, dChallengeSuccess float64) float64 {
	x := -clampRange(dConformity, -1, 1) +
		clampRange(dDiversity, -1, 1) +
		clampRange(dChallengeSuccess, -1, 1)
	return clampRange(x/3.0, -1, 1)
}

func clampRange(x, lo, hi float64) float64 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

// === RNG helpers ===

func randUint64() uint64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fallback: deterministic but vanishingly unlikely path; favors
		// arm[0] which is fine because only used when crypto rand fails.
		return 0
	}
	return binary.BigEndian.Uint64(b[:])
}

func randFloat() float64 {
	return float64(randUint64()) / float64(^uint64(0))
}
