// Thread tracker — owns thread state, drives readiness recomputation.
//
// Storage (design.md §3):
//   th:meta:<thread_id>                       → Thread JSON (primary)
//   th:active:<last_activity_ms>:<thread_id>  → []  (active-thread index)
//   th:by_participant:<did>:<thread_id>       → []  (per-citizen view)
//
// Public surface:
//   Tracker.OnSignal(snap, parentID) — append a signal to its thread, recompute
//                                      readiness, persist
//   Tracker.Get(threadID)            — fetch a Thread by ID
//   Tracker.Active(level)            — most-recent threads at or above `level`
//
// Crystallization (state-machine + Praxon emission) lives in crystallize.go;
// the tracker exposes a `Subscriber` callback so that file can hook in.

package thread

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// ErrUnknownThread is returned when an operation references a thread we
// haven't seen yet AND the caller didn't opt in to bootstrapping it.
var ErrUnknownThread = errors.New("thread: unknown thread")

// SignalRef is the lightweight envelope OnSignal accepts. It contains the
// snapshot used for readiness math plus a parent pointer (InReplyTo).
type SignalRef struct {
	Snapshot SignalSnapshot

	// ParentID is the InReplyTo signal ID. Empty parent means "this signal
	// starts a new thread, ID = Snapshot.ID".
	ParentID string

	// NetworkSize is the active-citizen count at signal-arrival time. Used
	// to pick the participation target and maturation period.
	NetworkSize int
}

// LevelChange is emitted by the tracker when a signal causes the thread's
// crystallization level to move. Subscribers use this to drive Praxon
// production / retraction.
type LevelChange struct {
	ThreadID string
	From     CrystallizationLevel
	To       CrystallizationLevel
	Score    ReadinessScore
	Snapshots []SignalSnapshot // current full snapshot list, for Praxon body
	DominantPosition  string
	DominantConfidence float64
}

// Subscriber is invoked by the tracker on every level change. It must be
// safe to call concurrently — the tracker holds no locks while invoking.
type Subscriber func(change LevelChange)

// Tracker is the BadgerDB-backed thread state manager.
type Tracker struct {
	db          *badger.DB
	mu          sync.RWMutex
	cache       map[string]*threadState
	subscribers []Subscriber
}

// threadState holds the in-memory representation of a thread plus the
// snapshot sequence — the snapshots are NOT persisted (they'd be redundant
// with the signal store), but we keep them in memory as long as the thread
// is hot so readiness can be recomputed cheaply.
type threadState struct {
	thread    *Thread
	snapshots []SignalSnapshot
}

// NewTracker opens (or creates) a tracker store at `dir`.
func NewTracker(dir string) (*Tracker, error) {
	opts := badger.DefaultOptions(dir).
		WithLogger(nil).
		WithValueLogFileSize(16 << 20)
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("thread tracker open: %w", err)
	}
	t := &Tracker{db: db, cache: map[string]*threadState{}}
	if err := t.loadAll(); err != nil {
		db.Close()
		return nil, err
	}
	return t, nil
}

// Close shuts down the underlying BadgerDB.
func (t *Tracker) Close() error { return t.db.Close() }

// Subscribe registers a callback for level-change events. Multiple
// subscribers are fan-outed in registration order.
func (t *Tracker) Subscribe(s Subscriber) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.subscribers = append(t.subscribers, s)
}

func (t *Tracker) loadAll() error {
	return t.db.View(func(txn *badger.Txn) error {
		prefix := []byte("th:meta:")
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var th Thread
			if err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &th)
			}); err != nil {
				continue
			}
			cp := th
			t.cache[th.ID] = &threadState{thread: &cp}
		}
		return nil
	})
}

