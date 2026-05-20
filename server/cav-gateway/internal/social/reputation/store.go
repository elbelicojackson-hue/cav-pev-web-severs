// Persistent reputation store backed by BadgerDB.
//
// Key schema (design.md §3):
//   rep:v:<did>                          → Vector JSON (cached in memory)
//   rep:event:<did>:<ts_ms>:<event_id>   → Event JSON (append-only mutation log)
//
// All reputation mutations MUST flow through Apply(Event). Direct vector
// mutation is intentionally not exposed — the Event log is the source of truth
// and is what allows audit, retrospective updates, and replay.

package reputation

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// Store is the persistent reputation registry. Hot data (every active citizen's
// vector) lives in `cache` for O(1) reads; writes are write-through to BadgerDB.
type Store struct {
	db    *badger.DB
	mu    sync.RWMutex
	cache map[string]*Vector
}

// NewStore opens (or creates) a reputation store at `dir`. Mirrors the tuning
// of citizen.NewPersistentRegistry — these stores have similar access patterns
// (small JSON values, dominated by reads).
func NewStore(dir string) (*Store, error) {
	opts := badger.DefaultOptions(dir).
		WithLogger(nil).
		WithValueLogFileSize(16 << 20)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("reputation store open failed: %w", err)
	}

	s := &Store{
		db:    db,
		cache: make(map[string]*Vector),
	}
	if err := s.loadAll(); err != nil {
		db.Close()
		return nil, err
	}

	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			db.RunValueLogGC(0.5)
		}
	}()

	return s, nil
}

// Close shuts down the store.
func (s *Store) Close() error {
	return s.db.Close()
}

// loadAll warms the cache from disk on startup.
func (s *Store) loadAll() error {
	return s.db.View(func(txn *badger.Txn) error {
		prefix := []byte("rep:v:")
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var v Vector
			if err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &v)
			}); err != nil {
				continue
			}
			// Migrate vectors written before SchemaVersion existed.
			if v.SchemaVersion == "" {
				v.SchemaVersion = SchemaVersion
			}
			cp := v
			s.cache[v.DID] = &cp
		}
		return nil
	})
}

// Get returns the citizen's vector. If none exists, returns a fresh empty
// Vector (NOT persisted) — Get must be safe to call from read-only paths.
func (s *Store) Get(did string) *Vector {
	s.mu.RLock()
	v, ok := s.cache[did]
	s.mu.RUnlock()
	if ok {
		// Defensive copy so callers cannot mutate cached state.
		return cloneVector(v)
	}
	return NewVector(did, time.Now())
}

// Has reports whether a vector has been persisted for this DID.
func (s *Store) Has(did string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.cache[did]
	return ok
}

// All returns defensive copies of all stored vectors.
func (s *Store) All() []*Vector {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Vector, 0, len(s.cache))
	for _, v := range s.cache {
		out = append(out, cloneVector(v))
	}
	return out
}

// EffectiveScore is the convenience wrapper used by the convergence engine.
// Equivalent to Store.Get(did).EffectiveScore(domain) but avoids the defensive
// copy on the hot path.
func (s *Store) EffectiveScore(did, domain string) float64 {
	s.mu.RLock()
	v, ok := s.cache[did]
	s.mu.RUnlock()
	if !ok {
		return 0
	}
	return v.EffectiveScore(domain)
}

// Bootstrap creates a Vector for `did` seeded from the legacy scalar Level.
// No-op if a vector already exists. Used by gateway startup migration to keep
// existing Level-1/2/3 citizens functional under the new convergence path.
func (s *Store) Bootstrap(did string, level int, domains []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.cache[did]; exists {
		return nil
	}
	v := LegacyLevelToVector(did, level, domains, time.Now())
	s.cache[did] = v
	return s.persistLocked(v)
}

// SetInactive flips the inactive flag (used by the digest subsystem when no
// signed digest has arrived in N days). Persisted but does NOT generate an
// Event — inactive is a derived liveness signal, not a reputation mutation.
func (s *Store) SetInactive(did string, inactive bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.cache[did]
	if !ok {
		return false
	}
	if v.Inactive == inactive {
		return true
	}
	v.Inactive = inactive
	if err := s.persistLocked(v); err != nil {
		return false
	}
	return true
}

// SetBehavioral overwrites the behavioral subvector. Called by the digest
// subsystem after each digest period. Like SetInactive, this is derived from
// public records, not a reputation Event.
func (s *Store) SetBehavioral(did string, b BehavioralSubvector) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.cache[did]
	if !ok {
		// Auto-create — behavioral can be computed before any reputation
		// Event has fired (e.g. for a citizen still in probation).
		v = NewVector(did, time.Now())
		s.cache[did] = v
	}
	v.Behavioral = b
	if err := s.persistLocked(v); err != nil {
		return false
	}
	return true
}

// persistLocked writes the vector to BadgerDB. Caller must hold s.mu.
func (s *Store) persistLocked(v *Vector) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte("rep:v:"+v.DID), data)
	})
}

// putVectorLocked replaces the cached vector and persists it. Used by Apply()
// (in update.go) which owns the only legitimate write path.
func (s *Store) putVectorLocked(v *Vector) error {
	cp := cloneVector(v)
	s.cache[v.DID] = cp
	return s.persistLocked(cp)
}

// recordEventLocked appends an Event to the per-DID event log. Caller must
// hold s.mu. The key uses zero-padded ms timestamp so iteration is in
// chronological order.
func (s *Store) recordEventLocked(ev Event) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	key := []byte(fmt.Sprintf("rep:event:%s:%020d:%s", ev.DID, ev.OccurredAt.UnixMilli(), ev.ID))
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
}

// Events returns all events for a DID in chronological order.
// Used for retrospective audit and tests.
func (s *Store) Events(did string) ([]Event, error) {
	prefix := []byte("rep:event:" + did + ":")
	var out []Event
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var ev Event
			if err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &ev)
			}); err != nil {
				continue
			}
			out = append(out, ev)
		}
		return nil
	})
	return out, err
}

// cloneVector returns a deep copy of v. Used to keep the cache isolated from
// caller mutation.
func cloneVector(v *Vector) *Vector {
	if v == nil {
		return nil
	}
	cp := *v
	cp.Operational = TierVector{Domains: cloneDomains(v.Operational.Domains)}
	cp.Deliberation = TierVector{Domains: cloneDomains(v.Deliberation.Domains)}
	return &cp
}

func cloneDomains(in map[string]DomainScore) map[string]DomainScore {
	if in == nil {
		return map[string]DomainScore{}
	}
	out := make(map[string]DomainScore, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// ErrUnknownDID is returned when an operation references a DID that has no
// vector and the operation cannot auto-create one.
var ErrUnknownDID = errors.New("reputation: unknown DID")
