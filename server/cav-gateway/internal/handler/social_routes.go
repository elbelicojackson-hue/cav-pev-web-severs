// Social-trust HTTP routes for the CAV gateway.
//
// Mounted by main.go. Requires a JWT-authenticated citizen for any write
// operation. State-gated: trust mutations are only allowed when the citizen's
// effective state is `active` (canary subsystem from M3 sets probation; until
// it ships, all migrated citizens are active).

package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/anthropic-cav/cav-gateway/internal/auth"
	"github.com/anthropic-cav/cav-gateway/internal/citizen"
	"github.com/anthropic-cav/cav-gateway/internal/social/canary"
	"github.com/anthropic-cav/cav-gateway/internal/social/digest"
	"github.com/anthropic-cav/cav-gateway/internal/social/recommend"
	"github.com/anthropic-cav/cav-gateway/internal/social/reputation"
	"github.com/anthropic-cav/cav-gateway/internal/social/risk"
	"github.com/anthropic-cav/cav-gateway/internal/social/trust"
	"github.com/anthropic-cav/cav-gateway/internal/social/visibility"
)

// Deps bundles the social-trust dependencies. The owning service constructs
// it once at startup and passes it into the route registrations below.
type Deps struct {
	Citizens *citizen.PersistentRegistry
	Trust    *trust.Store
	Risk     *risk.Engine
	Audit    *risk.AuditStore
	Rep      *reputation.Store

	// M3 — canary subsystem (optional; may be nil if probation isn't wired)
	Canary    *canary.Assigner
	CanaryPool *canary.Pool

	// M4 — digest subsystem (optional)
	Digest *digest.Store

	// M5 — recommendation + visibility (optional)
	Recommend  *recommend.Engine
	Bandit     *recommend.Bandit
	Feedback   *recommend.FeedbackStore
	Visibility *visibility.Store
}

// Register mounts all /v1/social/* routes onto mux, wrapping write paths with
// the JWT middleware.
func Register(mux *http.ServeMux, jm *auth.JWTManager, d Deps) {
	mux.HandleFunc("POST /v1/social/trust/preview", auth.WithAuth(jm, d.previewTrust))
	mux.HandleFunc("POST /v1/social/trust", auth.WithAuth(jm, d.addTrust))
	mux.HandleFunc("DELETE /v1/social/trust", auth.WithAuth(jm, d.revokeTrust))
	mux.HandleFunc("GET /v1/social/trust", auth.WithAuth(jm, d.listTrust))
	mux.HandleFunc("GET /v1/social/trust/{target}", auth.WithAuth(jm, d.listTrustOf))
	mux.HandleFunc("GET /v1/social/risk/audit/{hash}", d.getAudit)
	mux.HandleFunc("GET /v1/social/reputation/{did}", d.getReputation)

	if d.Canary != nil {
		mux.HandleFunc("POST /v1/social/probation/start", auth.WithAuth(jm, d.startProbation))
		mux.HandleFunc("GET /v1/social/probation/status", auth.WithAuth(jm, d.probationStatus))
		mux.HandleFunc("GET /v1/social/probation/tasks", auth.WithAuth(jm, d.probationTasks))
		mux.HandleFunc("POST /v1/social/probation/submit", auth.WithAuth(jm, d.probationSubmit))
	}
	if d.Digest != nil {
		mux.HandleFunc("POST /v1/social/digest", auth.WithAuth(jm, d.submitDigest))
		mux.HandleFunc("GET /v1/social/digest/{did}", d.getLatestDigest)
	}
	if d.Recommend != nil {
		mux.HandleFunc("GET /v1/social/recommend", auth.WithAuth(jm, d.listRecommendations))
		mux.HandleFunc("POST /v1/social/recommend/{id}/feedback", auth.WithAuth(jm, d.recommendFeedback))
	}
	if d.Visibility != nil {
		mux.HandleFunc("PUT /v1/social/visibility", auth.WithAuth(jm, d.setVisibility))
		mux.HandleFunc("GET /v1/social/visibility/{did}", auth.WithAuth(jm, d.getVisibility))
	}
}

// === Preview ===

type previewReq struct {
	Subject string          `json:"subject"`
	Kind    trust.TrustKind `json:"kind"`
	Domain  string          `json:"domain,omitempty"`
}

