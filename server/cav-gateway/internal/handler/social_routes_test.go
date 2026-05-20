package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/anthropic-cav/cav-gateway/internal/auth"
	"github.com/anthropic-cav/cav-gateway/internal/citizen"
	"github.com/anthropic-cav/cav-gateway/internal/social/reputation"
	"github.com/anthropic-cav/cav-gateway/internal/social/risk"
	"github.com/anthropic-cav/cav-gateway/internal/social/trust"
)

// minimalProvider yields just enough data to produce a deterministic vector
// for handler tests without dragging in the entire signal subsystem.
type minimalProvider struct {
	requesterDoms risk.DomainActivityVector
	subjectDoms   risk.DomainActivityVector
}

func (p *minimalProvider) SubjectPraxons(ctx context.Context, _ string) ([]risk.PraxonRecord, error) {
	return nil, nil
}
func (p *minimalProvider) SubjectChallenges(ctx context.Context, _ string) ([]risk.ChallengeRecord, error) {
	return nil, nil
}
func (p *minimalProvider) SubjectRetractions(ctx context.Context, _ string) ([]risk.RetractionRecord, error) {
	return nil, nil
}
func (p *minimalProvider) SubjectVotes(ctx context.Context, _ string) ([]risk.VoteRecord, error) {
	return nil, nil
}
func (p *minimalProvider) SubjectFingerprint(ctx context.Context, _ string) (risk.FingerprintFeatures, float64, error) {
	return risk.FingerprintFeatures{}, 0, nil
}
func (p *minimalProvider) OtherFingerprints(ctx context.Context, _ string) (map[string]risk.FingerprintFeatures, error) {
	return nil, nil
}
func (p *minimalProvider) SubjectActivity(ctx context.Context, _ string) ([]float64, int, error) {
	return nil, 0, nil
}
func (p *minimalProvider) NetworkBaselineActivity(ctx context.Context) ([]float64, error) {
	return nil, nil
}
func (p *minimalProvider) RequesterDomains(ctx context.Context, _ string) (risk.DomainActivityVector, error) {
	return p.requesterDoms, nil
}
func (p *minimalProvider) SubjectDomains(ctx context.Context, _ string) (risk.DomainActivityVector, error) {
	return p.subjectDoms, nil
}
func (p *minimalProvider) ExistingCorrelations(ctx context.Context, _, _ string) ([]float64, error) {
	return nil, nil
}

type testHarness struct {
	mux       *http.ServeMux
	jwt       *auth.JWTManager
	registry  *citizen.PersistentRegistry
	trustStor *trust.Store
	repStor   *reputation.Store
	auditStor *risk.AuditStore
}

