package reputation

import (
	"encoding/json"
	"math"
	"path/filepath"
	"testing"
	"time"
)

func TestVectorJSONRoundtrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	v := NewVector("did:cav:a", now)
	v.Operational.Domains["crypto"] = DomainScore{
		Score: 0.7, Confidence: 0.8, SampleSize: 12, LastUpdated: now,
	}
	v.Behavioral = BehavioralSubvector{
		ConformityIndex: 0.3, ChallengeSuccessRate: 0.6,
		DiversityContribution: 0.5, SampleSize: 20,
	}

	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Vector
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.DID != "did:cav:a" {
		t.Errorf("DID lost: %q", got.DID)
	}
	d, ok := got.Operational.Domains["crypto"]
	if !ok {
		t.Fatal("crypto domain lost")
	}
	if d.Score != 0.7 || d.Confidence != 0.8 || d.SampleSize != 12 {
		t.Errorf("DomainScore corrupted: %+v", d)
	}
	if got.Behavioral.ConformityIndex != 0.3 {
		t.Errorf("Behavioral subvector corrupted: %+v", got.Behavioral)
	}
	if got.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion lost: %q", got.SchemaVersion)
	}
}

func TestEffectiveScoreZeroOnUnknownDomain(t *testing.T) {
	v := NewVector("did:cav:a", time.Now())
	if got := v.EffectiveScore("unknown"); got != 0 {
		t.Errorf("unknown domain → 0, got %v", got)
	}
}

func TestEffectiveScoreZeroOnZeroConfidence(t *testing.T) {
	now := time.Now()
	v := NewVector("did:cav:a", now)
	v.Operational.Domains["crypto"] = DomainScore{
		Score: 1.0, Confidence: 0, SampleSize: 5, LastUpdated: now,
	}
	if got := v.EffectiveScore("crypto"); got != 0 {
		t.Errorf("zero confidence → 0, got %v", got)
	}
}

func TestEffectiveScoreReadDecay(t *testing.T) {
	// 90 days = one half-life ⇒ 0.5x
	now := time.Now()
	v := NewVector("did:cav:a", now)
	v.Operational.Domains["crypto"] = DomainScore{
		Score:       1.0,
		Confidence:  1.0,
		SampleSize:  5,
		LastUpdated: now.Add(-HalfLifeOperational),
	}
	got := v.EffectiveScore("crypto")
	if math.Abs(got-0.5) > 0.01 {
		t.Errorf("expected ~0.5 after one half-life, got %v", got)
	}
}

func TestEffectiveScoreInactiveHalves(t *testing.T) {
	now := time.Now()
	v := NewVector("did:cav:a", now)
	v.Operational.Domains["crypto"] = DomainScore{
		Score: 0.8, Confidence: 1.0, SampleSize: 5, LastUpdated: now,
	}
	v.Inactive = true
	got := v.EffectiveScore("crypto")
	if math.Abs(got-0.4) > 0.001 {
		t.Errorf("inactive should halve effective score, got %v", got)
	}
}

func TestLegacyLevelToVector(t *testing.T) {
	now := time.Now()
	cases := []struct {
		level                  int
		wantScore, wantConfidence float64
	}{
		{1, 0.20, 0.30},
		{2, 0.50, 0.50},
		{3, 0.80, 0.70},
	}
	for _, tc := range cases {
		v := LegacyLevelToVector("did:cav:a", tc.level, []string{"crypto", "ml"}, now)
		for _, dom := range []string{"crypto", "ml"} {
			d, ok := v.Operational.Domains[dom]
			if !ok {
				t.Fatalf("level %d: missing domain %q", tc.level, dom)
			}
			if d.Score != tc.wantScore || d.Confidence != tc.wantConfidence {
				t.Errorf("level %d %s: got score=%v conf=%v, want %v/%v",
					tc.level, dom, d.Score, d.Confidence, tc.wantScore, tc.wantConfidence)
			}
			if d.SampleSize != 0 {
				t.Errorf("legacy seed must have SampleSize=0 (no real evidence)")
			}
		}
	}

	// Unknown level → no domains seeded
	zero := LegacyLevelToVector("did:cav:a", 0, []string{"crypto"}, now)
	if len(zero.Operational.Domains) != 0 {
		t.Errorf("level=0 should not seed any domain, got %v", zero.Operational.Domains)
	}
}