func (d Deps) previewTrust(w http.ResponseWriter, r *http.Request) {
	requester := didFrom(r)
	var req previewReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid body")
		return
	}
	if req.Subject == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "subject required")
		return
	}
	if !req.Kind.IsValid() {
		writeErr(w, http.StatusBadRequest, "invalid_request", "kind must be cognitive or social")
		return
	}
	v, err := d.Risk.Compute(r.Context(), requester, req.Subject)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "risk_compute", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// === Add ===

type addTrustReq struct {
	Subject          string          `json:"subject"`
	Kind             trust.TrustKind `json:"kind"`
	Domain           string          `json:"domain,omitempty"`
	Weight           float64         `json:"weight"`
	AcceptRiskClass  string          `json:"accept_risk_class,omitempty"` // "high" | "critical" requires explicit consent
}

func (d Deps) addTrust(w http.ResponseWriter, r *http.Request) {
	requester := didFrom(r)
	if state := d.Citizens.EffectiveState(requester); state != citizen.StateActive {
		writeErr(w, http.StatusForbidden, "not_active",
			"only active citizens may build trust; current state="+string(state))
		return
	}

	var req addTrustReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid body")
		return
	}

	v, err := d.Risk.Compute(r.Context(), requester, req.Subject)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "risk_compute", err.Error())
		return
	}

	// Caller must opt in if the recommendation is to defer/reject.
	if (v.Recommendation == risk.RecDefer || v.Recommendation == risk.RecReject) &&
		req.AcceptRiskClass != v.RiskClass {
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"error": map[string]string{
				"code":    "risk_consent_required",
				"message": "recommendation=" + v.Recommendation + "; resubmit with accept_risk_class=" + v.RiskClass,
			},
			"risk": v,
		})
		return
	}

	snap, err := risk.Stamp(v)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "stamp", err.Error())
		return
	}

	edge := &trust.TrustEdge{
		From: requester, To: req.Subject,
		Kind: req.Kind, Domain: req.Domain,
		Weight: req.Weight,
		RiskSnapshot: trust.RiskVectorSnapshot{
			VectorHash:     snap.VectorHash,
			RiskClass:      snap.RiskClass,
			Recommendation: snap.Recommendation,
			AggregateScore: snap.AggregateScore,
			AuditRef:       snap.AuditRef,
		},
	}
	got, err := d.Trust.AddTrust(edge)
	if err != nil {
		if errors.Is(err, trust.ErrAlreadyExists) {
			writeErr(w, http.StatusConflict, "already_exists", err.Error())
			return
		}
		writeErr(w, http.StatusBadRequest, "add_trust", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, got)
}

// === Revoke ===

type revokeReq struct {
	Subject string          `json:"subject"`
	Kind    trust.TrustKind `json:"kind"`
	Domain  string          `json:"domain,omitempty"`
	Reason  string          `json:"reason,omitempty"`
}

func (d Deps) revokeTrust(w http.ResponseWriter, r *http.Request) {
	requester := didFrom(r)
	var req revokeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid body")
		return
	}
	got, err := d.Trust.RevokeTrust(requester, req.Kind, req.Domain, req.Subject, req.Reason)
	if err != nil {
		if errors.Is(err, trust.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not_found", err.Error())
			return
		}
		writeErr(w, http.StatusBadRequest, "revoke", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, got)
}

// === List ===

func (d Deps) listTrust(w http.ResponseWriter, r *http.Request) {
	requester := didFrom(r)
	q := r.URL.Query()
	f := trust.Filter{
		Kind:           trust.TrustKind(q.Get("kind")),
		Domain:         q.Get("domain"),
		IncludeRevoked: q.Get("include_revoked") == "true",
	}
	edges := d.Trust.Edges(requester, f)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"edges": edges,
		"count": len(edges),
	})
}

// === Audit fetch ===

func (d Deps) getAudit(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	if hash == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "hash required")
		return
	}
	v, err := d.Audit.Get(hash)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "audit_get", err.Error())
		return
	}
	if v == nil {
		writeErr(w, http.StatusNotFound, "not_found", "no audit record for hash")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// === Reputation fetch ===

func (d Deps) getReputation(w http.ResponseWriter, r *http.Request) {
	did := r.PathValue("did")
	if did == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "did required")
		return
	}
	v := d.Rep.Get(did)
	writeJSON(w, http.StatusOK, v)
}

