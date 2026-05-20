// Signal persistence — stores all entropic signals in BadgerDB for
// history replay, audit, and offline agent catch-up.
//
// Key schema:
//   s:<timestamp_ms>:<signal_id>  → signal JSON (chronological scan)
//   s:from:<fingerprint>:<seq>    → signal JSON (per-sender history)
//   s:tag:<tag>:<signal_id>       → [] (tag index for filtered queries)
//   s:reply:<in_reply_to>:<id>    → [] (thread index)
//   s:meta:seq:<fingerprint>      → uint64 (last sequence number per sender)
//   s:meta:count                  → uint64 (total signal count)
package signal

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// Store persists entropic signals with secondary indexes.
type Store struct {
	db  *badger.DB
	mu  sync.RWMutex
	seq map[string]uint64 // per-sender sequence counters (cached)
}

// NewStore opens a BadgerDB for signal persistence.
// Tuned for high write throughput (10k agents broadcasting).
func NewStore(dir string) (*Store, error) {
	opts := badger.DefaultOptions(dir).
		WithLogger(nil).
		WithValueLogFileSize(64 << 20).       // 64MB value log files
		WithNumMemtables(4).                   // More memtables for write buffering
		WithNumLevelZeroTables(8).             // Allow more L0 tables before compaction
		WithNumLevelZeroTablesStall(16).       // Higher stall threshold
		WithValueThreshold(256).               // Inline small values (signals are ~500B)
		WithNumCompactors(4)                   // Parallel compaction

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("signal store open failed: %w", err)
	}

	// Background GC
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			db.RunValueLogGC(0.5)
		}
	}()

	return &Store{
		db:  db,
		seq: make(map[string]uint64),
	}, nil
}

// Close shuts down the store.
func (s *Store) Close() error {
	return s.db.Close()
}

// Append persists a signal and assigns it a sequence number.
func (s *Store) Append(sig *EntropicSignal) error {
	if err := sig.Validate(); err != nil {
		return err
	}

	// Assign sequence number
	s.mu.Lock()
	s.seq[sig.From]++
	sig.Sequence = s.seq[sig.From]
	s.mu.Unlock()

	// Assign ID and timestamp if missing
	if sig.ID == "" {
		sig.ID = NewSignalID()
	}
	if sig.Timestamp == "" {
		sig.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	data, err := sig.ToJSON()
	if err != nil {
		return err
	}

	nowMs := time.Now().UnixMilli()

	return s.db.Update(func(txn *badger.Txn) error {
		// Primary: chronological
		chronKey := []byte(fmt.Sprintf("s:%020d:%s", nowMs, sig.ID))
		if err := txn.Set(chronKey, data); err != nil {
			return err
		}

		// Per-sender history
		senderKey := []byte(fmt.Sprintf("s:from:%s:%020d", sig.From, sig.Sequence))
		if err := txn.Set(senderKey, data); err != nil {
			return err
		}

		// Tag indexes
		for _, tag := range sig.Tags {
			tagKey := []byte(fmt.Sprintf("s:tag:%s:%s", tag, sig.ID))
			if err := txn.Set(tagKey, []byte{}); err != nil {
				return err
			}
		}

		// Reply thread index
		if sig.InReplyTo != "" {
			replyKey := []byte(fmt.Sprintf("s:reply:%s:%s", sig.InReplyTo, sig.ID))
			if err := txn.Set(replyKey, []byte{}); err != nil {
				return err
			}
		}

		// Increment total count
		return s.incrementCount(txn)
	})
}

// Recent returns the last N signals (most recent first).
func (s *Store) Recent(limit int) ([]*EntropicSignal, error) {
	var results []*EntropicSignal

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Reverse = true
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte("s:")
		// Seek to end of "s:" prefix range
		seekKey := []byte("s:\xff")
		count := 0

		for it.Seek(seekKey); it.ValidForPrefix(prefix); it.Next() {
			if limit > 0 && count >= limit {
				break
			}

			key := it.Item().Key()
			// Skip index keys (s:from:, s:tag:, s:reply:, s:meta:)
			if len(key) > 2 && key[2] != ':' {
				// This is a chronological key like s:00000timestamp:id
				var sig EntropicSignal
				err := it.Item().Value(func(val []byte) error {
					return json.Unmarshal(val, &sig)
				})
				if err == nil {
					results = append(results, &sig)
					count++
				}
			}
		}
		return nil
	})

	return results, err
}

// BySender returns signals from a specific sender.
func (s *Store) BySender(fingerprint string, limit int) ([]*EntropicSignal, error) {
	var results []*EntropicSignal
	prefix := []byte(fmt.Sprintf("s:from:%s:", fingerprint))

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Reverse = true
		it := txn.NewIterator(opts)
		defer it.Close()

		seekKey := append(prefix, 0xFF)
		count := 0

		for it.Seek(seekKey); it.ValidForPrefix(prefix); it.Next() {
			if limit > 0 && count >= limit {
				break
			}
			var sig EntropicSignal
			err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &sig)
			})
			if err == nil {
				results = append(results, &sig)
				count++
			}
		}
		return nil
	})

	return results, err
}

// RepliesTo returns signals whose `in_reply_to` field equals signalID.
// Walks the secondary index `s:reply:<signalID>:<id>` then loads each signal
// from the chronological keyspace via a per-id scan.
//
// Limit applies after collection; pass 0 for "no limit". Results are returned
// in oldest-first index order — callers that need most-recent-first should
// reverse the slice.
func (s *Store) RepliesTo(signalID string, limit int) ([]*EntropicSignal, error) {
	if signalID == "" {
		return nil, nil
	}
	prefix := []byte(fmt.Sprintf("s:reply:%s:", signalID))

	// Step 1: collect reply IDs from the index.
	var replyIDs []string
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := string(it.Item().Key())
			id := key[len(prefix):]
			if id != "" {
				replyIDs = append(replyIDs, id)
			}
			if limit > 0 && len(replyIDs) >= limit {
				break
			}
		}
		return nil
	})
	if err != nil || len(replyIDs) == 0 {
		return nil, err
	}

	// Step 2: load each reply by ID. The chronological key embeds the ID
	// suffix so we scan with prefix `s:` and match by suffix — cheaper than
	// keeping a separate by-id index for this rare lookup.
	want := make(map[string]bool, len(replyIDs))
	for _, id := range replyIDs {
		want[id] = true
	}
	var results []*EntropicSignal
	err = s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		scanPrefix := []byte("s:")
		for it.Seek(scanPrefix); it.ValidForPrefix(scanPrefix); it.Next() {
			if len(want) == 0 {
				break
			}
			key := it.Item().Key()
			// Skip secondary index keys (s:from:, s:tag:, s:reply:, s:meta:).
			// Chronological keys have shape s:<20-digit-ms>:<id>, so the
			// fourth byte is a digit. Index keys use letters here.
			if len(key) < 4 || key[2] < '0' || key[2] > '9' {
				continue
			}
			var sig EntropicSignal
			if err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &sig)
			}); err != nil {
				continue
			}
			if want[sig.ID] {
				results = append(results, &sig)
				delete(want, sig.ID)
			}
		}
		return nil
	})
	return results, err
}

// Count returns total stored signals.
func (s *Store) Count() uint64 {
	var count uint64
	s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("s:meta:count"))
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

func (s *Store) incrementCount(txn *badger.Txn) error {
	key := []byte("s:meta:count")
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
