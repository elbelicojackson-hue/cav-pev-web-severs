// BadgerDB-backed Praxon store with secondary indexes.
//
// Key schema (all keys are byte slices, prefixed for namespace separation):
//
//   Primary:
//     p:<praxon_id>                → raw Praxon JSON bytes
//
//   Secondary indexes (for query):
//     i:issuer:<did>:<praxon_id>   → [] (empty value, key-only index)
//     i:class:<class>:<praxon_id>  → []
//     i:time:<unix_ms>:<praxon_id> → []
//     i:all:<praxon_id>            → [] (global ordered list)
//
//   Metadata:
//     m:count                      → uint64 (total praxon count)
//     m:citizen:<did>              → JSON citizen metadata
//
// This gives us O(1) primary lookup + O(n) prefix-scan for filtered queries.
// No external dependencies. Pure Go. Single binary.
package store

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// BadgerStore is a BadgerDB-backed Praxon store with secondary indexes.
type BadgerStore struct {
	db *badger.DB
}

// NewBadgerStore opens (or creates) a BadgerDB at the given directory.
func NewBadgerStore(dir string) (*BadgerStore, error) {
	opts := badger.DefaultOptions(dir).
		WithLogger(nil). // suppress badger's internal logging
		WithValueLogFileSize(64 << 20) // 64MB value log files

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("badger open failed: %w", err)
	}

	// Start background GC
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			db.RunValueLogGC(0.5)
		}
	}()

	return &BadgerStore{db: db}, nil
}

// Close cleanly shuts down the database.
func (s *BadgerStore) Close() error {
	return s.db.Close()
}

// --- Store interface implementation ---

// Put stores a Praxon and builds secondary indexes.
func (s *BadgerStore) Put(praxonID string, content []byte) error {
	// Check if already exists (idempotent)
	if s.Has(praxonID) {
		return nil
	}

	// Extract issuer and class for indexing
	issuer, class := extractIndexFields(content)
	nowMs := time.Now().UnixMilli()

	return s.db.Update(func(txn *badger.Txn) error {
		// Primary key
		if err := txn.Set(primaryKey(praxonID), content); err != nil {
			return err
		}

		// Secondary indexes (empty values — key-only)
		empty := []byte{}

		// Index by issuer
		if issuer != "" {
			if err := txn.Set(issuerIndexKey(issuer, praxonID), empty); err != nil {
				return err
			}
		}

		// Index by class
		if class != "" {
			if err := txn.Set(classIndexKey(class, praxonID), empty); err != nil {
				return err
			}
		}

		// Index by time (for chronological listing)
		if err := txn.Set(timeIndexKey(nowMs, praxonID), empty); err != nil {
			return err
		}

		// Global index
		if err := txn.Set(allIndexKey(praxonID), empty); err != nil {
			return err
		}

		// Increment count
		return s.incrementCount(txn)
	})
}

// Get retrieves a Praxon by ID.
func (s *BadgerStore) Get(praxonID string) ([]byte, error) {
	var result []byte
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(primaryKey(praxonID))
		if err == badger.ErrKeyNotFound {
			return fmt.Errorf("praxon %s not found", praxonID)
		}
		if err != nil {
			return err
		}
		result, err = item.ValueCopy(nil)
		return err
	})
	return result, err
}

// Has checks if a Praxon exists.
func (s *BadgerStore) Has(praxonID string) bool {
	err := s.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(primaryKey(praxonID))
		return err
	})
	return err == nil
}

// --- Extended query methods (beyond basic Store interface) ---

// Count returns the total number of stored Praxons.
func (s *BadgerStore) Count() uint64 {
	var count uint64
	s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("m:count"))
		if err != nil {
			return nil // not found = 0
		}
		item.Value(func(val []byte) error {
			if len(val) == 8 {
				count = binary.BigEndian.Uint64(val)
			}
			return nil
		})
		return nil
	})
	return count
}

