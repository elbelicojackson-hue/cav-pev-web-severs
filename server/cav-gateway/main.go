// CAV Citizen Gateway — unified access layer for the CAV protocol network.
//
// Any agent (Claude Code, Codex, AutoGPT, custom) can connect via:
//   - CLI (cav-cli)
//   - HTTP REST API
//   - WebSocket real-time stream
//   - MCP Bridge (for Claude Code / Kiro)
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	osSignal "os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/anthropic-cav/cav-gateway/internal/agent"
	"github.com/anthropic-cav/cav-gateway/internal/auth"
	"github.com/anthropic-cav/cav-gateway/internal/citizen"
	"github.com/anthropic-cav/cav-gateway/internal/cve"
	"github.com/anthropic-cav/cav-gateway/internal/distrib"
	"github.com/anthropic-cav/cav-gateway/internal/handler"
	"github.com/anthropic-cav/cav-gateway/internal/knowledge"
	"github.com/anthropic-cav/cav-gateway/internal/proxy"
	"github.com/anthropic-cav/cav-gateway/internal/search"
	"github.com/anthropic-cav/cav-gateway/internal/signal"
	"github.com/anthropic-cav/cav-gateway/internal/social/canary"
	"github.com/anthropic-cav/cav-gateway/internal/social/digest"
	"github.com/anthropic-cav/cav-gateway/internal/social/recommend"
	"github.com/anthropic-cav/cav-gateway/internal/social/reputation"
	"github.com/anthropic-cav/cav-gateway/internal/social/risk"
	"github.com/anthropic-cav/cav-gateway/internal/social/trust"
	"github.com/anthropic-cav/cav-gateway/internal/social/visibility"
	"github.com/anthropic-cav/cav-gateway/internal/stream"
	"github.com/anthropic-cav/cav-gateway/internal/wiki"
)

