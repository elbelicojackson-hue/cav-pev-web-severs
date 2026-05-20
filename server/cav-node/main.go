// CAV Praxon Node — public network server for Praxon store + routing.
//
// Endpoints:
//   POST /api/praxon       — publish (Gate 1 validated, rate-limited)
//   GET  /api/praxon/{id}  — fetch by praxon_id
//   POST /api/announce     — receive + relay announcement
//   POST /api/subscribe    — register webhook URL
//   DELETE /api/subscribe  — unregister webhook URL
//   GET  /api/subscribers  — list registered webhooks
//   GET  /api/health       — liveness check
//
// Spec: .kiro/specs/cav-praxon/design.md
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/anthropic-cav/cav-node/internal/audit"
	"github.com/anthropic-cav/cav-node/internal/handler"
	"github.com/anthropic-cav/cav-node/internal/ratelimit"
	"github.com/anthropic-cav/cav-node/internal/store"
	"github.com/anthropic-cav/cav-node/internal/webhook"
)

func main() {
	port := flag.Int("port", 8420, "HTTP listen port")
	dataDir := flag.String("data", "./data", "Data directory for praxon store and audit log")
	flag.Parse()

	// Ensure data directories exist
	storeDir := filepath.Join(*dataDir, "praxons")
	auditPath := filepath.Join(*dataDir, "audit.ndjson")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		log.Fatalf("failed to create store dir: %v", err)
	}

	// Initialize components — BadgerDB for persistent KV storage
	praxonStore, err := store.NewBadgerStore(storeDir)
	if err != nil {
		log.Fatalf("failed to open badger store: %v", err)
	}
	defer praxonStore.Close()
	auditLog := audit.NewNDJSONLog(auditPath)
	limiter := ratelimit.New(10) // 10 publish/sec per issuer
	relay := webhook.NewRelay()

	// Periodic cleanup of rate limiter windows
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			limiter.Cleanup()
		}
	}()

	// Build router
	mux := handler.NewRouter(praxonStore, auditLog, limiter, relay)

	addr := fmt.Sprintf(":%d", *port)
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()

	log.Printf("CAV Praxon Node v1.0 starting on %s", addr)
	log.Printf("  Store: %s", storeDir)
	log.Printf("  Audit: %s", auditPath)
	log.Printf("  Rate limit: %d publish/sec/issuer", 10)

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
	log.Println("Server stopped.")
}
