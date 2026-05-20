package praxon

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropic-cav/cav-node/internal/jcs"
)

// Gate1 errors
var (
	ErrEmptyGrounding     = errors.New("grounding array is empty (Axiom 4 violation)")
	ErrInvalidVersion     = errors.New("unsupported version")
	ErrInvalidPraxonClass = errors.New("invalid praxon_class")
	ErrHashMismatch       = errors.New("praxon_id does not match computed hash")
	ErrSignatureInvalid   = errors.New("Ed25519 signature verification failed")
	ErrMalformedIssuer    = errors.New("issuer is not a valid did:key or did:cav:thread")
	ErrTooLarge           = errors.New("praxon exceeds 256KB size limit")

	// Thread-issuer specific.
	ErrThreadMissingEpisode = errors.New("thread-issued praxon missing provenance.consensus_episode")
	ErrThreadInsufficientDerivedFrom = errors.New("thread-issued praxon must reference at least 2 derived_from signals")
)

const maxPraxonBytes = 256 * 1024

// threadIssuerPrefix marks Praxons synthesized by the social-trust thread
// crystallizer (cav-social-trust §R5). These have no private key — their
// authenticity comes from the signed signals they aggregate
// (provenance.derived_from), so we skip the Ed25519 path and instead
// require provenance metadata that lets verifiers re-derive the chain.
const threadIssuerPrefix = "did:cav:thread:"

// ValidateGate1 performs Gate 1 validation: schema + hash + signature.
// This is the only validation the node server performs — Gates 2 and 3
// are agent-side responsibilities.
func ValidateGate1(raw []byte) (*Praxon, error) {
	// Size check
	if len(raw) > maxPraxonBytes {
		return nil, ErrTooLarge
	}

	// Parse
	var p Praxon
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("JSON parse error: %w", err)
	}

	// Version
	if p.Version != "1.0" {
		return nil, ErrInvalidVersion
	}

	// PraxonClass
	switch p.PraxonClass {
	case ClassOperational, ClassDeliberationMotion, ClassDeliberationResolution:
		// ok
	default:
		return nil, ErrInvalidPraxonClass
	}

	// Grounding non-empty (Axiom 4)
	if len(p.Grounding) == 0 {
		return nil, ErrEmptyGrounding
	}

	// Issuer format
	switch {
	case strings.HasPrefix(p.Issuer, "did:key:z"):
		// standard signed Praxon path — falls through to hash + signature check
	case strings.HasPrefix(p.Issuer, threadIssuerPrefix):
		// Thread-crystallized Praxon. Authenticity is provided structurally:
		//   - consensus_episode must reference the thread that produced this
		//   - derived_from must list ≥2 signals (otherwise it isn't a thread,
		//     it's a single agent's signal in disguise)
		// We still recompute the hash in the standard path below so the
		// praxon_id is content-addressable, but we skip signature
		// verification (no key to verify against).
		if p.Provenance.ConsensusEpisode == "" {
			return nil, ErrThreadMissingEpisode
		}
		if len(p.Provenance.DerivedFrom) < 2 {
			return nil, ErrThreadInsufficientDerivedFrom
		}
		// Extra structural check: the thread ID encoded in the issuer must
		// match the consensus_episode reference, otherwise an attacker could
		// claim to be one thread while pointing at another.
		threadID := strings.TrimPrefix(p.Issuer, threadIssuerPrefix)
		if threadID == "" || threadID != p.Provenance.ConsensusEpisode {
			return nil, fmt.Errorf("%w: issuer thread_id != consensus_episode", ErrMalformedIssuer)
		}
	default:
		return nil, ErrMalformedIssuer
	}

	// Hash verification: recompute praxon_id from body without praxon_id and signature
	computedID, err := computeID(raw)
	if err != nil {
		return nil, fmt.Errorf("hash computation failed: %w", err)
	}
	if computedID != p.PraxonID {
		return nil, ErrHashMismatch
	}

	// Signature verification — only for did:key issuers. Thread-issued
	// Praxons (did:cav:thread:*) are not individually signed; their
	// authenticity flows from the signed signals in derived_from.
	if strings.HasPrefix(p.Issuer, "did:key:z") {
		if err := verifySignature(&p, raw); err != nil {
			return nil, err
		}
	}

	return &p, nil
}

// computeID recomputes the praxon_id from the raw JSON.
// It removes praxon_id and signature fields, then JCS-canonicalizes and SHA-256 hashes.
func computeID(raw []byte) (string, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", err
	}
	delete(obj, "praxon_id")
	delete(obj, "signature")

	stripped, err := json.Marshal(obj)
	if err != nil {
		return "", err
	}

	canonical, err := jcs.Canonicalize(stripped)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(canonical)
	return hex.EncodeToString(hash[:]), nil
}

// verifySignature checks the Ed25519 signature over the JCS-canonicalized body
// (with praxon_id present, signature removed).
func verifySignature(p *Praxon, raw []byte) error {
	// Extract public key from did:key
	pubKey, err := PublicKeyFromDID(p.Issuer)
	if err != nil {
		return fmt.Errorf("cannot extract public key: %w", err)
	}

	// Build signing payload: body with praxon_id, without signature
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return err
	}
	delete(obj, "signature")

	payload, err := json.Marshal(obj)
	if err != nil {
		return err
	}

	canonical, err := jcs.Canonicalize(payload)
	if err != nil {
		return err
	}

	// Decode signature
	sig, err := base64.RawURLEncoding.DecodeString(p.Signature)
	if err != nil {
		return fmt.Errorf("signature base64 decode failed: %w", err)
	}

	if !ed25519.Verify(pubKey, canonical, sig) {
		return ErrSignatureInvalid
	}

	return nil
}

// PublicKeyFromDID extracts an Ed25519 public key from a did:key:z... identifier.
// Format: did:key:z + base58btc(multicodec_prefix + raw_pubkey)
// Ed25519 multicodec prefix: 0xed01
func PublicKeyFromDID(did string) (ed25519.PublicKey, error) {
	if !strings.HasPrefix(did, "did:key:z") {
		return nil, ErrMalformedIssuer
	}

	encoded := did[len("did:key:z"):]
	decoded, err := base58Decode(encoded)
	if err != nil {
		return nil, fmt.Errorf("base58 decode failed: %w", err)
	}

	// Check multicodec prefix for Ed25519: 0xed 0x01
	if len(decoded) < 34 || decoded[0] != 0xed || decoded[1] != 0x01 {
		return nil, fmt.Errorf("not an Ed25519 did:key (wrong multicodec prefix)")
	}

	return ed25519.PublicKey(decoded[2:]), nil
}

// base58Decode decodes a base58btc string (Bitcoin alphabet).
func base58Decode(s string) ([]byte, error) {
	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := strings.IndexByte(alphabet, s[i])
		if c < 0 {
			return nil, fmt.Errorf("invalid base58 character: %c", s[i])
		}
		carry := c
		for j := 0; j < len(result); j++ {
			carry += int(result[j]) * 58
			result[j] = byte(carry & 0xff)
			carry >>= 8
		}
		for carry > 0 {
			result = append(result, byte(carry&0xff))
			carry >>= 8
		}
	}

	// Leading zeros
	for i := 0; i < len(s) && s[i] == '1'; i++ {
		result = append(result, 0)
	}

	// Reverse
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return result, nil
}
