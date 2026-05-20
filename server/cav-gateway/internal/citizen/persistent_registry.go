// Persistent citizen registry backed by BadgerDB.
// All citizen data survives gateway restarts.
//
// Key schema:
//   c:did:<did>              → Citizen JSON (primary)
//   c:fp:<fingerprint>       → did (fingerprint → DID lookup)
//   c:level:<level>:<did>    → [] (index by level)
//   c:meta:count             → uint64
package citizen

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// PersistentRegistry stores citizens in BadgerDB.
type PersistentRegistry struct {
	db    *badger.DB
	cache map[string]*Citizen // in-memory cache for fast reads
	mu    sync.RWMutex
}

// NewPersistentRegistry opens a BadgerDB-backed citizen registry.
func NewPersistentRegistry(dir string) (*PersistentRegistry, error) {
	opts := badger.DefaultOptions(dir).
		WithLogger(nil).
		WithValueLogFileSize(16 << 20)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("citizen registry open failed: %w", err)
	}

	reg := &PersistentRegistry{
		db:    db,
		cache: make(map[string]*Citizen),
	}

	// Load all citizens into cache on startup
	if err := reg.loadAll(); err != nil {
		db.Close()
		return nil, err
	}

	// Background GC
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			db.RunValueLogGC(0.5)
		}
	}()

	return reg, nil
}

// Close shuts down the registry.
func (reg *PersistentRegistry) Close() error {
	return reg.db.Close()
}

// loadAll reads all citizens from disk into the cache.
//
// Migration: legacy citizen JSON predates the State field. After unmarshal
// State == "" — we leave it empty in storage but treat it as StateActive
// for query/filter purposes (see ByState and EffectiveState). This keeps
// backward compatibility without forcing a rewrite of all rows.
func (reg *PersistentRegistry) loadAll() error {
	return reg.db.View(func(txn *badger.Txn) error {
		prefix := []byte("c:did:")
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var c Citizen
			err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &c)
			})
			if err == nil {
				reg.cache[c.DID] = &c
			}
		}
		return nil
	})
}

// EnsureRegistered registers a citizen or updates last_seen. Returns level.
func (reg *PersistentRegistry) EnsureRegistered(did string, fingerprint string) int {
	reg.mu.Lock()
	defer reg.mu.Unlock()

	if c, ok := reg.cache[did]; ok {
		c.LastSeenAt = time.Now()
		// Persist update
		reg.persist(c)
		return c.Level
	}

	// New citizen
	c := &Citizen{
		DID:          did,
		Fingerprint:  fingerprint,
		Level:        1,
		RegisteredAt: time.Now(),
		LastSeenAt:   time.Now(),
	}
	reg.cache[did] = c
	reg.persist(c)
	reg.persistIndex(c)

	return 1
}

// Get returns citizen info.
func (reg *PersistentRegistry) Get(did string) interface{} {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	if c, ok := reg.cache[did]; ok {
		return c
	}
	return map[string]string{"did": did, "level": "0"}
}

// SetCapabilities updates a citizen's capabilities.
func (reg *PersistentRegistry) SetCapabilities(did string, caps *Capabilities) {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	if c, ok := reg.cache[did]; ok {
		c.Capabilities = caps
		reg.persist(c)
	}
}

// SetState updates a citizen's probation lifecycle state and persists it.
// nextRetryAt is only persisted when state == StateRestricted; otherwise cleared.
// Returns false if DID is unknown.
func (reg *PersistentRegistry) SetState(did string, state ProbationState, nextRetryAt *time.Time) bool {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	c, ok := reg.cache[did]
	if !ok {
		return false
	}
	c.State = state
	if state == StateRestricted {
		c.NextRetryAt = nextRetryAt
	} else {
		c.NextRetryAt = nil
	}
	reg.persist(c)
	return true
}

// SetPubKey stores the citizen's Ed25519 public key (used to verify
// behavioral digests in the social trust layer). Returns false if DID is unknown.
func (reg *PersistentRegistry) SetPubKey(did, pubkey string) bool {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	c, ok := reg.cache[did]
	if !ok {
		return false
	}
	c.PubKey = pubkey
	reg.persist(c)
	return true
}

// ByState returns all citizens currently in the given state.
// Empty State on a stored citizen is treated as StateActive (legacy migration).
func (reg *PersistentRegistry) ByState(state ProbationState) []*Citizen {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	result := make([]*Citizen, 0)
	for _, c := range reg.cache {
		s := c.State
		if s == "" {
			s = StateActive
		}
		if s == state {
			result = append(result, c)
		}
	}
	return result
}

// EffectiveState returns the citizen's lifecycle state with legacy migration
// applied (empty → active). Returns "" if DID is unknown.
func (reg *PersistentRegistry) EffectiveState(did string) ProbationState {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	c, ok := reg.cache[did]
	if !ok {
		return ""
	}
	if c.State == "" {
		return StateActive
	}
	return c.State
}

// GetLevel returns the citizen's current level. Returns 0 if DID is unknown.
// Satisfies auth.CitizenLevelProvider for the level-gate middleware.
func (reg *PersistentRegistry) GetLevel(did string) int {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	if c, ok := reg.cache[did]; ok {
		return c.Level
	}
	return 0
}

// All returns all registered citizens.
func (reg *PersistentRegistry) All() []*Citizen {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	result := make([]*Citizen, 0, len(reg.cache))
	for _, c := range reg.cache {
		result = append(result, c)
	}
	return result
}

// Stats returns network statistics.
func (reg *PersistentRegistry) Stats() NetworkStats {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	var s NetworkStats
	s.Total = len(reg.cache)
	for _, c := range reg.cache {
		switch c.Level {
		case 3:
			s.Level3++
		case 2:
			s.Level2++
		case 1:
			s.Level1++
		}
	}
	return s
}

// persist writes a citizen to BadgerDB.
func (reg *PersistentRegistry) persist(c *Citizen) {
	data, err := json.Marshal(c)
	if err != nil {
		return
	}
	reg.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte("c:did:"+c.DID), data)
	})
}

// persistIndex writes secondary indexes for a new citizen.
func (reg *PersistentRegistry) persistIndex(c *Citizen) {
	reg.db.Update(func(txn *badger.Txn) error {
		// Fingerprint → DID lookup
		if c.Fingerprint != "" {
			txn.Set([]byte("c:fp:"+c.Fingerprint), []byte(c.DID))
		}
		// Level index
		txn.Set([]byte(fmt.Sprintf("c:level:%d:%s", c.Level, c.DID)), []byte{})
		// Increment count
		return reg.incrementCount(txn)
	})
}

func (reg *PersistentRegistry) incrementCount(txn *badger.Txn) error {
	key := []byte("c:meta:count")
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
