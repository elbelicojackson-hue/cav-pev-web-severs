// Distance metrics used by the recommendation engine.

package recommend

import "math"

// MethodologyDistance is the Jensen-Shannon divergence between two
// (prior_source_tag, inference_method_tag) joint distributions, normalized
// to [0, 1] (JS over base-2 log is bounded by 1).
//
// Two agents that publish with identical methodology mix → 0.
// Two agents with completely disjoint mixes → 1.
func MethodologyDistance(a, b map[string]float64) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	keys := map[string]struct{}{}
	for k := range a {
		keys[k] = struct{}{}
	}
	for k := range b {
		keys[k] = struct{}{}
	}

	pa := normalize(a, keys)
	pb := normalize(b, keys)

	// M = 0.5 * (P + Q)
	m := make(map[string]float64, len(keys))
	for k := range keys {
		m[k] = 0.5 * (pa[k] + pb[k])
	}
	js := 0.5*klDivergenceLog2(pa, m) + 0.5*klDivergenceLog2(pb, m)
	if js < 0 {
		return 0
	}
	if js > 1 {
		return 1
	}
	return js
}

// DomainOverlap is the Jaccard similarity of the two agents' domain sets.
// 0 means no shared domain; 1 means identical sets.
func DomainOverlap(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for k := range a {
		if _, ok := b[k]; ok {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func normalize(d map[string]float64, keys map[string]struct{}) map[string]float64 {
	total := 0.0
	for _, v := range d {
		if v > 0 {
			total += v
		}
	}
	out := make(map[string]float64, len(keys))
	if total == 0 {
		uniform := 1.0 / float64(len(keys))
		for k := range keys {
			out[k] = uniform
		}
		return out
	}
	for k := range keys {
		v := d[k]
		if v < 0 {
			v = 0
		}
		out[k] = v / total
	}
	return out
}

func klDivergenceLog2(p, q map[string]float64) float64 {
	const eps = 1e-9
	var kl float64
	for k, pv := range p {
		qv := q[k] + eps
		pv += eps
		kl += pv * math.Log2(pv/qv)
	}
	if kl < 0 {
		return 0
	}
	return kl
}
