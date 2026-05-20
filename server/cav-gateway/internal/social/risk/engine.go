// Risk engine — orchestrates the 9 dimension calculators, aggregates them
// into a single TrustRiskVector, classifies, and writes the audit record.
//
// To avoid coupling the engine to any one upstream store (signal, citizen,
// etc.) it operates on a `DataProvider` interface that callers populate from
// whatever sources they have. A reference InMemoryProvider exists for tests
// (engine_test.go); production wiring will adapt the gateway's Stores in
// handler/social_routes.go (T7).

package risk

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// DataProvider is the read-only interface the engine uses to fetch the inputs
// for each dimension calculator. Implementations may be backed by BadgerDB,
// in-memory caches, mocks, etc.
type DataProvider interface {
	// Subject's published praxons (most recent first; engine uses up to N).
	SubjectPraxons(ctx context.Context, subject string) ([]PraxonRecord, error)
	// Challenges issued against the subject's claims.
	SubjectChallenges(ctx context.Context, subject string) ([]ChallengeRecord, error)
	// Retraction events on the subject's claims.
	SubjectRetractions(ctx context.Context, subject string) ([]RetractionRecord, error)
	// Subject's vote history (with majority-position tags from upstream).
	SubjectVotes(ctx context.Context, subject string) ([]VoteRecord, error)

	// Subject's behavioral fingerprint and how long they've been on the network.
	SubjectFingerprint(ctx context.Context, subject string) (FingerprintFeatures, float64, error)
	// All other agents' fingerprints, used for sybil similarity. Excludes
	// the subject itself.
	OtherFingerprints(ctx context.Context, exclude string) (map[string]FingerprintFeatures, error)

	// Subject's recent activity sample distribution + days observed.
	SubjectActivity(ctx context.Context, subject string) (subjectDist []float64, days int, err error)
	NetworkBaselineActivity(ctx context.Context) ([]float64, error)

	// Domain activity vectors for the requester (already-trusted graph) and
	// the subject (their own published praxons).
	RequesterDomains(ctx context.Context, requester string) (DomainActivityVector, error)
	SubjectDomains(ctx context.Context, subject string) (DomainActivityVector, error)

	// Behavioral correlations between subject and every agent the requester
	// already trusts. Empty slice if no existing trust.
	ExistingCorrelations(ctx context.Context, requester, subject string) ([]float64, error)
}

// Engine computes TrustRiskVectors via the 9 dimension calculators.
type Engine struct {
	data  DataProvider
	audit *AuditStore
}

// NewEngine wires the engine to a data provider and an audit store.
// Both must be non-nil.
func NewEngine(data DataProvider, audit *AuditStore) (*Engine, error) {
	if data == nil {
		return nil, errors.New("risk: nil DataProvider")
	}
	if audit == nil {
		return nil, errors.New("risk: nil AuditStore")
	}
	return &Engine{data: data, audit: audit}, nil
}

