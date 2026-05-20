// Package digest implements the agent-self-signed BehavioralDigest from
// cav-social-trust §R8 / §5.6.
//
// Each active agent emits a digest hourly. The digest contains public-only
// statistics (vote alignment with majority, unique active domains, signal
// diversity entropy) — never the agent's private trust graph. The agent
// signs the JCS-canonicalized payload with their Ed25519 private key. The
// gateway verifies the signature against the citizen's stored public key
// (citizen.PubKey) and persists the digest under d:digest:<did>:<period_ms>.
//
// Inactive detection: a citizen with no fresh digest in the last 7 days is
// flagged on their reputation vector (reputation.SetInactive(true)) which
// halves their effective convergence weight. The flag clears automatically
// when a fresh digest arrives.

package digest

import "time"

// SchemaVersion is stamped into every digest for forward-compat audit.
const SchemaVersion = "1.0"

// InactivityWindow is how stale the latest digest can be before the citizen
// is flagged inactive. Per R8-6.
var InactivityWindow = 7 * 24 * time.Hour

// BehavioralDigest is the public hourly statistics envelope.
//
// Wire format: when serialized for signing, the Signature and PublicKey
// fields are blanked, then JCS-canonicalized. The digest is round-tripped
// through this same canonicalization on the receive side.
type BehavioralDigest struct {
	DID           string    `json:"did"`
	PeriodStart   time.Time `json:"period_start"`
	PeriodEnd     time.Time `json:"period_end"`
	SchemaVersion string    `json:"schema_version"`

	// Public statistics — see R8-2.
	VoteAlignmentWithMajority float64 `json:"vote_alignment_with_majority"` // [0, 1]
	UniqueDomainsActive       int     `json:"unique_domains_active"`
	SignalDiversityEntropy    float64 `json:"signal_diversity_entropy"`

	// Aggregate counts for transparency.
	SignalCount int `json:"signal_count"`
	VoteCount   int `json:"vote_count"`

	// Set by the agent. Verified by the gateway.
	Signature string `json:"signature,omitempty"` // base64 RawURLEncoding
	PublicKey string `json:"public_key,omitempty"` // base64 RawURLEncoding (Ed25519, 32 bytes)
}
