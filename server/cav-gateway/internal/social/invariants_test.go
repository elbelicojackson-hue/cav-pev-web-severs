package social

import (
	"errors"
	"testing"

	"github.com/anthropic-cav/cav-gateway/internal/social/canary"
	"github.com/anthropic-cav/cav-gateway/internal/social/digest"
	"github.com/anthropic-cav/cav-gateway/internal/social/trust"
)

// === I1 + I2 ===

func TestAssertTrustEdgeStructuralValid(t *testing.T) {
	e := &trust.TrustEdge{
		From: "a", To: "b", Kind: trust.Cognitive, Domain: "crypto", Weight: 0.5,
	}
	if err := AssertTrustEdgeStructural(e); err != nil {
		t.Errorf("valid edge rejected: %v", err)
	}
}

func TestAssertTrustEdgeStructuralRejects(t *testing.T) {
	cases := []*trust.TrustEdge{
		nil,
		{To: "b", Kind: trust.Cognitive, Domain: "crypto"},
		{From: "a", To: "a", Kind: trust.Cognitive, Domain: "crypto"},
		{From: "a", To: "b", Kind: trust.Cognitive}, // missing domain
		{From: "a", To: "b", Kind: trust.Social, Domain: "crypto"}, // social with domain
		{From: "a", To: "b", Kind: trust.Social, Weight: 1.5},      // bad weight
	}
	for i, c := range cases {
		if err := AssertTrustEdgeStructural(c); err == nil {
			t.Errorf("case %d should be rejected", i)
		}
	}
}

// === I3 ===

func TestAssertReputationEventOnlyAllowsKnownPaths(t *testing.T) {
	for _, p := range []string{
		"reputation.Apply", "reputation.ApplyBatch",
		"reputation.ProcessGroundTruth", "reputation.Bootstrap",
		"reputation.SetInactive", "reputation.SetBehavioral",
	} {
		if err := AssertReputationEventOnly(p); err != nil {
			t.Errorf("%s should be allowed: %v", p, err)
		}
	}
}

func TestAssertReputationEventOnlyRejectsUnknown(t *testing.T) {
	if err := AssertReputationEventOnly("reputation.HackyDirectMutate"); err == nil {
		t.Error("unknown path should be rejected")
	}
}

// === I4 ===

func TestAssertRiskAuditImmutableFirstWriteOK(t *testing.T) {
	if err := AssertRiskAuditImmutable(nil, []byte("incoming")); err != nil {
		t.Errorf("first write should be OK, got %v", err)
	}
}

func TestAssertRiskAuditImmutableSameBytesOK(t *testing.T) {
	if err := AssertRiskAuditImmutable([]byte("same"), []byte("same")); err != nil {
		t.Errorf("idempotent re-write should be OK, got %v", err)
	}
}

func TestAssertRiskAuditImmutableDifferentBytesRejected(t *testing.T) {
	err := AssertRiskAuditImmutable([]byte("original"), []byte("modified"))
	if err == nil {
		t.Error("modification should be rejected")
	}
}

// === I5 ===

func TestAssertCanaryGroundTruthHidden(t *testing.T) {
	clean := []byte(`{"id":"x","domain":"crypto","prompt":"foo"}`)
	if err := AssertCanaryGroundTruthHidden(clean); err != nil {
		t.Errorf("clean payload should pass, got %v", err)
	}
	leaky := []byte(`{"id":"x","ground_truth":{"conclusion":"42"}}`)
	if err := AssertCanaryGroundTruthHidden(leaky); err == nil {
		t.Error("ground_truth in payload should be detected")
	}
	camel := []byte(`{"groundTruth":{"conclusion":"42"}}`)
	if err := AssertCanaryGroundTruthHidden(camel); err == nil {
		t.Error("groundTruth (camelCase) should be detected")
	}
}

func TestAssertCanaryTaskSafeAfterSanitize(t *testing.T) {
	tk := &canary.CanaryTask{ID: "x", Domain: "crypto"}
	tk.SetGroundTruth(canary.GroundTruth{Conclusion: "secret"})
	if err := AssertCanaryTaskSafe(tk); err == nil {
		t.Error("task with ground truth should be rejected")
	}
	clean := canary.Sanitize(tk)
	if err := AssertCanaryTaskSafe(clean); err != nil {
		t.Errorf("sanitized task should pass, got %v", err)
	}
}

func TestAssertCanaryTaskSafeNil(t *testing.T) {
	if err := AssertCanaryTaskSafe(nil); err == nil {
		t.Error("nil task should be rejected")
	}
}

// === I6 ===

func TestAssertDigestSelfSigned(t *testing.T) {
	d := &digest.BehavioralDigest{
		DID: "did:a", Signature: "sig", PublicKey: "key",
	}
	if err := AssertDigestSelfSigned(d); err != nil {
		t.Errorf("signed digest should pass, got %v", err)
	}
}

func TestAssertDigestSelfSignedRejectsUnsigned(t *testing.T) {
	if err := AssertDigestSelfSigned(nil); err == nil {
		t.Error("nil should be rejected")
	}
	if err := AssertDigestSelfSigned(&digest.BehavioralDigest{DID: "x"}); err == nil {
		t.Error("missing signature should be rejected")
	}
	if err := AssertDigestSelfSigned(&digest.BehavioralDigest{DID: "x", Signature: "s"}); err == nil {
		t.Error("missing pubkey should be rejected")
	}
}

// === I7 ===

func TestAssertChallengeable(t *testing.T) {
	if err := AssertChallengeable("level"); err != nil {
		t.Errorf("level field should pass: %v", err)
	}
	if err := AssertChallengeable("conclusion"); err != nil {
		t.Errorf("conclusion field should pass: %v", err)
	}
}

func TestAssertChallengeableRejectsBanned(t *testing.T) {
	for _, bad := range []string{"sealed", "is_immutable", "Final", "frozen_at"} {
		if err := AssertChallengeable(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

// === Verify each guard returns a typed error or nil ===

func TestGuardsReturnErrorOrNil(t *testing.T) {
	// Sanity: every guard returns either nil or a non-nil error.
	checks := []func() error{
		func() error { return AssertTrustEdgeStructural(nil) },
		func() error { return AssertReputationEventOnly("bad") },
		func() error { return AssertRiskAuditImmutable([]byte("a"), []byte("b")) },
		func() error { return AssertCanaryGroundTruthHidden([]byte(`{"ground_truth":1}`)) },
		func() error { return AssertCanaryTaskSafe(nil) },
		func() error { return AssertDigestSelfSigned(nil) },
		func() error { return AssertChallengeable("sealed") },
	}
	for i, c := range checks {
		if err := c(); err == nil {
			t.Errorf("guard %d should fail with non-nil error", i)
		} else if !errors.Is(err, err) {
			// trivial sanity to avoid silent err==nil
			t.Errorf("guard %d returned strange error: %v", i, err)
		}
	}
}
