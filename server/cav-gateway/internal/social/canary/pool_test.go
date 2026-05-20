package canary

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func newTestPool(t *testing.T) *Pool {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "canary")
	p, err := NewPool(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

// === Ground truth privacy invariant (I5) ===

func TestCanaryTaskJSONExcludesGroundTruth(t *testing.T) {
	t1 := &CanaryTask{
		ID: "x", Domain: "crypto",
	}
	t1.SetGroundTruth(GroundTruth{Conclusion: "SECRET-ANSWER"})
	data, err := json.Marshal(t1)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "SECRET-ANSWER") {
		t.Errorf("ground truth leaked into JSON: %s", string(data))
	}
	if strings.Contains(string(data), "ground_truth") {
		t.Errorf("ground_truth field name should not appear in JSON: %s", string(data))
	}
}

func TestSanitizeStripsGroundTruth(t *testing.T) {
	t1 := &CanaryTask{ID: "x", Domain: "crypto"}
	t1.SetGroundTruth(GroundTruth{Conclusion: "answer"})
	clean := Sanitize(t1)
	gt := clean.GroundTruthRef()
	if gt.Conclusion != "" {
		t.Errorf("sanitize did not strip ground truth: %+v", gt)
	}
	// Original is unaffected
	orig := t1.GroundTruthRef()
	if orig.Conclusion != "answer" {
		t.Errorf("sanitize mutated source: %+v", orig)
	}
}

// === Upsert + Get ===

func TestUpsertAndGet(t *testing.T) {
	p := newTestPool(t)
	task := &CanaryTask{ID: "t1", Domain: "crypto", Prompt: "?"}
	gt := GroundTruth{Conclusion: "42"}
	if err := p.Upsert(task, gt); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := p.Get("t1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != "t1" || got.Domain != "crypto" {
		t.Errorf("task fields lost: %+v", got)
	}
	if got.GroundTruthRef().Conclusion != "42" {
		t.Errorf("ground truth not round-tripped: %+v", got.GroundTruthRef())
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should be auto-stamped")
	}
}

