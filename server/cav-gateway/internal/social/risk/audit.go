// Risk audit log.
//
// Each computed TrustRiskVector is persisted to BadgerDB under
//   r:audit:<vector_hash>
// where vector_hash is SHA-256 over a canonical JSON encoding of the vector.
// A secondary index
//   r:by_pair:<requester>:<subject>:<ts_ms>
// makes "show me the most recent risk evals for this pair" cheap.
//
// Audit records are immutable once written (invariant I4 from design §11).
// The store enforces this at write time: re-writing the same hash is a no-op
// (the existing record wins) so re-computation never corrupts history.

package risk

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// AuditStore persists TrustRiskVector records by content hash.
type AuditStore struct {
	db *badger.DB
	mu sync.Mutex // serializes writes; reads are lockless via Badger's MVCC
}

// NewAuditStore opens (or creates) the audit log at `dir`.
func NewAuditStore(dir string) (*AuditStore, error) {
	opts := badger.DefaultOptions(dir).
		WithLogger(nil).
		WithValueLogFileSize(16 << 20)
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("audit store open failed: %w", err)
	}
	return &AuditStore{db: db}, nil
}

// Close shuts down the store.
func (a *AuditStore) Close() error {
	return a.db.Close()
}

// Persist writes a vector to the audit log and stamps it with the audit_ref
// (the storage key) plus its content hash. Returns the populated hash.
//
// The vector_hash field on the input is overwritten with the canonical hash;
// any caller-supplied value is ignored (we treat it as authoritative output).
//
// Invariant: a given vector_hash maps to exactly one stored vector. If the
// same hash is written twice, the second write is a no-op (the bytes are
// already on disk; this is what guarantees immutability).
func (a *AuditStore) Persist(v *TrustRiskVector) (string, error) {
	if v == nil {
		return "", errors.New("audit: nil vector")
	}
	hash, canonical, err := canonicalHash(v)
	if err != nil {
		return "", err
	}
	auditKey := "r:audit:" + hash

	a.mu.Lock()
	defer a.mu.Unlock()

	err = a.db.Update(func(txn *badger.Txn) error {
		// If the hash already exists, leave the existing record untouched.
		if _, err := txn.Get([]byte(auditKey)); err == nil {
			return nil
		}
		if err := txn.Set([]byte(auditKey), canonical); err != nil {
			return err
		}
		// Secondary index for "by pair, in chronological order".
		idxKey := fmt.Sprintf("r:by_pair:%s:%s:%020d:%s",
			v.Requester, v.Subject, v.ComputedAt.UnixMilli(), hash)
		return txn.Set([]byte(idxKey), []byte{})
	})
	if err != nil {
		return "", err
	}
	return hash, nil
}

// Get returns the stored vector for `hash`, or nil if absent.
func (a *AuditStore) Get(hash string) (*TrustRiskVector, error) {
	var out *TrustRiskVector
	err := a.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("r:audit:" + hash))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil
			}
			return err
		}
		return item.Value(func(val []byte) error {
			var v TrustRiskVector
			if err := json.Unmarshal(val, &v); err != nil {
				return err
			}
			out = &v
			return nil
		})
	})
	return out, err
}

// HashOnly computes the canonical content hash of a vector without persisting.
// Useful for tests and for stamping snapshots before write.
func HashOnly(v *TrustRiskVector) (string, error) {
	hash, _, err := canonicalHash(v)
	return hash, err
}

// canonicalHash returns (hex SHA-256, canonical JSON bytes) of the vector.
// We perform two passes:
//   1. Marshal with encoding/json (which already produces deterministic field
//      order since Go 1.12+ does map keys alphabetically — but to be defensive
//      across struct field orders, we re-encode through a sorted map).
//   2. SHA-256 over the resulting bytes.
//
// VectorHash is blanked before hashing — it's an output, not an input.
func canonicalHash(v *TrustRiskVector) (string, []byte, error) {
	cp := *v
	cp.VectorHash = ""
	intermediate, err := json.Marshal(&cp)
	if err != nil {
		return "", nil, err
	}
	canonical, err := canonicalize(intermediate)
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), canonical, nil
}

// canonicalize re-encodes a JSON byte stream with object keys sorted
// alphabetically at every level. Numbers are passed through as their
// json.Number representation to avoid float formatting drift.
func canonicalize(in []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(in))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	v = sortKeys(v)
	return json.Marshal(v)
}

// sortKeys recursively sorts every map[string]interface{}'s keys.
func sortKeys(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		// Use ordered slice of pairs serialized via json.Encoder marshalling
		// is awkward; easier to build a fresh map and rely on encoding/json
		// emitting alphabetically (which it does for map[string]interface{}).
		out := make(map[string]interface{}, len(t))
		for _, k := range keys {
			out[k] = sortKeys(t[k])
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, e := range t {
			out[i] = sortKeys(e)
		}
		return out
	default:
		return v
	}
}

// Stamp computes the canonical hash of v, sets v's SchemaVersion if empty,
// and returns the resulting RiskVectorSnapshot ready to attach to a
// TrustEdge. It does NOT persist — call Persist for that.
//
// Stamp can be called multiple times (e.g. once at handler-time to inspect
// the hash, once during persist) — the hash is determined entirely by the
// non-VectorHash fields, so repeats yield the same value.
func Stamp(v *TrustRiskVector) (RiskVectorSnapshotForEdge, error) {
	if v.SchemaVersion == "" {
		v.SchemaVersion = SchemaVersion
	}
	if v.ComputedAt.IsZero() {
		v.ComputedAt = time.Now()
	}
	hash, err := HashOnly(v)
	if err != nil {
		return RiskVectorSnapshotForEdge{}, err
	}
	return RiskVectorSnapshotForEdge{
		VectorHash:     hash,
		RiskClass:      v.RiskClass,
		Recommendation: v.Recommendation,
		AggregateScore: v.AggregateScore,
		AuditRef:       "r:audit:" + hash,
	}, nil
}

// RiskVectorSnapshotForEdge mirrors trust.RiskVectorSnapshot. Defined here
// so the risk package doesn't import trust; the caller (engine wiring or
// handler) does the cross-package conversion.
type RiskVectorSnapshotForEdge struct {
	VectorHash     string
	RiskClass      string
	Recommendation string
	AggregateScore float64
	AuditRef       string
}