// Compute runs all dimensions in parallel, aggregates, classifies, and
// persists to the audit log. Returns the populated vector (with VectorHash
// set on success).
//
// On dimension errors: a single dimension failing does not abort the run;
// the dimension is set to nil and added to InsufficientDimensions with an
// "error: ..." prefix. The overall call only fails if the audit write fails.
func (e *Engine) Compute(ctx context.Context, requester, subject string) (*TrustRiskVector, error) {
	if requester == "" || subject == "" {
		return nil, errors.New("risk: requester and subject required")
	}
	if requester == subject {
		return nil, errors.New("risk: cannot evaluate self-trust")
	}

	v := &TrustRiskVector{
		Subject:       subject,
		Requester:     requester,
		ComputedAt:    time.Now(),
		SchemaVersion: SchemaVersion,
		Epistemic:     &EpistemicRisk{},
		Behavioral:    &BehavioralRisk{},
		Structural:    &StructuralRisk{},
	}

	var insufficient []string
	var insufficientMu sync.Mutex
	noteInsufficient := func(name string) {
		insufficientMu.Lock()
		insufficient = append(insufficient, name)
		insufficientMu.Unlock()
	}

	// Pre-fetch shared inputs (used by multiple dimensions) and inputs for
	// the calculators that have a strict dependency order. This serializes a
	// few cheap calls before the parallel fan-out, which is fine for the
	// p95-target of 500ms.
	praxons, _ := e.data.SubjectPraxons(ctx, subject)         // ground_truth + methodology
	votes, _ := e.data.SubjectVotes(ctx, subject)              // conformity (also needed for echo)
	requesterDoms, _ := e.data.RequesterDomains(ctx, requester)
	subjectDoms, _ := e.data.SubjectDomains(ctx, subject)
	corr, _ := e.data.ExistingCorrelations(ctx, requester, subject)

	// Conformity is a prerequisite for echo_chamber_delta.
	conformityDim := ConformityIndex(votes)
	v.Behavioral.ConformityIndex = conformityDim
	if !conformityDim.Sufficient {
		noteInsufficient("behavioral.conformity_index")
	}

	// Fan-out the remaining dimensions in parallel.
	var wg sync.WaitGroup
	wg.Add(7)

	go func() {
		defer wg.Done()
		d := GroundTruthAlignment(praxons)
		v.Epistemic.GroundTruthAlignment = d
		if !d.Sufficient {
			noteInsufficient("epistemic.ground_truth_alignment")
		}
	}()
	go func() {
		defer wg.Done()
		d := MethodologyStability(praxons)
		v.Epistemic.MethodologyStability = d
		if !d.Sufficient {
			noteInsufficient("epistemic.methodology_stability")
		}
	}()
	go func() {
		defer wg.Done()
		cs, err := e.data.SubjectChallenges(ctx, subject)
		if err != nil {
			noteInsufficient("epistemic.challenge_survival_rate:err")
			return
		}
		d := ChallengeSurvivalRate(cs)
		v.Epistemic.ChallengeSurvivalRate = d
		if !d.Sufficient {
			noteInsufficient("epistemic.challenge_survival_rate")
		}
	}()
	go func() {
		defer wg.Done()
		rs, err := e.data.SubjectRetractions(ctx, subject)
		if err != nil {
			noteInsufficient("epistemic.retraction_responsiveness:err")
			return
		}
		d := RetractionResponsiveness(rs)
		v.Epistemic.RetractionResponsiveness = d
		if !d.Sufficient {
			noteInsufficient("epistemic.retraction_responsiveness")
		}
	}()
	go func() {
		defer wg.Done()
		fp, ageHours, err := e.data.SubjectFingerprint(ctx, subject)
		if err != nil {
			noteInsufficient("behavioral.sybil_similarity_max:err")
			return
		}
		others, err := e.data.OtherFingerprints(ctx, subject)
		if err != nil {
			noteInsufficient("behavioral.sybil_similarity_max:err")
			return
		}
		d := SybilSimilarityMax(fp, ageHours, others)
		v.Behavioral.SybilSimilarityMax = d
		if !d.Sufficient {
			noteInsufficient("behavioral.sybil_similarity_max")
		}
	}()
	go func() {
		defer wg.Done()
		subj, days, err := e.data.SubjectActivity(ctx, subject)
		if err != nil {
			noteInsufficient("behavioral.activity_anomaly_score:err")
			return
		}
		base, err := e.data.NetworkBaselineActivity(ctx)
		if err != nil {
			noteInsufficient("behavioral.activity_anomaly_score:err")
			return
		}
		d := ActivityAnomalyScore(subj, base, days)
		v.Behavioral.ActivityAnomalyScore = d
		if !d.Sufficient {
			noteInsufficient("behavioral.activity_anomaly_score")
		}
	}()
	go func() {
		defer wg.Done()
		d := DiversityImpact(requesterDoms, subjectDoms)
		v.Structural.DiversityImpact = d
		if !d.Sufficient {
			noteInsufficient("structural.diversity_impact")
		}
	}()

	wg.Wait()

	// Echo chamber is computed after conformity is known.
	v.Structural.EchoChamberDelta = EchoChamberDelta(conformityDim.Score, corr)
	if !v.Structural.EchoChamberDelta.Sufficient {
		noteInsufficient("structural.echo_chamber_delta")
	}

	// === Aggregation ===
	v.InsufficientDimensions = dedupe(insufficient)
	v.AggregateScore, v.DominantFactors = aggregateAndRank(v)
	v.RiskClass, v.Recommendation = ClassifyAggregate(v.AggregateScore)

	// Sufficiency gate: fewer than 2 sufficient dimensions → defer regardless.
	if sufficientCount(v) < 2 {
		v.Recommendation = RecDefer
	}

	v.Reasoning = buildReasoning(v)

	// Persist to audit log.
	hash, err := e.audit.Persist(v)
	if err != nil {
		return nil, fmt.Errorf("audit persist: %w", err)
	}
	// Stamp the hash on the vector for callers (handler will copy this into
	// the trust edge's RiskVectorSnapshot).
	v = withHash(v, hash)
	return v, nil
}

