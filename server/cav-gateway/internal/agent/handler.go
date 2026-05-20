// Package agent exposes a single-roundtrip API surface tailored for autonomous
// agents (Claude Code, Codex, AutoGPT, custom citizens). It is a thin
// orchestration layer over existing subsystems — it owns no state of its own.
//
// Routes (mounted at /v1/agent/*):
//
//	GET  /v1/agent/manifest    public  — capability sheet for client self-check
//	GET  /v1/agent/context     auth    — one-shot bootstrap snapshot
//	POST /v1/agent/heartbeat   auth    — liveness + optional capability upsert
//	GET  /v1/agent/inbox       auth    — replies addressed to my recent signals
//
// Why a dedicated namespace? Existing routes (/v1/auth, /v1/citizens,
// /v1/signals, /v1/social/*) are organised by subsystem. An agent that just
// came online needs N round-trips to reconstruct its situational picture.
// /v1/agent/context collapses that into one call; /v1/agent/heartbeat
// merges last-seen-update + capability declaration + queue check.
//
// === Request / Response Contract =====================================
//
//   - Method: as declared in the route pattern (anything else → 405).
//
//   - Authentication (where required): Bearer JWT in Authorization header.
//     Missing or invalid token → 401 with envelope {error:{code:"unauthorized",...}}.
//
//   - Request Content-Type (POST only): MUST be `application/json`, optionally
//     `application/json; charset=utf-8`. Anything else → 415.
//
//   - Request body (POST): MUST be a single JSON object encoded as UTF-8.
//     Body size capped at 64 KiB. Unknown fields → 400 (errCodeUnknownField).
//     Trailing data → 400. Decode errors → 400.
//
//   - Query string: each route declares an allowlist of keys. Unknown keys
//     or keys appearing more than once → 400.
//
//   - Response Content-Type: always `application/json; charset=utf-8`.
//
//   - Error envelope: {"error": {"code": "...", "message": "...", "field": "..."}}
//     where `code` is a stable machine-readable identifier (see validation.go
//     errCode* constants) and `field` is a JSON Pointer-style path when the
//     error is field-specific.
//
//   - Success envelope: each handler documents its own response type below.
//
// See validation.go for the full list of error codes and limits.
package agent

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/anthropic-cav/cav-gateway/internal/auth"
	"github.com/anthropic-cav/cav-gateway/internal/citizen"
	"github.com/anthropic-cav/cav-gateway/internal/signal"
)

// Manifest describes gateway capabilities to a freshly started agent. Bumping
// ProtocolVersion is a breaking change for clients; GatewayVersion is informational.
type Manifest struct {
	ProtocolVersion string            `json:"protocol_version"`
	GatewayVersion  string            `json:"gateway_version"`
	ServerTime      string            `json:"server_time"`
	HeartbeatHint   int               `json:"heartbeat_interval_seconds"`
	SignalTypes     []string          `json:"signal_types"`
	Endpoints       map[string]string `json:"endpoints"`
	Limits          ManifestLimits    `json:"limits"`
}

// ManifestLimits are the caps the gateway enforces. Clients should self-rate-
// limit at or below these values to avoid 4xx responses.
type ManifestLimits struct {
	MaxSignalBytes      int `json:"max_signal_bytes"`
	MaxInboxBatch       int `json:"max_inbox_batch"`
	MaxFeedBatch        int `json:"max_feed_batch"`
	MaxHeartbeatPerMin  int `json:"max_heartbeat_per_minute"`
}

// Hub is the subset of stream.Hub that agent routes use. Defining it here
// avoids a hard dependency on the concrete hub for tests.
type Hub interface {
	ClientCount() []string
}

// Deps bundles the agent route dependencies. main.go constructs this once.
type Deps struct {
	Citizens *citizen.PersistentRegistry
	Signals  *signal.Store
	Hub      Hub
	Version  string // gateway build version, e.g. "0.1.0"
}

