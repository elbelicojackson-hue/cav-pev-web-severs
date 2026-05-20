// Package secrets provides an AES-256-GCM encrypted local vault for API keys.
//
// The vault stores keys in a JSON file encrypted with a master password.
// At startup, the master password is provided via:
//   - CAV_NPC_MASTER_PASSWORD environment variable, or
//   - --master-password flag, or
//   - Interactive prompt (if stdin is a terminal)
//
// File format:
//
//	{
//	  "version": 1,
//	  "salt": "<base64 32-byte salt>",
//	  "nonce": "<base64 12-byte nonce>",
//	  "ciphertext": "<base64 AES-256-GCM encrypted payload>"
//	}
//
// The plaintext payload is a JSON object mapping key names to values:
//
//	{
//	  "DEEPSEEK_API_KEY": "sk-...",
//	  "VOLCENGINE_API_KEY": "...",
//	  "DASHSCOPE_API_KEY": "sk-..."
//	}
//
// Key derivation: Argon2id (time=3, memory=64MB, threads=4, keyLen=32)
// Encryption: AES-256-GCM (standard 12-byte nonce, 16-byte tag)
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"golang.org/x/crypto/argon2"
)

// Vault holds decrypted API keys in memory.
type Vault struct {
	keys map[string]string
}

// VaultFile is the on-disk encrypted format.
type VaultFile struct {
	Version    int    `json:"version"`
	Salt       string `json:"salt"`       // base64
	Nonce      string `json:"nonce"`      // base64
	Ciphertext string `json:"ciphertext"` // base64
}

// Argon2id parameters (tuned for server deployment — ~200ms on modern hardware)
const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // 64 MB
	argonThreads = 4
	argonKeyLen  = 32
	saltLen      = 32
	nonceLen     = 12 // AES-GCM standard
)

var (
	ErrBadPassword  = errors.New("secrets: decryption failed (wrong password or corrupted file)")
	ErrKeyNotFound  = errors.New("secrets: key not found in vault")
	ErrNoVaultFile  = errors.New("secrets: vault file not found")
)

// Open decrypts and loads a vault file using the given master password.
func Open(path, masterPassword string) (*Vault, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoVaultFile
		}
		return nil, fmt.Errorf("secrets: read vault: %w", err)
	}

	var vf VaultFile
	if err := json.Unmarshal(data, &vf); err != nil {
		return nil, fmt.Errorf("secrets: parse vault file: %w", err)
	}

	if vf.Version != 1 {
		return nil, fmt.Errorf("secrets: unsupported vault version %d", vf.Version)
	}

	salt, err := base64.StdEncoding.DecodeString(vf.Salt)
	if err != nil {
		return nil, fmt.Errorf("secrets: decode salt: %w", err)
	}

	nonce, err := base64.StdEncoding.DecodeString(vf.Nonce)
	if err != nil {
		return nil, fmt.Errorf("secrets: decode nonce: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(vf.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("secrets: decode ciphertext: %w", err)
	}

	// Derive key from password
	key := argon2.IDKey([]byte(masterPassword), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	// Decrypt
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secrets: create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secrets: create GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrBadPassword
	}

	// Parse decrypted JSON
	var keys map[string]string
	if err := json.Unmarshal(plaintext, &keys); err != nil {
		return nil, fmt.Errorf("secrets: parse decrypted payload: %w", err)
	}

	return &Vault{keys: keys}, nil
}

// Get retrieves a key by name. Returns ErrKeyNotFound if not present.
func (v *Vault) Get(name string) (string, error) {
	val, ok := v.keys[name]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrKeyNotFound, name)
	}
	return val, nil
}

// Has checks if a key exists in the vault.
func (v *Vault) Has(name string) bool {
	_, ok := v.keys[name]
	return ok
}

// Keys returns all key names (not values) in the vault.
func (v *Vault) Keys() []string {
	names := make([]string, 0, len(v.keys))
	for k := range v.keys {
		names = append(names, k)
	}
	return names
}

// InjectEnv sets all vault keys as environment variables.
// This allows the existing config.Validate() to work unchanged
// (it reads api_key_env from os.Getenv).
func (v *Vault) InjectEnv() {
	for k, val := range v.keys {
		os.Setenv(k, val)
	}
}

// --- Vault creation (used by CLI tool) ---

// Create encrypts a set of keys and writes them to a vault file.
func Create(path, masterPassword string, keys map[string]string) error {
	// Serialize plaintext
	plaintext, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("secrets: marshal keys: %w", err)
	}

	// Generate salt
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("secrets: generate salt: %w", err)
	}

	// Derive key
	key := argon2.IDKey([]byte(masterPassword), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	// Encrypt
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("secrets: create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("secrets: create GCM: %w", err)
	}

	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("secrets: generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	// Write vault file
	vf := VaultFile{
		Version:    1,
		Salt:       base64.StdEncoding.EncodeToString(salt),
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	}

	data, err := json.MarshalIndent(vf, "", "  ")
	if err != nil {
		return fmt.Errorf("secrets: marshal vault file: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("secrets: write vault file: %w", err)
	}

	return nil
}
