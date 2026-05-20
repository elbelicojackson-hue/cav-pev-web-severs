// Persistent digest store + inactivity sweep.
//
// Storage (design.md §3):
//   d:digest:<did>:<period_start_ms>  → BehavioralDigest JSON
//   d:latest:<did>                    → period_start_ms (uint64, big-endian)
//
// Period monotonicity: a digest with period_start <= latest is rejected so
// agents can't replay or reorder old observations.

package digest

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// ErrPeriodRegression is returned when a digest's period_start is at or
// before the citizen's most recently accepted period.
var ErrPeriodRegression = errors.New("digest: period_start regression")

// Store persists verified digests and tracks per-DID latest acceptance.
type Store struct {
	db *badger.DB
	mu sync.Mutex
}

// NewStore opens (or creates) a digest store at `dir`.
func NewStore(dir string) (*Store, error) {
	opts := badger.DefaultOptions(dir).
		WithLogger(nil).
		WithValueLogFileSize(16 << 20)
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("digest store open: %w", err)
	}
	return &Store{db: db}, nil
}

// Close shuts the store down.
func (s *Store) Close() error { return s.db.Close() }

// Put persists `d`. Caller must have already verified the signature.
// Enforces period monotonicity.
func (s *Store) Put(d *BehavioralDigest) error {
	if d == nil {
		return errors.New("digest: nil")
	}
	if d.DID == "" {
		return ErrMissingDID
	}
	if !d.PeriodEnd.After(d.PeriodStart) {
		return ErrInvalidPeriod
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	periodMs := d.PeriodStart.UnixMilli()

	return s.db.Update(func(txn *badger.Txn) error {
		// Latest check
		latestKey := []byte("d:latest:" + d.DID)
		if item, err := txn.Get(latestKey); err == nil {
			var prev int64
			item.Value(func(val []byte) error {
				if len(val) == 8 {
					prev = int64(binary.BigEndian.Uint64(val))
				}
				return nil
			})
			if periodMs <= prev {
				return ErrPeriodRegression
			}
		}

		data, err := json.Marshal(d)
		if err != nil {
			return err
		}
		key := []byte(fmt.Sprintf("d:digest:%s:%020d", d.DID, periodMs))
		if err := txn.Set(key, data); err != nil {
			return err
		}
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, uint64(periodMs))
		return txn.Set(latestKey, buf)
	})
}

// Latest returns the digest with the largest period_start for `did`, or nil.
func (s *Store) Latest(did string) (*BehavioralDigest, error) {
	var latest *BehavioralDigest
	prefix := []byte("d:digest:" + did + ":")

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Reverse = true
		it := txn.NewIterator(opts)
		defer it.Close()

		seekKey := append(prefix, 0xFF)
		for it.Seek(seekKey); it.ValidForPrefix(prefix); it.Next() {
			var d BehavioralDigest
			err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &d)
			})
			if err == nil {
				latest = &d
			}
			return nil // first match wins under reverse iteration
		}
		return nil
	})
	return latest, err
}

// LatestPeriodMs returns the latest accepted period_start in ms for `did`,
// or 0 if no digest has been accepted.
func (s *Store) LatestPeriodMs(did string) int64 {
	var out int64
	s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("d:latest:" + did))
		if err != nil {
			return nil
		}
		return item.Value(func(val []byte) error {
			if len(val) == 8 {
				out = int64(binary.BigEndian.Uint64(val))
			}
			return nil
		})
	})
	return out
}

// AllLatestDIDs returns the set of DIDs that have ever submitted a digest
// (i.e., have a `d:latest:*` row). Used by the inactivity sweep.
func (s *Store) AllLatestDIDs() []string {
	var out []string
	prefix := []byte("d:latest:")
	s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			k := it.Item().Key()
			out = append(out, string(k[len(prefix):]))
		}
		return nil
	})
	return out
}

// === Inactivity sweep ===

// InactivitySetter is the minimal contract Store needs from the reputation
// store to flip the inactive flag without importing reputation directly
// (avoids package-level cycles in tests).
type InactivitySetter interface {
	SetInactive(did string, inactive bool) bool
}

// SweepInactive walks every digest-emitting DID and flips reputation.Inactive
// based on whether the latest digest is older than InactivityWindow.
//
// `now` is the wall-clock anchor (caller controls for testability).
// Returns (flagged, cleared, total) counts.
func (s *Store) SweepInactive(rep InactivitySetter, now time.Time) (flagged, cleared, total int) {
	cutoff := now.Add(-InactivityWindow).UnixMilli()
	for _, did := range s.AllLatestDIDs() {
		total++
		latest := s.LatestPeriodMs(did)
		shouldFlag := latest < cutoff
		if rep == nil {
			continue
		}
		if shouldFlag {
			if rep.SetInactive(did, true) {
				flagged++
			}
		} else {
			if rep.SetInactive(did, false) {
				cleared++
			}
		}
	}
	return
}
