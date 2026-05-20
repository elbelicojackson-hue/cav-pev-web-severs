// Higher-level graph queries built on top of Store.
//
// These functions are stateless helpers — they operate on whatever Store
// instance you hand them. Kept separate from Store so the storage layer
// stays narrow (one-edge-at-a-time CRUD) while richer questions live here.

package trust

// HasMutualTrust reports whether `a` and `b` both trust each other under any
// kind/domain. Used by visibility.policy_test "mutual_only" and by future
// recommendation tie-breakers.
func HasMutualTrust(s *Store, a, b string) bool {
	if a == "" || b == "" || a == b {
		return false
	}
	atob := s.Edges(a, Filter{}) // any kind, exclude revoked (default)
	for _, e := range atob {
		if e.To == b {
			// Now check b → a
			btoa := s.Edges(b, Filter{})
			for _, e2 := range btoa {
				if e2.To == a {
					return true
				}
			}
			return false
		}
	}
	return false
}

// CognitiveTrustWeight returns the weight Agent `from` has placed on Agent
// `to` in the given domain, or 0 if no such (non-revoked) edge exists. This
// is what the convergence engine consumes when applying a trustor's filter
// to a trustee's reputation contribution.
func CognitiveTrustWeight(s *Store, from, to, domain string) float64 {
	e := s.Get(from, Cognitive, domain, to)
	if e == nil || e.IsRevoked() {
		return 0
	}
	return e.Weight
}

// SocialTrustWeight returns the global social-trust weight from `from` to
// `to`, or 0 if absent. Social trust must NOT influence convergence — this
// helper is for collaboration matching only.
func SocialTrustWeight(s *Store, from, to string) float64 {
	e := s.Get(from, Social, "", to)
	if e == nil || e.IsRevoked() {
		return 0
	}
	return e.Weight
}

// DomainsOfCognitiveTrust returns the distinct list of domains in which
// `from` has at least one non-revoked Cognitive edge. Used by the diversity
// recommendation engine (T17).
func DomainsOfCognitiveTrust(s *Store, from string) []string {
	seen := map[string]struct{}{}
	for _, e := range s.Edges(from, Filter{Kind: Cognitive}) {
		seen[e.Domain] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	return out
}
