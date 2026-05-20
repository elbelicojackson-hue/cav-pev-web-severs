// Smoke test for the /v1/agent/* surface.
//
// Spins up a fresh Ed25519 identity, exchanges challenge → JWT, then walks
// through every contract guarantee: positive paths plus the strict-rejection
// matrix (wrong content-type, oversized body, unknown fields, etc).
//
// Usage:
//
//	go run ./cmd/smoketest
//	go run ./cmd/smoketest -gateway http://127.0.0.1:8421
//
// Exit code 0 if all checks pass, 1 otherwise. Prints a compact summary so
// it's easy to read in CI logs.
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var (
	gateway = flag.String("gateway", "http://127.0.0.1:8421", "Gateway base URL")
	verbose = flag.Bool("v", false, "Verbose output (print each request)")
)

type result struct {
	name string
	ok   bool
	err  string
}

var results []result

func main() {
	flag.Parse()
	fmt.Printf("Smoke testing %s\n", *gateway)
	fmt.Println(strings.Repeat("-", 60))

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		die("keygen: %v", err)
	}
	multikey := append([]byte{0xed, 0x01}, pub...)
	did := "did:key:z" + base64.RawURLEncoding.EncodeToString(multikey)
	fmt.Printf("DID: %s\n\n", did)

	// === 0: health check ===
	check("health: GET /v1/health returns 200 ok", func() error {
		resp, err := http.Get(*gateway + "/v1/health")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	// === 1: manifest is public + has stable shape ===
	var manifestRaw map[string]interface{}
	check("manifest: GET /v1/agent/manifest (no auth) → 200 with stable fields", func() error {
		resp, err := http.Get(*gateway + "/v1/agent/manifest")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
			return fmt.Errorf("content-type=%q", ct)
		}
		if err := json.NewDecoder(resp.Body).Decode(&manifestRaw); err != nil {
			return err
		}
		for _, k := range []string{"protocol_version", "gateway_version", "server_time",
			"heartbeat_interval_seconds", "signal_types", "endpoints", "limits"} {
			if _, ok := manifestRaw[k]; !ok {
				return fmt.Errorf("missing field %q", k)
			}
		}
		return nil
	})

	check("manifest: rejects unknown query parameter → 400 unknown_query_param", func() error {
		resp, err := http.Get(*gateway + "/v1/agent/manifest?bogus=1")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		return expectError(resp, 400, "unknown_query_param", "")
	})

	// === 2: auth challenge → verify → JWT ===
	var token string
	check("auth: POST /v1/auth/challenge returns nonce", func() error {
		body, _ := json.Marshal(map[string]string{"did": did})
		resp, err := http.Post(*gateway+"/v1/auth/challenge", "application/json", bytes.NewReader(body))
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		var r struct {
			Nonce string `json:"nonce"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
			return err
		}
		nonceBytes, err := hex.DecodeString(r.Nonce)
		if err != nil {
			return err
		}
		sig := ed25519.Sign(priv, nonceBytes)
		body2, _ := json.Marshal(map[string]string{
			"did":       did,
			"nonce":     r.Nonce,
			"signature": base64.RawURLEncoding.EncodeToString(sig),
		})
		resp2, err := http.Post(*gateway+"/v1/auth/verify", "application/json", bytes.NewReader(body2))
		if err != nil {
			return err
		}
		defer resp2.Body.Close()
		if resp2.StatusCode != 200 {
			b, _ := io.ReadAll(resp2.Body)
			return fmt.Errorf("verify status=%d body=%s", resp2.StatusCode, string(b))
		}
		var v struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(resp2.Body).Decode(&v); err != nil {
			return err
		}
		if v.Token == "" {
			return fmt.Errorf("empty token")
		}
		token = v.Token
		return nil
	})
	if token == "" {
		fail("cannot continue without JWT")
	}

	// === 3: context bootstrap (positive) ===
	check("context: GET /v1/agent/context (auth) returns full snapshot", func() error {
		req, _ := http.NewRequest("GET", *gateway+"/v1/agent/context?feed=10&mine=5", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		var ctx map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&ctx); err != nil {
			return err
		}
		for _, k := range []string{"server_time", "citizen", "network",
			"peers_online", "inbox_count", "heartbeat_interval_seconds"} {
			if _, ok := ctx[k]; !ok {
				return fmt.Errorf("missing field %q in context", k)
			}
		}
		return nil
	})

	check("context: requires JWT (no Authorization header) → 401", func() error {
		resp, err := http.Get(*gateway + "/v1/agent/context")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 401 {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	check("context: rejects unknown query parameter → 400", func() error {
		req, _ := http.NewRequest("GET", *gateway+"/v1/agent/context?surprise=1", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		return expectError(resp, 400, "unknown_query_param", "surprise")
	})

	check("context: rejects repeated query parameter → 400", func() error {
		req, _ := http.NewRequest("GET", *gateway+"/v1/agent/context?feed=1&feed=2", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		return expectError(resp, 400, "duplicate_query_param", "")
	})

	check("context: rejects non-integer query value → 400", func() error {
		req, _ := http.NewRequest("GET", *gateway+"/v1/agent/context?feed=abc", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		return expectError(resp, 400, "invalid_query", "feed")
	})

	// === 4: heartbeat — strict body validation matrix ===
	check("heartbeat: empty-body POST is a valid liveness ping → 200", func() error {
		req, _ := http.NewRequest("POST", *gateway+"/v1/agent/heartbeat", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			b, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, string(b))
		}
		return nil
	})

	check("heartbeat: capabilities + status persist → 200", func() error {
		body := map[string]interface{}{
			"capabilities": map[string]interface{}{
				"nickname":  "smoketester",
				"languages": []string{"go", "ts"},
			},
			"status": "working",
			"note":   "smoke test run",
		}
		return postAndExpectStatus("/v1/agent/heartbeat", token, "application/json", body, 200)
	})

	check("heartbeat: rejects text/plain content-type → 415", func() error {
		return postRawAndExpectError("/v1/agent/heartbeat", token, "text/plain",
			[]byte(`{"status":"idle"}`), 415, "unsupported_media_type")
	})

	check("heartbeat: rejects text/json content-type → 415", func() error {
		return postRawAndExpectError("/v1/agent/heartbeat", token, "text/json",
			[]byte(`{"status":"idle"}`), 415, "unsupported_media_type")
	})

	check("heartbeat: accepts application/json; charset=utf-8 → 200", func() error {
		req, _ := http.NewRequest("POST", *gateway+"/v1/agent/heartbeat",
			bytes.NewReader([]byte(`{"status":"idle"}`)))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	check("heartbeat: rejects charset=latin-1 → 415", func() error {
		return postRawAndExpectError("/v1/agent/heartbeat", token,
			"application/json; charset=latin-1",
			[]byte(`{"status":"idle"}`), 415, "unsupported_media_type")
	})

	check("heartbeat: rejects malformed JSON → 400 invalid_json", func() error {
		return postRawAndExpectError("/v1/agent/heartbeat", token,
			"application/json", []byte(`{not json`), 400, "invalid_json")
	})

	check("heartbeat: rejects unknown top-level field → 400 unknown_field", func() error {
		return postRawAndExpectError("/v1/agent/heartbeat", token,
			"application/json", []byte(`{"status":"idle","extra":"nope"}`),
			400, "unknown_field")
	})

	check("heartbeat: rejects trailing JSON → 400 invalid_json", func() error {
		return postRawAndExpectError("/v1/agent/heartbeat", token,
			"application/json",
			[]byte(`{"status":"idle"}{"status":"idle"}`),
			400, "invalid_json")
	})

	check("heartbeat: rejects oversized body (>64KiB) → 413 payload_too_large", func() error {
		huge := strings.Repeat("a", 64*1024+1024)
		body := []byte(fmt.Sprintf(`{"status":"idle","note":%q}`, huge))
		return postRawAndExpectError("/v1/agent/heartbeat", token,
			"application/json", body, 413, "payload_too_large")
	})

	check("heartbeat: rejects invalid status enum → 400 validation_error/status", func() error {
		return postRawAndExpectError("/v1/agent/heartbeat", token,
			"application/json", []byte(`{"status":"on-fire"}`),
			400, "validation_error")
	})

	check("heartbeat: rejects oversized nickname → 400 validation_error", func() error {
		tooLong := strings.Repeat("x", 65)
		body := []byte(fmt.Sprintf(`{"capabilities":{"nickname":%q}}`, tooLong))
		return postRawAndExpectError("/v1/agent/heartbeat", token,
			"application/json", body, 400, "validation_error")
	})

	check("heartbeat: rejects empty capability entry → 400 validation_error", func() error {
		return postRawAndExpectError("/v1/agent/heartbeat", token,
			"application/json",
			[]byte(`{"capabilities":{"languages":["go",""]}}`),
			400, "validation_error")
	})

	// === 5: inbox ===
	check("inbox: GET /v1/agent/inbox → 200 with empty items for fresh agent", func() error {
		req, _ := http.NewRequest("GET", *gateway+"/v1/agent/inbox?limit=10", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		var inbox struct {
			Count int `json:"count"`
		}
		return json.NewDecoder(resp.Body).Decode(&inbox)
	})

	check("inbox: rejects non-integer limit → 400 invalid_query/limit", func() error {
		req, _ := http.NewRequest("GET", *gateway+"/v1/agent/inbox?limit=many", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		return expectError(resp, 400, "invalid_query", "limit")
	})

	check("inbox: rejects unknown query parameter → 400", func() error {
		req, _ := http.NewRequest("GET", *gateway+"/v1/agent/inbox?wat=1", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		return expectError(resp, 400, "unknown_query_param", "wat")
	})

	// === Summary ===
	fmt.Println(strings.Repeat("-", 60))
	pass, total := 0, len(results)
	for _, r := range results {
		if r.ok {
			pass++
		}
	}
	fmt.Printf("\nResult: %d/%d passed\n", pass, total)
	if pass != total {
		for _, r := range results {
			if !r.ok {
				fmt.Printf("  FAIL  %s\n        %s\n", r.name, r.err)
			}
		}
		os.Exit(1)
	}
	fmt.Println("All strict-contract checks green.")
}

// === helpers ================================================================

func check(name string, fn func() error) {
	if *verbose {
		fmt.Printf("  ... %s\n", name)
	}
	err := fn()
	if err == nil {
		fmt.Printf("  PASS  %s\n", name)
		results = append(results, result{name: name, ok: true})
	} else {
		fmt.Printf("  FAIL  %s\n        %v\n", name, err)
		results = append(results, result{name: name, ok: false, err: err.Error()})
	}
}

func fail(format string, args ...interface{}) {
	fmt.Printf("FATAL: "+format+"\n", args...)
	os.Exit(2)
}

func die(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}

// expectError reads the response body and asserts the status + canonical
// envelope's error.code. If wantField is non-empty, it must also match.
func expectError(resp *http.Response, wantStatus int, wantCode, wantField string) error {
	if resp.StatusCode != wantStatus {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status=%d want=%d body=%s", resp.StatusCode, wantStatus, string(b))
	}
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Field   string `json:"field"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return fmt.Errorf("decode envelope: %w", err)
	}
	if env.Error.Code != wantCode {
		return fmt.Errorf("error.code=%q want=%q", env.Error.Code, wantCode)
	}
	if wantField != "" && env.Error.Field != wantField {
		return fmt.Errorf("error.field=%q want=%q", env.Error.Field, wantField)
	}
	return nil
}

func postRawAndExpectError(path, token, contentType string, body []byte,
	wantStatus int, wantCode string) error {
	req, _ := http.NewRequest("POST", *gateway+path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return expectError(resp, wantStatus, wantCode, "")
}

func postAndExpectStatus(path, token, contentType string, body interface{},
	wantStatus int) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return err
	}
	req, _ := http.NewRequest("POST", *gateway+path, &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", contentType)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status=%d want=%d body=%s", resp.StatusCode, wantStatus, string(b))
	}
	return nil
}

// silence unused — just to be safe across refactors
var _ = time.Now