func TestGetUnknownReturnsErrNotFound(t *testing.T) {
	p := newTestPool(t)
	if _, err := p.Get("nope"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestUpsertRejectsMissingRequiredFields(t *testing.T) {
	p := newTestPool(t)
	if err := p.Upsert(&CanaryTask{Domain: "crypto"}, GroundTruth{}); err == nil {
		t.Error("missing ID should fail")
	}
	if err := p.Upsert(&CanaryTask{ID: "x"}, GroundTruth{}); err == nil {
		t.Error("missing domain should fail")
	}
}

// === Random selection ===

func TestRandomByDomainRespectsExclude(t *testing.T) {
	p := newTestPool(t)
	for _, id := range []string{"a", "b", "c"} {
		p.Upsert(&CanaryTask{ID: id, Domain: "crypto"}, GroundTruth{})
	}
	exclude := map[string]bool{"a": true, "b": true}
	for i := 0; i < 20; i++ {
		got, err := p.RandomByDomain("crypto", exclude)
		if err != nil {
			t.Fatalf("random: %v", err)
		}
		if got.ID != "c" {
			t.Errorf("expected 'c' (others excluded), got %q", got.ID)
		}
	}
}

func TestRandomByDomainEmpty(t *testing.T) {
	p := newTestPool(t)
	if _, err := p.RandomByDomain("nonexistent", nil); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRandomByDomainAllExcluded(t *testing.T) {
	p := newTestPool(t)
	p.Upsert(&CanaryTask{ID: "a", Domain: "crypto"}, GroundTruth{})
	if _, err := p.RandomByDomain("crypto", map[string]bool{"a": true}); err != ErrNotFound {
		t.Errorf("all excluded should give ErrNotFound, got %v", err)
	}
}

// === PickN spreads across domains ===

func TestPickNSpreadsAcrossDomains(t *testing.T) {
	p := newTestPool(t)
	for _, id := range []string{"c1", "c2", "c3"} {
		p.Upsert(&CanaryTask{ID: id, Domain: "crypto"}, GroundTruth{})
	}
	for _, id := range []string{"m1", "m2", "m3"} {
		p.Upsert(&CanaryTask{ID: id, Domain: "ml"}, GroundTruth{})
	}
	got, err := p.PickN([]string{"crypto", "ml"}, 4)
	if err != nil {
		t.Fatalf("pickN: %v", err)
	}
	if len(got) != 4 {
		t.Errorf("expected 4 tasks, got %d", len(got))
	}
	domains := map[string]int{}
	ids := map[string]bool{}
	for _, tk := range got {
		domains[tk.Domain]++
		if ids[tk.ID] {
			t.Errorf("duplicate task picked: %q", tk.ID)
		}
		ids[tk.ID] = true
	}
	if domains["crypto"] == 0 || domains["ml"] == 0 {
		t.Errorf("expected coverage of both domains, got %+v", domains)
	}
}

func TestPickNHandlesShortage(t *testing.T) {
	p := newTestPool(t)
	p.Upsert(&CanaryTask{ID: "only", Domain: "crypto"}, GroundTruth{})
	got, err := p.PickN([]string{"crypto", "ml"}, 5)
	if err != nil {
		t.Fatalf("pickN: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 task (only available), got %d", len(got))
	}
}

// === Size + AllByDomain ===

func TestSizeAndAllByDomain(t *testing.T) {
	p := newTestPool(t)
	if got := p.Size("crypto"); got != 0 {
		t.Errorf("empty domain size = %d", got)
	}
	for _, id := range []string{"a", "b", "c"} {
		p.Upsert(&CanaryTask{ID: id, Domain: "crypto"}, GroundTruth{})
	}
	if got := p.Size("crypto"); got != 3 {
		t.Errorf("expected size 3, got %d", got)
	}
	// Re-upsert should not double count
	p.Upsert(&CanaryTask{ID: "a", Domain: "crypto"}, GroundTruth{Conclusion: "updated"})
	if got := p.Size("crypto"); got != 3 {
		t.Errorf("re-upsert should keep size at 3, got %d", got)
	}

	ids := p.AllByDomain("crypto")
	if len(ids) != 3 {
		t.Errorf("expected 3 IDs, got %v", ids)
	}
}

// === Persistence ===

func TestPoolPersistence(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pool")
	p, err := NewPool(dir)
	if err != nil {
		t.Fatal(err)
	}
	p.Upsert(&CanaryTask{ID: "a", Domain: "crypto"}, GroundTruth{Conclusion: "answer-a"})
	p.Upsert(&CanaryTask{ID: "b", Domain: "ml"}, GroundTruth{Conclusion: "answer-b"})
	p.Close()

	p2, err := NewPool(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer p2.Close()

	a, err := p2.Get("a")
	if err != nil {
		t.Fatalf("get a: %v", err)
	}
	if a.GroundTruthRef().Conclusion != "answer-a" {
		t.Errorf("ground truth lost on restart")
	}
	if p2.Size("crypto") != 1 || p2.Size("ml") != 1 {
		t.Errorf("counts lost on restart: crypto=%d ml=%d",
			p2.Size("crypto"), p2.Size("ml"))
	}
}

// === Seed loader ===

func TestLoadDefaultSeeds(t *testing.T) {
	p := newTestPool(t)
	n, err := LoadDefaultSeeds(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if n < 3 {
		t.Errorf("expected at least 3 seed tasks loaded, got %d", n)
	}
	// Each of the 3 declared domains must have at least 3 tasks.
	for _, dom := range []string{"crypto", "ml", "forensics"} {
		if p.Size(dom) < 3 {
			t.Errorf("domain %s has only %d seed tasks", dom, p.Size(dom))
		}
	}
}

func TestLoadDefaultSeedsIdempotent(t *testing.T) {
	p := newTestPool(t)
	n1, _ := LoadDefaultSeeds(p)
	n2, _ := LoadDefaultSeeds(p)
	if n1 != n2 {
		t.Errorf("seed counts differ across runs: %d vs %d", n1, n2)
	}
	// Sizes should not double.
	for _, dom := range []string{"crypto", "ml", "forensics"} {
		if p.Size(dom) > 5 {
			t.Errorf("domain %s exceeded expected size after re-seed: %d", dom, p.Size(dom))
		}
	}
}

func TestSeedTasksDoNotLeakAnswersInWireFormat(t *testing.T) {
	p := newTestPool(t)
	LoadDefaultSeeds(p)
	for _, id := range p.AllByDomain("crypto") {
		tk, err := p.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		clean := Sanitize(tk)
		data, err := json.Marshal(clean)
		if err != nil {
			t.Fatal(err)
		}
		// The conclusion of seed_crypto_001 is "m=10".
		if id == "seed_crypto_001" && strings.Contains(string(data), "m=10") {
			t.Errorf("seed answer leaked to client wire: %s", string(data))
		}
	}
}