// OnSignal records a new signal. If `ref.ParentID` is empty, this signal
// starts a new thread. Otherwise, the signal is appended to the parent's
// thread (resolved by walking the InReplyTo chain — but since we only need
// the root, the upstream caller is expected to pass the resolved root
// thread ID directly via ParentID OR pass the immediate parent and let the
// tracker resolve).
//
// For Phase 1 we keep this simple: ParentID is treated as the **root thread
// ID** if it exists in our cache, otherwise we treat it as a new root.
func (t *Tracker) OnSignal(ref SignalRef) (*Thread, error) {
	if ref.Snapshot.ID == "" {
		return nil, errors.New("thread: signal snapshot missing ID")
	}
	if ref.Snapshot.IssuedAt.IsZero() {
		ref.Snapshot.IssuedAt = time.Now()
	}

	// Decide which thread receives this signal.
	threadID := ref.ParentID
	t.mu.Lock()

	var st *threadState
	if threadID != "" {
		if existing, ok := t.cache[threadID]; ok {
			st = existing
		}
		// If ParentID was supplied but we don't know it, we still treat it
		// as a new thread — the InReplyTo chain might land on a thread that
		// hasn't been observed yet (offline replay etc.). We use ParentID
		// as the new thread's ID for stability.
	}
	if st == nil {
		if threadID == "" {
			threadID = ref.Snapshot.ID
		}
		th := &Thread{
			ID:           threadID,
			StartedAt:    ref.Snapshot.IssuedAt,
			LastActivity: ref.Snapshot.IssuedAt,
			CurrentLevel: LevelNone,
		}
		st = &threadState{thread: th}
		t.cache[threadID] = st
	}

	// Append signal & participant.
	already := false
	for _, id := range st.thread.SignalIDs {
		if id == ref.Snapshot.ID {
			already = true
			break
		}
	}
	if !already {
		st.thread.SignalIDs = append(st.thread.SignalIDs, ref.Snapshot.ID)
		st.snapshots = append(st.snapshots, ref.Snapshot)
		st.thread.Participants = appendIfNew(st.thread.Participants, ref.Snapshot.From)
	}
	if ref.Snapshot.IssuedAt.After(st.thread.LastActivity) {
		st.thread.LastActivity = ref.Snapshot.IssuedAt
	}

	// Recompute readiness.
	score := ComputeReadiness(st.snapshots, ref.NetworkSize, st.thread.StartedAt, time.Now())
	prevLevel := st.thread.CurrentLevel
	newLevel := ClassifyReadiness(score.Total)
	st.thread.Readiness = score
	st.thread.CurrentLevel = newLevel
	st.thread.ReadinessHistory = appendBoundedHistory(st.thread.ReadinessHistory,
		ReadinessSnapshot{At: score.ComputedAt, Score: score, Level: newLevel})

	// Persist.
	if err := t.persistLocked(st.thread); err != nil {
		t.mu.Unlock()
		return nil, err
	}

	// Snapshot the values we need for fan-out under the read lock.
	subscribers := append([]Subscriber(nil), t.subscribers...)
	snapshotsCopy := append([]SignalSnapshot(nil), st.snapshots...)
	threadCopy := *st.thread
	t.mu.Unlock()

	if newLevel != prevLevel && len(subscribers) > 0 {
		dominantPos, dominantConf := DominantPosition(snapshotsCopy)
		change := LevelChange{
			ThreadID:           threadID,
			From:               prevLevel,
			To:                 newLevel,
			Score:              score,
			Snapshots:          snapshotsCopy,
			DominantPosition:   dominantPos,
			DominantConfidence: dominantConf,
		}
		for _, s := range subscribers {
			s(change)
		}
	}

	return &threadCopy, nil
}

// SetCrystallizedPraxonID is the back-reference path used by the
// crystallize subscriber once a Praxon ID is known.
func (t *Tracker) SetCrystallizedPraxonID(threadID, praxonID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	st, ok := t.cache[threadID]
	if !ok {
		return ErrUnknownThread
	}
	st.thread.CrystallizedPraxonID = praxonID
	return t.persistLocked(st.thread)
}

// Get returns a defensive copy of the thread or nil if unknown.
func (t *Tracker) Get(threadID string) *Thread {
	t.mu.RLock()
	defer t.mu.RUnlock()
	st, ok := t.cache[threadID]
	if !ok {
		return nil
	}
	cp := *st.thread
	return &cp
}

// Active returns threads currently at or above `minLevel`, sorted by
// LastActivity descending. Pass LevelNone to see every thread.
func (t *Tracker) Active(minLevel CrystallizationLevel) []*Thread {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]*Thread, 0)
	for _, st := range t.cache {
		if !atLeastLevel(st.thread.CurrentLevel, minLevel) {
			continue
		}
		cp := *st.thread
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastActivity.After(out[j].LastActivity)
	})
	return out
}

// === Persistence ===

func (t *Tracker) persistLocked(th *Thread) error {
	data, err := json.Marshal(th)
	if err != nil {
		return err
	}
	return t.db.Update(func(txn *badger.Txn) error {
		if err := txn.Set([]byte("th:meta:"+th.ID), data); err != nil {
			return err
		}
		idxKey := fmt.Sprintf("th:active:%020d:%s", th.LastActivity.UnixMilli(), th.ID)
		if err := txn.Set([]byte(idxKey), []byte{}); err != nil {
			return err
		}
		for _, p := range th.Participants {
			pk := fmt.Sprintf("th:by_participant:%s:%s", p, th.ID)
			if err := txn.Set([]byte(pk), []byte{}); err != nil {
				return err
			}
		}
		return nil
	})
}

// === Helpers ===

func appendIfNew(xs []string, x string) []string {
	if x == "" {
		return xs
	}
	for _, e := range xs {
		if e == x {
			return xs
		}
	}
	return append(xs, x)
}

func appendBoundedHistory(h []ReadinessSnapshot, snap ReadinessSnapshot) []ReadinessSnapshot {
	h = append(h, snap)
	if len(h) > MaxHistoryEntries {
		h = h[len(h)-MaxHistoryEntries:]
	}
	return h
}

func atLeastLevel(have, need CrystallizationLevel) bool {
	rank := map[CrystallizationLevel]int{
		LevelNone: 0, LevelDraft: 1, LevelProvisional: 2, LevelCanonical: 3,
	}
	return rank[have] >= rank[need]
}
