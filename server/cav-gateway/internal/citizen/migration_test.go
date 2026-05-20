package citizen

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

// TestLegacyJSONUnmarshal verifies that JSON written before the State/PubKey
// fields existed still deserializes cleanly into the extended struct.
func TestLegacyJSONUnmarshal(t *testing.T) {
	legacy := []byte(`{
		"did": "did:cav:0xabc",
		"fingerprint": "fp123",
		"level": 2,
		"verified_praxon_count": 5,
		"challenges_survived": 1,
		"registered_at": "2024-01-01T00:00:00Z",
		"last_seen_at": "2024-06-01T00:00:00Z"
	}`)

	var c Citizen
	if err := json.Unmarshal(legacy, &c); err != nil {
		t.Fatalf("legacy JSON failed to unmarshal: %v", err)
	}
	if c.State != "" {
		t.Errorf("expected empty State on legacy citizen, got %q", c.State)
	}
	if c.PubKey != "" {
		t.Errorf("expected empty PubKey on legacy citizen, got %q", c.PubKey)
	}
	if c.Level != 2 {
		t.Errorf("expected Level=2, got %d", c.Level)
	}
}

// TestEnsureRegisteredOmitsState verifies new citizens are stored without an
// explicit State (canary subsystem is responsible for setting it). Empty State
// must round-trip and be treated as active.
func TestEnsureRegisteredOmitsState(t *testing.T) {
	reg := NewRegistry()
	level := reg.EnsureRegistered("did:cav:new1", "fp-new1")
	if level != 1 {
		t.Errorf("expected level=1 for new citizen, got %d", level)
	}
	got := reg.Get("did:cav:new1")
	c, ok := got.(*Citizen)
	if !ok {
		t.Fatalf("expected *Citizen, got %T", got)
	}
	if c.State != "" {
		t.Errorf("new citizen State should be empty (probation hook not yet wired), got %q", c.State)
	}
	if reg.EffectiveState("did:cav:new1") != StateActive {
		t.Errorf("EffectiveState on empty State should be StateActive")
	}
}

func TestSetStateAndByState(t *testing.T) {
	reg := NewRegistry()
	reg.EnsureRegistered("did:cav:a", "fpA")
	reg.EnsureRegistered("did:cav:b", "fpB")
	reg.EnsureRegistered("did:cav:c", "fpC")

	if !reg.SetState("did:cav:a", StateProbation, nil) {
		t.Fatal("SetState returned false for known DID")
	}
	if reg.SetState("did:cav:missing", StateActive, nil) {
		t.Fatal("SetState should return false for unknown DID")
	}
	retry := time.Now().Add(24 * time.Hour)
	reg.SetState("did:cav:b", StateRestricted, &retry)

	probationers := reg.ByState(StateProbation)
	if len(probationers) != 1 || probationers[0].DID != "did:cav:a" {
		t.Errorf("expected exactly did:cav:a in probation, got %+v", probationers)
	}

	restricted := reg.ByState(StateRestricted)
	if len(restricted) != 1 || restricted[0].DID != "did:cav:b" {
		t.Errorf("expected exactly did:cav:b restricted, got %+v", restricted)
	}
	if restricted[0].NextRetryAt == nil || !restricted[0].NextRetryAt.Equal(retry) {
		t.Errorf("expected NextRetryAt=%v, got %v", retry, restricted[0].NextRetryAt)
	}

	// did:cav:c has empty State → should appear under StateActive (migration)
	actives := reg.ByState(StateActive)
	if len(actives) != 1 || actives[0].DID != "did:cav:c" {
		t.Errorf("expected did:cav:c in active (legacy migration), got %+v", actives)
	}
}

