// Package distrib serves the cav-cli one-line bootstrap assets:
//
//	GET /install.sh           — installer script (embedded; disk override allowed)
//	GET /dl/cav-cli-<os>-<arch>[.exe] — pre-built CLI binaries from --dist-dir
//
// Design notes:
//   - install.sh is small and rarely changes, so it ships embedded in the
//     gateway binary. An optional file at <DistDir>/install.sh overrides the
//     embedded copy without a restart, useful for hotfixes.
//   - Binaries are large (~10–20MB each), so they are read from disk only
//     (DistDir). Operators publish them with build-all.sh.
//   - Only filenames matching the cav-cli release pattern are served; the
//     handler refuses anything else even if it exists on disk.
package distrib

import (
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

//go:embed install.sh
var embeddedInstallScript []byte

// binaryNamePattern matches the exact set of release artifacts produced by
// cli/cav-cli/build-all.sh. It deliberately rejects path separators, dotfiles,
// and version suffixes so /dl/ cannot be coerced into serving anything else
// from the dist directory.
var binaryNamePattern = regexp.MustCompile(
	`^cav-cli-(linux|darwin|windows)-(amd64|arm64|arm)(\.exe)?$`,
)

// Server serves the bootstrap assets.
type Server struct {
	// DistDir is the on-disk directory holding cav-cli binaries
	// (and optionally an override install.sh). May be empty, in which
	// case only the embedded install.sh is served and /dl/ returns 404.
	DistDir string
}

// New creates a Server. distDir is resolved to an absolute path so later
// path-traversal checks work against a stable base.
func New(distDir string) *Server {
	abs := ""
	if distDir != "" {
		if a, err := filepath.Abs(distDir); err == nil {
			abs = a
		} else {
			abs = distDir
		}
	}
	return &Server{DistDir: abs}
}

// Register attaches the bootstrap routes to mux.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /install.sh", s.handleInstallScript)
	mux.HandleFunc("GET /dl/{filename}", s.handleDownload)
}

// handleInstallScript serves the installer. A file at <DistDir>/install.sh
// takes precedence over the embedded copy when present.
func (s *Server) handleInstallScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, max-age=60")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if s.DistDir != "" {
		override := filepath.Join(s.DistDir, "install.sh")
		if info, err := os.Stat(override); err == nil && !info.IsDir() {
			http.ServeFile(w, r, override)
			return
		}
	}

	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(embeddedInstallScript)))
	_, _ = w.Write(embeddedInstallScript)
}

// handleDownload serves a CLI binary from DistDir, gated by binaryNamePattern.
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	if s.DistDir == "" {
		http.Error(w, `{"error":{"code":"dist_unconfigured","message":"binary distribution directory not configured"}}`, http.StatusNotFound)
		return
	}

	name := r.PathValue("filename")
	// Defence-in-depth: reject any traversal attempt before regex check.
	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		http.Error(w, `{"error":{"code":"invalid_filename"}}`, http.StatusBadRequest)
		return
	}
	if !binaryNamePattern.MatchString(name) {
		http.Error(w, `{"error":{"code":"unknown_binary","message":"only cav-cli release binaries are served"}}`, http.StatusNotFound)
		return
	}

	full := filepath.Join(s.DistDir, name)
	// Confirm the resolved path is still inside DistDir (paranoia for symlinks).
	rel, err := filepath.Rel(s.DistDir, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		http.Error(w, `{"error":{"code":"invalid_filename"}}`, http.StatusBadRequest)
		return
	}

	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		http.Error(w, `{"error":{"code":"binary_unavailable","message":"binary not built for this platform yet"}}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=300")
	// http.ServeFile handles Range, ETag, and Last-Modified for us.
	http.ServeFile(w, r, full)
}
