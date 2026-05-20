package digest

import (
	"crypto/ed25519"
	"crypto/rand"
	"path/filepath"
	"testing"
	"time"
)

func newTestKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func sampleDigest(did string, periodStart time.Time) *BehavioralDigest {
	return &BehavioralDigest{
		DID:                       did,
		PeriodStart:               periodStart,
		PeriodEnd:                 periodStart.Add(time.Hour),
		VoteAlignmentWithMajority: 0.42,
		UniqueDomainsActive:       3,
		SignalDiversityEntropy:    0.7,
		SignalCount:               12,
		VoteCount:                 5,
	}
}

func TestSignVerifyRoundtrip(t *testing.T) {
	_, priv := newTestKey(t)
	d := sampleDigest("did:cav:a", time.Now().Truncate(time.Second))
	signed, err := Sign(d, priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if signed.Signature == "" || signed.PublicKey == "" {
		t.Errorf("signed digest missing signature/pubkey")
	}
	// Original is unmodified.
	if d.Signature != "" {
		t.Errorf("Sign mutated input")
	}
	if err := Verify(signed, ""); err != nil {
		t.Errorf("verify: %v", err)
	}
}

func TestVerifyRejectsTamperedField(t *testing.T) {
	_, priv := newTestKey(t)
	signed, _ := Sign(sampleDigest("did:cav:a", time.Now()), priv)
	signed.UniqueDomainsActive = 999
	if err := Verify(signed, ""); err != ErrSignatureMismatch {
		t.Errorf("tampered field should fail verify, got %v", err)
	}
}

func TestVerifyRejectsTamperedSignature(t *testing.T) {
	_, priv := newTestKey(t)
	signed, _ := Sign(sampleDigest("did:cav:a", time.Now()), priv)
	// Flip the signature
	if signed.Signature[0] == 'A' {
		signed.Signature = "B" + signed.Signature[1:]
	} else {
		signed.Signature = "A" + signed.Signature[1:]
	}
	if err := Verify(signed, ""); err == nil {
		t.Error("tampered signature should fail verify")
	}
}

func TestVerifyRejectsExpectedPubkeyMismatch(t *testing.T) {
	_, priv := newTestKey(t)
	signed, _ := Sign(sampleDigest("did:cav:a", time.Now()), priv)
	if err := Verify(signed, "different_key"); err != ErrPubkeyChanged {
		t.Errorf("expected ErrPubkeyChanged, got %v", err)
	}
}

func TestVerifyRejectsMissingPeriod(t *testing.T) {
	_, priv := newTestKey(t)
	d := sampleDigest("did:cav:a", time.Now())
	d.PeriodEnd = d.PeriodStart // not after
	signed, _ := Sign(d, priv)
	if err := Verify(signed, ""); err != ErrInvalidPeriod {
		t.Errorf("expected ErrInvalidPeriod, got %v", err)
	}
}

func TestVerifyRejectsMissingDID(t *testing.T) {
	_, priv := newTestKey(t)
	d := sampleDigest("", time.Now())
	signed, _ := Sign(d, priv)
	if err := Verify(signed, ""); err != ErrMissingDID {
		t.Errorf("expected ErrMissingDID, got %v", err)
	}
}

func TestVerifyRejectsMissingSignature(t *testing.T) {
	d := sampleDigest("did:cav:a", time.Now())
	if err := Verify(d, ""); err == nil {
		t.Error("unsigned digest should fail")
	}
}

func TestComputeBuildsDigest(t *testing.T) {
	in := ComputeInput{
		DID:                        "did:cav:a",
		PeriodStart:                time.Now(),
		PeriodEnd:                  time.Now().Add(time.Hour),
		SignalCount:                10,
		VoteCount:                  4,
		VotesAlignedWithMajority:   3,
		VotesWithMajorityAvailable: 4,
		DomainCounts: map[string]int{
			"crypto": 5, "ml": 3, "forensics": 2,
		},
	}
	d, err := Compute(in)
	if err != nil {
		t.Fatal(err)
	}
	if d.UniqueDomainsActive != 3 {
		t.Errorf("expected 3 domains, got %d", d.UniqueDomainsActive)
	}
	if d.VoteAlignmentWithMajority != 0.75 {
		t.Errorf("expected 3/4=0.75 alignment, got %v", d.VoteAlignmentWithMajority)
	}
	if d.SignalDiversityEntropy <= 0 || d.SignalDiversityEntropy > 1 {
		t.Errorf("entropy out of range: %v", d.SignalDiversityEntropy)
	}
}

func TestComputeNoMajoritySignal(t *testing.T) {
	d, err := Compute(ComputeInput{
		DID:         "did:cav:a",
		PeriodStart: time.Now(),
		PeriodEnd:   time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.VoteAlignmentWithMajority != 0.5 {
		t.Errorf("no majority data should report neutral 0.5, got %v", d.VoteAlignmentWithMajority)
	}
	if d.SignalDiversityEntropy != 0 {
		t.Errorf("no domains → entropy 0, got %v", d.SignalDiversityEntropy)
	}
}

func TestComputeRejectsBadPeriod(t *testing.T) {
	_, err := Compute(ComputeInput{
		DID:         "did:cav:a",
		PeriodStart: time.Now(),
		PeriodEnd:   time.Now().Add(-time.Hour),
	})
	if err == nil {
		t.Error("end before start should fail")
	}
}

// === Store ===

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "digest")
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStorePutAndLatest(t *testing.T) {
	s := newTestStore(t)
	d := sampleDigest("did:cav:a", time.Now().Truncate(time.Second))
	if err := s.Put(d); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Latest("did:cav:a")
	if got == nil || got.DID != "did:cav:a" {
		t.Errorf("latest lookup failed: %+v", got)
	}
}

func TestStorePeriodMonotonicity(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().Truncate(time.Second)
	d1 := sampleDigest("did:cav:a", now)
	if err := s.Put(d1); err != nil {
		t.Fatal(err)
	}
	// Same period — should be rejected
	d1Dup := sampleDigest("did:cav:a", now)
	if err := s.Put(d1Dup); err != ErrPeriodRegression {
		t.Errorf("duplicate period should regress, got %v", err)
	}
	// Earlier period — should be rejected
	d0 := sampleDigest("did:cav:a", now.Add(-time.Hour))
	if err := s.Put(d0); err != ErrPeriodRegression {
		t.Errorf("earlier period should regress, got %v", err)
	}
	// Later period — accepted
	d2 := sampleDigest("did:cav:a", now.Add(time.Hour))
	if err := s.Put(d2); err != nil {
		t.Errorf("later period should succeed, got %v", err)
	}
}

func TestStoreLatestReturnsNewest(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().Truncate(time.Second)
	for i := 0; i < 3; i++ {
		s.Put(sampleDigest("did:cav:a", now.Add(time.Duration(i)*time.Hour)))
	}
	got, _ := s.Latest("did:cav:a")
	if !got.PeriodStart.Equal(now.Add(2 * time.Hour)) {
		t.Errorf("latest should be newest, got %v", got.PeriodStart)
	}
}

// === Inactivity sweep ===

type fakeInactivity struct {
	flagged map[string]bool
}

func (f *fakeInactivity) SetInactive(did string, inactive bool) bool {
	if f.flagged == nil {
		f.flagged = map[string]bool{}
	}
	f.flagged[did] = inactive
	return true
}

func TestSweepInactiveFlagsStale(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()
	// Stale: 8 days ago
	s.Put(sampleDigest("did:stale", now.Add(-8*24*time.Hour)))
	// Fresh: 1 day ago
	s.Put(sampleDigest("did:fresh", now.Add(-1*24*time.Hour)))

	f := &fakeInactivity{}
	flagged, cleared, total := s.SweepInactive(f, now)
	if total != 2 {
		t.Errorf("expected total=2, got %d", total)
	}
	if !f.flagged["did:stale"] {
		t.Errorf("stale citizen should be flagged")
	}
	if f.flagged["did:fresh"] {
		t.Errorf("fresh citizen should not be flagged")
	}
	_ = flagged
	_ = cleared
}

func TestStorePersistence(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "digest")
	s, _ := NewStore(dir)
	now := time.Now().Truncate(time.Second)
	s.Put(sampleDigest("did:cav:a", now))
	s.Close()

	s2, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got, _ := s2.Latest("did:cav:a")
	if got == nil {
		t.Fatal("digest lost after restart")
	}
}
