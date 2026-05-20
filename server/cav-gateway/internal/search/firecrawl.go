// Package search provides web search and scraping via Firecrawl.
//
// Firecrawl (https://firecrawl.dev) is a web scraping API that converts
// any URL into clean markdown/structured data. Agents use it to:
//   - Search the web for evidence to ground their claims
//   - Scrape documentation for knowledge extraction
//   - Crawl sites for vulnerability research
//
// Authentication model:
//   - Each agent provides their OWN Firecrawl API key
//   - The gateway proxies requests but does NOT store keys
//   - Agents without a key get a 403 with instructions to register
//
// API: https://docs.firecrawl.dev/api-reference
package search

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	firecrawlBaseURL = "https://api.firecrawl.dev/v1"
	maxTimeout       = 60 * time.Second
)

// Client wraps the Firecrawl API.
type Client struct {
	httpClient *http.Client
}

// NewClient creates a Firecrawl client.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: maxTimeout},
	}
}

// ScrapeRequest is the input for scraping a URL.
type ScrapeRequest struct {
	URL     string   `json:"url"`
	Formats []string `json:"formats,omitempty"` // "markdown", "html", "links"
}

// ScrapeResponse is the Firecrawl scrape result.
type ScrapeResponse struct {
	Success bool   `json:"success"`
	Data    *struct {
		Markdown string            `json:"markdown,omitempty"`
		HTML     string            `json:"html,omitempty"`
		Links    []string          `json:"links,omitempty"`
		Metadata map[string]string `json:"metadata,omitempty"`
	} `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

// SearchRequest is the input for web search.
type SearchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"` // default 5
}

// SearchResult is a single search hit.
type SearchResult struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Markdown    string `json:"markdown,omitempty"`
}

// SearchResponse is the Firecrawl search result.
type SearchResponse struct {
	Success bool           `json:"success"`
	Data    []SearchResult `json:"data,omitempty"`
	Error   string         `json:"error,omitempty"`
}

// Scrape fetches and converts a URL to markdown.
func (c *Client) Scrape(apiKey string, req *ScrapeRequest) (*ScrapeResponse, error) {
	if req.Formats == nil {
		req.Formats = []string{"markdown"}
	}

	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest("POST", firecrawlBaseURL+"/scrape", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("firecrawl request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result ScrapeResponse
	json.Unmarshal(respBody, &result)

	if resp.StatusCode != 200 {
		return &result, fmt.Errorf("firecrawl returned %d: %s", resp.StatusCode, result.Error)
	}
	return &result, nil
}

// Search performs a web search.
func (c *Client) Search(apiKey string, req *SearchRequest) (*SearchResponse, error) {
	if req.Limit <= 0 {
		req.Limit = 5
	}

	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest("POST", firecrawlBaseURL+"/search", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("firecrawl request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result SearchResponse
	json.Unmarshal(respBody, &result)

	if resp.StatusCode != 200 {
		return &result, fmt.Errorf("firecrawl returned %d: %s", resp.StatusCode, result.Error)
	}
	return &result, nil
}

// HandleScrape returns the HTTP handler for POST /v1/search/scrape
func HandleScrape(client *Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-Firecrawl-Key")
		if apiKey == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"code":    "missing_api_key",
					"message": "Firecrawl API key required. Set X-Firecrawl-Key header.",
					"help":    "Register at https://firecrawl.dev to get your API key.",
				},
			})
			return
		}

		var req ScrapeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
			http.Error(w, `{"error":{"code":"invalid_request","message":"missing url"}}`, http.StatusBadRequest)
			return
		}

		result, err := client.Scrape(apiKey, &req)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

// HandleSearch returns the HTTP handler for POST /v1/search/web
func HandleSearch(client *Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-Firecrawl-Key")
		if apiKey == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"code":    "missing_api_key",
					"message": "Firecrawl API key required. Set X-Firecrawl-Key header.",
					"help":    "Register at https://firecrawl.dev to get your API key.",
				},
			})
			return
		}

		var req SearchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Query == "" {
			http.Error(w, `{"error":{"code":"invalid_request","message":"missing query"}}`, http.StatusBadRequest)
			return
		}

		result, err := client.Search(apiKey, &req)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

// HandleCapabilities returns GET /v1/search — lists available search tools
func HandleCapabilities() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"service": "firecrawl",
			"version": "v1",
			"tools": []map[string]string{
				{
					"name":        "web_search",
					"endpoint":    "POST /v1/search/web",
					"description": "Search the web and get structured results",
					"auth":        "X-Firecrawl-Key header (get key at https://firecrawl.dev)",
				},
				{
					"name":        "scrape",
					"endpoint":    "POST /v1/search/scrape",
					"description": "Scrape any URL and convert to clean markdown",
					"auth":        "X-Firecrawl-Key header (get key at https://firecrawl.dev)",
				},
			},
			"note": "Each agent must provide their own Firecrawl API key. Register at https://firecrawl.dev",
		})
	}
}
