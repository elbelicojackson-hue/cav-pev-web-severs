// Sign / verify for BehavioralDigest.
//
// Canonicalization rule (mirrors risk/audit.go canonicalize): blank the
// signature/public_key fields, then sort all object keys at every depth via
// json round-trip. SHA-2 isn't part of the path — Ed25519 signs the canonical
// bytes directly.

package digest

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// Sentinel errors.
var (
	ErrMissingDID        = errors.New("digest: missing DID")
	ErrInvalidPeriod     = errors.New("digest: period_start must be before period_end")
	ErrInvalidPubkey     = errors.New("digest: public key must be 32-byte Ed25519")
	ErrSignatureMismatch = errors.New("digest: Ed25519 signature verification failed")
	ErrPubkeyChanged     = errors.New("digest: public key differs from registered citizen pubkey")
)

// Sign canonicalizes `d`, signs with `priv`, and returns a copy with
// Signature + PublicKey populated. Caller's d is NOT mutated.
func Sign(d *BehavioralDigest, priv ed25519.PrivateKey) (*BehavioralDigest, error) {
	if d == nil {
		return nil, errors.New("digest: nil")
	}
	if len(priv) != ed25519.PrivateKeySize {
		return nil, errors.New("digest: invalid private key size")
	}
	cp := *d
	cp.SchemaVersion = SchemaVersion
	cp.Signature = ""
	cp.PublicKey = ""

	canon, err := canonicalBytes(&cp)
	if err != nil {
		return nil, err
	}
	sig := ed25519.Sign(priv, canon)
	cp.Signature = base64.RawURLEncoding.EncodeToString(sig)
	cp.PublicKey = base64.RawURLEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
	return &cp, nil
}

// Verify checks structural correctness + Ed25519 signature over the
// canonicalized payload. `expectedPubKey` may be empty during early
// bootstrapping; once a citizen's PubKey has been registered (via the auth
// flow or via SetPubKey), the gateway should pass it here so a malicious
// peer can't supply a different key alongside a valid signature.
func Verify(d *BehavioralDigest, expectedPubKey string) error {
	if d == nil {
		return errors.New("digest: nil")
	}
	if d.DID == "" {
		return ErrMissingDID
	}
	if !d.PeriodEnd.After(d.PeriodStart) {
		return ErrInvalidPeriod
	}
	if d.Signature == "" || d.PublicKey == "" {
		return errors.New("digest: missing signature or public key")
	}

	pub, err := base64.RawURLEncoding.DecodeString(d.PublicKey)
	if err != nil {
		return fmt.Errorf("digest: pubkey decode: %w", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		return ErrInvalidPubkey
	}
	if expectedPubKey != "" && expectedPubKey != d.PublicKey {
		return ErrPubkeyChanged
	}

	sig, err := base64.RawURLEncoding.DecodeString(d.Signature)
	if err != nil {
		return fmt.Errorf("digest: signature decode: %w", err)
	}

	cp := *d
	cp.Signature = ""
	cp.PublicKey = ""
	canon, err := canonicalBytes(&cp)
	if err != nil {
		return err
	}
	if !ed25519.Verify(pub, canon, sig) {
		return ErrSignatureMismatch
	}
	return nil
}

// === Canonicalization ===

// canonicalBytes returns the JCS-style canonical encoding of the digest.
// Object keys are sorted alphabetically at every depth; numbers preserve
// their JSON-Number representation to avoid float drift.
func canonicalBytes(d *BehavioralDigest) ([]byte, error) {
	intermediate, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(intermediate))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return json.Marshal(sortKeys(v))
}

func sortKeys(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]interface{}, len(t))
		for _, k := range keys {
			out[k] = sortKeys(t[k])
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, e := range t {
			out[i] = sortKeys(e)
		}
		return out
	default:
		return v
	}
}
