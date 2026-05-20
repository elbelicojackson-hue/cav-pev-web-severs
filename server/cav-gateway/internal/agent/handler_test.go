package agent

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropic-cav/cav-gateway/internal/auth"
	"github.com/anthropic-cav/cav-gateway/internal/citizen"
	"github.com/anthropic-cav/cav-gateway/internal/signal"
)

// stubHub fakes the Hub interface; ClientCount returns one entry per "client".
type stubHub struct{ n int }

func (s stubHub) ClientCount() []string {
	out := make([]string, s.n)
	for i := range out {
		out[i] = "x"
	}
	return out
}

// testFixture wires a fresh registry, on-disk signal store, JWT, and HTTP test
// server with the agent routes mounted. t.TempDir handles cleanup.
type testFixture struct {
	t        *testing.T
	srv      *httptest.Server
	jm       *auth.JWTManager
	registry *citizen.PersistentRegistry
	signals  *signal.Store
	did      string
	fp       string
	token    string
}

func newFixture(t *testing.T, hubClients int) *testFixture {
	t.Helper()
	dir := t.TempDir()
	registry, err := citizen.NewPersistentRegistry(dir + "/citizens")
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	t.Cleanup(func() { _ = registry.Close() })

	store, err := signal.NewStore(dir + "/signals")
	if err != nil {
		t.Fatalf("signal store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	jm := auth.NewJWTManager([]byte("test-secret"))

	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	multikey := append([]byte{0xed, 0x01}, pub...)
	did := "did:key:z" + base64.RawURLEncoding.EncodeToString(multikey)
	fp := auth.FingerprintFromDID(did)
	if fp == "" {
		t.Fatalf("fingerprint derivation returned empty string for valid did:key")
	}
	registry.EnsureRegistered(did, fp)

	tok, _, err := jm.Issue(did, 1)
	if err != nil {
		t.Fatalf("jwt issue: %v", err)
	}

	mux := http.NewServeMux()
	Register(mux, jm, Deps{
		Citizens: registry,
		Signals:  store,
		Hub:      stubHub{n: hubClients},
		Version:  "test",
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &testFixture{
		t: t, srv: srv, jm: jm,
		registry: registry, signals: store,
		did: did, fp: fp, token: tok,
	}
}

// === HTTP helpers ===========================================================

func (f *testFixture) get(path string, authed bool) *http.Response {
	f.t.Helper()
	req, _ := http.NewRequest("GET", f.srv.URL+path, nil)
	if authed {
		req.Header.Set("Authorization", "Bearer "+f.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		f.t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

// post sends body as application/json with the test JWT.
func (f *testFixture) post(path string, body interface{}) *http.Response {
	f.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	return f.postRaw(path, "application/json", &buf)
}

// postRaw lets a test override Content-Type and body to exercise the strict
// validation paths.
func (f *testFixture) postRaw(path, contentType string, body io.Reader) *http.Response {
	f.t.Helper()
	req, _ := http.NewRequest("POST", f.srv.URL+path, body)
	req.Header.Set("Authorization", "Bearer "+f.token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		f.t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

// decodeError parses the canonical error envelope.
func decodeError(t *testing.T, resp *http.Response) errorBody {
	t.Helper()
	var env errorEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	return env.Error
}

// === Manifest ===============================================================

func TestManifest_PublicAndStable(t *testing.T) {
	f := newFixture(t, 0)
	resp := f.get("/v1/agent/manifest", false /* no auth */)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("response Content-Type=%q", got)
	}
	var m Manifest
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m.ProtocolVersion == "" || m.GatewayVersion == "" || m.HeartbeatHint <= 0 {
		t.Fatalf("incomplete manifest: %+v", m)
	}
	if m.Limits.MaxSignalBytes != int(maxBodyBytes) {
		t.Errorf("manifest limit must match enforced cap: got %d want %d",
			m.Limits.MaxSignalBytes, maxBodyBytes)
	}
	if _, ok := m.Endpoints["context"]; !ok {
		t.Fatalf("manifest missing context endpoint: %+v", m.Endpoints)
	}
	if len(m.SignalTypes) == 0 {
		t.Fatalf("empty signal types")
	}
}

func TestManifest_RejectsUnknownQueryParam(t *testing.T) {
	f := newFixture(t, 0)
	resp := f.get("/v1/agent/manifest?surprise=1", false)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
	e := decodeError(t, resp)
	if e.Code != errCodeUnknownParam {
		t.Errorf("code=%q want %q", e.Code, errCodeUnknownParam)
	}
}

// === Context ================================================================

func TestContext_RequiresAuth(t *testing.T) {
	f := newFixture(t, 2)
	resp := f.get("/v1/agent/context", false)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestContext_OneShotBootstrap(t *testing.T) {
	f := newFixture(t, 3)

	parent := &signal.EntropicSignal{
		Type: signal.SignalLearning,
		From: f.fp,
		PosteriorShift: &signal.PosteriorShift{
			Subject: "x", Relation: "causes", Object: "y",
			PriorConfidence: 0.1, PosteriorConfidence: 0.7,
			DeltaBits: 1.0, Direction: "strengthen",
		},
	}
	if err := f.signals.Append(parent); err != nil {
		t.Fatalf("append: %v", err)
	}

	resp := f.get("/v1/agent/context?feed=10&mine=5", true)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var ctx ContextResponse
	if err := json.NewDecoder(resp.Body).Decode(&ctx); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ctx.PeersOnline != 3 {
		t.Errorf("peers_online=%d want 3", ctx.PeersOnline)
	}
	if len(ctx.MyRecent) != 1 || ctx.MyRecent[0].ID != parent.ID {
		t.Errorf("my_recent=%+v", ctx.MyRecent)
	}
	if len(ctx.Feed) < 1 {
		t.Errorf("feed empty")
	}
	if ctx.Heartbeat <= 0 {
		t.Errorf("heartbeat=%d", ctx.Heartbeat)
	}
}

func TestContext_RejectsUnknownQueryParam(t *testing.T) {
	f := newFixture(t, 0)
	resp := f.get("/v1/agent/context?bogus=1", true)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
	e := decodeError(t, resp)
	if e.Code != errCodeUnknownParam || e.Field != "bogus" {
		t.Errorf("envelope=%+v", e)
	}
}

func TestContext_RejectsRepeatedQueryParam(t *testing.T) {
	f := newFixture(t, 0)
	resp := f.get("/v1/agent/context?feed=1&feed=2", true)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
	e := decodeError(t, resp)
	if e.Code != errCodeDuplicateParam {
		t.Errorf("code=%q", e.Code)
	}
}

func TestContext_RejectsNonIntegerQueryValue(t *testing.T) {
	f := newFixture(t, 0)
	resp := f.get("/v1/agent/context?feed=abc", true)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
	e := decodeError(t, resp)
	if e.Code != errCodeInvalidQuery || e.Field != "feed" {
		t.Errorf("envelope=%+v", e)
	}
}

// === Heartbeat — strict body & content-type =================================

func TestHeartbeat_EmptyBodyIsLivenessPing(t *testing.T) {
	f := newFixture(t, 1)
	// Empty body POST — Content-Type may be omitted because nothing is being sent.
	resp := f.postRaw("/v1/agent/heartbeat", "" /* no content type */, nil)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var hb HeartbeatResponse
	if err := json.NewDecoder(resp.Body).Decode(&hb); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !hb.OK || hb.PeersOnline != 1 {
		t.Errorf("unexpected: %+v", hb)
	}
	if hb.State == "" {
		t.Errorf("state should be reported (defaulted to active for legacy citizens)")
	}
}

func TestHeartbeat_UpdatesCapabilities(t *testing.T) {
	f := newFixture(t, 0)
	body := HeartbeatRequest{
		Capabilities: &citizen.Capabilities{Nickname: "robo", Languages: []string{"go"}},
		Status:       "working",
		Note:         "starting analysis",
	}
	resp := f.post("/v1/agent/heartbeat", body)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	c, ok := f.registry.Get(f.did).(*citizen.Citizen)
	if !ok {
		t.Fatalf("citizen lookup failed")
	}
	if c.Capabilities == nil || c.Capabilities.Nickname != "robo" {
		t.Errorf("capabilities not persisted: %+v", c.Capabilities)
	}
}

func TestHeartbeat_RejectsWrongContentType(t *testing.T) {
	f := newFixture(t, 0)
	body := []byte(`{"status":"idle"}`)
	resp := f.postRaw("/v1/agent/heartbeat", "text/plain", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("want 415, got %d", resp.StatusCode)
	}
	e := decodeError(t, resp)
	if e.Code != errCodeContentType {
		t.Errorf("code=%q want %q", e.Code, errCodeContentType)
	}
}

func TestHeartbeat_RejectsApplicationXJsonStrictly(t *testing.T) {
	// "application/x-json" or "text/json" must NOT be accepted; the contract
	// says exactly application/json.
	f := newFixture(t, 0)
	resp := f.postRaw("/v1/agent/heartbeat", "text/json",
		bytes.NewReader([]byte(`{"status":"idle"}`)))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("want 415, got %d", resp.StatusCode)
	}
}

func TestHeartbeat_AcceptsCharsetUTF8(t *testing.T) {
	f := newFixture(t, 0)
	resp := f.postRaw("/v1/agent/heartbeat", "application/json; charset=utf-8",
		bytes.NewReader([]byte(`{"status":"idle"}`)))
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestHeartbeat_RejectsCharsetNonUTF8(t *testing.T) {
	f := newFixture(t, 0)
	resp := f.postRaw("/v1/agent/heartbeat", "application/json; charset=latin-1",
		bytes.NewReader([]byte(`{"status":"idle"}`)))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("want 415, got %d", resp.StatusCode)
	}
}

func TestHeartbeat_RejectsInvalidJSON(t *testing.T) {
	f := newFixture(t, 0)
	resp := f.postRaw("/v1/agent/heartbeat", "application/json",
		bytes.NewReader([]byte(`{not json`)))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
	e := decodeError(t, resp)
	if e.Code != errCodeInvalidJSON {
		t.Errorf("code=%q want %q", e.Code, errCodeInvalidJSON)
	}
}

func TestHeartbeat_RejectsUnknownTopLevelField(t *testing.T) {
	f := newFixture(t, 0)
	resp := f.postRaw("/v1/agent/heartbeat", "application/json",
		bytes.NewReader([]byte(`{"status":"idle","extra":"nope"}`)))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
	e := decodeError(t, resp)
	if e.Code != errCodeUnknownField {
		t.Errorf("code=%q want %q", e.Code, errCodeUnknownField)
	}
}

func TestHeartbeat_RejectsTrailingData(t *testing.T) {
	f := newFixture(t, 0)
	// Two concatenated objects must be rejected.
	resp := f.postRaw("/v1/agent/heartbeat", "application/json",
		bytes.NewReader([]byte(`{"status":"idle"}{"status":"idle"}`)))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
	e := decodeError(t, resp)
	if e.Code != errCodeInvalidJSON {
		t.Errorf("code=%q", e.Code)
	}
}

func TestHeartbeat_RejectsOversizedBody(t *testing.T) {
	f := newFixture(t, 0)
	// Build a payload whose JSON encoding exceeds maxBodyBytes.
	huge := strings.Repeat("a", int(maxBodyBytes)+1024)
	body := fmt.Sprintf(`{"status":"idle","note":%q}`, huge)
	resp := f.postRaw("/v1/agent/heartbeat", "application/json",
		bytes.NewReader([]byte(body)))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413, got %d", resp.StatusCode)
	}
	e := decodeError(t, resp)
	if e.Code != errCodeBodyTooLarge {
		t.Errorf("code=%q want %q", e.Code, errCodeBodyTooLarge)
	}
}

func TestHeartbeat_RejectsInvalidStatusEnum(t *testing.T) {
	f := newFixture(t, 0)
	resp := f.postRaw("/v1/agent/heartbeat", "application/json",
		bytes.NewReader([]byte(`{"status":"on-fire"}`)))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
	e := decodeError(t, resp)
	if e.Code != errCodeValidation || e.Field != "status" {
		t.Errorf("envelope=%+v", e)
	}
}

func TestHeartbeat_RejectsOversizedNickname(t *testing.T) {
	f := newFixture(t, 0)
	tooLong := strings.Repeat("x", maxNicknameLen+1)
	body := fmt.Sprintf(`{"capabilities":{"nickname":%q}}`, tooLong)
	resp := f.postRaw("/v1/agent/heartbeat", "application/json",
		bytes.NewReader([]byte(body)))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
	e := decodeError(t, resp)
	if e.Code != errCodeValidation || e.Field != "capabilities.nickname" {
		t.Errorf("envelope=%+v", e)
	}
}

func TestHeartbeat_RejectsTooManyCapabilityItems(t *testing.T) {
	f := newFixture(t, 0)
	tools := make([]string, maxCapabilitiesItems+1)
	for i := range tools {
		tools[i] = "t"
	}
	body := map[string]interface{}{
		"capabilities": map[string]interface{}{
			"tools": tools,
		},
	}
	resp := f.post("/v1/agent/heartbeat", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
	e := decodeError(t, resp)
	if e.Code != errCodeValidation || e.Field != "capabilities.tools" {
		t.Errorf("envelope=%+v", e)
	}
}

func TestHeartbeat_RejectsEmptyCapabilityEntry(t *testing.T) {
	f := newFixture(t, 0)
	resp := f.postRaw("/v1/agent/heartbeat", "application/json",
		bytes.NewReader([]byte(`{"capabilities":{"languages":["go",""]}}`)))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
	e := decodeError(t, resp)
	if e.Code != errCodeValidation || e.Field != "capabilities.languages[1]" {
		t.Errorf("envelope=%+v", e)
	}
}

// === Inbox ==================================================================

func TestInbox_ReturnsRepliesToMySignals(t *testing.T) {
	f := newFixture(t, 0)

	parent := &signal.EntropicSignal{
		Type: signal.SignalLearning,
		From: f.fp,
		PosteriorShift: &signal.PosteriorShift{
			Subject: "a", Relation: "causes", Object: "b",
			PriorConfidence: 0.2, PosteriorConfidence: 0.8,
			DeltaBits: 1.5, Direction: "strengthen",
		},
	}
	if err := f.signals.Append(parent); err != nil {
		t.Fatalf("append parent: %v", err)
	}

	reply := &signal.EntropicSignal{
		Type:      signal.SignalChallenge,
		From:      "CAV-PEER-PEER-PEER-PEER",
		InReplyTo: parent.ID,
		PosteriorShift: &signal.PosteriorShift{
			Subject: "a", Relation: "causes", Object: "b",
			PriorConfidence: 0.8, PosteriorConfidence: 0.4,
			DeltaBits: 0.5, Direction: "weaken",
		},
	}
	if err := f.signals.Append(reply); err != nil {
		t.Fatalf("append reply: %v", err)
	}

	resp := f.get("/v1/agent/inbox?limit=10", true)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var inbox InboxResponse
	if err := json.NewDecoder(resp.Body).Decode(&inbox); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if inbox.Count != 1 || len(inbox.Items) != 1 {
		t.Fatalf("want 1 item, got %d: %+v", inbox.Count, inbox.Items)
	}
	if inbox.Items[0].Parent.ID != parent.ID || inbox.Items[0].Reply.ID != reply.ID {
		t.Errorf("wrong pairing: %+v", inbox.Items[0])
	}
}

func TestInbox_LimitClamped(t *testing.T) {
	f := newFixture(t, 0)
	resp := f.get("/v1/agent/inbox?limit=99999", true)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestInbox_RejectsNonIntegerLimit(t *testing.T) {
	f := newFixture(t, 0)
	resp := f.get("/v1/agent/inbox?limit=many", true)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
	e := decodeError(t, resp)
	if e.Code != errCodeInvalidQuery || e.Field != "limit" {
		t.Errorf("envelope=%+v", e)
	}
}