// === helpers ===

func didFrom(r *http.Request) string {
	v := r.Context().Value(auth.CitizenDIDKey)
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]interface{}{
		"error": map[string]string{
			"code":    code,
			"message": msg,
		},
	})
}


// === Probation (M3) ===

type startProbationReq struct {
	Domains []string `json:"domains,omitempty"` // override capability domains
	Count   int      `json:"count,omitempty"`   // 3..5 per spec; 0 → default 3
}

func (d Deps) startProbation(w http.ResponseWriter, r *http.Request) {
	did := didFrom(r)
	if did == "" {
		writeErr(w, http.StatusUnauthorized, "no_identity", "missing DID")
		return
	}

	// Pull the citizen's declared capability domains as default.
	c := d.Citizens.Get(did)
	cit, ok := c.(*citizen.Citizen)
	domains := []string{}
	if ok && cit.Capabilities != nil {
		domains = append(domains, cit.Capabilities.HypothesisKinds...)
	}

	var req startProbationReq
	_ = json.NewDecoder(r.Body).Decode(&req) // body is optional
	if len(req.Domains) > 0 {
		domains = req.Domains
	}
	count := req.Count
	if count == 0 {
		count = 3
	}

	tasks, err := d.Canary.AssignTasks(did, domains, count)
	if err != nil {
		switch {
		case errors.Is(err, canary.ErrAlreadyAssigned):
			writeErr(w, http.StatusConflict, "already_assigned", err.Error())
		case errors.Is(err, canary.ErrCooldownActive):
			writeErr(w, http.StatusTooEarly, "cooldown", err.Error())
		default:
			writeErr(w, http.StatusInternalServerError, "assign", err.Error())
		}
		return
	}

	// Move the citizen registry's State to probation so other endpoints
	// (e.g. /trust) gate writes.
	d.Citizens.SetState(did, citizen.StateProbation, nil)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"tasks": tasks,
		"count": len(tasks),
	})
}

func (d Deps) probationStatus(w http.ResponseWriter, r *http.Request) {
	did := didFrom(r)
	if did == "" {
		writeErr(w, http.StatusUnauthorized, "no_identity", "missing DID")
		return
	}
	st := d.Canary.Status(did)
	writeJSON(w, http.StatusOK, st)
}

func (d Deps) probationTasks(w http.ResponseWriter, r *http.Request) {
	did := didFrom(r)
	if did == "" {
		writeErr(w, http.StatusUnauthorized, "no_identity", "missing DID")
		return
	}
	tasks, err := d.Canary.AssignedTasks(did)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "tasks", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tasks": tasks,
		"count": len(tasks),
	})
}

type probationSubmitReq struct {
	TaskID            string   `json:"task_id"`
	Conclusion        string   `json:"conclusion"`
	PriorSourceTag    string   `json:"prior_source_tag,omitempty"`
	InferenceMethodTag string  `json:"inference_method_tag,omitempty"`
	GroundingTags     []string `json:"grounding_tags,omitempty"`
	HasMethodology    bool     `json:"has_methodology"`
	HasGrounding      bool     `json:"has_grounding"`
	HasFalsifiability bool     `json:"has_falsifiability"`
}

