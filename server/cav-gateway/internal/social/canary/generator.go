// Canary task generator — derives new probation tasks from canonical
// (crystallized) Praxons.
//
// Phase 1 implementation per design §5.5 / OQ-1: a structural rewriter that
// turns a verified Praxon into a task by:
//
//   - placing the praxon's evidence_summary into the prompt
//   - placing the conclusion into the answer key (groundTruth)
//   - reusing methodology and grounding tags as expectations
//
// Phase 2 will swap in an LLM-aided rewriter via the TaskGenerator interface.
//
// SAFETY: derived tasks must wait at least `MinAgeForDerivation` before being
// exposed (R3-9 / OQ-1 30-day window). Without this gate, an attacker could
// publish a praxon, observe it crystallize, and immediately face the same
// content as a canary task with the answer they just wrote.

package canary

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// MinAgeForDerivation is the time a praxon must have been canonical before
// the generator may derive a canary task from it.
var MinAgeForDerivation = 30 * 24 * time.Hour

// MaskedPlaceholder is the string we substitute for the praxon's conclusion
// when it appears in the evidence summary, to avoid leaking the answer in the
// prompt.
const MaskedPlaceholder = "[REDACTED]"

// SourcePraxon is the minimal praxon-shaped record the generator consumes.
// Decoupling here lets the generator run against test fixtures or against
// the real cav-node praxon store without dragging in the full type.
type SourcePraxon struct {
	ID                 string
	Domain             string
	Capabilities       []string
	CrystallizedAt     time.Time
	EvidenceSummary    string
	Conclusion         string
	PriorSourceTag     string
	InferenceMethodTag string
	GroundingTags      []string
	Difficulty         float64 // optional caller-supplied; defaults to 0.5
}

// TaskGenerator is the abstraction. Phase 1 implementation: StructuralGenerator.
type TaskGenerator interface {
	// Derive creates a CanaryTask + GroundTruth from a SourcePraxon. Returns
	// an error if the praxon is too young or missing required fields.
	Derive(praxon *SourcePraxon, now time.Time) (*CanaryTask, GroundTruth, error)
}

// StructuralGenerator is the no-LLM Phase-1 derivation strategy.
type StructuralGenerator struct{}

// NewGenerator returns the default Phase-1 generator.
func NewGenerator() TaskGenerator { return StructuralGenerator{} }

// Derive turns a praxon into a canary task. Returns ErrTooYoung if the praxon
// hasn't been canonical long enough to be safely used as a test.
func (StructuralGenerator) Derive(p *SourcePraxon, now time.Time) (*CanaryTask, GroundTruth, error) {
	if p == nil {
		return nil, GroundTruth{}, errors.New("canary: nil source praxon")
	}
	if p.ID == "" {
		return nil, GroundTruth{}, errors.New("canary: source praxon missing ID")
	}
	if p.Domain == "" {
		return nil, GroundTruth{}, errors.New("canary: source praxon missing domain")
	}
	if p.Conclusion == "" {
		return nil, GroundTruth{}, errors.New("canary: source praxon missing conclusion")
	}
	if p.CrystallizedAt.IsZero() {
		return nil, GroundTruth{}, errors.New("canary: source praxon missing CrystallizedAt")
	}
	if now.Sub(p.CrystallizedAt) < MinAgeForDerivation {
		return nil, GroundTruth{}, ErrTooYoung
	}

	prompt := buildPrompt(p)
	task := &CanaryTask{
		ID:            "gen_" + p.ID,
		Domain:        p.Domain,
		Capabilities:  append([]string{}, p.Capabilities...),
		Prompt:        prompt,
		EvidencePack:  maskConclusion(p.EvidenceSummary, p.Conclusion),
		Difficulty:    coalesceDifficulty(p.Difficulty),
		GeneratedFrom: p.ID,
		CreatedAt:     now,
	}
	gt := GroundTruth{
		Conclusion: p.Conclusion,
		RequiredMethodology: MethodologyExpectations{
			PriorSourceTags:     stringsOrNil(p.PriorSourceTag),
			InferenceMethodTags: stringsOrNil(p.InferenceMethodTag),
		},
		RequiredGroundingTags: append([]string{}, p.GroundingTags...),
	}
	return task, gt, nil
}

// ErrTooYoung is returned when a praxon hasn't been canonical long enough.
var ErrTooYoung = errors.New("canary: praxon too young for derivation")

// === Helpers ===

func buildPrompt(p *SourcePraxon) string {
	var b strings.Builder
	b.WriteString("Given the evidence summary below, state the canonical conclusion ")
	b.WriteString("with full methodology, grounding, and falsifiability.\n\n")
	b.WriteString("Domain: ")
	b.WriteString(p.Domain)
	if len(p.Capabilities) > 0 {
		b.WriteString("\nCapabilities: ")
		b.WriteString(strings.Join(p.Capabilities, ", "))
	}
	if p.EvidenceSummary != "" {
		b.WriteString("\n\nEvidence:\n")
		b.WriteString(maskConclusion(p.EvidenceSummary, p.Conclusion))
	}
	return b.String()
}

// maskConclusion replaces any occurrence of the conclusion (case-insensitive)
// inside `text` with MaskedPlaceholder. This prevents the prompt from spelling
// the answer aloud — submissions that just echo the prompt should not pass.
func maskConclusion(text, conclusion string) string {
	if conclusion == "" || text == "" {
		return text
	}
	// Simple case-insensitive replace via repeated index-based scan.
	out := text
	lower := strings.ToLower(out)
	target := strings.ToLower(conclusion)
	for {
		idx := strings.Index(lower, target)
		if idx < 0 {
			return out
		}
		out = out[:idx] + MaskedPlaceholder + out[idx+len(target):]
		lower = strings.ToLower(out)
	}
}

func coalesceDifficulty(x float64) float64 {
	if x <= 0 || x >= 1 {
		return 0.5
	}
	return x
}

func stringsOrNil(s string) []string {
	if s == "" {
		return nil
	}
	return []string{s}
}

// === Pool refill helper ===

// RefillUnderfilled inserts derived tasks from the supplied praxons whenever
// a domain's pool size is below `targetPerDomain`. Skips praxons that are
// too young or already used as a source.
//
// Returns the count of new tasks inserted.
func RefillUnderfilled(pool *Pool, gen TaskGenerator, praxons []*SourcePraxon, targetPerDomain int, now time.Time) (int, error) {
	if pool == nil || gen == nil {
		return 0, errors.New("canary: nil pool or generator")
	}
	// Group by domain and dedupe by source ID against existing pool members.
	byDom := map[string][]*SourcePraxon{}
	for _, p := range praxons {
		if p == nil || p.Domain == "" {
			continue
		}
		byDom[p.Domain] = append(byDom[p.Domain], p)
	}

	inserted := 0
	for dom, sources := range byDom {
		need := targetPerDomain - pool.Size(dom)
		if need <= 0 {
			continue
		}
		existing := map[string]bool{}
		for _, id := range pool.AllByDomain(dom) {
			existing[id] = true
		}
		for _, p := range sources {
			if need <= 0 {
				break
			}
			if existing["gen_"+p.ID] {
				continue
			}
			task, gt, err := gen.Derive(p, now)
			if err != nil {
				if errors.Is(err, ErrTooYoung) {
					continue
				}
				return inserted, fmt.Errorf("derive %s: %w", p.ID, err)
			}
			if err := pool.Upsert(task, gt); err != nil {
				return inserted, err
			}
			inserted++
			need--
		}
	}
	return inserted, nil
}
