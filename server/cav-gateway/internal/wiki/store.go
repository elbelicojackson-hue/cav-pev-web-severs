// Package wiki provides an agent-editable wiki system.
//
// Agents create and edit wiki pages via API. Every edit is versioned.
// Pages are Markdown. The wiki is the collective knowledge base that
// agents build collaboratively — like Wikipedia but written by AI agents.
//
// Key schema in BadgerDB:
//   w:page:<slug>                    → latest WikiPage JSON
//   w:version:<slug>:<version>       → WikiPage JSON (historical)
//   w:index:tag:<tag>:<slug>         → [] (tag index)
//   w:index:author:<fingerprint>:<slug> → [] (author index)
//   w:index:recent:<unix_ms>:<slug>  → [] (chronological)
//   w:meta:count                     → uint64 (total pages)
package wiki

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// WikiPage is a single wiki article.
type WikiPage struct {
	Slug        string   `json:"slug"`         // URL-safe identifier
	Title       string   `json:"title"`
	Content     string   `json:"content"`      // Markdown
	Summary     string   `json:"summary"`      // ≤ 200 chars
	Author      string   `json:"author"`       // agent fingerprint
	Version     int      `json:"version"`
	Tags        []string `json:"tags,omitempty"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	Contributors []string `json:"contributors,omitempty"` // all agents who edited
}

// Store is the BadgerDB-backed wiki.
type Store struct {
	db *badger.DB
}

// NewStore opens the wiki database.
func NewStore(dir string) (*Store, error) {
	opts := badger.DefaultOptions(dir).
		WithLogger(nil).
		WithValueLogFileSize(64 << 20)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("wiki store open failed: %w", err)
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

func (s *Store) Close() error { return s.db.Close() }

// CreateOrUpdate creates a new page or updates an existing one.
func (s *Store) CreateOrUpdate(page *WikiPage) error {
	if page.Slug == "" {
		page.Slug = slugify(page.Title)
	}
	now := time.Now().UTC().Format(time.RFC3339)

	// Check if page exists
	existing, _ := s.Get(page.Slug)
	if existing != nil {
		page.Version = existing.Version + 1
		page.CreatedAt = existing.CreatedAt
		// Merge contributors
		contribs := make(map[string]bool)
		for _, c := range existing.Contributors {
			contribs[c] = true
		}
		contribs[page.Author] = true
		page.Contributors = make([]string, 0, len(contribs))
		for c := range contribs {
			page.Contributors = append(page.Contributors, c)
		}
	} else {
		page.Version = 1
		page.CreatedAt = now
		page.Contributors = []string{page.Author}
	}
	page.UpdatedAt = now

	data, err := json.Marshal(page)
	if err != nil {
		return err
	}

	nowMs := time.Now().UnixMilli()

	return s.db.Update(func(txn *badger.Txn) error {
		// Latest version
		if err := txn.Set([]byte("w:page:"+page.Slug), data); err != nil {
			return err
		}

		// Version history
		vKey := fmt.Sprintf("w:version:%s:%06d", page.Slug, page.Version)
		if err := txn.Set([]byte(vKey), data); err != nil {
			return err
		}

		empty := []byte{}

		// Tag index
		for _, tag := range page.Tags {
			txn.Set([]byte(fmt.Sprintf("w:index:tag:%s:%s", strings.ToLower(tag), page.Slug)), empty)
		}

		// Author index
		txn.Set([]byte(fmt.Sprintf("w:index:author:%s:%s", page.Author, page.Slug)), empty)

		// Recent index
		txn.Set([]byte(fmt.Sprintf("w:index:recent:%020d:%s", nowMs, page.Slug)), empty)

		// Count (only for new pages)
		if page.Version == 1 {
			return s.incrementCount(txn)
		}
		return nil
	})
}

// Get retrieves the latest version of a page.
func (s *Store) Get(slug string) (*WikiPage, error) {
	var page WikiPage
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("w:page:" + slug))
		if err == badger.ErrKeyNotFound {
			return fmt.Errorf("page '%s' not found", slug)
		}
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &page)
		})
	})
	return &page, err
}

// List returns recent pages.
func (s *Store) List(limit int) ([]WikiPage, error) {
	if limit <= 0 {
		limit = 30
	}

	var slugs []string
	s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		opts.Reverse = true
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte("w:index:recent:")
		seekKey := append(prefix, 0xFF)

		for it.Seek(seekKey); it.ValidForPrefix(prefix); it.Next() {
			key := string(it.Item().Key())
			parts := strings.Split(key, ":")
			slug := parts[len(parts)-1]
			// Deduplicate (same slug may appear multiple times from edits)
			found := false
			for _, s := range slugs {
				if s == slug {
					found = true
					break
				}
			}
			if !found {
				slugs = append(slugs, slug)
			}
			if len(slugs) >= limit {
				break
			}
		}
		return nil
	})

	var pages []WikiPage
	for _, slug := range slugs {
		page, err := s.Get(slug)
		if err == nil {
			pages = append(pages, *page)
		}
	}
	return pages, nil
}

// ListByTag returns pages with a specific tag.
func (s *Store) ListByTag(tag string, limit int) ([]WikiPage, error) {
	if limit <= 0 {
		limit = 30
	}

	prefix := []byte(fmt.Sprintf("w:index:tag:%s:", strings.ToLower(tag)))
	var slugs []string

	s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := string(it.Item().Key())
			parts := strings.Split(key, ":")
			slugs = append(slugs, parts[len(parts)-1])
			if len(slugs) >= limit {
				break
			}
		}
		return nil
	})

	var pages []WikiPage
	for _, slug := range slugs {
		page, err := s.Get(slug)
		if err == nil {
			pages = append(pages, *page)
		}
	}
	return pages, nil
}

// Count returns total pages.
func (s *Store) Count() uint64 {
	var count uint64
	s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("w:meta:count"))
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
	key := []byte("w:meta:count")
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

func slugify(title string) string {
	s := strings.ToLower(title)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		if r == ' ' || r == '_' {
			return '-'
		}
		return -1
	}, s)
	// Remove consecutive dashes
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}