func (d Deps) probationSubmit(w http.ResponseWriter, r *http.Request) {
	did := didFrom(r)
	if did == "" {
		writeErr(w, http.StatusUnauthorized, "no_identity", "missing DID")
		return
	}
	if state := d.Citizens.EffectiveState(did); state != citizen.StateProbation {
		writeErr(w, http.StatusForbidden, "not_in_probation",
			"submit is only valid while State=probation; current state="+string(state))
		return
	}

	var req probationSubmitReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid body")
		return
	}
	sub := &canary.Submission{
		TaskID:             req.TaskID,
		Conclusion:         req.Conclusion,
		PriorSourceTag:     req.PriorSourceTag,
		InferenceMethodTag: req.InferenceMethodTag,
		GroundingTags:      req.GroundingTags,
		HasMethodology:     req.HasMethodology,
		HasGrounding:       req.HasGrounding,
		HasFalsifiability:  req.HasFalsifiability,
		SubmittedAt:        time.Now(),
	}
	out, err := d.Canary.Submit(did, sub)
	if err != nil {
		switch {
		case errors.Is(err, canary.ErrTaskNotAssigned):
			writeErr(w, http.StatusForbidden, "not_assigned", err.Error())
		case errors.Is(err, canary.ErrAlreadySubmitted):
			writeErr(w, http.StatusConflict, "already_submitted", err.Error())
		default:
			writeErr(w, http.StatusInternalServerError, "submit", err.Error())
		}
		return
	}

	// Apply the assigner's desired state to the citizen registry.
	if out.Complete && out.Desired != nil {
		switch out.Desired.State {
		case "active":
			d.Citizens.SetState(did, citizen.StateActive, nil)
		case "restricted":
			d.Citizens.SetState(did, citizen.StateRestricted, out.Desired.NextRetryAt)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"result":   out.Result,
		"complete": out.Complete,
		"desired":  out.Desired,
	})
}


// === Digest (M4) ===

func (d Deps) submitDigest(w http.ResponseWriter, r *http.Request) {
	did := didFrom(r)
	if did == "" {
		writeErr(w, http.StatusUnauthorized, "no_identity", "missing DID")
		return
	}
	var body digest.BehavioralDigest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid body")
		return
	}
	// The agent's claimed DID on the digest must match the JWT-authenticated
	// DID — otherwise an authenticated agent could submit digests on behalf
	// of others.
	if body.DID == "" {
		body.DID = did
	}
	if body.DID != did {
		writeErr(w, http.StatusForbidden, "did_mismatch",
			"digest DID must match authenticated identity")
		return
	}

	// Verify against the citizen's registered public key, if known.
	expectedPubKey := ""
	if c, ok := d.Citizens.Get(did).(*citizen.Citizen); ok {
		expectedPubKey = c.PubKey
	}
	if err := digest.Verify(&body, expectedPubKey); err != nil {
		writeErr(w, http.StatusUnauthorized, "verify", err.Error())
		return
	}

	// Persist (period-monotonicity enforced inside Put).
	if err := d.Digest.Put(&body); err != nil {
		if errors.Is(err, digest.ErrPeriodRegression) {
			writeErr(w, http.StatusConflict, "period_regression", err.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, "store", err.Error())
		return
	}

	// A fresh digest implicitly clears the inactive flag.
	if d.Rep != nil {
		d.Rep.SetInactive(did, false)
		// Also fold the public stats into the citizen's behavioral subvector
		// so consumers can read them without joining against the digest store.
		d.Rep.SetBehavioral(did, reputation.BehavioralSubvector{
			ConformityIndex:       body.VoteAlignmentWithMajority,
			ChallengeSuccessRate:  0, // unknown until M5+ wiring
			DiversityContribution: body.SignalDiversityEntropy,
			SampleSize:            body.SignalCount,
		})
	}

	// First-ever digest path: register the public key on the citizen so
	// future verifications can pin it.
	if expectedPubKey == "" && body.PublicKey != "" {
		d.Citizens.SetPubKey(did, body.PublicKey)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":           true,
		"period_start": body.PeriodStart,
		"period_end":   body.PeriodEnd,
	})
}

func (d Deps) getLatestDigest(w http.ResponseWriter, r *http.Request) {
	did := r.PathValue("did")
	if did == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "did required")
		return
	}
	latest, err := d.Digest.Latest(did)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "store", err.Error())
		return
	}
	if latest == nil {
		writeErr(w, http.StatusNotFound, "not_found", "no digest for did")
		return
	}
	writeJSON(w, http.StatusOK, latest)
}

// suppress unused-import warnings on time when digest gate isn't wired yet.
var _ = time.Time{}


// === M5 — Visibility-gated peer trust list ===

// mutualAdapter bridges trust.HasMutualTrust into the visibility package's
// MutualTrustChecker contract without coupling the two packages.
type mutualAdapter struct{ store *trust.Store }

func (m mutualAdapter) HasMutual(a, b string) bool {
	return trust.HasMutualTrust(m.store, a, b)
}

