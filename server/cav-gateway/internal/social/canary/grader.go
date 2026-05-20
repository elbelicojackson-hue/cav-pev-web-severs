// Canary grader.
//
// Phase 1 implementation per design §5.5: 4-dimension scoring on a submitted
// praxon-shaped object using only structural / token-set features (no LLM
// dependency). M5+ may swap in semantic grading via a TaskGrader interface
// that the upstream wiring picks at construction time.
//
//   ground_truth_alignment : token-set Jaccard against {Conclusion} ∪ AcceptedAlternatives
//   methodology_quality    : tag-set match against RequiredMethodology
//   response_time_pattern  : penalize too-fast (gaming) and too-slow (insufficient effort)
//   grounding_quality      : presence + tag-set match against RequiredGroundingTags
//
// Pass criteria (R3-7): GroundTruthAlignment ≥ 0.6 AND MethodologyQuality ≥ 0.5.

package canary

import (
	"math"
	"strings"
	"time"
	"unicode"
)

// PassThresholds are the required scores for canary task pass/fail.
const (
	MinAlignmentToPass   = 0.60
	MinMethodologyToPass = 0.50
)

// Grader is the interface; in Phase 1 we ship one implementation (TokenGrader)
// but the abstraction lets future LLM-aided graders swap in cleanly.
type Grader interface {
	Grade(submission *Submission, task *CanaryTask, assignedAt time.Time) TaskScores
	Passed(scores TaskScores) bool
}

// TokenGrader is the Phase-1 lexical grader.
type TokenGrader struct{}

// NewGrader returns the default grader instance.
func NewGrader() Grader { return TokenGrader{} }

// Grade applies the 4-dimension scoring rubric.
func (TokenGrader) Grade(submission *Submission, task *CanaryTask, assignedAt time.Time) TaskScores {
	gt := task.GroundTruthRef()
	return TaskScores{
		GroundTruthAlignment: scoreAlignment(submission.Conclusion, gt),
		MethodologyQuality:   scoreMethodology(submission, gt.RequiredMethodology),
		ResponseTimePattern:  scoreTimePattern(assignedAt, submission.SubmittedAt),
		GroundingQuality:     scoreGrounding(submission, gt.RequiredGroundingTags),
	}
}

// Passed implements R3-7.
func (TokenGrader) Passed(s TaskScores) bool {
	return s.GroundTruthAlignment >= MinAlignmentToPass &&
		s.MethodologyQuality >= MinMethodologyToPass
}

// === Dimension scorers ===

// scoreAlignment is a Jaccard token similarity between the submission's
// conclusion and the union of canonical conclusion + accepted alternatives.
// Returns the MAX similarity over candidates, since any one accepted form
// counts as a pass.
func scoreAlignment(submitted string, gt GroundTruth) float64 {
	subj := tokenSet(submitted)
	if len(subj) == 0 {
		return 0
	}
	best := jaccard(subj, tokenSet(gt.Conclusion))
	for _, alt := range gt.AcceptedAlternatives {
		s := jaccard(subj, tokenSet(alt))
		if s > best {
			best = s
		}
	}
	return clamp01(best)
}

// scoreMethodology returns a [0, 1] score of how well the submission's
// methodology tags overlap with the required methodology expectations.
//
// If RequiredMethodology has no constraints, score=1 (anything is acceptable).
// Otherwise: we check both prior_source_tag and inference_method_tag. Each
// dimension that is constrained contributes equally.
//
// HasMethodology=false on the submission immediately scores 0 — Axiom 3
// requires the methodology field to be present.
func scoreMethodology(submission *Submission, exp MethodologyExpectations) float64 {
	if !submission.HasMethodology {
		return 0
	}
	dims := 0
	score := 0.0
	if len(exp.PriorSourceTags) > 0 {
		dims++
		if contains(exp.PriorSourceTags, submission.PriorSourceTag) {
			score += 1
		}
	}
	if len(exp.InferenceMethodTags) > 0 {
		dims++
		if contains(exp.InferenceMethodTags, submission.InferenceMethodTag) {
			score += 1
		}
	}
	if dims == 0 {
		return 1.0
	}
	return clamp01(score / float64(dims))
}

// scoreTimePattern penalizes submissions that are suspiciously fast (likely
// scripted) or very late (low effort). We use a Gaussian-shaped curve centered
// at 5 minutes with width ~30 minutes.
//
//   t < 30s       → 0.2  (likely automation gaming the test)
//   t ≈ 5 min     → 1.0  (ideal)
//   t > 4 hours   → ~0   (took too long, likely external help)
func scoreTimePattern(assignedAt, submittedAt time.Time) float64 {
	if assignedAt.IsZero() || submittedAt.IsZero() {
		return 0.7 // unknown timestamps — neutral but slightly penalized
	}
	delta := submittedAt.Sub(assignedAt).Seconds()
	if delta <= 0 {
		return 0.1 // submitted before assigned — clearly wrong
	}
	if delta < 30 {
		return 0.2
	}
	const idealSec = 300.0      // 5 min
	const widthSec = 1800.0     // 30 min
	x := (delta - idealSec) / widthSec
	return clamp01(math.Exp(-x * x))
}

// scoreGrounding checks that the submission has grounding and at least 50%
// of the required grounding tags. If the task has no required tags, presence
// alone scores 1.0.
func scoreGrounding(submission *Submission, required []string) float64 {
	if !submission.HasGrounding {
		return 0
	}
	if len(required) == 0 {
		return 1.0
	}
	hits := 0
	subjSet := map[string]bool{}
	for _, tag := range submission.GroundingTags {
		subjSet[strings.ToLower(strings.TrimSpace(tag))] = true
	}
	for _, want := range required {
		if subjSet[strings.ToLower(strings.TrimSpace(want))] {
			hits++
		}
	}
	frac := float64(hits) / float64(len(required))
	// A submission that has grounding but matches none of the required tags
	// still gets a small floor — it's better than no grounding at all.
	return clamp01(0.2 + 0.8*frac)
}

// === Helpers ===

// tokenSet returns the set of normalized tokens in s (lowercase, alphanum only).
func tokenSet(s string) map[string]bool {
	out := map[string]bool{}
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			out[strings.ToLower(cur.String())] = true
			cur.Reset()
		}
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	delete(out, "") // safety
	return out
}

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for k := range a {
		if b[k] {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func contains(ss []string, x string) bool {
	x = strings.ToLower(strings.TrimSpace(x))
	for _, s := range ss {
		if strings.ToLower(strings.TrimSpace(s)) == x {
			return true
		}
	}
	return false
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}
