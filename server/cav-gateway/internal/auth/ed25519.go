package auth

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"strings"
)

// VerifyEd25519Signature verifies an Ed25519 signature against a DID's public key.
//
// DID format: did:key:z<base64url of 0xed01 + pubkey>
// Signature: base64url encoded Ed25519 signature
// Message: raw bytes that were signed
func VerifyEd25519Signature(did, signatureB64 string, message []byte) error {
	// Extract public key from DID
	pubKey, err := PubKeyFromDID(did)
	if err != nil {
		return fmt.Errorf("invalid DID: %w", err)
	}

	// Decode signature
	sig, err := base64.RawURLEncoding.DecodeString(signatureB64)
	if err != nil {
		return fmt.Errorf("invalid signature encoding: %w", err)
	}

	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature length: got %d, want %d", len(sig), ed25519.SignatureSize)
	}

	// Verify
	if !ed25519.Verify(pubKey, message, sig) {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}

// PubKeyFromDID extracts the Ed25519 public key from a did:key identifier.
//
// Expected format: did:key:z<base64url of multicodec-prefixed key>
// Multicodec prefix for Ed25519: 0xed 0x01
func PubKeyFromDID(did string) (ed25519.PublicKey, error) {
	if !strings.HasPrefix(did, "did:key:z") {
		return nil, fmt.Errorf("unsupported DID format (expected did:key:z...)")
	}

	// Strip "did:key:z" prefix
	encoded := strings.TrimPrefix(did, "did:key:z")

	// Decode base64url
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("base64url decode failed: %w", err)
	}

	// Check multicodec prefix (0xed, 0x01 for Ed25519)
	if len(decoded) < 2 {
		return nil, fmt.Errorf("decoded key too short")
	}
	if decoded[0] != 0xed || decoded[1] != 0x01 {
		return nil, fmt.Errorf("unsupported key type (expected Ed25519 multicodec prefix 0xed01)")
	}

	// Extract raw public key (32 bytes after prefix)
	pubKeyBytes := decoded[2:]
	if len(pubKeyBytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid public key length: got %d, want %d", len(pubKeyBytes), ed25519.PublicKeySize)
	}

	return ed25519.PublicKey(pubKeyBytes), nil
}
