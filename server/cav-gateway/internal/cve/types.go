// Package cve provides a local CVE vulnerability database.
//
// Data sources:
//   - NVD (NIST National Vulnerability Database) — primary, ~250k CVEs
//   - CISA KEV (Known Exploited Vulnerabilities) — high-priority subset
//   - EPSS (Exploit Prediction Scoring System) — probability scores
//
// The CVE database serves as a "shared bootstrap corpus" for the CAV
// network — all agents can query it, reference CVEs in their Praxons,
// and use it as grounding evidence for security-related claims.
//
// Key schema in BadgerDB:
//   cve:id:<CVE-YYYY-NNNNN>           → CVE JSON
//   cve:product:<vendor>:<product>:<cve_id> → [] (product index)
//   cve:year:<YYYY>:<cve_id>          → [] (year index)
//   cve:severity:<level>:<cve_id>     → [] (severity index)
//   cve:kev:<cve_id>                  → [] (CISA KEV flag)
//   cve:meta:count                    → uint64
//   cve:meta:last_sync                → timestamp
package cve

import "time"

// CVE represents a single vulnerability entry.
type CVE struct {
	ID          string    `json:"id"`           // CVE-2024-12345
	Description string    `json:"description"`  // Human-readable description
	Published   time.Time `json:"published"`
	Modified    time.Time `json:"modified"`

	// Severity
	CVSS3Score    float64 `json:"cvss3_score,omitempty"`    // 0.0 - 10.0
	CVSS3Vector   string  `json:"cvss3_vector,omitempty"`   // CVSS:3.1/AV:N/AC:L/...
	Severity      string  `json:"severity"`                 // CRITICAL/HIGH/MEDIUM/LOW/NONE

	// Affected products (CPE)
	AffectedProducts []AffectedProduct `json:"affected_products,omitempty"`

	// References
	References []Reference `json:"references,omitempty"`

	// Exploit info
	EPSS          float64 `json:"epss,omitempty"`           // Exploit probability [0,1]
	IsKEV         bool    `json:"is_kev"`                   // In CISA KEV catalog
	ExploitPublic bool    `json:"exploit_public"`           // Public exploit exists

	// CWE
	CWEs []string `json:"cwes,omitempty"` // CWE-79, CWE-89, etc.
}

// AffectedProduct describes a vulnerable software product.
type AffectedProduct struct {
	Vendor       string `json:"vendor"`
	Product      string `json:"product"`
	VersionStart string `json:"version_start,omitempty"`
	VersionEnd   string `json:"version_end,omitempty"`
}

// Reference is a URL with context about the CVE.
type Reference struct {
	URL  string   `json:"url"`
	Tags []string `json:"tags,omitempty"` // "Patch", "Exploit", "Vendor Advisory"
}

// SearchQuery defines CVE search parameters.
type SearchQuery struct {
	Keyword    string  `json:"keyword,omitempty"`     // Full-text search
	Product    string  `json:"product,omitempty"`     // Filter by product name
	Vendor     string  `json:"vendor,omitempty"`      // Filter by vendor
	Severity   string  `json:"severity,omitempty"`    // CRITICAL/HIGH/MEDIUM/LOW
	Year       int     `json:"year,omitempty"`        // Filter by year
	KEVOnly    bool    `json:"kev_only,omitempty"`    // Only CISA KEV entries
	MinCVSS    float64 `json:"min_cvss,omitempty"`    // Minimum CVSS score
	MinEPSS    float64 `json:"min_epss,omitempty"`    // Minimum EPSS score
	Limit      int     `json:"limit,omitempty"`       // Max results (default 20)
	Offset     int     `json:"offset,omitempty"`      // Pagination offset
}

// SearchResult is the response from a CVE search.
type SearchResult struct {
	CVEs       []CVE  `json:"cves"`
	Total      int    `json:"total"`
	Query      SearchQuery `json:"query"`
}

// SyncStatus reports the state of the CVE database.
type SyncStatus struct {
	TotalCVEs    uint64    `json:"total_cves"`
	LastSync     time.Time `json:"last_sync"`
	KEVCount     int       `json:"kev_count"`
	Sources      []string  `json:"sources"`
	NextSync     time.Time `json:"next_sync"`
}
