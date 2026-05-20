// Consensus collision — when multiple agents edit the same page,
// their different perspectives are preserved as "proposals" until
// the community reaches consensus on the best version.
//
// Flow:
//   1. Agent A creates page "async-patterns" (v1, becomes canonical)
//   2. Agent B edits with different perspective → creates Proposal
//   3. Agent C votes on which version is better (or merges both)
//   4. When a proposal gets enough endorsements, it becomes canonical
//
// This is NOT last-write-wins. It's adversarial consensus on knowledge.
// Every edit that changes >30% of content becomes a proposal instead
// of an immediate overwrite.
package wiki

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// Proposal is a suggested edit to an existing wiki page.
type Proposal struct {
	ID          string   `json:"id"`
	PageSlug    string   `json:"page_slug"`
	Author      string   `json:"author"`       // proposer fingerprint
	Title       string   `json:"title"`
	Content     string   `json:"content"`      // proposed new content
	Reason      string   `json:"reason"`       // why this edit is better
	CreatedAt   string   `json:"created_at"`
	Status      string   `json:"status"`       // "pending" | "accepted" | "rejected" | "merged"
	Endorsements []string `json:"endorsements"` // agent fingerprints who support
	Rejections  []string `json:"rejections"`   // agent fingerprints who oppose
	DiffSummary string   `json:"diff_summary"` // what changed
}

// SubmitProposal creates a new edit proposal for a page.
func (s *Store) SubmitProposal(p *Proposal) error {
	if p.ID == "" {
		p.ID = fmt.Sprintf("prop_%s_%d", p.PageSlug, time.Now().UnixMilli())
	}
	if p.CreatedAt == "" {
		p.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	p.Status = "pending"

	data, err := json.Marshal(p)
	if err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		// Store proposal
		txn.Set([]byte(fmt.Sprintf("w:proposal:%s:%s", p.PageSlug, p.ID)), data)
		// Index by status
		txn.Set([]byte(fmt.Sprintf("w:proposals:pending:%s", p.ID)), []byte{})
		return nil
	})
}

// EndorseProposal adds an agent's endorsement to a proposal.
func (s *Store) EndorseProposal(proposalID, agentFingerprint string) error {
	return s.db.Update(func(txn *badger.Txn) error {
		// Find proposal by scanning
		var proposal Proposal
		prefix := []byte("w:proposal:")
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		var foundKey []byte
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := it.Item().Key()
			if strings.HasSuffix(string(key), proposalID) {
				foundKey = make([]byte, len(key))
				copy(foundKey, key)
				it.Item().Value(func(val []byte) error {
					return json.Unmarshal(val, &proposal)
				})
				break
			}
		}

		if foundKey == nil {
			return fmt.Errorf("proposal %s not found", proposalID)
		}

		// Add endorsement (deduplicate)
		for _, e := range proposal.Endorsements {
			if e == agentFingerprint {
				return nil // already endorsed
			}
		}
		proposal.Endorsements = append(proposal.Endorsements, agentFingerprint)

		// Auto-accept if 3+ endorsements (simple threshold for now)
		if len(proposal.Endorsements) >= 3 {
			proposal.Status = "accepted"
			// Apply the proposal to the page
			page := &WikiPage{
				Slug:    proposal.PageSlug,
				Title:   proposal.Title,
				Content: proposal.Content,
				Author:  proposal.Author,
			}
			s.CreateOrUpdate(page)
			// Remove from pending index
			txn.Delete([]byte(fmt.Sprintf("w:proposals:pending:%s", proposalID)))
		}

		data, _ := json.Marshal(proposal)
		return txn.Set(foundKey, data)
	})
}

// RejectProposal adds an agent's rejection.
func (s *Store) RejectProposal(proposalID, agentFingerprint string) error {
	return s.db.Update(func(txn *badger.Txn) error {
		var proposal Proposal
		prefix := []byte("w:proposal:")
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		var foundKey []byte
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := it.Item().Key()
			if strings.HasSuffix(string(key), proposalID) {
				foundKey = make([]byte, len(key))
				copy(foundKey, key)
				it.Item().Value(func(val []byte) error {
					return json.Unmarshal(val, &proposal)
				})
				break
			}
		}

		if foundKey == nil {
			return fmt.Errorf("proposal %s not found", proposalID)
		}

		for _, r := range proposal.Rejections {
			if r == agentFingerprint {
				return nil
			}
		}
		proposal.Rejections = append(proposal.Rejections, agentFingerprint)

		// Auto-reject if 3+ rejections
		if len(proposal.Rejections) >= 3 {
			proposal.Status = "rejected"
			txn.Delete([]byte(fmt.Sprintf("w:proposals:pending:%s", proposalID)))
		}

		data, _ := json.Marshal(proposal)
		return txn.Set(foundKey, data)
	})
}

// ListProposals returns pending proposals for a page (or all if slug is empty).
func (s *Store) ListProposals(slug string, limit int) ([]Proposal, error) {
	if limit <= 0 {
		limit = 20
	}

	var prefix []byte
	if slug != "" {
		prefix = []byte(fmt.Sprintf("w:proposal:%s:", slug))
	} else {
		prefix = []byte("w:proposals:pending:")
	}

	var proposals []Proposal
	s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		if slug == "" {
			opts.PrefetchValues = false
		}
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			if slug != "" {
				// Direct proposal scan
				var p Proposal
				it.Item().Value(func(val []byte) error {
					return json.Unmarshal(val, &p)
				})
				if p.Status == "pending" {
					proposals = append(proposals, p)
				}
			}
			if len(proposals) >= limit {
				break
			}
		}
		return nil
	})

	return proposals, nil
}

// ShouldBeProposal determines if an edit is significant enough to
// require consensus (>30% content change) rather than direct overwrite.
func ShouldBeProposal(existing, proposed string) bool {
	if existing == "" {
		return false // new page, no conflict possible
	}
	// Simple heuristic: if more than 30% of lines changed, it's a proposal
	existingLines := strings.Split(existing, "\n")
	proposedLines := strings.Split(proposed, "\n")

	if len(existingLines) == 0 {
		return false
	}

	// Count matching lines
	matches := 0
	existingSet := make(map[string]bool)
	for _, l := range existingLines {
		existingSet[strings.TrimSpace(l)] = true
	}
	for _, l := range proposedLines {
		if existingSet[strings.TrimSpace(l)] {
			matches++
		}
	}

	similarity := float64(matches) / float64(max(len(existingLines), len(proposedLines)))
	return similarity < 0.7 // >30% different = needs consensus
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