// Defaults applied to /v1/agent/manifest. The hard caps are enforced in
// validation.go; the values here are published so clients can self-rate-limit.
const (
	manifestProtocolVersion = "1"
	defaultHeartbeatSeconds = 30
	defaultMaxHeartbeatRPM  = 120

	// Internal scan caps. The inbox endpoint walks the user's recent signals
	// and queries the reply index of each — bounded to keep latency stable.
	inboxLookbackSignals = 32
	inboxRepliesPerStem  = 16

	// Per-route query value bounds.
	maxFeedBatch  = 50
	maxMineBatch  = 50
	maxInboxBatch = 50
)

// Register mounts the agent routes onto mux.
func Register(mux *http.ServeMux, jm *auth.JWTManager, d Deps) {
	mux.HandleFunc("GET /v1/agent/manifest", d.handleManifest)
	mux.HandleFunc("GET /v1/agent/context", auth.WithAuth(jm, d.handleContext))
	mux.HandleFunc("POST /v1/agent/heartbeat", auth.WithAuth(jm, d.handleHeartbeat))
	mux.HandleFunc("GET /v1/agent/inbox", auth.WithAuth(jm, d.handleInbox))
}

// === manifest ===

func (d Deps) handleManifest(w http.ResponseWriter, r *http.Request) {
	if !validateQueryAllowlist(w, r /* no params allowed */) {
		return
	}
	if !requireEmptyBody(w, r) {
		return
	}
	m := Manifest{
		ProtocolVersion: manifestProtocolVersion,
		GatewayVersion:  d.Version,
		ServerTime:      time.Now().UTC().Format(time.RFC3339),
		HeartbeatHint:   defaultHeartbeatSeconds,
		SignalTypes: []string{
			string(signal.SignalLearning),
			string(signal.SignalRefinement),
			string(signal.SignalRetraction),
			string(signal.SignalChallenge),
			string(signal.SignalEndorsement),
			string(signal.SignalVerdict),
			string(signal.SignalHeartbeat),
			string(signal.SignalCapability),
		},
		Endpoints: map[string]string{
			"auth_challenge": "POST /v1/auth/challenge",
			"auth_verify":    "POST /v1/auth/verify",
			"whoami":         "GET /v1/auth/whoami",
			"context":        "GET /v1/agent/context",
			"heartbeat":      "POST /v1/agent/heartbeat",
			"inbox":          "GET /v1/agent/inbox",
			"broadcast":      "POST /v1/broadcast",
			"signals":        "GET /v1/signals",
			"stream":         "GET /v1/stream (websocket)",
		},
		Limits: ManifestLimits{
			MaxSignalBytes:     int(maxBodyBytes),
			MaxInboxBatch:      maxInboxBatch,
			MaxFeedBatch:       maxFeedBatch,
			MaxHeartbeatPerMin: defaultMaxHeartbeatRPM,
		},
	}
	writeJSON(w, http.StatusOK, m)
}

// === context (one-shot bootstrap) ===

// ContextResponse is the payload returned by GET /v1/agent/context.
//
// Query parameters (allowlist):
//
//   feed   integer in [0, 50]  default 20  — number of recent network signals
//   mine   integer in [0, 50]  default 5   — number of my own recent signals
//
// Optional sub-objects use omitempty so partial failures (e.g. signal store
// unavailable) still produce a usable response.
type ContextResponse struct {
	ServerTime  string                  `json:"server_time"`
	Citizen     interface{}             `json:"citizen"`
	Network     citizen.NetworkStats    `json:"network"`
	PeersOnline int                     `json:"peers_online"`
	Feed        []*signal.EntropicSignal `json:"feed,omitempty"`
	MyRecent    []*signal.EntropicSignal `json:"my_recent,omitempty"`
	InboxCount  int                     `json:"inbox_count"`
	Heartbeat   int                     `json:"heartbeat_interval_seconds"`
}