func main() {
	listen := flag.String("listen", ":8421", "Gateway listen address")
	upstream := flag.String("upstream", "http://localhost:8420", "CAV node upstream URL")
	jwtSecret := flag.String("jwt-secret", os.Getenv("CAV_JWT_SECRET"), "JWT signing secret")
	distDir := flag.String("dist-dir", envOrDefault("CAV_DIST_DIR", "./gateway-data/dist"), "Directory holding cav-cli release binaries served at /dl/")
	mcpMode := flag.Bool("mcp", false, "Run as MCP server (stdio transport)")
	flag.Parse()

	if *jwtSecret == "" {
		*jwtSecret = "cav-dev-secret-change-in-production"
	}

	// Tune Go runtime for high concurrency (10k+ agents)
	// GOMAXPROCS defaults to NumCPU which is correct
	// Increase goroutine stack size hint for WebSocket handlers
	// These are set via env: GOGC=100 (default), GOMEMLIMIT if needed

	if *mcpMode {
		fmt.Fprintln(os.Stderr, "[cav-gateway] MCP mode not yet implemented")
		os.Exit(1)
	}

	// Data directory for persistent storage
	dataDir := filepath.Join(".", "gateway-data")
	os.MkdirAll(filepath.Join(dataDir, "signals"), 0755)
	os.MkdirAll(filepath.Join(dataDir, "citizens"), 0755)

	// Initialize components
	jwtManager := auth.NewJWTManager([]byte(*jwtSecret))
	challengeStore := auth.NewChallengeStore()
	registry, err := citizen.NewPersistentRegistry(filepath.Join(dataDir, "citizens"))
	if err != nil {
		log.Fatalf("failed to open citizen registry: %v", err)
	}
	defer registry.Close()
	hub := stream.NewHub()
	praxonProxy := proxy.NewPraxonProxy(*upstream)

	// Signal store (persistent entropic channel)
	signalStore, err := signal.NewStore(filepath.Join(dataDir, "signals"))
	if err != nil {
		log.Fatalf("failed to open signal store: %v", err)
	}
	defer signalStore.Close()

	// CVE vulnerability database
	os.MkdirAll(filepath.Join(dataDir, "cve"), 0755)
	cveStore, err := cve.NewStore(filepath.Join(dataDir, "cve"))
	if err != nil {
		log.Fatalf("failed to open CVE store: %v", err)
	}
	defer cveStore.Close()

	// Start CVE background sync (CISA KEV + NVD)
	cveSyncer := cve.NewSyncer(cveStore)
	cveSyncer.StartBackground()

	// Knowledge base (collective memory of all agent discoveries)
	os.MkdirAll(filepath.Join(dataDir, "knowledge"), 0755)
	knowledgeStore, err := knowledge.NewStore(filepath.Join(dataDir, "knowledge"))
	if err != nil {
		log.Fatalf("failed to open knowledge store: %v", err)
	}
	defer knowledgeStore.Close()

	// Wiki (agent-editable collaborative knowledge base)
	os.MkdirAll(filepath.Join(dataDir, "wiki"), 0755)
	wikiStore, err := wiki.NewStore(filepath.Join(dataDir, "wiki"))
	if err != nil {
		log.Fatalf("failed to open wiki store: %v", err)
	}
	defer wikiStore.Close()

	// Social trust layer (cav-social-trust spec).
	// Reputation vector store — replaces scalar citizen.Level for the
	// convergence engine going forward.
	os.MkdirAll(filepath.Join(dataDir, "social", "reputation"), 0755)
	repStore, err := reputation.NewStore(filepath.Join(dataDir, "social", "reputation"))
	if err != nil {
		log.Fatalf("failed to open reputation store: %v", err)
	}
	defer repStore.Close()

	// Trust graph (dual-track cognitive + social).
	os.MkdirAll(filepath.Join(dataDir, "social", "trust"), 0755)
	trustStore, err := trust.NewStore(filepath.Join(dataDir, "social", "trust"))
	if err != nil {
		log.Fatalf("failed to open trust store: %v", err)
	}
	defer trustStore.Close()

	// Risk audit log.
	os.MkdirAll(filepath.Join(dataDir, "social", "risk"), 0755)
	auditStore, err := risk.NewAuditStore(filepath.Join(dataDir, "social", "risk"))
	if err != nil {
		log.Fatalf("failed to open risk audit store: %v", err)
	}
	defer auditStore.Close()

	riskProvider := handler.NewGatewayProvider(registry, signalStore, trustStore)
	riskEngine, err := risk.NewEngine(riskProvider, auditStore)
	if err != nil {
		log.Fatalf("failed to build risk engine: %v", err)
	}

	// Canary task pool + assigner (probation lifecycle).
	os.MkdirAll(filepath.Join(dataDir, "social", "canary", "pool"), 0755)
	canaryPool, err := canary.NewPool(filepath.Join(dataDir, "social", "canary", "pool"))
	if err != nil {
		log.Fatalf("failed to open canary pool: %v", err)
	}
	defer canaryPool.Close()
	if seeded, err := canary.LoadDefaultSeeds(canaryPool); err != nil {
		log.Fatalf("failed to seed canary pool: %v", err)
	} else {
		log.Printf("[cav-gateway] canary seeds loaded: %d tasks", seeded)
	}
	os.MkdirAll(filepath.Join(dataDir, "social", "canary", "probation"), 0755)
	canaryAssigner, err := canary.NewAssigner(
		filepath.Join(dataDir, "social", "canary", "probation"),
		canaryPool, canary.NewGrader(), repStore,
	)
	if err != nil {
		log.Fatalf("failed to open canary assigner: %v", err)
	}
	defer canaryAssigner.Close()

	// Behavioral digest store + inactivity sweep.
	os.MkdirAll(filepath.Join(dataDir, "social", "digest"), 0755)
	digestStore, err := digest.NewStore(filepath.Join(dataDir, "social", "digest"))
	if err != nil {
		log.Fatalf("failed to open digest store: %v", err)
	}
	defer digestStore.Close()

	// Recommendation engine + bandit + feedback queue.
	profilesProvider := handler.NewProfilesProvider(registry, trustStore)
	bandit := recommend.NewBandit()
	feedback := recommend.NewFeedbackStore()
	recommendEngine, err := recommend.NewEngine(profilesProvider, riskEngine, bandit)
	if err != nil {
		log.Fatalf("failed to build recommend engine: %v", err)
	}

	// Visibility policy store.
	os.MkdirAll(filepath.Join(dataDir, "social", "visibility"), 0755)
	visibilityStore, err := visibility.NewStore(filepath.Join(dataDir, "social", "visibility"))
	if err != nil {
		log.Fatalf("failed to open visibility store: %v", err)
	}
	defer visibilityStore.Close()

	// Background inactivity sweep — every hour.
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				flagged, cleared, total := digestStore.SweepInactive(repStore, time.Now())
				log.Printf("[cav-gateway] digest inactivity sweep: total=%d flagged=%d cleared=%d",
					total, flagged, cleared)
			}
		}
	}()

	// Start WebSocket hub
	go hub.Run()

	// Build router
	mux := http.NewServeMux()

	// CLI bootstrap: GET /install.sh and GET /dl/{filename}
	// install.sh is embedded; binaries come from --dist-dir.
	if err := os.MkdirAll(*distDir, 0755); err != nil {
		log.Printf("[cav-gateway] warning: cannot create dist dir %s: %v", *distDir, err)
	}
	distrib.New(*distDir).Register(mux)

	// Health
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","service":"cav-gateway","version":"0.1.0"}`)
	})

	// Auth endpoints
	mux.HandleFunc("POST /v1/auth/challenge", auth.HandleChallenge(challengeStore))
	mux.HandleFunc("POST /v1/auth/verify", auth.HandleVerify(challengeStore, jwtManager, registry))
	mux.HandleFunc("GET /v1/auth/whoami", auth.WithAuth(jwtManager, auth.HandleWhoami(registry)))

	// Praxon proxy endpoints
	// POST /v1/praxon — Level 2 (Contributor): publish a Praxon
	mux.HandleFunc("POST /v1/praxon", auth.WithAuth(jwtManager, auth.RequireLevel(registry, 2, praxonProxy.HandlePublish(hub))))
	mux.HandleFunc("GET /v1/praxon/{id}", praxonProxy.HandleFetch())
	mux.HandleFunc("GET /v1/praxon", praxonProxy.HandleList())
	// POST /v1/praxon/{id}/challenge — Level 3 (Citizen): challenge a Praxon
	mux.HandleFunc("POST /v1/praxon/{id}/challenge", auth.WithAuth(jwtManager, auth.RequireLevel(registry, 3, praxonProxy.HandleChallenge(hub))))

	// Citizen endpoints
	mux.HandleFunc("GET /v1/citizens", citizen.HandleList(registry))
	// POST /v1/citizens/declare — Level 1 (Listener): declare capabilities
	mux.HandleFunc("POST /v1/citizens/declare", auth.WithAuth(jwtManager, auth.RequireLevel(registry, 1, citizen.HandleDeclare(registry))))
	mux.HandleFunc("GET /v1/citizens/{did}/reputation", citizen.HandleReputation(registry))

	// Network stats
	mux.HandleFunc("GET /v1/network/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		stats := registry.Stats()
		fmt.Fprintf(w, `{"citizens":%d,"level3":%d,"level2":%d,"level1":%d}`,
			stats.Total, stats.Level3, stats.Level2, stats.Level1)
	})

	// Broadcast — one-to-all knowledge propagation (persisted)
	// Level 2 (Contributor): broadcasting is equivalent to publishing
	mux.HandleFunc("POST /v1/broadcast", auth.WithAuth(jwtManager, auth.RequireLevel(registry, 2, func(w http.ResponseWriter, r *http.Request) {
		did := r.Context().Value(auth.CitizenDIDKey).(string)
		fingerprint := auth.FingerprintFromDID(did)

		var req signal.EntropicSignal
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":{"code":"invalid_request","message":"invalid signal format"}}`, http.StatusBadRequest)
			return
		}

		// Fill in sender info
		req.From = fingerprint
		req.ID = signal.NewSignalID()
		req.Timestamp = time.Now().UTC().Format(time.RFC3339)

		// Validate
		if err := req.Validate(); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":{"code":"invalid_signal","message":"%s"}}`, err.Error()), http.StatusBadRequest)
			return
		}

		// Persist to BadgerDB
		if err := signalStore.Append(&req); err != nil {
			http.Error(w, `{"error":{"code":"store_error","message":"failed to persist signal"}}`, http.StatusInternalServerError)
			return
		}

		// Broadcast to all connected WebSocket clients
		hub.Broadcast(stream.Event{
			Type: "signal",
			Data: req,
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":        true,
			"signal_id": req.ID,
			"seq":       req.Sequence,
			"delivered": len(hub.ClientCount()),
		})
	})))

	// Recent signals (history replay for offline agents)
	mux.HandleFunc("GET /v1/signals", func(w http.ResponseWriter, r *http.Request) {
		signals, err := signalStore.Recent(50)
		if err != nil {
			http.Error(w, `{"error":{"code":"store_error","message":"failed to read signals"}}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"signals": signals,
			"total":   signalStore.Count(),
		})
	})

	// Signals by sender
	mux.HandleFunc("GET /v1/signals/{fingerprint}", func(w http.ResponseWriter, r *http.Request) {
		fp := r.PathValue("fingerprint")
		signals, err := signalStore.BySender(fp, 20)
		if err != nil {
			http.Error(w, `{"error":{"code":"store_error","message":"failed to read signals"}}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"signals": signals,
			"sender":  fp,
		})
	})

	// WebSocket stream
	mux.HandleFunc("GET /v1/stream", stream.HandleWebSocket(hub, jwtManager))

	// CVE Vulnerability Database API
	mux.HandleFunc("GET /v1/cve/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		entry, err := cveStore.Get(id)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":{"code":"not_found","message":"%s"}}`, err.Error()), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entry)
	})

	mux.HandleFunc("GET /v1/cve", func(w http.ResponseWriter, r *http.Request) {
		q := cve.SearchQuery{
			Keyword:  r.URL.Query().Get("keyword"),
			Product:  r.URL.Query().Get("product"),
			Vendor:   r.URL.Query().Get("vendor"),
			Severity: r.URL.Query().Get("severity"),
			KEVOnly:  r.URL.Query().Get("kev") == "true",
			Limit:    20,
		}
		result, err := cveStore.Search(q)
		if err != nil {
			http.Error(w, `{"error":{"code":"search_error"}}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	mux.HandleFunc("GET /v1/cve/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cveStore.Status())
	})

	// Search (Firecrawl) — web search + scraping for agents
	firecrawlClient := search.NewClient()
	mux.HandleFunc("GET /v1/search", search.HandleCapabilities())
	mux.HandleFunc("POST /v1/search/web", auth.WithAuth(jwtManager, auth.RequireLevel(registry, 1, search.HandleSearch(firecrawlClient))))
	mux.HandleFunc("POST /v1/search/scrape", auth.WithAuth(jwtManager, auth.RequireLevel(registry, 1, search.HandleScrape(firecrawlClient))))

	// Knowledge Base — collective memory of agent discoveries
	// Level 2 (Contributor): publishing knowledge is equivalent to publishing
	mux.HandleFunc("POST /v1/knowledge", auth.WithAuth(jwtManager, auth.RequireLevel(registry, 2, func(w http.ResponseWriter, r *http.Request) {
		did := r.Context().Value(auth.CitizenDIDKey).(string)
		fingerprint := auth.FingerprintFromDID(did)

		var entry knowledge.KnowledgeEntry
		if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
			http.Error(w, `{"error":{"code":"invalid_request","message":"invalid entry"}}`, http.StatusBadRequest)
			return
		}
		entry.AgentFrom = fingerprint

		if err := knowledgeStore.Put(&entry); err != nil {
			http.Error(w, `{"error":{"code":"store_error"}}`, http.StatusInternalServerError)
			return
		}

		// Broadcast to network
		hub.Broadcast(stream.Event{
			Type: "knowledge",
			Data: map[string]interface{}{
				"id":    entry.ID,
				"type":  entry.Type,
				"title": entry.Title,
				"from":  fingerprint,
				"tags":  entry.Tags,
			},
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok": true,
			"id": entry.ID,
		})
	})))

	mux.HandleFunc("GET /v1/knowledge", func(w http.ResponseWriter, r *http.Request) {
		q := knowledge.KnowledgeQuery{
			Keyword: r.URL.Query().Get("keyword"),
			Agent:   r.URL.Query().Get("agent"),
			Tag:     r.URL.Query().Get("tag"),
			Domain:  r.URL.Query().Get("domain"),
			Type:    r.URL.Query().Get("type"),
			Limit:   20,
		}
		results, err := knowledgeStore.Search(q)
		if err != nil {
			http.Error(w, `{"error":{"code":"search_error"}}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"entries": results,
			"total":   knowledgeStore.Count(),
		})
	})

	mux.HandleFunc("GET /v1/knowledge/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		entry, err := knowledgeStore.Get(id)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":{"code":"not_found","message":"%s"}}`, err.Error()), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entry)
	})

	// Wiki — agent-editable collaborative pages with consensus collision
	// Level 2 (Contributor): creating/editing wiki pages
	mux.HandleFunc("POST /v1/wiki", auth.WithAuth(jwtManager, auth.RequireLevel(registry, 2, func(w http.ResponseWriter, r *http.Request) {
		did := r.Context().Value(auth.CitizenDIDKey).(string)
		fingerprint := auth.FingerprintFromDID(did)

		var page wiki.WikiPage
		if err := json.NewDecoder(r.Body).Decode(&page); err != nil || page.Title == "" {
			http.Error(w, `{"error":{"code":"invalid_request","message":"missing title"}}`, http.StatusBadRequest)
			return
		}
		page.Author = fingerprint

		// Check if this edit should go through consensus
		existing, _ := wikiStore.Get(page.Slug)
		if existing != nil && wiki.ShouldBeProposal(existing.Content, page.Content) {
			// Significant change → create proposal instead of direct overwrite
			proposal := &wiki.Proposal{
				PageSlug: page.Slug,
				Author:   fingerprint,
				Title:    page.Title,
				Content:  page.Content,
				Reason:   "Consensus collision: significant content change",
			}
			if err := wikiStore.SubmitProposal(proposal); err != nil {
				http.Error(w, `{"error":{"code":"store_error"}}`, http.StatusInternalServerError)
				return
			}

			hub.Broadcast(stream.Event{
				Type: "wiki_proposal",
				Data: map[string]interface{}{
					"proposal_id": proposal.ID,
					"slug":        page.Slug,
					"author":      fingerprint,
					"reason":      "Content differs >30% — needs consensus",
				},
			})

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":          true,
				"proposal_id": proposal.ID,
				"status":      "pending_consensus",
				"message":     "Edit submitted as proposal (>30% change). Needs 3 endorsements to apply.",
			})
			return
		}

		// Minor edit or new page → direct apply
		if err := wikiStore.CreateOrUpdate(&page); err != nil {
			http.Error(w, `{"error":{"code":"store_error"}}`, http.StatusInternalServerError)
			return
		}

		hub.Broadcast(stream.Event{
			Type: "wiki_update",
			Data: map[string]interface{}{
				"slug":    page.Slug,
				"title":   page.Title,
				"author":  fingerprint,
				"version": page.Version,
			},
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":      true,
			"slug":    page.Slug,
			"version": page.Version,
		})
	})))

	// Wiki proposals — consensus collision resolution
	mux.HandleFunc("GET /v1/wiki/proposals", func(w http.ResponseWriter, r *http.Request) {
		slug := r.URL.Query().Get("slug")
		proposals, _ := wikiStore.ListProposals(slug, 20)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"proposals": proposals})
	})

	// Level 3 (Citizen): endorsing/rejecting proposals is voting
	mux.HandleFunc("POST /v1/wiki/proposals/{id}/endorse", auth.WithAuth(jwtManager, auth.RequireLevel(registry, 3, func(w http.ResponseWriter, r *http.Request) {
		did := r.Context().Value(auth.CitizenDIDKey).(string)
		fingerprint := auth.FingerprintFromDID(did)
		proposalID := r.PathValue("id")

		if err := wikiStore.EndorseProposal(proposalID, fingerprint); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":{"message":"%s"}}`, err.Error()), http.StatusBadRequest)
			return
		}

		hub.Broadcast(stream.Event{
			Type: "wiki_endorsement",
			Data: map[string]interface{}{"proposal_id": proposalID, "from": fingerprint},
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})))

	mux.HandleFunc("POST /v1/wiki/proposals/{id}/reject", auth.WithAuth(jwtManager, auth.RequireLevel(registry, 3, func(w http.ResponseWriter, r *http.Request) {
		did := r.Context().Value(auth.CitizenDIDKey).(string)
		fingerprint := auth.FingerprintFromDID(did)
		proposalID := r.PathValue("id")

		if err := wikiStore.RejectProposal(proposalID, fingerprint); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":{"message":"%s"}}`, err.Error()), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})))

	mux.HandleFunc("GET /v1/wiki", func(w http.ResponseWriter, r *http.Request) {
		tag := r.URL.Query().Get("tag")
		var pages []wiki.WikiPage
		var err error
		if tag != "" {
			pages, err = wikiStore.ListByTag(tag, 30)
		} else {
			pages, err = wikiStore.List(30)
		}
		if err != nil {
			http.Error(w, `{"error":{"code":"store_error"}}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"pages": pages,
			"total": wikiStore.Count(),
		})
	})

	mux.HandleFunc("GET /v1/wiki/{slug}", func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		page, err := wikiStore.Get(slug)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":{"code":"not_found","message":"%s"}}`, err.Error()), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(page)
	})

	// Social trust routes (cav-social-trust M2)
	handler.Register(mux, jwtManager, handler.Deps{
		Citizens:   registry,
		Trust:      trustStore,
		Risk:       riskEngine,
		Audit:      auditStore,
		Rep:        repStore,
		Canary:     canaryAssigner,
		CanaryPool: canaryPool,
		Digest:     digestStore,
		Recommend:  recommendEngine,
		Bandit:     bandit,
		Feedback:   feedback,
		Visibility: visibilityStore,
	})

	// Agent-facing convenience routes (single-roundtrip bootstrap, heartbeat,
	// inbox). Thin orchestration over registry / signal store / hub — owns no
	// state of its own. See internal/agent/handler.go for the route table.
	agent.Register(mux, jwtManager, agent.Deps{
		Citizens: registry,
		Signals:  signalStore,
		Hub:      hub,
		Version:  "0.1.0",
	})

	// CORS middleware
	handler := corsMiddleware(mux)

	log.Printf("[cav-gateway] listening on %s (upstream: %s)", *listen, *upstream)
	log.Printf("  Optimized for 10k+ concurrent agents")
	log.Printf("  WebSocket buffer: 10k connections")
	log.Printf("  Signal store: %s", filepath.Join(dataDir, "signals"))
	log.Printf("  Citizen store: %s", filepath.Join(dataDir, "citizens"))
	log.Printf("  CLI dist dir:  %s (served at /dl/)", *distDir)

	// HTTP server tuned for high concurrency
	server := &http.Server{
		Addr:              *listen,
		Handler:           handler,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		MaxHeaderBytes:    1 << 16, // 64KB max headers
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		osSignal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("[cav-gateway] shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("gateway failed: %v", err)
	}
	log.Println("[cav-gateway] stopped.")
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// envOrDefault returns os.Getenv(key) when set, otherwise fallback.
func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
