package instance

import (
	"log/slog"
	"math"
	"sync"

	"github.com/anthropic-cav/cav-npc/internal/signal"
)

// DiversityChecker performs 24h self-checks on output correlation (R9.4-5).
type DiversityChecker struct {
	npcDID string

	mu      sync.Mutex
	vectors [][]float64 // normalized vectors from recent signals
}

// NewDiversityChecker creates a diversity checker.
func NewDiversityChecker(npcDID string) *DiversityChecker {
	return &DiversityChecker{
		npcDID: npcDID,
	}
}

// RecordSignal adds a published signal's posterior_shift to the vector buffer.
func (d *DiversityChecker) RecordSignal(sig *signal.EntropicSignal) {
	if sig.PosteriorShift == nil {
		return
	}

	// Build a simple feature vector from the posterior_shift
	// (simplified from design §5.3 — uses string hashes + delta_bits)
	v := d.buildVector(sig.PosteriorShift)

	d.mu.Lock()
	d.vectors = append(d.vectors, v)
	// Keep last 200 signals max
	if len(d.vectors) > 200 {
		d.vectors = d.vectors[len(d.vectors)-200:]
	}
	d.mu.Unlock()
}

// Check performs the diversity self-check.
// Returns (medianSimilarity, shouldAdjust).
// shouldAdjust is true if median cosine similarity > 0.85 (R9.4).
func (d *DiversityChecker) Check() (float64, bool) {
	d.mu.Lock()
	vecs := make([][]float64, len(d.vectors))
	copy(vecs, d.vectors)
	d.mu.Unlock()

	// Need at least 5 samples (design §5.3 implicit)
	if len(vecs) < 5 {
		return 0, false
	}

	// Compute pairwise cosine similarities
	var sims []float64
	for i := 0; i < len(vecs); i++ {
		for j := i + 1; j < len(vecs); j++ {
			sim := cosine(vecs[i], vecs[j])
			sims = append(sims, sim)
		}
	}

	if len(sims) == 0 {
		return 0, false
	}

	// Compute median
	med := median(sims)

	if med > 0.85 {
		slog.Warn("diversity self-check: high output correlation detected",
			"npc_did", d.npcDID,
			"median_similarity", med,
			"sample_count", len(vecs),
		)
		return med, true
	}

	return med, false
}

// Reset clears the vector buffer (called after 24h window).
func (d *DiversityChecker) Reset() {
	d.mu.Lock()
	d.vectors = nil
	d.mu.Unlock()
}

// buildVector creates a normalized feature vector from a PosteriorShift.
func (d *DiversityChecker) buildVector(ps *signal.PosteriorShift) []float64 {
	// Simple feature extraction:
	// [hash(subject) features, hash(relation) features, delta_bits normalized]
	v := make([]float64, 9)

	// Subject hash → 4 float features
	h := simpleHash(ps.Subject)
	v[0] = float64((h>>0)&0xFF) / 255.0
	v[1] = float64((h>>8)&0xFF) / 255.0
	v[2] = float64((h>>16)&0xFF) / 255.0
	v[3] = float64((h>>24)&0xFF) / 255.0

	// Relation hash → 4 float features
	h2 := simpleHash(ps.Relation)
	v[4] = float64((h2>>0)&0xFF) / 255.0
	v[5] = float64((h2>>8)&0xFF) / 255.0
	v[6] = float64((h2>>16)&0xFF) / 255.0
	v[7] = float64((h2>>24)&0xFF) / 255.0

	// Delta bits normalized to [0,1] (cap at 16)
	v[8] = math.Min(ps.DeltaBits, 16.0) / 16.0

	// Normalize
	return normalize(v)
}

func simpleHash(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

func normalize(v []float64) []float64 {
	var norm float64
	for _, x := range v {
		norm += x * x
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		return v
	}
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = x / norm
	}
	return out
}

func cosine(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

func median(vals []float64) float64 {
	n := len(vals)
	if n == 0 {
		return 0
	}
	// Simple selection (not full sort for efficiency with small N)
	// For our use case N < 20000, insertion sort is fine
	sorted := make([]float64, n)
	copy(sorted, vals)
	for i := 1; i < n; i++ {
		key := sorted[i]
		j := i - 1
		for j >= 0 && sorted[j] > key {
			sorted[j+1] = sorted[j]
			j--
		}
		sorted[j+1] = key
	}
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}
