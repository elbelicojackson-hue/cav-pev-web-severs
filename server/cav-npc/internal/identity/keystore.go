// Package identity manages Ed25519 key pairs for NPC instances.
//
// Each NPC has a unique key pair stored on disk. The key pair is used for:
//   - DID derivation (did:key:z<base64url(0xed01 + pubkey)>)
//   - Gateway authentication (challenge-verify flow)
//   - Signal signing (JCS + Ed25519)
//   - Behavioral digest signing
package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// KeyPair holds an Ed25519 identity for a single NPC instance.
type KeyPair struct {
	DID        string
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
}

// Keystore manages key pairs on the filesystem.
type Keystore struct {
	dir string
}

// NewKeystore creates a Keystore rooted at the given directory.
// The directory is created with 0700 permissions if it doesn't exist.
func NewKeystore(dir string) (*Keystore, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("identity: create keys dir: %w", err)
	}

	// Verify directory permissions (skip on Windows — NTFS ACLs differ)
	if runtime.GOOS != "windows" {
		info, err := os.Stat(dir)
		if err != nil {
			return nil, fmt.Errorf("identity: stat keys dir: %w", err)
		}
		if perm := info.Mode().Perm(); perm&0077 != 0 {
			return nil, fmt.Errorf("identity: keys dir %q has insecure permissions %o (want 0700)", dir, perm)
		}
	}

	return &Keystore{dir: dir}, nil
}

// LoadOrGenerate loads an existing key pair for the named NPC, or generates
// a new one if none exists. Key files are stored as:
//   - <dir>/<name>.ed25519  (64-byte private key, permissions 0600)
//   - <dir>/<name>.did      (DID string cache)
func (k *Keystore) LoadOrGenerate(name string) (*KeyPair, error) {
	privPath := filepath.Join(k.dir, name+".ed25519")
	didPath := filepath.Join(k.dir, name+".did")

	// Try loading existing key
	privBytes, err := os.ReadFile(privPath)
	if err == nil {
		// Verify file permissions (skip on Windows)
		if runtime.GOOS != "windows" {
			if err := checkFilePermissions(privPath); err != nil {
				return nil, err
			}
		}

		if len(privBytes) != ed25519.PrivateKeySize {
			return nil, fmt.Errorf("identity: %s has invalid key size %d (want %d)", privPath, len(privBytes), ed25519.PrivateKeySize)
		}

		priv := ed25519.PrivateKey(privBytes)
		pub := priv.Public().(ed25519.PublicKey)
		did := DIDFromPublicKey(pub)

		return &KeyPair{
			DID:        did,
			PublicKey:  pub,
			PrivateKey: priv,
		}, nil
	}

	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("identity: read key file: %w", err)
	}

	// Generate new key pair
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("identity: generate key: %w", err)
	}

	// Write private key with strict permissions
	if err := os.WriteFile(privPath, priv, 0600); err != nil {
		return nil, fmt.Errorf("identity: write private key: %w", err)
	}

	// Write DID cache
	did := DIDFromPublicKey(pub)
	if err := os.WriteFile(didPath, []byte(did), 0600); err != nil {
		// Non-fatal — DID can be recomputed
		_ = err
	}

	return &KeyPair{
		DID:        did,
		PublicKey:  pub,
		PrivateKey: priv,
	}, nil
}

// checkFilePermissions verifies a file has 0600 permissions (owner read/write only).
func checkFilePermissions(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("identity: stat %s: %w", path, err)
	}
	perm := info.Mode().Perm()
	if perm&0077 != 0 {
		return fmt.Errorf("identity: %s has insecure permissions %o (want 0600)", path, perm)
	}
	return nil
}
