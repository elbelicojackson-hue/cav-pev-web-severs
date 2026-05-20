package canary

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func sampleSourcePraxon(crystallizedAt time.Time) *SourcePraxon {
	return &SourcePraxon{
		ID:                 "px_001",
		Domain:             "crypto",
		Capabilities:       []string{"rsa", "modular_arithmetic"},
		CrystallizedAt:     crystallizedAt,
		EvidenceSummary:    "RSA n=21=3*7, e=5; ciphertext c=10 maps to plaintext m=10.",
		Conclusion:         "m=10",
		PriorSourceTag:     "tool",
		InferenceMethodTag: "deductive",
		GroundingTags:      []string{"modular_arithmetic", "rsa_decryption"},
		Difficulty:         0.3,
	}
}

func TestGeneratorRejectsYoungPraxon(t *testing.T) {
	gen := NewGenerator()
	now := time.Now()
	p := sampleSourcePraxon(now.Add(-7 * 24 * time.Hour)) // 7 days < 30
	_, _, err := gen.Derive(p, now)
	if !errors.Is(err, ErrTooYoung) {
		t.Errorf("expected ErrTooYoung, got %v", err)
	}
}

func TestGeneratorAcceptsAgedPraxon(t *testing.T) {
	gen := NewGenerator()
	now := time.Now()
	p := sampleSourcePraxon(now.Add(-31 * 24 * time.Hour))
	task, gt, err := gen.Derive(p, now)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if task.ID != "gen_px_001" {
		t.Errorf("derived ID should prefix gen_; got %q", task.ID)
	}
	if task.Domain != "crypto" {
		t.Errorf("domain lost: %q", task.Domain)
	}
	if gt.Conclusion != "m=10" {
		t.Errorf("conclusion lost: %q", gt.Conclusion)
	}
	if task.GeneratedFrom != "px_001" {
		t.Errorf("GeneratedFrom should reference source praxon, got %q", task.GeneratedFrom)
	}
}

func TestGeneratorMasksAnswerInPrompt(t *testing.T) {
	gen := NewGenerator()
	now := time.Now()
	p := sampleSourcePraxon(now.Add(-31 * 24 * time.Hour))
	task, _, err := gen.Derive(p, now)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(task.Prompt, "m=10") {
		t.Errorf("prompt must mask the conclusion: %s", task.Prompt)
	}
	if strings.Contains(task.EvidencePack, "m=10") {
		t.Errorf("evidence pack must mask the conclusion: %s", task.EvidencePack)
	}
	if !strings.Contains(task.Prompt, MaskedPlaceholder) {
		t.Errorf("expected %s placeholder in prompt", MaskedPlaceholder)
	}
}

func TestGeneratorRejectsMissingFields(t *testing.T) {
	gen := NewGenerator()
	now := time.Now()
	cases := []struct {
		name string
		mut  func(*SourcePraxon)
	}{
		{"missing ID", func(p *SourcePraxon) { p.ID = "" }},
		{"missing domain", func(p *SourcePraxon) { p.Domain = "" }},
		{"missing conclusion", func(p *SourcePraxon) { p.Conclusion = "" }},
		{"missing CrystallizedAt", func(p *SourcePraxon) { p.CrystallizedAt = time.Time{} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := sampleSourcePraxon(now.Add(-31 * 24 * time.Hour))
			tc.mut(p)
			if _, _, err := gen.Derive(p, now); err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}
}

func TestGeneratorPreservesMethodologyExpectations(t *testing.T) {
	gen := NewGenerator()
	now := time.Now()
	p := sampleSourcePraxon(now.Add(-31 * 24 * time.Hour))
	_, gt, _ := gen.Derive(p, now)

	if len(gt.RequiredMethodology.PriorSourceTags) != 1 ||
		gt.RequiredMethodology.PriorSourceTags[0] != "tool" {
		t.Errorf("PriorSourceTags lost: %+v", gt.RequiredMethodology.PriorSourceTags)
	}
	if len(gt.RequiredMethodology.InferenceMethodTags) != 1 ||
		gt.RequiredMethodology.InferenceMethodTags[0] != "deductive" {
		t.Errorf("InferenceMethodTags lost: %+v", gt.RequiredMethodology.InferenceMethodTags)
	}
	if len(gt.RequiredGroundingTags) != 2 {
		t.Errorf("RequiredGroundingTags lost: %+v", gt.RequiredGroundingTags)
	}
}

func TestGeneratorWireFormatHidesGroundTruth(t *testing.T) {
	gen := NewGenerator()
	now := time.Now()
	p := sampleSourcePraxon(now.Add(-31 * 24 * time.Hour))
	task, gt, _ := gen.Derive(p, now)
	task.SetGroundTruth(gt)

	clean := Sanitize(task)
	if clean.GroundTruthRef().Conclusion != "" {
		t.Errorf("sanitized derived task must hide conclusion: %+v", clean.GroundTruthRef())
	}
}

// === RefillUnderfilled ===

func TestRefillSkipsTooYoung(t *testing.T) {
	pool := newTestPool(t)
	gen := NewGenerator()
	now := time.Now()
	young := sampleSourcePraxon(now.Add(-5 * 24 * time.Hour))
	young.ID = "young_1"
	old := sampleSourcePraxon(now.Add(-60 * 24 * time.Hour))
	old.ID = "old_1"

	n, err := RefillUnderfilled(pool, gen, []*SourcePraxon{young, old}, 5, now)
	if err != nil {
		t.Fatalf("refill: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 inserted (only old), got %d", n)
	}
	if _, err := pool.Get("gen_old_1"); err != nil {
		t.Errorf("old praxon should be in pool, got %v", err)
	}
	if _, err := pool.Get("gen_young_1"); err == nil {
		t.Error("young praxon should NOT be in pool")
	}
}

func TestRefillSkipsAlreadySourced(t *testing.T) {
	pool := newTestPool(t)
	gen := NewGenerator()
	now := time.Now()
	src := sampleSourcePraxon(now.Add(-60 * 24 * time.Hour))
	src.ID = "src_dup"

	if n, err := RefillUnderfilled(pool, gen, []*SourcePraxon{src}, 5, now); err != nil || n != 1 {
		t.Fatalf("first refill: n=%d err=%v", n, err)
	}
	// Re-running with the same praxon must not double-insert.
	n, _ := RefillUnderfilled(pool, gen, []*SourcePraxon{src}, 5, now)
	if n != 0 {
		t.Errorf("re-refill must skip already-sourced; got %d", n)
	}
}

func TestRefillRespectsTargetCount(t *testing.T) {
	pool := newTestPool(t)
	// pre-fill the crypto domain to its target
	for i := 0; i < 5; i++ {
		pool.Upsert(&CanaryTask{ID: "fill_" + string(rune('a'+i)), Domain: "crypto"}, GroundTruth{})
	}
	gen := NewGenerator()
	now := time.Now()
	src := sampleSourcePraxon(now.Add(-60 * 24 * time.Hour))

	n, _ := RefillUnderfilled(pool, gen, []*SourcePraxon{src}, 5, now)
	if n != 0 {
		t.Errorf("pool already at target, expected no inserts; got %d", n)
	}
}
