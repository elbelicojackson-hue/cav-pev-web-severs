// Package trust implements the dual-track trust graph (cognitive + social)
// described in cav-social-trust/design.md §2.1.
//
// Two independent kinds of edges:
//   Cognitive : per-domain belief in claim reliability (must have Domain)
//   Social    : global willingness to collaborate (must NOT have Domain)
//
// Edges are directed (A → B does not imply B → A) and immutable except for
// soft-revocation (RevokedAt + RevokeReason). Each edge carries a snapshot of
// the TrustRiskVector that was computed when the edge was added; the full
// vector lives in the risk audit log keyed by `RiskSnapshot.VectorHash`.
package trust

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// TrustKind partitions the graph into two completely separate layers.
type TrustKind string

const (
	Cognitive TrustKind = "cognitive"
	Social    TrustKind = "social"
)

// IsValid returns true if k is a recognized trust kind.
func (k TrustKind) IsValid() bool {
	return k == Cognitive || k == Social
}

// TrustEdge is a single directed trust relationship.
//
// Storage key (BadgerDB):
//   t:edge:<from>:<kind>:<domain>:<to>
// For Social edges, <domain> is the literal string "_" (kept stable for
// iteration) since Social edges have no domain.
type TrustEdge struct {
	From          string    `json:"from"`
	To            string    `json:"to"`
	Kind          TrustKind `json:"kind"`
	Domain        string    `json:"domain,omitempty"`
	Weight        float64   `json:"weight"` // [0, 1]
	EstablishedAt time.Time `json:"established_at"`
	LastDecayAt   time.Time `json:"last_decay_at"`

	// Snapshot of the TrustRiskVector at the moment this edge was added.
	// Full vector lives in r:audit:<vector_hash>.
	RiskSnapshot RiskVectorSnapshot `json:"risk_snapshot"`

	// Soft revocation. Edges with RevokedAt != nil are excluded from default
	// query results unless `includeRevoked` is set.
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	RevokeReason string     `json:"revoke_reason,omitempty"`
}

// RiskVectorSnapshot is the audit-trail bookmark stamped onto every edge at
// creation time.
type RiskVectorSnapshot struct {
	VectorHash     string  `json:"vector_hash"`     // SHA-256 of full TrustRiskVector JCS
	RiskClass      string  `json:"risk_class"`      // low | moderate | elevated | high | critical
	Recommendation string  `json:"recommendation"`  // proceed | proceed_with_caution | defer | reject
	AggregateScore float64 `json:"aggregate_score"`
	AuditRef       string  `json:"audit_ref"`       // key into r:audit:* (typically `r:audit:<hash>`)
}

// IsRevoked returns true if the edge has been soft-deleted.
func (e *TrustEdge) IsRevoked() bool {
	return e.RevokedAt != nil
}

// EdgeKey returns the canonical storage key for this edge.
func (e *TrustEdge) EdgeKey() string {
	return EdgeKey(e.From, e.Kind, e.Domain, e.To)
}

// EdgeKey builds the storage key for the (from, kind, domain, to) tuple.
// For Social edges domain MUST be empty; we substitute "_" so the key shape
// remains uniform for iteration.
func EdgeKey(from string, kind TrustKind, domain, to string) string {
	d := domain
	if d == "" {
		d = "_"
	}
	return fmt.Sprintf("t:edge:%s:%s:%s:%s", from, kind, d, to)
}

// ReverseKey is a secondary index for "who trusts me" queries.
//   t:rev:<to>:<from>:<kind>:<domain>
func ReverseKey(to, from string, kind TrustKind, domain string) string {
	d := domain
	if d == "" {
		d = "_"
	}
	return fmt.Sprintf("t:rev:%s:%s:%s:%s", to, from, kind, d)
}

// Filter narrows a query.
type Filter struct {
	Kind           TrustKind // empty = both kinds
	Domain         string    // applies to Cognitive only; empty = any domain
	IncludeRevoked bool
}

// === Validation ===

// Validate checks an edge against the structural invariants from design §11.
//
//   - From and To must be non-empty and different
//   - Kind must be Cognitive or Social
//   - Cognitive MUST have a Domain
//   - Social MUST NOT have a Domain
//   - Weight in [0, 1]
//
// This does NOT check uniqueness — that's the responsibility of the Store on
// AddTrust (since uniqueness depends on storage state).
func (e *TrustEdge) Validate() error {
	if e.From == "" {
		return errors.New("trust: From is required")
	}
	if e.To == "" {
		return errors.New("trust: To is required")
	}
	if e.From == e.To {
		return errors.New("trust: self-trust is not allowed")
	}
	if !e.Kind.IsValid() {
		return fmt.Errorf("trust: invalid Kind %q", e.Kind)
	}
	if e.Kind == Cognitive && strings.TrimSpace(e.Domain) == "" {
		return errors.New("trust: Cognitive edge requires Domain")
	}
	if e.Kind == Social && e.Domain != "" {
		return errors.New("trust: Social edge must not have Domain")
	}
	if e.Weight < 0 || e.Weight > 1 {
		return fmt.Errorf("trust: Weight must be in [0,1], got %v", e.Weight)
	}
	return nil
}
