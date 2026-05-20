// Recommendation `ProfilesProvider` adapter wired to the gateway's stores.
//
// In Phase 1 we don't yet have a per-citizen "methodology distribution" — the
// downstream signal store records praxon publication, but the projection
// (count by prior_source_tag × inference_method_tag) requires reading the
// signal corpus and joining against the praxon types in cav-node. For now
// we surface every active citizen with an empty methodology distribution
// + the trust-graph-derived domain set, which lets the engine produce
// exploratory recommendations without crashing. Once T15+ digest payloads
// are flowing, the projection can be enriched here without touching the
// recommender.

package handler

import (
	"context"

	"github.com/anthropic-cav/cav-gateway/internal/citizen"
	"github.com/anthropic-cav/cav-gateway/internal/social/recommend"
	"github.com/anthropic-cav/cav-gateway/internal/social/trust"
)

// SocialProfilesProvider is the recommend.ProfilesProvider implementation.
type SocialProfilesProvider struct {
	Citizens *citizen.PersistentRegistry
	Trust    *trust.Store
}

// NewProfilesProvider builds a Phase 1 provider.
func NewProfilesProvider(c *citizen.PersistentRegistry, t *trust.Store) *SocialProfilesProvider {
	return &SocialProfilesProvider{Citizens: c, Trust: t}
}

// List returns every active citizen as a candidate trustee.
func (p *SocialProfilesProvider) List(ctx context.Context) ([]recommend.SourceProfile, error) {
	all := p.Citizens.All()
	out := make([]recommend.SourceProfile, 0, len(all))
	for _, c := range all {
		state := c.State
		if state == "" {
			state = citizen.StateActive
		}
		if state != citizen.StateActive {
			continue
		}
		out = append(out, p.toProfile(c.DID))
	}
	return out, nil
}

// Get returns the profile for one citizen. Unknown DIDs get an empty
// profile so the engine can still compute distance/overlap without panicking.
func (p *SocialProfilesProvider) Get(ctx context.Context, did string) (recommend.SourceProfile, error) {
	return p.toProfile(did), nil
}

// AlreadyTrusted returns the set of subjects `requester` has any non-revoked
// trust edge to (Cognitive or Social).
func (p *SocialProfilesProvider) AlreadyTrusted(ctx context.Context, requester string) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	for _, e := range p.Trust.Edges(requester, trust.Filter{}) {
		out[e.To] = struct{}{}
	}
	return out, nil
}

func (p *SocialProfilesProvider) toProfile(did string) recommend.SourceProfile {
	domains := map[string]struct{}{}
	for _, d := range trust.DomainsOfCognitiveTrust(p.Trust, did) {
		domains[d] = struct{}{}
	}
	return recommend.SourceProfile{
		DID:                     did,
		MethodologyDistribution: map[string]float64{},
		Domains:                 domains,
	}
}
