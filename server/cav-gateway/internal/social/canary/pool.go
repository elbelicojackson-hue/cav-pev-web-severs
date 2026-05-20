// Canary task pool — persistent storage for available tasks plus secondary
// indexes for "give me a random task in domain X".
//
// Key schema (design.md §3):
//   c:task:<task_id>             → CanaryTask JSON (with private groundTruth)
//   c:by_domain:<domain>:<task_id> → []
//   c:meta:pool_size:<domain>    → uint32
//
// Internal storage marshals the full task INCLUDING ground truth (we own the
// disk). When tasks leave the package via API, callers must use Sanitize().

package canary

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// ErrNotFound indicates the requested task ID does not exist in the pool.
var ErrNotFound = errors.New("canary: task not found")

// poolEnvelope is the wire format we use ON DISK to round-trip ground truth.
// CanaryTask's JSON tag for groundTruth is "-" (correctly, for client safety),
// so we serialize via this private envelope instead of relying on the JSON
// representation of CanaryTask itself.
type poolEnvelope struct {
	Task        CanaryTask  `json:"task"`
	GroundTruth GroundTruth `json:"ground_truth"`
}

// Pool is the BadgerDB-backed canary task store.
type Pool struct {
	db  *badger.DB
	mu  sync.RWMutex
	rng *rand.Rand // not crypto — diversity in random selection is enough
}

// NewPool opens (or creates) the canary pool at `dir`.
func NewPool(dir string) (*Pool, error) {
	opts := badger.DefaultOptions(dir).
		WithLogger(nil).
		WithValueLogFileSize(16 << 20)
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("canary pool open failed: %w", err)
	}
	return &Pool{
		db:  db,
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

// Close shuts down the pool.
func (p *Pool) Close() error {
	return p.db.Close()
}

// Upsert stores or replaces a task and its ground truth.
// CreatedAt is filled to time.Now() if zero.
func (p *Pool) Upsert(t *CanaryTask, gt GroundTruth) error {
	if t.ID == "" {
		return errors.New("canary: task ID required")
	}
	if t.Domain == "" {
		return errors.New("canary: task domain required")
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}

	env := poolEnvelope{Task: *t, GroundTruth: gt}
	// Strip groundTruth from the embedded Task's runtime field — the wire
	// format keeps them separate, so we don't double-store.
	env.Task.groundTruth = GroundTruth{}

	data, err := json.Marshal(env)
	if err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	return p.db.Update(func(txn *badger.Txn) error {
		exists := false
		if _, err := txn.Get([]byte("c:task:" + t.ID)); err == nil {
			exists = true
		}
		if err := txn.Set([]byte("c:task:"+t.ID), data); err != nil {
			return err
		}
		if err := txn.Set([]byte(fmt.Sprintf("c:by_domain:%s:%s", t.Domain, t.ID)), []byte{}); err != nil {
			return err
		}
		if !exists {
			if err := bumpDomainCount(txn, t.Domain, +1); err != nil {
				return err
			}
		}
		return nil
	})
}

// Get returns the task (with groundTruth populated for in-process use).
// Callers serializing to clients must use Sanitize().
func (p *Pool) Get(taskID string) (*CanaryTask, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var env poolEnvelope
	err := p.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("c:task:" + taskID))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return ErrNotFound
			}
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &env)
		})
	})
	if err != nil {
		return nil, err
	}
	t := env.Task
	t.groundTruth = env.GroundTruth
	return &t, nil
}

// RandomByDomain returns a task selected uniformly at random from the given
// domain, excluding any IDs in `exclude`. Returns ErrNotFound if no eligible
// task exists.
func (p *Pool) RandomByDomain(domain string, exclude map[string]bool) (*CanaryTask, error) {
	p.mu.RLock()
	prefix := []byte(fmt.Sprintf("c:by_domain:%s:", domain))
	var candidates []string
	err := p.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			k := it.Item().Key()
			id := string(k[len(prefix):])
			if exclude[id] {
				continue
			}
			candidates = append(candidates, id)
		}
		return nil
	})
	p.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, ErrNotFound
	}
	idx := p.rng.Intn(len(candidates))
	return p.Get(candidates[idx])
}

// PickN returns up to `n` distinct tasks, attempting to spread them across
// the given `domains`. If fewer than n are available, returns what we have.
func (p *Pool) PickN(domains []string, n int) ([]*CanaryTask, error) {
	if n <= 0 {
		return nil, nil
	}
	exclude := map[string]bool{}
	out := make([]*CanaryTask, 0, n)
	for len(out) < n {
		picked := false
		for _, dom := range domains {
			if len(out) >= n {
				break
			}
			t, err := p.RandomByDomain(dom, exclude)
			if err == ErrNotFound {
				continue
			}
			if err != nil {
				return out, err
			}
			out = append(out, t)
			exclude[t.ID] = true
			picked = true
		}
		if !picked {
			break
		}
	}
	return out, nil
}

// Size returns the count of tasks in `domain`.
func (p *Pool) Size(domain string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var n uint32
	p.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("c:meta:pool_size:" + domain))
		if err != nil {
			return nil
		}
		return item.Value(func(val []byte) error {
			if len(val) == 4 {
				n = binary.BigEndian.Uint32(val)
			}
			return nil
		})
	})
	return int(n)
}

// AllByDomain returns task IDs in the given domain (used by tests + admin).
func (p *Pool) AllByDomain(domain string) []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	prefix := []byte(fmt.Sprintf("c:by_domain:%s:", domain))
	var ids []string
	p.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			k := it.Item().Key()
			ids = append(ids, string(k[len(prefix):]))
		}
		return nil
	})
	return ids
}

func bumpDomainCount(txn *badger.Txn, domain string, delta int) error {
	key := []byte("c:meta:pool_size:" + domain)
	var n uint32
	if item, err := txn.Get(key); err == nil {
		item.Value(func(val []byte) error {
			if len(val) == 4 {
				n = binary.BigEndian.Uint32(val)
			}
			return nil
		})
	}
	switch {
	case delta > 0:
		n += uint32(delta)
	case delta < 0:
		d := uint32(-delta)
		if d > n {
			n = 0
		} else {
			n -= d
		}
	}
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, n)
	return txn.Set(key, buf)
}
