// Package social — central registry of the seven invariants from
// cav-social-trust/design.md §11.
//
// This file is the single source of truth for the invariant *contract*. Each
// invariant is a named guard function that the relevant write path must call
// before persisting. The actual storage logic lives in the per-domain
// packages (trust, reputation, canary, digest, etc.); this file is the
// audit-friendly index that documents WHAT must hold and WHY.
//
// Invariants:
//
//   I1. Trust edge dual-track uniqueness — same (from, to, kind, domain)
//       can have at most ONE non-revoked edge.
//   I2. Cognitive must have domain; Social must NOT have domain.
//   I3. Reputation mutates only via reputation.Event records.
//   I4. Risk audit records are immutable once written.
//   I5. Canary ground truth never crosses package boundaries via JSON.
//   I6. Behavioral digests are agent-self-signed; gateway never signs.
//   I7. Crystallized Praxons remain Always-Challengeable (no sealed flag).
//
// Each guard function below either returns nil (invariant holds) or an
// error describing the violation. Guards are intentionally narrow — they
// validate the *input* to a write path; the storage layer is responsible
// for ensuring the guard is called.

package social

import (
	"errors"
	"fmt"
	"strings"

	"github.com/anthropic-cav/cav-gateway/internal/social/canary"
	"github.com/anthropic-cav/cav-gateway/internal/social/digest"
	"github.com/anthropic-cav/cav-gateway/internal/social/trust"
)

// === I1 + I2: Trust edge structural invariants ===

// AssertTrustEdgeStructural enforces I1 (dual-track uniqueness checked at
// the Store layer via cache lookup; we delegate to TrustEdge.Validate()
// which already covers I2 + structural rules) for a fresh edge.
//
// Returns the wrapped store error if any validation fails.
func AssertTrustEdgeStructural(e *trust.TrustEdge) error {
	if e == nil {
		return errors.New("invariant I1/I2: nil trust edge")
	}
	if err := e.Validate(); err != nil {
		return fmt.Errorf("invariant I1/I2 violated: %w", err)
	}
	return nil
}

// === I3: Reputation event-only mutation ===

// AssertReputationEventOnly is a documentation guard. We rely on the
// reputation package's API surface (Apply, Bootstrap, ProcessGroundTruth)
// to be the only paths that mutate Vector data. Direct calls to
// store.putVectorLocked are package-private and only invoked from Apply().
//
// This function exists so test suites and audits can call it explicitly,
// even though the actual enforcement is structural (Go's package-private
// access).
func AssertReputationEventOnly(callPath string) error {
	allowed := []string{
		"reputation.Apply",
		"reputation.ApplyBatch",
		"reputation.ProcessGroundTruth",
		"reputation.Bootstrap",
		// SetInactive / SetBehavioral are derived liveness signals and do
		// not need to flow through Event — they are explicitly NOT mutations
		// of the score itself.
		"reputation.SetInactive",
		"reputation.SetBehavioral",
	}
	for _, ok := range allowed {
		if callPath == ok {
			return nil
		}
	}
	return fmt.Errorf("invariant I3 violated: reputation mutation must flow through Event; got %q", callPath)
}

// === I4: Risk audit immutability ===

// AssertRiskAuditImmutable is the documentation/test guard for the
// audit-write path. The actual immutability is enforced by AuditStore.Persist
// which short-circuits on an existing hash:
//
//   if _, err := txn.Get(auditKey); err == nil { return nil /* no-op */ }
//
// We expose this assertion so a test can pass two non-equal vectors with
// the same intended hash and verify the second one is rejected/ignored.
func AssertRiskAuditImmutable(existing, incoming []byte) error {
	if len(existing) == 0 {
		// First write — always allowed.
		return nil
	}
	if string(existing) == string(incoming) {
		return nil
	}
	return errors.New("invariant I4 violated: risk audit record cannot be modified")
}

// === I5: Canary ground truth opacity ===

// AssertCanaryGroundTruthHidden inspects a JSON-encoded CanaryTask payload
// and reports an error if any of the fields that should never appear in a
// client-facing serialization are present.
//
// Fields that MUST NOT appear in client wire format:
//   - "ground_truth"
//   - "groundTruth"
//   - any conclusion / accepted_alternatives / required_methodology /
//     required_grounding_tags inside a public envelope
//
// This is a defensive cross-check: the primary defense is the unexported
// `groundTruth` field on CanaryTask + the JSON tag "-". This guard exists
// so any future hand-rolled JSON path can be smoke-tested.
func AssertCanaryGroundTruthHidden(jsonBytes []byte) error {
	body := string(jsonBytes)
	for _, banned := range []string{`"ground_truth"`, `"groundTruth"`} {
		if strings.Contains(body, banned) {
			return fmt.Errorf("invariant I5 violated: canary wire payload contains forbidden token %s", banned)
		}
	}
	return nil
}

// AssertCanaryTaskSafe is the typed counterpart — returns an error if a
// CanaryTask still has its private groundTruth populated AND the caller
// intends to ship it externally. Use Sanitize() before calling externally
// and pass through this guard for paranoia.
func AssertCanaryTaskSafe(t *canary.CanaryTask) error {
	if t == nil {
		return errors.New("invariant I5: nil canary task")
	}
	gt := t.GroundTruthRef()
	if gt.Conclusion != "" || len(gt.AcceptedAlternatives) > 0 ||
		len(gt.RequiredGroundingTags) > 0 ||
		len(gt.RequiredMethodology.PriorSourceTags) > 0 ||
		len(gt.RequiredMethodology.InferenceMethodTags) > 0 {
		return errors.New("invariant I5 violated: canary task carries ground truth — call Sanitize() before serializing to a client")
	}
	return nil
}

// === I6: Digest self-signed ===

// AssertDigestSelfSigned verifies that the gateway has a non-empty
// signature + public key on the digest. Full Ed25519 verification is the
// digest package's Verify(); this guard documents the contract that the
// gateway never produces signatures itself.
func AssertDigestSelfSigned(d *digest.BehavioralDigest) error {
	if d == nil {
		return errors.New("invariant I6: nil digest")
	}
	if d.Signature == "" || d.PublicKey == "" {
		return errors.New("invariant I6 violated: digest must carry agent's signature and public key")
	}
	return nil
}

// === I7: Always-challengeable Praxons ===

// AssertChallengeable rejects any field name on a Praxon-shaped record
// that would imply "sealed" / "immutable" / "final". The crystallization
// state machine in thread/crystallize.go publishes/upgrades records using
// only the `level` field, which is mutable by design.
func AssertChallengeable(fieldName string) error {
	bannedTokens := []string{"sealed", "immutable", "final", "frozen"}
	low := strings.ToLower(fieldName)
	for _, b := range bannedTokens {
		if strings.Contains(low, b) {
			return fmt.Errorf("invariant I7 violated: praxon field %q implies un-challengeable state", fieldName)
		}
	}
	return nil
}
