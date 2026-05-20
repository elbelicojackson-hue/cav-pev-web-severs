package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/anthropic-cav/cav-node/internal/audit"
	"github.com/anthropic-cav/cav-node/internal/handler"
	"github.com/anthropic-cav/cav-node/internal/jcs"
	"github.com/anthropic-cav/cav-node/internal/ratelimit"
	"github.com/anthropic-cav/cav-node/internal/store"
	"github.com/anthropic-cav/cav-node/internal/webhook"
)

func setupTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "praxons")
	os.MkdirAll(storeDir, 0o755)
	auditPath := filepath.Join(dir, "audit.ndjson")

	s := store.NewFSStore(storeDir)
	a := audit.NewNDJSONLog(auditPath)
	l := ratelimit.New(10)
	r := webhook.NewRelay()

	mux := handler.NewRouter(s, a, l, r)
	ts := httptest.NewServer(mux)
	return ts, dir
}

func buildTestPraxon(t *testing.T) ([]byte, string) {
	t.Helper()

	// Generate Ed25519 keypair
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	// Build did:key from public key
	// multicodec ed25519-pub = 0xed01
	multicodec := append([]byte{0xed, 0x01}, pub...)
	did := "did:key:z" + base58Encode(multicodec)

	// Build praxon body (without praxon_id and signature)
	body := map[string]interface{}{
		"version":      "1.0",
		"praxon_class": "operational",
		"issuer":       did,
		"issued_at":    "2026-05-19T12:00:00Z",
		"claim": map[string]interface{}{
			"causal_skeleton": map[string]interface{}{
				"subject":              "unhandled_promise",
				"relation":            "causes",
				"object":              "runtime_error",
				"mechanism_hypothesis": "Async function without await drops the Promise",
				"strength":            0.85,
			},
			"uncertainty_geometry": map[string]interface{}{
				"confidence":                 0.85,
				"counterfactual_neighborhood": "Would not apply if function is fire-and-forget by design",
				"known_failure_modes":         []string{"intentional fire-and-forget"},
			},
			"methodology": map[string]interface{}{
				"prior_source_tag":     "demonstration",
				"inference_method_tag": "pattern_recognition",
				"data_source_hashes":   []string{},
			},
			"falsifiability": map[string]interface{}{
				"would_be_retracted_if": "Counter-example where unhandled promise is intentional and correct",
			},
		},
		"grounding": []map[string]interface{}{
			{
				"type":                     "demonstration_trace",
				"trace_hash":              "abc123",
				"task_description":        "Review PR #42 for unhandled promises",
				"reasoning_steps_summary": "1. Scan async functions 2. Check await usage 3. Found bug in fetchUser()",
				"outcome":                "success",
			},
		},
		"provenance": map[string]interface{}{
			"derived_from": []string{},
		},
	}

	// Compute praxon_id
	bodyJSON, _ := json.Marshal(body)
	canonical, _ := jcs.Canonicalize(bodyJSON)
	hash := sha256.Sum256(canonical)
	praxonID := hex.EncodeToString(hash[:])

	// Add praxon_id to body
	body["praxon_id"] = praxonID

	// Sign (body with praxon_id, without signature)
	bodyWithID, _ := json.Marshal(body)
	canonicalForSig, _ := jcs.Canonicalize(bodyWithID)
	sig := ed25519.Sign(priv, canonicalForSig)
	sigB64 := base64.RawURLEncoding.EncodeToString(sig)

	// Add signature
	body["signature"] = sigB64

	final, _ := json.Marshal(body)
	return final, praxonID
}

func TestHealthEndpoint(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["protocol"] != "cav-praxon" {
		t.Fatalf("expected protocol=cav-praxon, got %s", result["protocol"])
	}
}

func TestPublishAndFetch(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	praxonJSON, praxonID := buildTestPraxon(t)

	// Publish
	resp, err := http.Post(ts.URL+"/api/praxon", "application/json", bytes.NewReader(praxonJSON))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("publish failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	// Fetch
	resp2, err := http.Get(ts.URL + "/api/praxon/" + praxonID)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != 200 {
		t.Fatalf("fetch failed: status=%d", resp2.StatusCode)
	}

	fetched, _ := io.ReadAll(resp2.Body)
	if !bytes.Equal(fetched, praxonJSON) {
		t.Fatal("fetched content does not match published content")
	}
}

func TestPublishRejectsEmptyGrounding(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	// Minimal invalid praxon with empty grounding
	invalid := `{"version":"1.0","praxon_class":"operational","issuer":"did:key:zTest","issued_at":"2026-01-01T00:00:00Z","claim":{},"grounding":[],"provenance":{"derived_from":[]},"praxon_id":"fake","signature":"fake"}`

	resp, err := http.Post(ts.URL+"/api/praxon", "application/json", bytes.NewReader([]byte(invalid)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", resp.StatusCode)
	}
}

func TestFetchNotFound(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/praxon/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// base58Encode encodes bytes to base58btc (Bitcoin alphabet).
func base58Encode(data []byte) string {
	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

	// Count leading zeros
	var leadingZeros int
	for _, b := range data {
		if b != 0 {
			break
		}
		leadingZeros++
	}

	// Convert to base58
	size := len(data)*138/100 + 1
	buf := make([]byte, size)
	var length int

	for _, b := range data {
		carry := int(b)
		for i := 0; i < length || carry != 0; i++ {
			if i < length {
				carry += int(buf[i]) << 8
			}
			buf[i] = byte(carry % 58)
			carry /= 58
			if i >= length {
				length = i + 1
			}
		}
	}

	// Build result
	result := make([]byte, 0, leadingZeros+length)
	for i := 0; i < leadingZeros; i++ {
		result = append(result, alphabet[0])
	}
	for i := length - 1; i >= 0; i-- {
		result = append(result, alphabet[buf[i]])
	}

	return string(result)
}
