package identity

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// DIDFromPublicKey derives a did:key identifier from an Ed25519 public key.
//
// Format: did:key:z<base64url(0xed01 + pubkey)>
//
// This is compatible with cav-gateway/internal/auth/ed25519.go PubKeyFromDID.
// The multicodec prefix 0xed 0x01 identifies Ed25519 public keys.
func DIDFromPublicKey(pub ed25519.PublicKey) string {
	// Multicodec prefix for Ed25519: 0xed, 0x01
	prefixed := make([]byte, 2+len(pub))
	prefixed[0] = 0xed
	prefixed[1] = 0x01
	copy(prefixed[2:], pub)

	encoded := base64.RawURLEncoding.EncodeToString(prefixed)
	return "did:key:z" + encoded
}

// PubKeyFromDID extracts the Ed25519 public key from a did:key identifier.
// Compatible with cav-gateway's implementation.
func PubKeyFromDID(did string) (ed25519.PublicKey, error) {
	if !strings.HasPrefix(did, "did:key:z") {
		return nil, fmt.Errorf("identity: unsupported DID format (expected did:key:z...)")
	}

	encoded := strings.TrimPrefix(did, "did:key:z")

	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("identity: base64url decode failed: %w", err)
	}

	if len(decoded) < 2 {
		return nil, fmt.Errorf("identity: decoded key too short")
	}
	if decoded[0] != 0xed || decoded[1] != 0x01 {
		return nil, fmt.Errorf("identity: unsupported key type (expected Ed25519 multicodec prefix 0xed01)")
	}

	pubKeyBytes := decoded[2:]
	if len(pubKeyBytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("identity: invalid public key length: got %d, want %d", len(pubKeyBytes), ed25519.PublicKeySize)
	}

	return ed25519.PublicKey(pubKeyBytes), nil
}

// FingerprintFromPublicKey computes the CAV fingerprint from an Ed25519 public key.
// Format: CAV-XXXX-XXXX-XXXX-XXXX (16 hex chars from SHA-256 of pubkey)
//
// Compatible with cav-gateway/internal/auth/fingerprint.go ComputeFingerprint.
func FingerprintFromPublicKey(pub ed25519.PublicKey) string {
	hash := sha256.Sum256(pub)
	hexStr := strings.ToUpper(hex.EncodeToString(hash[:8]))
	return fmt.Sprintf("CAV-%s-%s-%s-%s", hexStr[0:4], hexStr[4:8], hexStr[8:12], hexStr[12:16])
}

// FingerprintFromDID computes the fingerprint from a did:key identifier.
func FingerprintFromDID(did string) (string, error) {
	pub, err := PubKeyFromDID(did)
	if err != nil {
		return "", err
	}
	return FingerprintFromPublicKey(pub), nil
}