// listTrustOf returns another citizen's trust graph if visibility allows.
func (d Deps) listTrustOf(w http.ResponseWriter, r *http.Request) {
	viewer := didFrom(r)
	target := r.PathValue("target")
	if target == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "target required")
		return
	}
	if d.Visibility != nil {
		if err := d.Visibility.Decide(viewer, target, mutualAdapter{store: d.Trust}); err != nil {
			writeErr(w, http.StatusForbidden, "hidden", err.Error())
			return
		}
	}
	q := r.URL.Query()
	f := trust.Filter{
		Kind:           trust.TrustKind(q.Get("kind")),
		Domain:         q.Get("domain"),
		IncludeRevoked: q.Get("include_revoked") == "true",
	}
	edges := d.Trust.Edges(target, f)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"target": target,
		"edges":  edges,
		"count":  len(edges),
	})
}

// === M5 — Recommendations ===

func (d Deps) listRecommendations(w http.ResponseWriter, r *http.Request) {
	requester := didFrom(r)
	if requester == "" {
		writeErr(w, http.StatusUnauthorized, "no_identity", "missing DID")
		return
	}
	limit := 5
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := parsePositiveInt(v); err == nil && n > 0 {
			limit = n
		}
	}
	recs, err := d.Recommend.Generate(r.Context(), requester, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "recommend", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"recommendations": recs,
		"count":           len(recs),
	})
}

type recommendFeedbackReq struct {
	Strategy             string  `json:"strategy"`
	Outcome              float64 `json:"outcome,omitempty"`           // optional immediate reward
	BaselineConformity   float64 `json:"baseline_conformity"`         // for delayed observation
	BaselineDiversity    float64 `json:"baseline_diversity"`
	BaselineChallengeWin float64 `json:"baseline_challenge_win"`
	Subject              string  `json:"subject,omitempty"`
}

func (d Deps) recommendFeedback(w http.ResponseWriter, r *http.Request) {
	requester := didFrom(r)
	if requester == "" {
		writeErr(w, http.StatusUnauthorized, "no_identity", "missing DID")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "recommendation id required")
		return
	}
	var body recommendFeedbackReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid body")
		return
	}
	// Immediate-reward path (e.g. quick rejection of a bad recommendation).
	if body.Outcome != 0 && d.Bandit != nil && body.Strategy != "" {
		d.Bandit.Record(body.Strategy, body.Outcome)
	}
	// Delayed-observation path (acceptance → schedule for 30 days out).
	if d.Feedback != nil && body.Strategy != "" {
		now := time.Now()
		_ = d.Feedback.ScheduleObservation(recommend.FeedbackRecord{
			RecommendationID:     id,
			Strategy:             body.Strategy,
			Requester:            requester,
			Subject:              body.Subject,
			BaselineAt:           now,
			ObserveAt:            now.Add(recommend.ObservationWindow),
			BaselineConformity:   body.BaselineConformity,
			BaselineDiversity:    body.BaselineDiversity,
			BaselineChallengeWin: body.BaselineChallengeWin,
		})
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// === M5 — Visibility ===

type setVisibilityReq struct {
	TrustGraphVisibility visibility.Mode `json:"trust_graph_visibility"`
}

func (d Deps) setVisibility(w http.ResponseWriter, r *http.Request) {
	did := didFrom(r)
	if did == "" {
		writeErr(w, http.StatusUnauthorized, "no_identity", "missing DID")
		return
	}
	var body setVisibilityReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid body")
		return
	}
	if err := d.Visibility.Set(did, body.TrustGraphVisibility); err != nil {
		writeErr(w, http.StatusBadRequest, "visibility", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"did":                    did,
		"trust_graph_visibility": body.TrustGraphVisibility,
	})
}

func (d Deps) getVisibility(w http.ResponseWriter, r *http.Request) {
	target := r.PathValue("did")
	if target == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "did required")
		return
	}
	mode := d.Visibility.Get(target)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"did":                    target,
		"trust_graph_visibility": mode,
	})
}

// === Helpers ===

func parsePositiveInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errors.New("not a positive int")
		}
		n = n*10 + int(c-'0')
		if n > 1000 {
			return 1000, nil
		}
	}
	return n, nil
}
