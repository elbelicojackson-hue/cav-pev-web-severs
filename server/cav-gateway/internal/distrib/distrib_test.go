package distrib

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestServer(t *testing.T, distDir string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	New(distDir).Register(mux)
	return httptest.NewServer(mux)
}

func TestEmbeddedInstallScript(t *testing.T) {
	srv := newTestServer(t, "")
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/install.sh")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "cav-cli") {
		t.Fatalf("install.sh body missing cav-cli marker: %q", string(body))
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/x-shellscript") {
		t.Fatalf("unexpected content-type: %q", got)
	}
}

func TestInstallScriptDiskOverride(t *testing.T) {
	dir := t.TempDir()
	override := []byte("#!/bin/sh\necho override\n")
	if err := os.WriteFile(filepath.Join(dir, "install.sh"), override, 0644); err != nil {
		t.Fatal(err)
	}

	srv := newTestServer(t, dir)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/install.sh")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != string(override) {
		t.Fatalf("expected disk override to win, got %q", string(body))
	}
}

func TestDownloadAllowlist(t *testing.T) {
	dir := t.TempDir()
	// Place an allowed and a forbidden file alongside.
	allowed := filepath.Join(dir, "cav-cli-linux-amd64")
	forbidden := filepath.Join(dir, "secrets.txt")
	if err := os.WriteFile(allowed, []byte("ELF-pretend"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(forbidden, []byte("topsecret"), 0644); err != nil {
		t.Fatal(err)
	}

	srv := newTestServer(t, dir)
	defer srv.Close()

	cases := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{"valid linux binary", "/dl/cav-cli-linux-amd64", 200},
		{"valid windows binary missing", "/dl/cav-cli-windows-amd64.exe", 404},
		{"forbidden filename", "/dl/secrets.txt", 404},
		{"traversal segments", "/dl/..%2Fmain.go", 400},
		{"unknown arch", "/dl/cav-cli-plan9-mips", 404},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(srv.URL + tc.path)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.wantStatus {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("want %d, got %d (body=%s)", tc.wantStatus, resp.StatusCode, string(body))
			}
		})
	}
}

func TestDownloadServesBinaryContents(t *testing.T) {
	dir := t.TempDir()
	want := []byte("\x7fELFfake-binary-payload")
	if err := os.WriteFile(filepath.Join(dir, "cav-cli-darwin-arm64"), want, 0755); err != nil {
		t.Fatal(err)
	}

	srv := newTestServer(t, dir)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/dl/cav-cli-darwin-arm64")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	got, _ := io.ReadAll(resp.Body)
	if string(got) != string(want) {
		t.Fatalf("body mismatch")
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, `cav-cli-darwin-arm64`) {
		t.Fatalf("Content-Disposition missing filename: %q", cd)
	}
}

func TestDownloadWithoutDistDir(t *testing.T) {
	srv := newTestServer(t, "")
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/dl/cav-cli-linux-amd64")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("want 404 when DistDir empty, got %d", resp.StatusCode)
	}
}
