// Minimal DataProvider for the risk engine wired against the gateway's
// existing stores.
//
// Phase 1 scope: the social-trust stores (signal index, fingerprint cache,
// retraction log, etc.) the engine ideally consumes don't all exist yet —
// they're the responsibility of the canary, digest, and behavioral subsystems
// in M3-M4. Until those land, this provider returns minimal/zero data so the
// engine produces a `defer` recommendation due to insufficient sample size,
// which is the correct conservative behavior during bootstrap.

package handler

import (
	"context"

	"github.com/anthropic-cav/cav-gateway/internal/auth"
	"github.com/anthropic-cav/cav-gateway/internal/citizen"
	"github.com/anthropic-cav/cav-gateway/internal/signal"
	"github.com/anthropic-cav/cav-gateway/internal/social/risk"
	"github.com/anthropic-cav/cav-gateway/internal/social/trust"
)

// GatewayProvider implements risk.DataProvider against the gateway's stores.
type GatewayProvider struct {
	Citizens *citizen.PersistentRegistry
	Signals  *signal.Store
	Trust    *trust.Store
}

// NewGatewayProvider builds a DataProvider from the existing gateway stores.
func NewGatewayProvider(c *citizen.PersistentRegistry, s *signal.Store, t *trust.Store) *GatewayProvider {
	return &GatewayProvider{Citizens: c, Signals: s, Trust: t}
}

// SubjectPraxons returns the subject's recently published signals interpreted
// as praxon records. Phase 1 stub: no praxon-specific metadata yet, so we
// return empty (engine reports insufficient).
func (g *GatewayProvider) SubjectPraxons(ctx context.Context, subject string) ([]risk.PraxonRecord, error) {
	return nil, nil
}

// SubjectChallenges - Phase 1 stub. Challenge tracking lives in the praxon
// node, not the gateway, so we report none until the cross-service pipeline
// is wired.
func (g *GatewayProvider) SubjectChallenges(ctx context.Context, subject string) ([]risk.ChallengeRecord, error) {
	return nil, nil
}

// SubjectRetractions - Phase 1 stub.
func (g *GatewayProvider) SubjectRetractions(ctx context.Context, subject string) ([]risk.RetractionRecord, error) {
	return nil, nil
}

// SubjectVotes returns the subject's vote history derived from their signals
// of type endorsement / challenge. Phase 1: we walk signals BySender(fp)
// and project each into a VoteRecord; majority position is unknown without
// the consensus-episode index, so it's left empty.
func (g *GatewayProvider) SubjectVotes(ctx context.Context, subject string) ([]risk.VoteRecord, error) {
	fp := didToFingerprint(subject)
	if fp == "" {
		return nil, nil
	}
	sigs, err := g.Signals.BySender(fp, 200)
	if err != nil {
		return nil, err
	}
	out := make([]risk.VoteRecord, 0, len(sigs))
	for _, s := range sigs {
		if s == nil {
			continue
		}
		var pos string
		switch s.Type {
		case signal.SignalEndorsement:
			pos = "endorse"
		case signal.SignalChallenge:
			pos = "reject"
		default:
			continue
		}
		out = append(out, risk.VoteRecord{
			Position: pos,
			// MajorityPosition unknown at this layer; leave empty.
		})
	}
	return out, nil
}

// SubjectFingerprint - Phase 1 returns a degenerate fingerprint and a small
// age estimate so sybil similarity is computable but always reported as
// insufficient (age below MinHoursForSybilDetection).
func (g *GatewayProvider) SubjectFingerprint(ctx context.Context, subject string) (risk.FingerprintFeatures, float64, error) {
	return risk.FingerprintFeatures{}, 0, nil
}

// OtherFingerprints - Phase 1 stub.
func (g *GatewayProvider) OtherFingerprints(ctx context.Context, exclude string) (map[string]risk.FingerprintFeatures, error) {
	return nil, nil
}

// SubjectActivity - Phase 1 stub returning empty distribution.
func (g *GatewayProvider) SubjectActivity(ctx context.Context, subject string) ([]float64, int, error) {
	return nil, 0, nil
}

// NetworkBaselineActivity - Phase 1 stub.
func (g *GatewayProvider) NetworkBaselineActivity(ctx context.Context) ([]float64, error) {
	return nil, nil
}

// RequesterDomains derives the requester's domain activity from the existing
// trust graph. We walk the requester's outgoing Cognitive edges and produce
// a uniform distribution over their distinct domains.
func (g *GatewayProvider) RequesterDomains(ctx context.Context, requester string) (risk.DomainActivityVector, error) {
	doms := trust.DomainsOfCognitiveTrust(g.Trust, requester)
	if len(doms) == 0 {
		return risk.DomainActivityVector{}, nil
	}
	v := make(risk.DomainActivityVector, len(doms))
	share := 1.0 / float64(len(doms))
	for _, d := range doms {
		v[d] = share
	}
	return v, nil
}

// SubjectDomains - Phase 1: we don't track per-agent domain activity yet.
// Return empty so DiversityImpact reports insufficient on this dimension.
func (g *GatewayProvider) SubjectDomains(ctx context.Context, subject string) (risk.DomainActivityVector, error) {
	return risk.DomainActivityVector{}, nil
}

// ExistingCorrelations - Phase 1: behavioral correlation requires the digest
// subsystem (T15-T16). Return empty so EchoChamberDelta only reflects the
// subject's own conformity_index.
func (g *GatewayProvider) ExistingCorrelations(ctx context.Context, requester, subject string) ([]float64, error) {
	return nil, nil
}

// didToFingerprint converts a DID into the fingerprint our signal store
// indexes by. Returns empty string if the DID can't be parsed.
func didToFingerprint(did string) string {
	return auth.FingerprintFromDID(did)
}
