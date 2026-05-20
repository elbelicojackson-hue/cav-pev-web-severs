package trust

import (
	"math"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "trust")
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func validCognitive(from, to string) *TrustEdge {
	return &TrustEdge{
		From: from, To: to, Kind: Cognitive,
		Domain: "crypto", Weight: 0.7,
		RiskSnapshot: RiskVectorSnapshot{
			VectorHash: "h_" + from + "_" + to, RiskClass: "low",
			Recommendation: "proceed", AggregateScore: 0.15,
		},
	}
}

func validSocial(from, to string) *TrustEdge {
	return &TrustEdge{
		From: from, To: to, Kind: Social, Weight: 0.6,
		RiskSnapshot: RiskVectorSnapshot{
			VectorHash: "h_" + from + "_" + to + "_s", RiskClass: "low",
			Recommendation: "proceed", AggregateScore: 0.10,
		},
	}
}

// === Validation ===

func TestEdgeValidation(t *testing.T) {
	cases := []struct {
		name string
		e    *TrustEdge
	}{
		{"missing From", &TrustEdge{To: "b", Kind: Cognitive, Domain: "crypto"}},
		{"missing To", &TrustEdge{From: "a", Kind: Cognitive, Domain: "crypto"}},
		{"self trust", &TrustEdge{From: "a", To: "a", Kind: Cognitive, Domain: "crypto"}},
		{"bad kind", &TrustEdge{From: "a", To: "b", Kind: "weird"}},
		{"cognitive without domain", &TrustEdge{From: "a", To: "b", Kind: Cognitive}},
		{"cognitive with whitespace domain", &TrustEdge{From: "a", To: "b", Kind: Cognitive, Domain: "  "}},
		{"social with domain", &TrustEdge{From: "a", To: "b", Kind: Social, Domain: "crypto"}},
		{"weight too high", &TrustEdge{From: "a", To: "b", Kind: Social, Weight: 1.5}},
		{"weight negative", &TrustEdge{From: "a", To: "b", Kind: Social, Weight: -0.1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.e.Validate(); err == nil {
				t.Errorf("expected validation error for %s", tc.name)
			}
		})
	}
}

func TestEdgeValidationOK(t *testing.T) {
	for _, e := range []*TrustEdge{validCognitive("a", "b"), validSocial("a", "b")} {
		if err := e.Validate(); err != nil {
			t.Errorf("expected valid, got %v: %+v", err, e)
		}
	}
}

// === AddTrust + uniqueness ===

func TestAddTrustHappyPath(t *testing.T) {
	s := newTestStore(t)
	got, err := s.AddTrust(validCognitive("a", "b"))
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if got.EstablishedAt.IsZero() || got.LastDecayAt.IsZero() {
		t.Errorf("timestamps should be auto-stamped: %+v", got)
	}
	if got.IsRevoked() {
		t.Errorf("new edge must not be revoked")
	}
}

