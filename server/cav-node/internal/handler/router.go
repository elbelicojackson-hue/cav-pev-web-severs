// Package handler provides HTTP handlers for the CAV Praxon node.
package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/anthropic-cav/cav-node/internal/audit"
	"github.com/anthropic-cav/cav-node/internal/praxon"
	"github.com/anthropic-cav/cav-node/internal/ratelimit"
	"github.com/anthropic-cav/cav-node/internal/store"
	"github.com/anthropic-cav/cav-node/internal/webhook"
)

// NewRouter builds the HTTP mux for the CAV node.
func NewRouter(s store.Store, auditLog *audit.Log, limiter *ratelimit.Limiter, relay *webhook.Relay) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/praxon", publishHandler(s, auditLog, limiter, relay))
	mux.HandleFunc("GET /api/praxon/{id}", fetchHandler(s, auditLog))
	mux.HandleFunc("POST /api/announce", announceHandler(auditLog, relay))
	mux.HandleFunc("POST /api/subscribe", subscribeHandler(relay))
	mux.HandleFunc("DELETE /api/subscribe", unsubscribeHandler(relay))
	mux.HandleFunc("GET /api/subscribers", listSubscribersHandler(relay))
	mux.HandleFunc("GET /api/health", healthHandler())

	// Wrap with CORS middleware
	return corsMiddleware(mux)
}

// POST /api/praxon — publish a new Praxon
func publishHandler(s store.Store, auditLog *audit.Log, limiter *ratelimit.Limiter, relay *webhook.Relay) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 256*1024+1))
		if err != nil {
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}
		if len(body) > 256*1024 {
			http.Error(w, "praxon exceeds 256KB limit", http.StatusRequestEntityTooLarge)
			auditLog.Append("publish_rejected", "", "", "size_exceeded")
			return
		}

		// Gate 1 validation
		p, err := praxon.ValidateGate1(body)
		if err != nil {
			auditLog.Append("publish_rejected", "", "", err.Error())
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}

		// Rate limit per issuer
		if !limiter.Allow(p.Issuer) {
			auditLog.Append("publish_rate_limited", p.PraxonID, p.Issuer, "")
			http.Error(w, "rate limited: max 10 praxon/sec per issuer", http.StatusTooManyRequests)
			return
		}

		// Store (idempotent)
		if err := s.Put(p.PraxonID, body); err != nil {
			http.Error(w, "store error", http.StatusInternalServerError)
			return
		}

		auditLog.Append("published", p.PraxonID, p.Issuer, "")

		// Auto-announce to subscribers
		relay.Broadcast(praxon.Announcement{
			PraxonID:    p.PraxonID,
			Issuer:      p.Issuer,
			StoreHints:  []string{r.Host + "/api/praxon/" + p.PraxonID},
			AnnouncedAt: time.Now().UTC().Format(time.RFC3339),
		})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"ok":         "true",
			"praxon_id":  p.PraxonID,
			"praxon_uri": "/api/praxon/" + p.PraxonID,
		})
	}
}

// GET /api/praxon/{id} — fetch a Praxon by ID
func fetchHandler(s store.Store, auditLog *audit.Log) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "missing praxon id", http.StatusBadRequest)
			return
		}

		data, err := s.Get(id)
		if err != nil {
			auditLog.Append("fetch_not_found", id, "", "")
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		auditLog.Append("fetched", id, "", "")

		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}
}

// POST /api/announce — receive an announcement and relay to subscribers
func announceHandler(auditLog *audit.Log, relay *webhook.Relay) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var ann praxon.Announcement
		if err := json.NewDecoder(r.Body).Decode(&ann); err != nil {
			http.Error(w, "invalid announcement", http.StatusBadRequest)
			return
		}

		if ann.PraxonID == "" || ann.Issuer == "" {
			http.Error(w, "missing required fields", http.StatusBadRequest)
			return
		}

		auditLog.Append("announcement_received", ann.PraxonID, ann.Issuer, "")

		// Relay to all subscribers
		relay.Broadcast(ann)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
	}
}

// POST /api/subscribe — register a webhook URL
func subscribeHandler(relay *webhook.Relay) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
			http.Error(w, "missing url field", http.StatusBadRequest)
			return
		}
		relay.Subscribe(req.URL)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"ok": "true", "subscribed": req.URL})
	}
}

// DELETE /api/subscribe — unregister a webhook URL
func unsubscribeHandler(relay *webhook.Relay) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
			http.Error(w, "missing url field", http.StatusBadRequest)
			return
		}
		relay.Unsubscribe(req.URL)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"ok": "true", "unsubscribed": req.URL})
	}
}

// GET /api/subscribers — list registered webhook URLs
func listSubscribersHandler(relay *webhook.Relay) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"subscribers": relay.Subscribers(),
		})
	}
}

// GET /api/health — liveness check
func healthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":   "ok",
			"protocol": "cav-praxon",
			"version":  "1.0",
		})
	}
}

// corsMiddleware adds CORS headers for cross-origin TS client access.
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
