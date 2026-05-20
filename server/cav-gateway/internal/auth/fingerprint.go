// Package auth — Agent Fingerprint system.
//
// CAV uses Ed25519 asymmetric cryptography for agent identity:
//
//   Private Key (agent-local, NEVER leaves the machine)
//       ↓ derives
//   Public Key (shared with gateway)
//       ↓ SHA-256 hash
//   Fingerprint: CAV-XXXX-XXXX-XXXX-XXXX
//
// Authentication flow (zero-knowledge proof of key ownership):
//
//   1. Agent sends fingerprint + public key to gateway
//   2. Gateway generates random challenge (32 bytes)
//   3. Gateway encrypts challenge with agent's PUBLIC key (X25519 ECDH + AES)
//   4. Agent decrypts with PRIVATE key → proves ownership
//   5. Agent signs the decrypted challenge → proves identity
//   6. Gateway verifies signature → issues JWT
//
// This is stronger than simple sign-challenge because:
//   - The challenge is ENCRYPTED to the agent (only they can read it)
//   - The response is SIGNED by the agent (proves they have the key)
//   - Double proof: decryption + signature = undeniable identity
//
// Fingerprint properties:
//   - Deterministic: same key → same fingerprint (always)
//   - Short: 16 hex chars (64 bits) — human-readable
//   - Cryptographically bound: forging requires breaking SHA-256
//   - Format: CAV-XXXX-XXXX-XXXX-XXXX
package auth

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// ComputeFingerprint derives the agent fingerprint from an Ed25519 public key.
// Format: CAV-XXXX-XXXX-XXXX-XXXX (16 hex chars from SHA-256 of pubkey)
//
// The fingerprint is the agent's "face" on the network — it appears in:
//   - CLI output (cav-cli status)
//   - Dashboard citizen list
//   - Praxon issuer field (human-readable alias)
//   - Challenge notifications
//   - Audit logs
func ComputeFingerprint(pubKey ed25519.PublicKey) string {
	hash := sha256.Sum256(pubKey)
	hexStr := strings.ToUpper(hex.EncodeToString(hash[:8]))
	return fmt.Sprintf("CAV-%s-%s-%s-%s", hexStr[0:4], hexStr[4:8], hexStr[8:12], hexStr[12:16])
}

// FingerprintFromDID computes the fingerprint from a did:key identifier.
// Returns empty string if the DID is invalid.
func FingerprintFromDID(did string) string {
	pubKey, err := PubKeyFromDID(did)
	if err != nil {
		return ""
	}
	return ComputeFingerprint(pubKey)
}

// ValidateFingerprint checks if a fingerprint matches a DID.
// This is the gateway's "door check" — only agents whose fingerprint
// matches their presented DID are allowed through.
func ValidateFingerprint(fingerprint, did string) bool {
	computed := FingerprintFromDID(did)
	return computed != "" && strings.EqualFold(computed, fingerprint)
}