func TestSetStateClearsRetryWhenLeavingRestricted(t *testing.T) {
	reg := NewRegistry()
	reg.EnsureRegistered("did:cav:a", "fpA")
	retry := time.Now().Add(24 * time.Hour)
	reg.SetState("did:cav:a", StateRestricted, &retry)
	if reg.ByState(StateRestricted)[0].NextRetryAt == nil {
		t.Fatal("precondition: NextRetryAt should be set")
	}
	reg.SetState("did:cav:a", StateActive, nil)
	got := reg.Get("did:cav:a").(*Citizen)
	if got.NextRetryAt != nil {
		t.Errorf("NextRetryAt should be cleared when leaving Restricted, got %v", got.NextRetryAt)
	}
}

func TestSetPubKey(t *testing.T) {
	reg := NewRegistry()
	reg.EnsureRegistered("did:cav:a", "fpA")
	if !reg.SetPubKey("did:cav:a", "ed25519-pubkey-base64") {
		t.Fatal("SetPubKey returned false for known DID")
	}
	if reg.SetPubKey("did:cav:missing", "key") {
		t.Fatal("SetPubKey should return false for unknown DID")
	}
	got := reg.Get("did:cav:a").(*Citizen)
	if got.PubKey != "ed25519-pubkey-base64" {
		t.Errorf("expected PubKey to be set, got %q", got.PubKey)
	}
}

// TestPersistentMigration verifies that a PersistentRegistry restart does not
// corrupt or lose any of the new fields, and that legacy rows (no State) come
// back as effectively-active citizens.
func TestPersistentMigration(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "citizens")

	// Write a legacy row directly through the registry, then a fresh row,
	// then close and reopen — both must survive.
	reg, err := NewPersistentRegistry(dir)
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}

	reg.EnsureRegistered("did:cav:legacy", "fpL") // State is "" — represents legacy
	reg.EnsureRegistered("did:cav:fresh", "fpF")
	reg.SetState("did:cav:fresh", StateProbation, nil)
	retry := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	reg.SetState("did:cav:legacy", StateRestricted, &retry)
	reg.SetPubKey("did:cav:legacy", "pk1")

	if err := reg.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Reopen
	reg2, err := NewPersistentRegistry(dir)
	if err != nil {
		t.Fatalf("reopen registry: %v", err)
	}
	defer reg2.Close()

	// did:cav:legacy → restricted with retry + pubkey persisted
	c1, ok := reg2.Get("did:cav:legacy").(*Citizen)
	if !ok {
		t.Fatalf("legacy citizen lost after restart")
	}
	if c1.State != StateRestricted {
		t.Errorf("expected State=restricted after restart, got %q", c1.State)
	}
	if c1.PubKey != "pk1" {
		t.Errorf("expected PubKey to persist, got %q", c1.PubKey)
	}
	if c1.NextRetryAt == nil || !c1.NextRetryAt.Equal(retry) {
		t.Errorf("expected NextRetryAt=%v, got %v", retry, c1.NextRetryAt)
	}

	// did:cav:fresh → probation
	c2, ok := reg2.Get("did:cav:fresh").(*Citizen)
	if !ok {
		t.Fatalf("fresh citizen lost after restart")
	}
	if c2.State != StateProbation {
		t.Errorf("expected State=probation, got %q", c2.State)
	}

	// Indexes still work
	if len(reg2.ByState(StateProbation)) != 1 {
		t.Errorf("expected 1 probation citizen after restart")
	}
	if len(reg2.ByState(StateRestricted)) != 1 {
		t.Errorf("expected 1 restricted citizen after restart")
	}
}

// TestPersistentEffectiveStateLegacy confirms that a row written without ever
// calling SetState is treated as active.
func TestPersistentEffectiveStateLegacy(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "citizens")
	reg, err := NewPersistentRegistry(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	reg.EnsureRegistered("did:cav:legacy2", "fpL2")
	reg.Close()

	reg2, err := NewPersistentRegistry(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reg2.Close()

	if got := reg2.EffectiveState("did:cav:legacy2"); got != StateActive {
		t.Errorf("expected legacy citizen to be effectively active, got %q", got)
	}
	if got := reg2.EffectiveState("did:cav:nonexistent"); got != "" {
		t.Errorf("expected empty state for unknown DID, got %q", got)
	}
}