func newHarness(t *testing.T) *testHarness {
	t.Helper()
	dir := t.TempDir()

	registry, err := citizen.NewPersistentRegistry(filepath.Join(dir, "citizens"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { registry.Close() })

	trustStor, err := trust.NewStore(filepath.Join(dir, "trust"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { trustStor.Close() })

	auditStor, err := risk.NewAuditStore(filepath.Join(dir, "audit"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { auditStor.Close() })

	repStor, err := reputation.NewStore(filepath.Join(dir, "rep"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repStor.Close() })

	prov := &minimalProvider{
		requesterDoms: risk.DomainActivityVector{},
		subjectDoms:   risk.DomainActivityVector{"crypto": 1},
	}
	engine, err := risk.NewEngine(prov, auditStor)
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	jm := auth.NewJWTManager([]byte("test-secret"))
	Register(mux, jm, Deps{
		Citizens: registry, Trust: trustStor, Risk: engine,
		Audit: auditStor, Rep: repStor,
	})

	return &testHarness{
		mux: mux, jwt: jm, registry: registry,
		trustStor: trustStor, repStor: repStor, auditStor: auditStor,
	}
}

func (h *testHarness) tokenFor(t *testing.T, did string) string {
	t.Helper()
	tok, _, err := h.jwt.Issue(did, 1)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func (h *testHarness) request(t *testing.T, method, path, token string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	return rec
}

// === Tests ===

func TestPreviewReturnsRiskVector(t *testing.T) {
	h := newHarness(t)
	requester := "did:cav:requester"
	subject := "did:cav:subject"
	h.registry.EnsureRegistered(requester, "fp-req")
	h.registry.EnsureRegistered(subject, "fp-subj")

	rec := h.request(t, "POST", "/v1/social/trust/preview", h.tokenFor(t, requester),
		previewReq{Subject: subject, Kind: trust.Cognitive, Domain: "crypto"})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var v risk.TrustRiskVector
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v.Requester != requester || v.Subject != subject {
		t.Errorf("requester/subject lost in preview: %+v", v)
	}
	if v.RiskClass == "" {
		t.Errorf("expected risk class to be populated")
	}
}

func TestPreviewRequiresAuth(t *testing.T) {
	h := newHarness(t)
	rec := h.request(t, "POST", "/v1/social/trust/preview", "",
		previewReq{Subject: "did:cav:s", Kind: trust.Cognitive, Domain: "crypto"})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestPreviewRejectsInvalidKind(t *testing.T) {
	h := newHarness(t)
	requester := "did:cav:r"
	h.registry.EnsureRegistered(requester, "fp-r")
	rec := h.request(t, "POST", "/v1/social/trust/preview", h.tokenFor(t, requester),
		previewReq{Subject: "did:cav:s", Kind: "weird", Domain: "crypto"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestAddTrustHappyPath(t *testing.T) {
	h := newHarness(t)
	requester := "did:cav:requester"
	subject := "did:cav:subject"
	h.registry.EnsureRegistered(requester, "fp-r")
	h.registry.EnsureRegistered(subject, "fp-s")

	// With minimal provider, recommendation will be defer due to insufficient
	// dimensions. Caller must provide accept_risk_class to override.
	body := addTrustReq{
		Subject: subject, Kind: trust.Cognitive, Domain: "crypto", Weight: 0.7,
		AcceptRiskClass: "low",
	}
	rec := h.request(t, "POST", "/v1/social/trust", h.tokenFor(t, requester), body)
	// 201 if recommendation==proceed (which it can be with score=0 from
	// insufficient data but pre-defer), or 409 if defer.
	if rec.Code == http.StatusConflict {
		// Re-submit with the right class
		var resp map[string]interface{}
		json.Unmarshal(rec.Body.Bytes(), &resp)
		risk := resp["risk"].(map[string]interface{})
		body.AcceptRiskClass = risk["risk_class"].(string)
		rec = h.request(t, "POST", "/v1/social/trust", h.tokenFor(t, requester), body)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	var got trust.TrustEdge
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.From != requester || got.To != subject {
		t.Errorf("edge identity wrong: %+v", got)
	}
	if got.RiskSnapshot.VectorHash == "" {
		t.Errorf("risk snapshot hash should be stamped")
	}

	// Audit record must be retrievable by the snapshot's hash.
	rec = h.request(t, "GET", "/v1/social/risk/audit/"+got.RiskSnapshot.VectorHash, "", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("audit lookup failed: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAddTrustRejectedWhenProbation(t *testing.T) {
	h := newHarness(t)
	requester := "did:cav:requester"
	h.registry.EnsureRegistered(requester, "fp-r")
	h.registry.SetState(requester, citizen.StateProbation, nil)

	body := addTrustReq{
		Subject: "did:cav:subject", Kind: trust.Cognitive, Domain: "crypto", Weight: 0.5,
	}
	rec := h.request(t, "POST", "/v1/social/trust", h.tokenFor(t, requester), body)
	if rec.Code != http.StatusForbidden {
		t.Errorf("probation citizen should be blocked, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestListReturnsOnlyRequestersEdges(t *testing.T) {
	h := newHarness(t)
	requester := "did:cav:r"
	h.registry.EnsureRegistered(requester, "fp-r")
	h.registry.EnsureRegistered("did:cav:other", "fp-o")

	// Plant edges directly through the store (bypassing risk gate).
	h.trustStor.AddTrust(&trust.TrustEdge{
		From: requester, To: "did:cav:b", Kind: trust.Cognitive, Domain: "crypto", Weight: 0.5,
	})
	h.trustStor.AddTrust(&trust.TrustEdge{
		From: requester, To: "did:cav:c", Kind: trust.Social, Weight: 0.3,
	})
	h.trustStor.AddTrust(&trust.TrustEdge{
		From: "did:cav:other", To: "did:cav:b", Kind: trust.Cognitive, Domain: "crypto", Weight: 0.9,
	})

	rec := h.request(t, "GET", "/v1/social/trust", h.tokenFor(t, requester), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp struct {
		Edges []trust.TrustEdge `json:"edges"`
		Count int               `json:"count"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Count != 2 {
		t.Errorf("expected 2 edges for requester, got %d (%+v)", resp.Count, resp.Edges)
	}
}

func TestRevokeReturnsNotFoundForUnknownEdge(t *testing.T) {
	h := newHarness(t)
	requester := "did:cav:r"
	h.registry.EnsureRegistered(requester, "fp-r")
	body := revokeReq{Subject: "did:cav:nobody", Kind: trust.Cognitive, Domain: "crypto"}
	rec := h.request(t, "DELETE", "/v1/social/trust", h.tokenFor(t, requester), body)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetReputationReturnsEmptyVectorForUnknown(t *testing.T) {
	h := newHarness(t)
	rec := h.request(t, "GET", "/v1/social/reputation/did:cav:unknown", "", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var v reputation.Vector
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v.DID != "did:cav:unknown" {
		t.Errorf("expected DID=did:cav:unknown, got %q", v.DID)
	}
}
