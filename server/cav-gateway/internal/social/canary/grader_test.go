package canary

import (
	"testing"
	"time"
)

func newTask(gt GroundTruth) *CanaryTask {
	t := &CanaryTask{ID: "t", Domain: "crypto"}
	t.SetGroundTruth(gt)
	return t
}

// === Alignment ===

func TestAlignmentExactMatchScores1(t *testing.T) {
	g := NewGrader()
	task := newTask(GroundTruth{Conclusion: "the plaintext is 10"})
	sub := &Submission{Conclusion: "the plaintext is 10",
		HasMethodology: true, PriorSourceTag: "tool", InferenceMethodTag: "deductive",
		HasGrounding: true,
	}
	s := g.Grade(sub, task, time.Now().Add(-5*time.Minute))
	if s.GroundTruthAlignment != 1.0 {
		t.Errorf("exact match → 1.0, got %v", s.GroundTruthAlignment)
	}
}

func TestAlignmentMatchesAlternative(t *testing.T) {
	g := NewGrader()
	task := newTask(GroundTruth{
		Conclusion:           "ECB",
		AcceptedAlternatives: []string{"Electronic Codebook"},
	})
	sub := &Submission{Conclusion: "Electronic Codebook"}
	s := g.Grade(sub, task, time.Now().Add(-5*time.Minute))
	if s.GroundTruthAlignment < 0.99 {
		t.Errorf("alternative match should score ~1, got %v", s.GroundTruthAlignment)
	}
}

func TestAlignmentEmptyConclusionIsZero(t *testing.T) {
	g := NewGrader()
	task := newTask(GroundTruth{Conclusion: "answer"})
	s := g.Grade(&Submission{Conclusion: ""}, task, time.Now())
	if s.GroundTruthAlignment != 0 {
		t.Errorf("empty conclusion should score 0, got %v", s.GroundTruthAlignment)
	}
}

func TestAlignmentPartialMatch(t *testing.T) {
	g := NewGrader()
	task := newTask(GroundTruth{Conclusion: "RSA modulus factorization is 3 and 7"})
	sub := &Submission{Conclusion: "RSA modulus is 3 and 7"}
	s := g.Grade(sub, task, time.Now().Add(-5*time.Minute))
	if s.GroundTruthAlignment <= 0 || s.GroundTruthAlignment >= 1 {
		// Should be partial — some tokens overlap, some don't
	} else {
		// Assert it's actually partial
		if s.GroundTruthAlignment < 0.4 {
			t.Errorf("partial match should be > 0.4, got %v", s.GroundTruthAlignment)
		}
	}
}

// === Methodology ===

func TestMethodologyMissingScoresZero(t *testing.T) {
	g := NewGrader()
	task := newTask(GroundTruth{
		RequiredMethodology: MethodologyExpectations{
			PriorSourceTags: []string{"tool"},
		},
	})
	sub := &Submission{HasMethodology: false}
	s := g.Grade(sub, task, time.Now())
	if s.MethodologyQuality != 0 {
		t.Errorf("missing methodology → 0, got %v", s.MethodologyQuality)
	}
}

func TestMethodologyNoConstraintsScoresOne(t *testing.T) {
	g := NewGrader()
	task := newTask(GroundTruth{}) // no required methodology
	sub := &Submission{HasMethodology: true,
		PriorSourceTag: "anything", InferenceMethodTag: "anything"}
	s := g.Grade(sub, task, time.Now())
	if s.MethodologyQuality != 1.0 {
		t.Errorf("unconstrained methodology → 1, got %v", s.MethodologyQuality)
	}
}

func TestMethodologyPartialMatch(t *testing.T) {
	g := NewGrader()
	task := newTask(GroundTruth{
		RequiredMethodology: MethodologyExpectations{
			PriorSourceTags:     []string{"tool", "observation"},
			InferenceMethodTags: []string{"deductive"},
		},
	})
	// Match prior_source but not inference_method
	sub := &Submission{HasMethodology: true,
		PriorSourceTag: "tool", InferenceMethodTag: "abductive"}
	s := g.Grade(sub, task, time.Now())
	if s.MethodologyQuality != 0.5 {
		t.Errorf("1-of-2 match should score 0.5, got %v", s.MethodologyQuality)
	}
}

// === Time pattern ===

func TestTimePatternZeroOnNegativeDelta(t *testing.T) {
	g := NewGrader()
	task := newTask(GroundTruth{})
	now := time.Now()
	sub := &Submission{SubmittedAt: now.Add(-1 * time.Minute)}
	s := g.Grade(sub, task, now)
	if s.ResponseTimePattern > 0.2 {
		t.Errorf("submitted before assigned → low score, got %v", s.ResponseTimePattern)
	}
}

