// Package knowledge provides a persistent knowledge base for agent-discovered
// information. When agents search the web, scrape docs, or extract insights,
// the results are stored here as structured knowledge entries.
//
// This is the collective memory of the CAV network — every agent's discoveries
// become searchable by all other agents.
//
// Key schema in BadgerDB:
//   k:<knowledge_id>                        → KnowledgeEntry JSON (primary)
//   k:agent:<fingerprint>:<knowledge_id>    → [] (per-agent index)
//   k:tag:<tag>:<knowledge_id>              → [] (tag index)
//   k:source:<domain>:<knowledge_id>        → [] (source domain index)
//   k:type:<type>:<knowledge_id>            → [] (type index)
//   k:time:<unix_ms>:<knowledge_id>         → [] (chronological)
//   k:meta:count                            → uint64
package knowledge

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// EntryType classifies the knowledge.
type EntryType string

const (
	TypeWebSearch   EntryType = "web_search"    // Search result
	TypeScrape      EntryType = "scrape"        // Scraped page content
	TypeInsight     EntryType = "insight"       // Agent-derived insight
	TypeReference   EntryType = "reference"     // Documentation reference
	TypeExploit     EntryType = "exploit"       // Exploit/vuln info
	TypePattern     EntryType = "pattern"       // Code/behavior pattern
)

// KnowledgeEntry is a single piece of stored knowledge.
type KnowledgeEntry struct {
	ID          string    `json:"id"`
	Type        EntryType `json:"type"`
	AgentFrom   string    `json:"agent_from"`    // fingerprint of discoverer
	CreatedAt   string    `json:"created_at"`
	
	// Content
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`       // ≤ 500 chars
	Content     string    `json:"content"`       // full markdown content
	SourceURL   string    `json:"source_url,omitempty"`
	SourceDomain string   `json:"source_domain,omitempty"`

	// Metadata
	Tags        []string  `json:"tags,omitempty"`
	Confidence  float64   `json:"confidence,omitempty"`  // [0,1] how reliable
	PraxonRef   string    `json:"praxon_ref,omitempty"`  // linked Praxon ID
	SignalRef    string    `json:"signal_ref,omitempty"`  // linked Signal ID

	// Usage stats
	CitedBy     int       `json:"cited_by"`      // how many Praxons reference this
	ViewCount   int       `json:"view_count"`
}

// Store is the BadgerDB-backed knowledge base.
type Store struct {
	db *badger.DB
}

// NewStore opens the knowledge database.
func NewStore(dir string) (*Store, error) {
	opts := badger.DefaultOptions(dir).
		WithLogger(nil).
		WithValueLogFileSize(128 << 20).
		WithNumCompactors(2)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("knowledge store open failed: %w", err)
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

// Put stores a knowledge entry with indexes.
func (s *Store) Put(entry *KnowledgeEntry) error {
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("kn_%d_%s", time.Now().UnixMilli(), randomSuffix())
	}
	if entry.CreatedAt == "" {
		entry.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if entry.SourceURL != "" && entry.SourceDomain == "" {
		if u, err := url.Parse(entry.SourceURL); err == nil {
			entry.SourceDomain = u.Hostname()
		}
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	nowMs := time.Now().UnixMilli()

	return s.db.Update(func(txn *badger.Txn) error {
		// Primary
		if err := txn.Set([]byte("k:"+entry.ID), data); err != nil {
			return err
		}

		empty := []byte{}

		// Per-agent index
		if entry.AgentFrom != "" {
			txn.Set([]byte(fmt.Sprintf("k:agent:%s:%s", entry.AgentFrom, entry.ID)), empty)
		}

		// Tag index
		for _, tag := range entry.Tags {
			txn.Set([]byte(fmt.Sprintf("k:tag:%s:%s", strings.ToLower(tag), entry.ID)), empty)
		}

		// Source domain index
		if entry.SourceDomain != "" {
			txn.Set([]byte(fmt.Sprintf("k:source:%s:%s", entry.SourceDomain, entry.ID)), empty)
		}

		// Type index
		txn.Set([]byte(fmt.Sprintf("k:type:%s:%s", entry.Type, entry.ID)), empty)

		// Chronological index
		txn.Set([]byte(fmt.Sprintf("k:time:%020d:%s", nowMs, entry.ID)), empty)

		// Increment count
		return s.incrementCount(txn)
	})
}

// Get retrieves a knowledge entry by ID.
func (s *Store) Get(id string) (*KnowledgeEntry, error) {
	var entry KnowledgeEntry
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("k:" + id))
		if err == badger.ErrKeyNotFound {
			return fmt.Errorf("knowledge %s not found", id)
		}
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &entry)
		})
	})
	return &entry, err
}

// Search queries the knowledge base.
func (s *Store) Search(q KnowledgeQuery) ([]KnowledgeEntry, error) {
	if q.Limit <= 0 {
		q.Limit = 20
	}

	var prefix []byte
	switch {
	case q.Agent != "":
		prefix = []byte(fmt.Sprintf("k:agent:%s:", q.Agent))
	case q.Tag != "":
		prefix = []byte(fmt.Sprintf("k:tag:%s:", strings.ToLower(q.Tag)))
	case q.Domain != "":
		prefix = []byte(fmt.Sprintf("k:source:%s:", q.Domain))
	case q.Type != "":
		prefix = []byte(fmt.Sprintf("k:type:%s:", q.Type))
	default:
		prefix = []byte("k:time:")
	}

	var ids []string
	s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		opts.Reverse = true
		it := txn.NewIterator(opts)
		defer it.Close()

		seekKey := append(prefix, 0xFF)
		for it.Seek(seekKey); it.ValidForPrefix(prefix); it.Next() {
			key := string(it.Item().Key())
			parts := strings.Split(key, ":")
			id := parts[len(parts)-1]
			if strings.HasPrefix(id, "kn_") {
				ids = append(ids, id)
			}
			if len(ids) >= q.Limit {
				break
			}
		}
		return nil
	})

	// Fetch full entries
	var results []KnowledgeEntry
	for _, id := range ids {
		entry, err := s.Get(id)
		if err == nil {
			// Keyword filter
			if q.Keyword != "" {
				lower := strings.ToLower(q.Keyword)
				if !strings.Contains(strings.ToLower(entry.Title), lower) &&
					!strings.Contains(strings.ToLower(entry.Summary), lower) {
					continue
				}
			}
			results = append(results, *entry)
		}
	}

	return results, nil
}

// Recent returns the latest N entries.
func (s *Store) Recent(limit int) ([]KnowledgeEntry, error) {
	return s.Search(KnowledgeQuery{Limit: limit})
}

// Count returns total entries.
func (s *Store) Count() uint64 {
	var count uint64
	s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("k:meta:count"))
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
	key := []byte("k:meta:count")
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

func randomSuffix() string {
	t := time.Now().UnixNano()
	const hex = "0123456789abcdef"
	result := make([]byte, 8)
	for i := range result {
		result[i] = hex[(t>>(i*4))&0x0f]
	}
	return string(result)
}

// KnowledgeQuery defines search parameters.
type KnowledgeQuery struct {
	Keyword string `json:"keyword,omitempty"`
	Agent   string `json:"agent,omitempty"`   // filter by agent fingerprint
	Tag     string `json:"tag,omitempty"`
	Domain  string `json:"domain,omitempty"`  // source domain
	Type    string `json:"type,omitempty"`    // EntryType
	Limit   int    `json:"limit,omitempty"`
}