func TestAddDuplicateRejected(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.AddTrust(validCognitive("a", "b")); err != nil {
		t.Fatalf("first add: %v", err)
	}
	_, err := s.AddTrust(validCognitive("a", "b"))
	if err != ErrAlreadyExists {
		t.Errorf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestAddTrustDistinctTuplesAllowed(t *testing.T) {
	s := newTestStore(t)
	// (a,b,Cognitive,crypto) and (a,b,Cognitive,ml) are different
	if _, err := s.AddTrust(validCognitive("a", "b")); err != nil {
		t.Fatal(err)
	}
	e2 := validCognitive("a", "b")
	e2.Domain = "ml"
	if _, err := s.AddTrust(e2); err != nil {
		t.Fatalf("different domain should be allowed: %v", err)
	}
	// Cognitive vs Social against same pair are different
	if _, err := s.AddTrust(validSocial("a", "b")); err != nil {
		t.Fatalf("social separate from cognitive: %v", err)
	}
	// Reverse direction is different
	if _, err := s.AddTrust(validCognitive("b", "a")); err != nil {
		t.Fatalf("reverse direction should be allowed: %v", err)
	}

	if got := s.EdgeCount("a"); got != 3 {
		t.Errorf("expected 3 outgoing edges from a, got %d", got)
	}
}

func TestAddTrustReplacesRevoked(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.AddTrust(validCognitive("a", "b")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RevokeTrust("a", Cognitive, "crypto", "b", "test"); err != nil {
		t.Fatal(err)
	}
	// After revocation, re-adding the same tuple must succeed
	got, err := s.AddTrust(validCognitive("a", "b"))
	if err != nil {
		t.Fatalf("re-add after revoke: %v", err)
	}
	if got.IsRevoked() {
		t.Error("new edge should not carry over revoked state")
	}
}

// === Revoke ===

func TestRevokeMissingEdge(t *testing.T) {
	s := newTestStore(t)
	_, err := s.RevokeTrust("nope", Cognitive, "crypto", "nope2", "x")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRevokeIdempotent(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.AddTrust(validCognitive("a", "b")); err != nil {
		t.Fatal(err)
	}
	first, _ := s.RevokeTrust("a", Cognitive, "crypto", "b", "first reason")
	second, _ := s.RevokeTrust("a", Cognitive, "crypto", "b", "second reason")

	if first.RevokedAt == nil || second.RevokedAt == nil {
		t.Fatal("RevokedAt should be set")
	}
	if !first.RevokedAt.Equal(*second.RevokedAt) {
		t.Errorf("RevokedAt must not change on idempotent revoke; got %v vs %v",
			first.RevokedAt, second.RevokedAt)
	}
	if second.RevokeReason != "second reason" {
		t.Errorf("reason should be updatable, got %q", second.RevokeReason)
	}
}

// === Queries ===

func TestEdgesFiltersAndExcludesRevokedByDefault(t *testing.T) {
	s := newTestStore(t)
	s.AddTrust(validCognitive("a", "b"))
	s.AddTrust(validCognitive("a", "c"))
	s.AddTrust(validSocial("a", "d"))
	s.RevokeTrust("a", Cognitive, "crypto", "b", "")

	// Default filter excludes revoked
	def := s.Edges("a", Filter{})
	if len(def) != 2 {
		t.Errorf("default query should exclude revoked, got %d edges", len(def))
	}

	// Include revoked
	all := s.Edges("a", Filter{IncludeRevoked: true})
	if len(all) != 3 {
		t.Errorf("IncludeRevoked should return all, got %d", len(all))
	}

	// Filter by kind
	cog := s.Edges("a", Filter{Kind: Cognitive})
	if len(cog) != 1 || cog[0].To != "c" {
		t.Errorf("Cognitive-only should return 1 edge to c, got %+v", cog)
	}
	soc := s.Edges("a", Filter{Kind: Social})
	if len(soc) != 1 || soc[0].To != "d" {
		t.Errorf("Social-only should return 1 edge to d, got %+v", soc)
	}

	// Filter by domain
	dm := s.Edges("a", Filter{Domain: "crypto"})
	// only c's edge remains under crypto (b revoked)
	if len(dm) != 1 || dm[0].To != "c" {
		t.Errorf("domain crypto expected 1 edge to c, got %+v", dm)
	}
}

func TestReverseQuery(t *testing.T) {
	s := newTestStore(t)
	s.AddTrust(validCognitive("a", "z"))
	s.AddTrust(validCognitive("b", "z"))
	s.AddTrust(validCognitive("c", "z"))

	got := s.Reverse("z", Filter{})
	if len(got) != 3 {
		t.Errorf("expected 3 incoming edges to z, got %d", len(got))
	}
	froms := map[string]bool{}
	for _, e := range got {
		froms[e.From] = true
	}
	for _, want := range []string{"a", "b", "c"} {
		if !froms[want] {
			t.Errorf("missing reverse edge from %s", want)
		}
	}
}

// === Persistence ===

func TestPersistenceRoundtrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "trust")
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.AddTrust(validCognitive("a", "b"))
	s.AddTrust(validSocial("a", "c"))
	s.RevokeTrust("a", Social, "", "c", "left network")
	s.Close()

	s2, err := NewStore(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()

	all := s2.All()
	if len(all) != 2 {
		t.Errorf("expected 2 edges after restart, got %d", len(all))
	}
	revoked := 0
	for _, e := range all {
		if e.IsRevoked() {
			revoked++
		}
	}
	if revoked != 1 {
		t.Errorf("expected 1 revoked edge after restart, got %d", revoked)
	}
}

// === Defensive copying ===

func TestQueryReturnsDefensiveCopy(t *testing.T) {
	s := newTestStore(t)
	s.AddTrust(validCognitive("a", "b"))
	es := s.Edges("a", Filter{})
	if len(es) != 1 {
		t.Fatalf("setup")
	}
	es[0].Weight = 999 // mutate the copy

	es2 := s.Edges("a", Filter{})
	if es2[0].Weight == 999 {
		t.Error("Edges() must return a defensive copy")
	}
}

// === Decay ===

func TestBatchDecayHalfLife(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()
	e := validCognitive("a", "b")
	e.Weight = 1.0
	e.EstablishedAt = now.Add(-HalfLifeCognitive)
	e.LastDecayAt = now.Add(-HalfLifeCognitive)
	s.AddTrust(e)

	if _, err := s.BatchDecay(now); err != nil {
		t.Fatalf("decay: %v", err)
	}
	got := s.Get("a", Cognitive, "crypto", "b")
	if math.Abs(got.Weight-0.5) > 0.01 {
		t.Errorf("after one half-life expected ~0.5, got %v", got.Weight)
	}
}

func TestBatchDecaySocialFasterThanCognitive(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()

	cog := validCognitive("a", "b")
	cog.Weight = 1.0
	cog.LastDecayAt = now.Add(-30 * 24 * time.Hour)
	s.AddTrust(cog)

	soc := validSocial("a", "c")
	soc.Weight = 1.0
	soc.LastDecayAt = now.Add(-30 * 24 * time.Hour)
	s.AddTrust(soc)

	s.BatchDecay(now)

	cogW := s.Get("a", Cognitive, "crypto", "b").Weight
	socW := s.Get("a", Social, "", "c").Weight
	if socW >= cogW {
		t.Errorf("social trust should decay faster: cognitive=%v social=%v", cogW, socW)
	}
}

func TestBatchDecaySkipsRevoked(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()
	e := validCognitive("a", "b")
	e.LastDecayAt = now.Add(-HalfLifeCognitive)
	s.AddTrust(e)
	s.RevokeTrust("a", Cognitive, "crypto", "b", "test")

	got := s.Get("a", Cognitive, "crypto", "b")
	weightBefore := got.Weight

	if _, err := s.BatchDecay(now); err != nil {
		t.Fatalf("decay: %v", err)
	}

	got2 := s.Get("a", Cognitive, "crypto", "b")
	if got2.Weight != weightBefore {
		t.Errorf("revoked edges should not decay; before=%v after=%v", weightBefore, got2.Weight)
	}
}

func TestBatchDecayIdempotent(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()
	e := validCognitive("a", "b")
	e.LastDecayAt = now.Add(-30 * 24 * time.Hour)
	s.AddTrust(e)

	s.BatchDecay(now)
	first := s.Get("a", Cognitive, "crypto", "b").Weight
	s.BatchDecay(now)
	second := s.Get("a", Cognitive, "crypto", "b").Weight

	if first != second {
		t.Errorf("BatchDecay not idempotent: first=%v second=%v", first, second)
	}
}

// === Graph helpers ===

func TestHasMutualTrust(t *testing.T) {
	s := newTestStore(t)
	s.AddTrust(validCognitive("a", "b"))
	if HasMutualTrust(s, "a", "b") {
		t.Error("a→b only is not mutual")
	}
	s.AddTrust(validCognitive("b", "a"))
	if !HasMutualTrust(s, "a", "b") {
		t.Error("a↔b should be mutual")
	}

	// Self pair is not mutual
	if HasMutualTrust(s, "a", "a") {
		t.Error("self-self is not mutual")
	}
}

func TestCognitiveWeightZeroForRevoked(t *testing.T) {
	s := newTestStore(t)
	s.AddTrust(validCognitive("a", "b"))
	if w := CognitiveTrustWeight(s, "a", "b", "crypto"); w == 0 {
		t.Error("expected non-zero weight before revocation")
	}
	s.RevokeTrust("a", Cognitive, "crypto", "b", "x")
	if w := CognitiveTrustWeight(s, "a", "b", "crypto"); w != 0 {
		t.Errorf("expected 0 after revocation, got %v", w)
	}
}

func TestDomainsOfCognitiveTrustDistinct(t *testing.T) {
	s := newTestStore(t)
	s.AddTrust(validCognitive("a", "b")) // crypto
	e2 := validCognitive("a", "c")
	e2.Domain = "ml"
	s.AddTrust(e2)
	e3 := validCognitive("a", "d")
	e3.Domain = "crypto"
	s.AddTrust(e3)

	doms := DomainsOfCognitiveTrust(s, "a")
	if len(doms) != 2 {
		t.Errorf("expected 2 distinct domains (crypto, ml), got %v", doms)
	}
}