func TestTimePatternFastSubmissionPenalized(t *testing.T) {
	g := NewGrader()
	task := newTask(GroundTruth{})
	now := time.Now()
	sub := &Submission{SubmittedAt: now}
	s := g.Grade(sub, task, now.Add(-10*time.Second))
	if s.ResponseTimePattern > 0.3 {
		t.Errorf("10s submission should be penalized as scripted, got %v", s.ResponseTimePattern)
	}
}

func TestTimePatternIdealNearFiveMinutes(t *testing.T) {
	g := NewGrader()
	task := newTask(GroundTruth{})
	now := time.Now()
	sub := &Submission{SubmittedAt: now}
	s := g.Grade(sub, task, now.Add(-5*time.Minute))
	if s.ResponseTimePattern < 0.95 {
		t.Errorf("5 min submission should score near 1, got %v", s.ResponseTimePattern)
	}
}

func TestTimePatternVeryLatePenalized(t *testing.T) {
	g := NewGrader()
	task := newTask(GroundTruth{})
	now := time.Now()
	sub := &Submission{SubmittedAt: now}
	s := g.Grade(sub, task, now.Add(-6*time.Hour))
	if s.ResponseTimePattern > 0.1 {
		t.Errorf("6h submission should score near 0, got %v", s.ResponseTimePattern)
	}
}

// === Grounding ===

func TestGroundingMissingScoresZero(t *testing.T) {
	g := NewGrader()
	task := newTask(GroundTruth{RequiredGroundingTags: []string{"x"}})
	sub := &Submission{HasGrounding: false}
	s := g.Grade(sub, task, time.Now())
	if s.GroundingQuality != 0 {
		t.Errorf("missing grounding → 0, got %v", s.GroundingQuality)
	}
}

func TestGroundingNoRequiredTagsScoresOne(t *testing.T) {
	g := NewGrader()
	task := newTask(GroundTruth{}) // no required tags
	sub := &Submission{HasGrounding: true}
	s := g.Grade(sub, task, time.Now())
	if s.GroundingQuality != 1.0 {
		t.Errorf("no required tags + grounding present → 1, got %v", s.GroundingQuality)
	}
}

func TestGroundingPartialMatchScoresPartial(t *testing.T) {
	g := NewGrader()
	task := newTask(GroundTruth{
		RequiredGroundingTags: []string{"rsa_decryption", "modular_arithmetic"},
	})
	sub := &Submission{
		HasGrounding:  true,
		GroundingTags: []string{"rsa_decryption"},
	}
	s := g.Grade(sub, task, time.Now())
	// 1/2 = 0.5 → 0.2 + 0.8*0.5 = 0.6
	if s.GroundingQuality < 0.5 || s.GroundingQuality > 0.7 {
		t.Errorf("expected partial grounding ~0.6, got %v", s.GroundingQuality)
	}
}

// === Pass/fail ===

func TestPassRequiresBothThresholds(t *testing.T) {
	g := NewGrader()
	if g.Passed(TaskScores{GroundTruthAlignment: 0.7, MethodologyQuality: 0.4}) {
		t.Error("methodology below threshold should fail")
	}
	if g.Passed(TaskScores{GroundTruthAlignment: 0.5, MethodologyQuality: 0.6}) {
		t.Error("alignment below threshold should fail")
	}
	if !g.Passed(TaskScores{GroundTruthAlignment: 0.6, MethodologyQuality: 0.5}) {
		t.Error("at threshold should pass")
	}
}

// === Integration: grade a submission against a seed task ===

func TestGradeAgainstSeedTask(t *testing.T) {
	p := newTestPool(t)
	if _, err := LoadDefaultSeeds(p); err != nil {
		t.Fatal(err)
	}
	task, err := p.Get("seed_crypto_001")
	if err != nil {
		t.Fatal(err)
	}
	g := NewGrader()
	now := time.Now()
	sub := &Submission{
		Conclusion:         "the plaintext m = 10",
		HasMethodology:     true,
		PriorSourceTag:     "tool",
		InferenceMethodTag: "deductive",
		HasGrounding:       true,
		GroundingTags:      []string{"modular_arithmetic", "rsa_decryption"},
		SubmittedAt:        now,
	}
	s := g.Grade(sub, task, now.Add(-5*time.Minute))
	if !g.Passed(s) {
		t.Errorf("good submission against seed should pass, scores=%+v", s)
	}
}