// withHash returns a vector copy with the given hash stamped on the
// VectorHash field. We do NOT alter Reasoning here — callers that want a
// fingerprint in their logs should read VectorHash directly.
func withHash(v *TrustRiskVector, hash string) *TrustRiskVector {
	cp := *v
	cp.VectorHash = hash
	return &cp
}

// === Aggregation helpers ===

func aggregateAndRank(v *TrustRiskVector) (float64, []DominantFactor) {
	type item struct {
		name   string
		dim    *Dimension
		weight float64
	}
	dims := []item{
		{"epistemic.ground_truth_alignment", v.Epistemic.GroundTruthAlignment, WeightEpistemic / 4},
		{"epistemic.methodology_stability", v.Epistemic.MethodologyStability, WeightEpistemic / 4},
		{"epistemic.challenge_survival_rate", v.Epistemic.ChallengeSurvivalRate, WeightEpistemic / 4},
		{"epistemic.retraction_responsiveness", v.Epistemic.RetractionResponsiveness, WeightEpistemic / 4},
		{"behavioral.conformity_index", v.Behavioral.ConformityIndex, WeightBehavioral / 3},
		{"behavioral.sybil_similarity_max", v.Behavioral.SybilSimilarityMax, WeightBehavioral / 3},
		{"behavioral.activity_anomaly_score", v.Behavioral.ActivityAnomalyScore, WeightBehavioral / 3},
		{"structural.diversity_impact", v.Structural.DiversityImpact, WeightStructural / 2},
		{"structural.echo_chamber_delta", v.Structural.EchoChamberDelta, WeightStructural / 2},
	}

	var weighted, totalW float64
	for _, it := range dims {
		if it.dim == nil || !it.dim.Sufficient {
			continue
		}
		weighted += it.weight * it.dim.Score
		totalW += it.weight
	}

	var agg float64
	if totalW > 0 {
		agg = weighted / totalW
	}

	// Top-3 factors among sufficient dims sorted by score desc.
	type ranked struct {
		name  string
		score float64
	}
	var sortable []ranked
	for _, it := range dims {
		if it.dim == nil || !it.dim.Sufficient {
			continue
		}
		sortable = append(sortable, ranked{it.name, it.dim.Score})
	}
	sort.SliceStable(sortable, func(i, j int) bool {
		return sortable[i].score > sortable[j].score
	})
	top := sortable
	if len(top) > 3 {
		top = top[:3]
	}
	out := make([]DominantFactor, 0, len(top))
	for _, r := range top {
		out = append(out, DominantFactor{Name: r.name, Score: r.score})
	}
	return agg, out
}

func sufficientCount(v *TrustRiskVector) int {
	n := 0
	check := func(d *Dimension) {
		if d != nil && d.Sufficient {
			n++
		}
	}
	if v.Epistemic != nil {
		check(v.Epistemic.GroundTruthAlignment)
		check(v.Epistemic.MethodologyStability)
		check(v.Epistemic.ChallengeSurvivalRate)
		check(v.Epistemic.RetractionResponsiveness)
	}
	if v.Behavioral != nil {
		check(v.Behavioral.ConformityIndex)
		check(v.Behavioral.SybilSimilarityMax)
		check(v.Behavioral.ActivityAnomalyScore)
	}
	if v.Structural != nil {
		check(v.Structural.DiversityImpact)
		check(v.Structural.EchoChamberDelta)
	}
	return n
}

func buildReasoning(v *TrustRiskVector) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%.2f)", v.RiskClass, v.AggregateScore)
	if len(v.DominantFactors) > 0 {
		b.WriteString(" — top: ")
		for i, f := range v.DominantFactors {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s=%.2f", shortName(f.Name), f.Score)
		}
	}
	if len(v.InsufficientDimensions) > 0 {
		fmt.Fprintf(&b, " | %d insufficient", len(v.InsufficientDimensions))
	}
	return b.String()
}

func shortName(full string) string {
	parts := strings.Split(full, ".")
	if len(parts) == 0 {
		return full
	}
	return parts[len(parts)-1]
}

func dedupe(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
