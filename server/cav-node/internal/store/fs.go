// Package store provides Praxon storage backends.
package store

import (
	"fmt"
	"os"
	"path/filepath"
)

// Store is the interface for Praxon persistence.
type Store interface {
	Put(praxonID string, content []byte) error
	Get(praxonID string) ([]byte, error)
	Has(praxonID string) bool
}

// FSStore is a filesystem-backed Praxon store.
// Each Praxon is stored as <dir>/<praxon_id>.json.
type FSStore struct {
	dir string
}

// NewFSStore creates a new filesystem store at the given directory.
func NewFSStore(dir string) *FSStore {
	return &FSStore{dir: dir}
}

func (s *FSStore) path(id string) string {
	return filepath.Join(s.dir, id+".json")
}

// Put stores a Praxon. Idempotent: re-putting the same ID is a no-op.
func (s *FSStore) Put(praxonID string, content []byte) error {
	p := s.path(praxonID)
	if _, err := os.Stat(p); err == nil {
		// Already exists — idempotent no-op
		return nil
	}
	return os.WriteFile(p, content, 0o644)
}

// Get retrieves a Praxon by ID. Returns os.ErrNotExist if not found.
func (s *FSStore) Get(praxonID string) ([]byte, error) {
	data, err := os.ReadFile(s.path(praxonID))
	if err != nil {
		return nil, fmt.Errorf("praxon %s not found: %w", praxonID, err)
	}
	return data, nil
}

// Has checks if a Praxon exists in the store.
func (s *FSStore) Has(praxonID string) bool {
	_, err := os.Stat(s.path(praxonID))
	return err == nil
}
