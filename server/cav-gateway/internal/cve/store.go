package cve

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// Store is the BadgerDB-backed CVE database.
type Store struct {
	db *badger.DB
}

// NewStore opens the CVE database.
func NewStore(dir string) (*Store, error) {
	opts := badger.DefaultOptions(dir).
		WithLogger(nil).
		WithValueLogFileSize(128 << 20). // 128MB (CVE data is large)
		WithNumCompactors(2)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("cve store open failed: %w", err)
	}

	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			db.RunValueLogGC(0.5)
		}
	}()

	return &Store{db: db}, nil
}

// Close shuts down the store.
func (s *Store) Close() error {
	return s.db.Close()
}

// Put stores a CVE entry with indexes.
func (s *Store) Put(cve *CVE) error {
	data, err := json.Marshal(cve)
	if err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		// Primary
		if err := txn.Set([]byte("cve:id:"+cve.ID), data); err != nil {
			return err
		}

		// Product index
		for _, p := range cve.AffectedProducts {
			key := fmt.Sprintf("cve:product:%s:%s:%s", strings.ToLower(p.Vendor), strings.ToLower(p.Product), cve.ID)
			txn.Set([]byte(key), []byte{})
		}

		// Year index
		year := cve.Published.Format("2006")
		txn.Set([]byte(fmt.Sprintf("cve:year:%s:%s", year, cve.ID)), []byte{})

		// Severity index
		if cve.Severity != "" {
			txn.Set([]byte(fmt.Sprintf("cve:severity:%s:%s", strings.ToUpper(cve.Severity), cve.ID)), []byte{})
		}

		// KEV flag
		if cve.IsKEV {
			txn.Set([]byte("cve:kev:"+cve.ID), []byte{})
		}

		return nil
	})
}

// Get retrieves a CVE by ID.
func (s *Store) Get(id string) (*CVE, error) {
	var cve CVE
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("cve:id:" + id))
		if err == badger.ErrKeyNotFound {
			return fmt.Errorf("CVE %s not found", id)
		}
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &cve)
		})
	})
	return &cve, err
}

// Search queries CVEs by various criteria.
func (s *Store) Search(q SearchQuery) (*SearchResult, error) {
	if q.Limit <= 0 {
		q.Limit = 20
	}
	if q.Limit > 100 {
		q.Limit = 100
	}

	var ids []string

	// Determine which index to scan
	var prefix []byte
	switch {
	case q.KEVOnly:
		prefix = []byte("cve:kev:")
	case q.Severity != "":
		prefix = []byte(fmt.Sprintf("cve:severity:%s:", strings.ToUpper(q.Severity)))
	case q.Year > 0:
		prefix = []byte(fmt.Sprintf("cve:year:%d:", q.Year))
	case q.Product != "":
		if q.Vendor != "" {
			prefix = []byte(fmt.Sprintf("cve:product:%s:%s:", strings.ToLower(q.Vendor), strings.ToLower(q.Product)))
		} else {
			// Scan all vendors for this product — expensive but functional
			prefix = []byte("cve:product:")
		}
	default:
		prefix = []byte("cve:id:")
	}

	// Scan index
	s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		opts.Reverse = true // Most recent first
		it := txn.NewIterator(opts)
		defer it.Close()

		seekKey := append(prefix, 0xFF)
		skipped := 0

		for it.Seek(seekKey); it.ValidForPrefix(prefix); it.Next() {
			key := string(it.Item().Key())
			// Extract CVE ID from key
			parts := strings.Split(key, ":")
			cveID := parts[len(parts)-1]

			if !strings.HasPrefix(cveID, "CVE-") {
				continue
			}

			// Pagination
			if skipped < q.Offset {
				skipped++
				continue
			}

			ids = append(ids, cveID)
			if len(ids) >= q.Limit {
				break
			}
		}
		return nil
	})

	// Fetch full CVE records
	cves := make([]CVE, 0, len(ids))
	for _, id := range ids {
		cve, err := s.Get(id)
		if err == nil {
			// Apply additional filters
			if q.MinCVSS > 0 && cve.CVSS3Score < q.MinCVSS {
				continue
			}
			if q.MinEPSS > 0 && cve.EPSS < q.MinEPSS {
				continue
			}
			if q.Keyword != "" && !strings.Contains(strings.ToLower(cve.Description), strings.ToLower(q.Keyword)) {
				continue
			}
			cves = append(cves, *cve)
		}
	}

	return &SearchResult{
		CVEs:  cves,
		Total: len(cves),
		Query: q,
	}, nil
}

// Count returns total CVEs stored.
func (s *Store) Count() uint64 {
	var count uint64
	s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("cve:meta:count"))
		if err != nil {
			return nil
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

// Status returns sync status.
func (s *Store) Status() *SyncStatus {
	var lastSync time.Time
	s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("cve:meta:last_sync"))
		if err != nil {
			return nil
		}
		item.Value(func(val []byte) error {
			lastSync, _ = time.Parse(time.RFC3339, string(val))
			return nil
		})
		return nil
	})

	return &SyncStatus{
		TotalCVEs: s.Count(),
		LastSync:  lastSync,
		Sources:   []string{"NVD", "CISA-KEV", "EPSS"},
		NextSync:  lastSync.Add(6 * time.Hour),
	}
}
