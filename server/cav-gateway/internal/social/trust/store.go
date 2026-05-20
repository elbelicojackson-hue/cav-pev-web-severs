// Persistent trust graph backed by BadgerDB.
//
// Key schema (design.md §3):
//   t:edge:<from>:<kind>:<domain>:<to>      → TrustEdge JSON  (primary)
//   t:rev:<to>:<from>:<kind>:<domain>       → []              (reverse index)
//   t:meta:edge_count:<did>                 → uint32          (per-DID outgoing edge count)
//
// Concurrency: a single sync.RWMutex guards the in-memory cache. BadgerDB
// transactions handle disk-level consistency. Cache lookups are O(N) over a
// DID's outgoing edges — fine while N stays small (target ≤ 10k edges per DID).
package trust

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// ErrAlreadyExists is returned when AddTrust would create a duplicate
// (from, to, kind, domain) edge that is not currently revoked.
var ErrAlreadyExists = errors.New("trust: edge already exists")

// ErrNotFound is returned when an operation references a nonexistent edge.
var ErrNotFound = errors.New("trust: edge not found")

// Store is the persistent trust graph.
type Store struct {
	db    *badger.DB
	mu    sync.RWMutex
	cache map[string]*TrustEdge // keyed by EdgeKey
}

// NewStore opens (or creates) the trust store at `dir`.
func NewStore(dir string) (*Store, error) {
	opts := badger.DefaultOptions(dir).
		WithLogger(nil).
		WithValueLogFileSize(16 << 20)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("trust store open failed: %w", err)
	}

	s := &Store{
		db:    db,
		cache: make(map[string]*TrustEdge),
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

func (s *Store) loadAll() error {
	return s.db.View(func(txn *badger.Txn) error {
		prefix := []byte("t:edge:")
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var e TrustEdge
			if err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &e)
			}); err != nil {
				continue
			}
			cp := e
			s.cache[e.EdgeKey()] = &cp
		}
		return nil
	})
}

// AddTrust persists a new edge. Enforces:
//   - structural validity (TrustEdge.Validate)
//   - uniqueness: a non-revoked (from, to, kind, domain) tuple cannot exist.
//     If a previously-revoked edge with the same tuple exists, it is replaced.
//
// Timestamps: EstablishedAt is stamped to time.Now() if zero; LastDecayAt
// follows. RevokedAt/RevokeReason on the input are cleared (callers shouldn't
// set them; use RevokeTrust for that).
func (s *Store) AddTrust(e *TrustEdge) (*TrustEdge, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := e.EdgeKey()
	if existing, ok := s.cache[key]; ok && !existing.IsRevoked() {
		return nil, ErrAlreadyExists
	}

	now := time.Now()
	if e.EstablishedAt.IsZero() {
		e.EstablishedAt = now
	}
	if e.LastDecayAt.IsZero() {
		e.LastDecayAt = e.EstablishedAt
	}
	e.RevokedAt = nil
	e.RevokeReason = ""

	cp := *e
	s.cache[key] = &cp

	if err := s.persistLocked(&cp); err != nil {
		// Roll back cache to keep on-disk and in-memory consistent
		delete(s.cache, key)
		return nil, fmt.Errorf("persist edge: %w", err)
	}
	if err := s.persistReverseLocked(&cp); err != nil {
		return nil, fmt.Errorf("persist reverse index: %w", err)
	}
	return &cp, nil
}

// RevokeTrust soft-deletes an edge. The edge stays in the store (preserves
// audit trail) but is excluded from default queries. Returns the updated edge
// or ErrNotFound.
//
// Idempotent: revoking an already-revoked edge updates the reason but keeps
// the original RevokedAt timestamp.
func (s *Store) RevokeTrust(from string, kind TrustKind, domain, to, reason string) (*TrustEdge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := EdgeKey(from, kind, domain, to)
	e, ok := s.cache[key]
	if !ok {
		return nil, ErrNotFound
	}

	if !e.IsRevoked() {
		now := time.Now()
		e.RevokedAt = &now
	}
	e.RevokeReason = reason

	if err := s.persistLocked(e); err != nil {
		return nil, fmt.Errorf("persist revoke: %w", err)
	}
	cp := *e
	return &cp, nil
}

// Edges returns outgoing edges from `from` matching the filter.
// Order is undefined; callers should sort if needed.
func (s *Store) Edges(from string, f Filter) []*TrustEdge {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*TrustEdge, 0)
	for _, e := range s.cache {
		if e.From != from {
			continue
		}
		if !match(e, f) {
			continue
		}
		cp := *e
		out = append(out, &cp)
	}
	return out
}

// Reverse returns incoming edges to `to` matching the filter (i.e. "who
// trusts me").
func (s *Store) Reverse(to string, f Filter) []*TrustEdge {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*TrustEdge, 0)
	for _, e := range s.cache {
		if e.To != to {
			continue
		}
		if !match(e, f) {
			continue
		}
		cp := *e
		out = append(out, &cp)
	}
	return out
}

// Get returns a specific edge or nil if it doesn't exist.
func (s *Store) Get(from string, kind TrustKind, domain, to string) *TrustEdge {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.cache[EdgeKey(from, kind, domain, to)]
	if !ok {
		return nil
	}
	cp := *e
	return &cp
}

// All returns every edge regardless of state — used for admin / migration.
func (s *Store) All() []*TrustEdge {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*TrustEdge, 0, len(s.cache))
	for _, e := range s.cache {
		cp := *e
		out = append(out, &cp)
	}
	return out
}

// EdgeCount returns the number of non-revoked outgoing edges from `from`.
func (s *Store) EdgeCount(from string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, e := range s.cache {
		if e.From == from && !e.IsRevoked() {
			n++
		}
	}
	return n
}

// match applies the Filter to an edge.
func match(e *TrustEdge, f Filter) bool {
	if f.Kind != "" && e.Kind != f.Kind {
		return false
	}
	if f.Domain != "" && e.Domain != f.Domain {
		return false
	}
	if !f.IncludeRevoked && e.IsRevoked() {
		return false
	}
	return true
}

func (s *Store) persistLocked(e *TrustEdge) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(e.EdgeKey()), data)
	})
}

func (s *Store) persistReverseLocked(e *TrustEdge) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(ReverseKey(e.To, e.From, e.Kind, e.Domain)), []byte{})
	})
}
