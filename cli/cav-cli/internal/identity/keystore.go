// Package identity manages Ed25519 key pairs for CAV citizen identity.
//
// Keys are stored at ~/.cav/identity.json in the format:
//
//	{
//	  "did": "did:key:z6Mk...",
//	  "fingerprint": "CAV-XXXX-XXXX-XXXX-XXXX",
//	  "public_key": "base64url",
//	  "private_key": "base64url (encrypted in future)"
//	}
package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Identity holds the Ed25519 key pair and derived DID.
type Identity struct {
	DID         string `json:"did"`
	Fingerprint string `json:"fingerprint"`
	PublicKey    string `json:"public_key"`
	PrivateKey  string `json:"private_key"`
}

// cavDir returns ~/.cav/ path.
func cavDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cav")
}

// identityPath returns ~/.cav/identity.json path.
func identityPath() string {
	return filepath.Join(cavDir(), "identity.json")
}

// SessionPath returns ~/.cav/session.json path.
func SessionPath() string {
	return filepath.Join(cavDir(), "session.json")
}

// Exists checks if an identity file already exists.
func Exists() bool {
	_, err := os.Stat(identityPath())
	return err == nil
}

// Generate creates a new Ed25519 key pair and saves it.
func Generate() (*Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("keygen failed: %w", err)
	}

	did := pubKeyToDID(pub)
	fingerprint := computeFingerprint(pub)
	id := &Identity{
		DID:         did,
		Fingerprint: fingerprint,
		PublicKey:    base64.RawURLEncoding.EncodeToString(pub),
		PrivateKey:  base64.RawURLEncoding.EncodeToString(priv),
	}

	// Ensure directory exists
	if err := os.MkdirAll(cavDir(), 0700); err != nil {
		return nil, err
	}

	data, _ := json.MarshalIndent(id, "", "  ")
	if err := os.WriteFile(identityPath(), data, 0600); err != nil {
		return nil, err
	}

	return id, nil
}

// Load reads the identity from disk.
func Load() (*Identity, error) {
	data, err := os.ReadFile(identityPath())
	if err != nil {
		return nil, fmt.Errorf("no identity found (run 'cav-cli init' first): %w", err)
	}
	var id Identity
	if err := json.Unmarshal(data, &id); err != nil {
		return nil, err
	}
	return &id, nil
}

// PrivateKeyBytes decodes the private key from base64url.
func (id *Identity) PrivateKeyBytes() (ed25519.PrivateKey, error) {
	b, err := base64.RawURLEncoding.DecodeString(id.PrivateKey)
	if err != nil {
		return nil, err
	}
	return ed25519.PrivateKey(b), nil
}

// PublicKeyBytes decodes the public key from base64url.
func (id *Identity) PublicKeyBytes() (ed25519.PublicKey, error) {
	b, err := base64.RawURLEncoding.DecodeString(id.PublicKey)
	if err != nil {
		return nil, err
	}
	return ed25519.PublicKey(b), nil
}

// Sign signs a message with the identity's private key.
func (id *Identity) Sign(message []byte) (string, error) {
	priv, err := id.PrivateKeyBytes()
	if err != nil {
		return "", err
	}
	sig := ed25519.Sign(priv, message)
	return base64.RawURLEncoding.EncodeToString(sig), nil
}

// pubKeyToDID converts an Ed25519 public key to a did:key identifier.
// Format: did:key:z6Mk<base58btc of multicodec-prefixed pubkey>
// Simplified: we use base64url for MVP (proper multibase in v2).
func pubKeyToDID(pub ed25519.PublicKey) string {
	// Multicodec prefix for Ed25519 public key: 0xed01
	prefixed := append([]byte{0xed, 0x01}, pub...)
	encoded := base64.RawURLEncoding.EncodeToString(prefixed)
	return "did:key:z" + encoded
}

// computeFingerprint derives CAV-XXXX-XXXX-XXXX-XXXX from the public key.
// This is the agent's unique, human-readable identity on the network.
func computeFingerprint(pub ed25519.PublicKey) string {
	hash := sha256.Sum256(pub)
	hexStr := strings.ToUpper(hex.EncodeToString(hash[:8]))
	return fmt.Sprintf("CAV-%s-%s-%s-%s", hexStr[0:4], hexStr[4:8], hexStr[8:12], hexStr[12:16])
}
