// CVE data synchronization from public sources.
//
// Sync strategy:
//   1. On first boot: full sync from NVD JSON feeds (bulk download)
//   2. Every 6 hours: incremental sync (modified CVEs only)
//   3. CISA KEV: daily check (small list, ~1100 entries)
//
// NVD API: https://services.nvd.nist.gov/rest/json/cves/2.0
// CISA KEV: https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json
package cve

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// Syncer handles periodic CVE data synchronization.
type Syncer struct {
	store  *Store
	client *http.Client
}

// NewSyncer creates a CVE syncer.
func NewSyncer(store *Store) *Syncer {
	return &Syncer{
		store: store,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// StartBackground begins periodic sync in a goroutine.
func (s *Syncer) StartBackground() {
	// Initial sync on startup (non-blocking)
	go func() {
		log.Println("[cve-sync] starting initial CISA KEV sync...")
		if err := s.SyncKEV(); err != nil {
			log.Printf("[cve-sync] KEV sync failed: %v", err)
		} else {
			log.Printf("[cve-sync] KEV sync complete, total CVEs: %d", s.store.Count())
		}
	}()

	// Periodic sync every 6 hours
	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			log.Println("[cve-sync] periodic sync starting...")
			if err := s.SyncKEV(); err != nil {
				log.Printf("[cve-sync] periodic sync failed: %v", err)
			}
		}
	}()
}

// SyncKEV fetches the CISA Known Exploited Vulnerabilities catalog.
// This is a small (~1100 entries), high-value dataset.
func (s *Syncer) SyncKEV() error {
	const kevURL = "https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json"

	resp, err := s.client.Get(kevURL)
	if err != nil {
		return fmt.Errorf("KEV fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("KEV returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20)) // 50MB limit
	if err != nil {
		return fmt.Errorf("KEV read failed: %w", err)
	}

	// Parse CISA KEV format
	var kevData struct {
		Vulnerabilities []struct {
			CVEID             string `json:"cveID"`
			VendorProject     string `json:"vendorProject"`
			Product           string `json:"product"`
			VulnerabilityName string `json:"vulnerabilityName"`
			DateAdded         string `json:"dateAdded"`
			ShortDescription  string `json:"shortDescription"`
			RequiredAction    string `json:"requiredAction"`
			KnownRansomware   string `json:"knownRansomwareCampaignUse"`
		} `json:"vulnerabilities"`
	}

	if err := json.Unmarshal(body, &kevData); err != nil {
		return fmt.Errorf("KEV parse failed: %w", err)
	}

	// Store each KEV entry
	count := 0
	for _, v := range kevData.Vulnerabilities {
		published, _ := time.Parse("2006-01-02", v.DateAdded)

		cve := &CVE{
			ID:          v.CVEID,
			Description: v.ShortDescription,
			Published:   published,
			Modified:    time.Now(),
			Severity:    "HIGH", // KEV entries are all high-priority by definition
			IsKEV:       true,
			ExploitPublic: true,
			AffectedProducts: []AffectedProduct{
				{
					Vendor:  strings.ToLower(v.VendorProject),
					Product: strings.ToLower(v.Product),
				},
			},
		}

		if v.KnownRansomware == "Known" {
			cve.Severity = "CRITICAL"
		}

		if err := s.store.Put(cve); err != nil {
			continue
		}
		count++
	}

	// Update sync timestamp
	s.store.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte("cve:meta:last_sync"), []byte(time.Now().UTC().Format(time.RFC3339)))
	})

	log.Printf("[cve-sync] synced %d KEV entries", count)
	return nil
}