func TestStoreBootstrapAndGet(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rep")
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	if err := s.Bootstrap("did:cav:a", 2, []string{"crypto"}); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	v := s.Get("did:cav:a")
	d, ok := v.Operational.Domains["crypto"]
	if !ok {
		t.Fatal("crypto domain missing after bootstrap")
	}
	if d.Score != 0.5 {
		t.Errorf("expected level-2 score 0.5, got %v", d.Score)
	}

	// Bootstrap is no-op if vector exists
	if err := s.Bootstrap("did:cav:a", 3, []string{"crypto"}); err != nil {
		t.Fatalf("re-bootstrap: %v", err)
	}
	d2 := s.Get("did:cav:a").Operational.Domains["crypto"]
	if d2.Score != 0.5 {
		t.Errorf("bootstrap must not overwrite existing vector, got %v", d2.Score)
	}
}

func TestStoreCacheDiskConsistency(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rep")
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := s.Bootstrap("did:cav:a", 3, []string{"crypto", "ml"}); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	s.SetBehavioral("did:cav:a", BehavioralSubvector{
		ConformityIndex: 0.42, SampleSize: 7,
	})
	s.SetInactive("did:cav:a", true)
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	s2, err := NewStore(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()

	v := s2.Get("did:cav:a")
	if !v.Inactive {
		t.Error("inactive flag did not persist")
	}
	if v.Behavioral.ConformityIndex != 0.42 || v.Behavioral.SampleSize != 7 {
		t.Errorf("behavioral subvector did not persist: %+v", v.Behavioral)
	}
	if len(v.Operational.Domains) != 2 {
		t.Errorf("operational tier lost: %+v", v.Operational.Domains)
	}
}

func TestStoreGetReturnsDefensiveCopy(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rep")
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	if err := s.Bootstrap("did:cav:a", 2, []string{"crypto"}); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	v := s.Get("did:cav:a")
	v.Operational.Domains["crypto"] = DomainScore{Score: 999} // mutate the copy
	v.Inactive = true

	v2 := s.Get("did:cav:a")
	if got := v2.Operational.Domains["crypto"].Score; got != 0.5 {
		t.Errorf("Get copy mutation leaked into cache: score=%v", got)
	}
	if v2.Inactive {
		t.Error("Get copy mutation leaked Inactive flag")
	}
}

func TestStoreEffectiveScoreUnknownDID(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rep")
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	if got := s.EffectiveScore("did:cav:nobody", "crypto"); got != 0 {
		t.Errorf("unknown DID → 0, got %v", got)
	}
}

func TestStoreSetInactiveUnknownDID(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rep")
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	if s.SetInactive("did:cav:nobody", true) {
		t.Error("SetInactive must return false for unknown DID")
	}
}

func TestStoreEventsEmpty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rep")
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	evs, err := s.Events("did:cav:a")
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(evs) != 0 {
		t.Errorf("expected no events, got %d", len(evs))
	}
}

func TestStoreEventsChronological(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rep")
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	// Record a few events directly through the unexported helper to verify
	// the chronological iteration. (T2 will expose this via Apply.)
	s.mu.Lock()
	for i := 0; i < 5; i++ {
		ev := Event{
			ID:         eventIDForTest(i),
			DID:        "did:cav:a",
			Domain:     "crypto",
			Tier:       TierOperational,
			Trigger:    "bootstrap",
			Delta:      0.1,
			Reason:     "test",
			OccurredAt: time.Unix(int64(1000+i), 0).UTC(),
		}
		if err := s.recordEventLocked(ev); err != nil {
			s.mu.Unlock()
			t.Fatalf("record: %v", err)
		}
	}
	s.mu.Unlock()

	evs, err := s.Events("did:cav:a")
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(evs) != 5 {
		t.Fatalf("expected 5 events, got %d", len(evs))
	}
	for i := 1; i < len(evs); i++ {
		if !evs[i].OccurredAt.After(evs[i-1].OccurredAt) {
			t.Errorf("events not chronological at %d: %v vs %v",
				i, evs[i-1].OccurredAt, evs[i].OccurredAt)
		}
	}
}

func eventIDForTest(i int) string {
	return "ev_test_" + string(rune('a'+i))
}
