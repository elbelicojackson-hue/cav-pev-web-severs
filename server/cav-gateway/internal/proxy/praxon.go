package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/anthropic-cav/cav-gateway/internal/stream"
)

// PraxonProxy forwards Praxon operations to the upstream cav-node.
type PraxonProxy struct {
	upstream string
	client   *http.Client
}

func NewPraxonProxy(upstream string) *PraxonProxy {
	return &PraxonProxy{upstream: upstream, client: &http.Client{}}
}

// HandlePublish proxies POST /v1/praxon → upstream POST /api/praxon
func (p *PraxonProxy) HandlePublish(hub *stream.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 256*1024+1))
		if err != nil {
			http.Error(w, `{"error":{"code":"read_error","message":"failed to read body"}}`, http.StatusBadRequest)
			return
		}

		// Forward to upstream
		resp, err := http.Post(p.upstream+"/api/praxon", "application/json", io.NopCloser(io.Reader(nil)))
		if err != nil {
			// If upstream is down, store locally (future: queue)
			_ = body
			http.Error(w, `{"error":{"code":"upstream_error","message":"cav-node unreachable"}}`, http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// Forward upstream response
		respBody, _ := io.ReadAll(resp.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(respBody)

		// Broadcast announcement to WebSocket subscribers
		if resp.StatusCode == http.StatusCreated {
			var result map[string]string
			if json.Unmarshal(respBody, &result) == nil {
				hub.Broadcast(stream.Event{
					Type: "announcement",
					Data: map[string]string{
						"praxon_id": result["praxon_id"],
						"issuer":    r.Context().Value("citizen_did").(string),
					},
				})
			}
		}
	}
}

// HandleFetch proxies GET /v1/praxon/:id → upstream GET /api/praxon/:id
func (p *PraxonProxy) HandleFetch() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		resp, err := p.client.Get(fmt.Sprintf("%s/api/praxon/%s", p.upstream, id))
		if err != nil {
			http.Error(w, `{"error":{"code":"upstream_error","message":"cav-node unreachable"}}`, http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}
}

// HandleList proxies GET /v1/praxon?... → upstream (future: index query)
func (p *PraxonProxy) HandleList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO: implement praxon listing/search against upstream index
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"praxons": []interface{}{},
			"total":   0,
		})
	}
}

// HandleChallenge handles POST /v1/praxon/:id/challenge
func (p *PraxonProxy) HandleChallenge(hub *stream.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		did := r.Context().Value("citizen_did").(string)

		var req struct {
			Reason         string      `json:"reason"`
			CounterEvidence interface{} `json:"counter_evidence,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Reason == "" {
			http.Error(w, `{"error":{"code":"invalid_request","message":"missing reason"}}`, http.StatusBadRequest)
			return
		}

		// TODO: store challenge, route to praxon issuer, initiate consensus
		challengeID := fmt.Sprintf("ch_%s_%d", id[:8], 1)

		// Broadcast challenge event
		hub.Broadcast(stream.Event{
			Type: "challenge",
			Data: map[string]string{
				"challenge_id": challengeID,
				"praxon_id":    id,
				"challenger":   did,
				"reason":       req.Reason,
			},
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"challenge_id": challengeID,
			"status":       "pending",
		})
	}
}
