// Package visibility implements the trust-graph privacy controls
// (cav-social-trust §R7).
//
// Three modes for the *trust graph* — i.e. the literal "who I trust" edge
// list:
//
//   public      — any authenticated citizen can read
//   private     — only the owner can read
//   mutual_only — only readable by citizens with a bidirectional Cognitive
//                 OR Social trust edge with the owner
//
// Out of scope for the policy gate (always public per R7-2..R7-4):
//   - Reputation Vector
//   - Behavioral Digest summary stats
//   - Domain Activity Vector derived from public Praxons
//
// The policy lives in BadgerDB at:
//
//   v:policy:<did>  → VisibilityPolicy JSON
//
// `Decide` is the gate every read path on the trust graph must call.

package visibility

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// Mode is one of public / private / mutual_only.
type Mode string

const (
	Public     Mode = "public"
	Private    Mode = "private"
	MutualOnly Mode = "mutual_only"
)

// IsValid reports whether `m` is a recognized mode.
func (m Mode) IsValid() bool {
	switch m {
	case Public, Private, MutualOnly:
		return true
	}
	return false
}

// VisibilityPolicy is the persisted setting for one citizen.
type VisibilityPolicy struct {
	DID                  string    `json:"did"`
	TrustGraphVisibility Mode      `json:"trust_graph_visibility"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// MutualTrustChecker is the contract for "are these two citizens mutually
// trusting?" — implemented in the trust package as HasMutualTrust. We keep
// this as an interface so visibility doesn't import trust directly (avoids
// a cycle when trust eventually wants to consult visibility).
type MutualTrustChecker interface {
	HasMutual(a, b string) bool
}

// Store persists visibility policies and answers Decide().
type Store struct {
	db    *badger.DB
	mu    sync.RWMutex
	cache map[string]Mode
}

// NewStore opens (or creates) the visibility store at `dir`.
func NewStore(dir string) (*Store, error) {
	opts := badger.DefaultOptions(dir).
		WithLogger(nil).
		WithValueLogFileSize(8 << 20)
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("visibility store open: %w", err)
	}
	s := &Store{db: db, cache: map[string]Mode{}}
	if err := s.loadAll(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close shuts down the store.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) loadAll() error {
	return s.db.View(func(txn *badger.Txn) error {
		prefix := []byte("v:policy:")
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var p VisibilityPolicy
			if err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &p)
			}); err != nil {
				continue
			}
			s.cache[p.DID] = p.TrustGraphVisibility
		}
		return nil
	})
}

// Get returns the policy for `did`. If unset, returns Public (the spec
// default per R7-7).
func (s *Store) Get(did string) Mode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if m, ok := s.cache[did]; ok && m.IsValid() {
		return m
	}
	return Public
}

// Set persists a new policy for `did`.
func (s *Store) Set(did string, mode Mode) error {
	if did == "" {
		return errors.New("visibility: did required")
	}
	if !mode.IsValid() {
		return fmt.Errorf("visibility: invalid mode %q", mode)
	}
	policy := VisibilityPolicy{
		DID:                  did,
		TrustGraphVisibility: mode,
		UpdatedAt:            time.Now(),
	}
	data, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte("v:policy:"+did), data)
	}); err != nil {
		return err
	}
	s.cache[did] = mode
	return nil
}

// === Gate ===

// Decide returns nil when `viewer` is allowed to read `target`'s trust
// graph; otherwise it returns ErrHidden with a human-readable reason.
//
// `mutualChecker` may be nil when MutualOnly is not configured for any
// citizen — in that case we treat MutualOnly as Private (deny non-self).
func (s *Store) Decide(viewer, target string, mutualChecker MutualTrustChecker) error {
	if target == "" {
		return errors.New("visibility: target required")
	}
	mode := s.Get(target)
	if viewer == target {
		return nil
	}
	switch mode {
	case Public:
		return nil
	case Private:
		return ErrHidden{Mode: Private}
	case MutualOnly:
		if mutualChecker != nil && mutualChecker.HasMutual(viewer, target) {
			return nil
		}
		return ErrHidden{Mode: MutualOnly}
	default:
		return ErrHidden{Mode: mode}
	}
}

// ErrHidden is returned by Decide when access is denied.
type ErrHidden struct {
	Mode Mode
}

func (e ErrHidden) Error() string {
	return "visibility: target trust graph hidden by policy=" + string(e.Mode)
}