// ListByIssuer returns all praxon IDs published by a given issuer.
func (s *BadgerStore) ListByIssuer(issuer string, limit int) []string {
	prefix := []byte(fmt.Sprintf("i:issuer:%s:", issuer))
	return s.scanPrefix(prefix, limit)
}

// ListByClass returns all praxon IDs of a given class.
func (s *BadgerStore) ListByClass(class string, limit int) []string {
	prefix := []byte(fmt.Sprintf("i:class:%s:", class))
	return s.scanPrefix(prefix, limit)
}

// ListRecent returns the most recent praxon IDs (by publish time).
func (s *BadgerStore) ListRecent(limit int) []string {
	prefix := []byte("i:time:")
	return s.scanPrefixReverse(prefix, limit)
}

// ListAll returns all praxon IDs.
func (s *BadgerStore) ListAll(limit int) []string {
	prefix := []byte("i:all:")
	return s.scanPrefix(prefix, limit)
}

// --- Internal helpers ---

func primaryKey(id string) []byte {
	return []byte("p:" + id)
}

func issuerIndexKey(issuer, praxonID string) []byte {
	return []byte(fmt.Sprintf("i:issuer:%s:%s", issuer, praxonID))
}

func classIndexKey(class, praxonID string) []byte {
	return []byte(fmt.Sprintf("i:class:%s:%s", class, praxonID))
}

func timeIndexKey(unixMs int64, praxonID string) []byte {
	// Zero-padded for lexicographic ordering
	return []byte(fmt.Sprintf("i:time:%020d:%s", unixMs, praxonID))
}

func allIndexKey(praxonID string) []byte {
	return []byte("i:all:" + praxonID)
}

// extractIndexFields pulls issuer and praxon_class from raw JSON.
// Minimal parsing — just enough for indexing.
func extractIndexFields(content []byte) (issuer, class string) {
	var partial struct {
		Issuer      string `json:"issuer"`
		PraxonClass string `json:"praxon_class"`
	}
	json.Unmarshal(content, &partial)
	return partial.Issuer, partial.PraxonClass
}

func (s *BadgerStore) incrementCount(txn *badger.Txn) error {
	key := []byte("m:count")
	var count uint64

	item, err := txn.Get(key)
	if err == nil {
		item.Value(func(val []byte) error {
			if len(val) == 8 {
				count = binary.BigEndian.Uint64(val)
			}
			return nil
		})
	}

	count++
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, count)
	return txn.Set(key, buf)
}

// scanPrefix does a forward prefix scan and extracts the trailing ID from each key.
func (s *BadgerStore) scanPrefix(prefix []byte, limit int) []string {
	var results []string
	s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // key-only scan
		it := txn.NewIterator(opts)
		defer it.Close()

		count := 0
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			if limit > 0 && count >= limit {
				break
			}
			key := it.Item().Key()
			// Extract praxon_id from the last segment after the last ':'
			id := extractLastSegment(key)
			if id != "" {
				results = append(results, id)
			}
			count++
		}
		return nil
	})
	return results
}

// scanPrefixReverse does a reverse prefix scan (most recent first).
func (s *BadgerStore) scanPrefixReverse(prefix []byte, limit int) []string {
	var results []string
	s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		opts.Reverse = true
		it := txn.NewIterator(opts)
		defer it.Close()

		// Seek to the end of the prefix range
		seekKey := append(prefix, 0xFF)
		count := 0
		for it.Seek(seekKey); it.ValidForPrefix(prefix); it.Next() {
			if limit > 0 && count >= limit {
				break
			}
			key := it.Item().Key()
			id := extractLastSegment(key)
			if id != "" {
				results = append(results, id)
			}
			count++
		}
		return nil
	})
	return results
}

// extractLastSegment gets the part after the last ':' in a key.
func extractLastSegment(key []byte) string {
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == ':' {
			return string(key[i+1:])
		}
	}
	return string(key)
}