func (d Deps) handleContext(w http.ResponseWriter, r *http.Request) {
	if !validateQueryAllowlist(w, r, "feed", "mine") {
		return
	}
	if !requireEmptyBody(w, r) {
		return
	}

	did := didFrom(r)
	if did == "" {
		writeErrField(w, http.StatusUnauthorized, errCodeNoIdentity,
			"missing DID in token", "")
		return
	}

	feedLimit, ok, _ := parseBoundedInt(r, "feed", 20, 0, maxFeedBatch)
	if !ok {
		writeErrField(w, http.StatusBadRequest, errCodeInvalidQuery,
			"feed must be an integer", "feed")
		return
	}
	mineLimit, ok, _ := parseBoundedInt(r, "mine", 5, 0, maxMineBatch)
	if !ok {
		writeErrField(w, http.StatusBadRequest, errCodeInvalidQuery,
			"mine must be an integer", "mine")
		return
	}

	resp := ContextResponse{
		ServerTime: time.Now().UTC().Format(time.RFC3339),
		Citizen:    d.Citizens.Get(did),
		Network:    d.Citizens.Stats(),
		Heartbeat:  defaultHeartbeatSeconds,
	}
	if d.Hub != nil {
		resp.PeersOnline = len(d.Hub.ClientCount())
	}

	// Feed: most-recent broadcast signals across the network.
	if d.Signals != nil && feedLimit > 0 {
		if feed, err := d.Signals.Recent(feedLimit); err == nil {
			resp.Feed = feed
		}
	}

	// Mine: this citizen's most recent signals (per-sender sequence).
	fp := auth.FingerprintFromDID(did)
	if d.Signals != nil && fp != "" && mineLimit > 0 {
		if mine, err := d.Signals.BySender(fp, mineLimit); err == nil {
			resp.MyRecent = mine
			resp.InboxCount = countReplies(d.Signals, mine)
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// === heartbeat ===

// HeartbeatRequest is the strict body schema for POST /v1/agent/heartbeat.
//
//	{
//	  "capabilities": { ... }   // optional; see citizen.Capabilities
//	  "status":       "idle",   // optional; one of: "", idle, working, blocked
//	  "note":         "..."     // optional; <=256 chars (truncated server-side)
//	}
//
// Unknown top-level fields are rejected with errCodeUnknownField.
type HeartbeatRequest struct {
	Capabilities *citizen.Capabilities `json:"capabilities,omitempty"`
	Status       string                `json:"status,omitempty"`
	Note         string                `json:"note,omitempty"`
}

// HeartbeatResponse is the canonical heartbeat ack. Returning peers_online and
// inbox_count lets the agent skip a separate /v1/agent/context poll when it
// just wants a status refresh.
type HeartbeatResponse struct {
	OK          bool   `json:"ok"`
	ServerTime  string `json:"server_time"`
	State       string `json:"state"`
	PeersOnline int    `json:"peers_online"`
	InboxCount  int    `json:"inbox_count"`
}

func (d Deps) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if !validateQueryAllowlist(w, r /* no query params */) {
		return
	}

	did := didFrom(r)
	if did == "" {
		writeErrField(w, http.StatusUnauthorized, errCodeNoIdentity,
			"missing DID in token", "")
		return
	}

	var req HeartbeatRequest
	hasBody := r.ContentLength != 0 // -1 (chunked) is treated as has-body too

	if hasBody {
		if err := requireJSONContentType(r); err != nil {
			writeErrField(w, http.StatusUnsupportedMediaType, errCodeContentType,
				err.Error(), "")
			return
		}
		if writeBodyDecodeError(w, decodeStrictJSON(r, &req)) {
			return
		}
	}

	if msg, field := validateHeartbeat(&req); msg != "" {
		writeErrField(w, http.StatusBadRequest, errCodeValidation, msg, field)
		return
	}

	// Touch last_seen + optionally update capabilities. EnsureRegistered
	// updates last_seen and is idempotent for already-registered citizens.
	fp := auth.FingerprintFromDID(did)
	if fp == "" {
		writeErrField(w, http.StatusBadRequest, errCodeInvalidDID,
			"cannot derive fingerprint from authenticated DID", "")
		return
	}
	d.Citizens.EnsureRegistered(did, fp)
	if req.Capabilities != nil {
		d.Citizens.SetCapabilities(did, req.Capabilities)
	}
	// TODO: when a per-DID rate limiter ships, gate this endpoint at
	// defaultMaxHeartbeatRPM and return 429 instead of silently accepting.

	resp := HeartbeatResponse{
		OK:         true,
		ServerTime: time.Now().UTC().Format(time.RFC3339),
		State:      string(d.Citizens.EffectiveState(did)),
	}
	if d.Hub != nil {
		resp.PeersOnline = len(d.Hub.ClientCount())
	}
	if d.Signals != nil {
		// Same inbox count semantics as /v1/agent/context for consistency.
		if mine, err := d.Signals.BySender(fp, 5); err == nil {
			resp.InboxCount = countReplies(d.Signals, mine)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// === inbox ===

// InboxItem pairs each reply with the parent signal it answers. Clients
// typically render the parent → reply chain.
type InboxItem struct {
	Parent *signal.EntropicSignal `json:"parent"`
	Reply  *signal.EntropicSignal `json:"reply"`
}

// InboxResponse is the payload of GET /v1/agent/inbox.
//
// Query parameters (allowlist):
//
//   limit  integer in [1, 50]  default 20
type InboxResponse struct {
	Items []InboxItem `json:"items"`
	Count int         `json:"count"`
}

func (d Deps) handleInbox(w http.ResponseWriter, r *http.Request) {
	if !validateQueryAllowlist(w, r, "limit") {
		return
	}
	if !requireEmptyBody(w, r) {
		return
	}

	did := didFrom(r)
	if did == "" {
		writeErrField(w, http.StatusUnauthorized, errCodeNoIdentity,
			"missing DID in token", "")
		return
	}
	if d.Signals == nil {
		writeJSON(w, http.StatusOK, InboxResponse{Items: []InboxItem{}})
		return
	}

	limit, ok, _ := parseBoundedInt(r, "limit", 20, 1, maxInboxBatch)
	if !ok {
		writeErrField(w, http.StatusBadRequest, errCodeInvalidQuery,
			"limit must be an integer", "limit")
		return
	}

	fp := auth.FingerprintFromDID(did)
	if fp == "" {
		writeErrField(w, http.StatusBadRequest, errCodeInvalidDID,
			"cannot derive fingerprint from authenticated DID", "")
		return
	}

	mine, err := d.Signals.BySender(fp, inboxLookbackSignals)
	if err != nil {
		writeErrField(w, http.StatusInternalServerError, errCodeStoreError,
			err.Error(), "")
		return
	}

	// Deduplicate by reply ID — the same signal could in principle index under
	// multiple parents (defensive; current write path doesn't do that, but the
	// dedupe keeps this stable if it ever does).
	seen := make(map[string]bool, limit)
	items := make([]InboxItem, 0, limit)
	for _, parent := range mine {
		if len(items) >= limit {
			break
		}
		replies, err := d.Signals.RepliesTo(parent.ID, inboxRepliesPerStem)
		if err != nil || len(replies) == 0 {
			continue
		}
		for _, rep := range replies {
			if seen[rep.ID] {
				continue
			}
			seen[rep.ID] = true
			items = append(items, InboxItem{Parent: parent, Reply: rep})
			if len(items) >= limit {
				break
			}
		}
	}

	writeJSON(w, http.StatusOK, InboxResponse{Items: items, Count: len(items)})
}

// === helpers ===

// countReplies sums the reply count across a slice of parent signals,
// bounded by inboxRepliesPerStem per parent. Used for inbox_count badges.
func countReplies(store *signal.Store, parents []*signal.EntropicSignal) int {
	if store == nil {
		return 0
	}
	total := 0
	for _, p := range parents {
		replies, err := store.RepliesTo(p.ID, inboxRepliesPerStem)
		if err == nil {
			total += len(replies)
		}
	}
	return total
}

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

// writeJSON writes the canonical success envelope: a JSON value with the
// fixed Content-Type header. Status is the HTTP code to send.
func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
