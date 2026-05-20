package visibility

import (
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "vis")
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

type alwaysMutual struct{}

func (alwaysMutual) HasMutual(a, b string) bool { return true }

type neverMutual struct{}

func (neverMutual) HasMutual(a, b string) bool { return false }

func TestModeIsValid(t *testing.T) {
	for _, m := range []Mode{Public, Private, MutualOnly} {
		if !m.IsValid() {
			t.Errorf("%s should be valid", m)
		}
	}
	if Mode("nope").IsValid() {
		t.Error("'nope' must not be valid")
	}
}

func TestDefaultIsPublic(t *testing.T) {
	s := newTestStore(t)
	if got := s.Get("did:cav:never_set"); got != Public {
		t.Errorf("default mode should be public, got %v", got)
	}
}

func TestSetAndGet(t *testing.T) {
	s := newTestStore(t)
	if err := s.Set("did:a", Private); err != nil {
		t.Fatal(err)
	}
	if got := s.Get("did:a"); got != Private {
		t.Errorf("expected private, got %v", got)
	}
}

func TestSetRejectsInvalid(t *testing.T) {
	s := newTestStore(t)
	if err := s.Set("did:a", Mode("nope")); err == nil {
		t.Error("invalid mode should error")
	}
	if err := s.Set("", Public); err == nil {
		t.Error("empty did should error")
	}
}

func TestDecidePublic(t *testing.T) {
	s := newTestStore(t)
	s.Set("target", Public)
	if err := s.Decide("viewer", "target", neverMutual{}); err != nil {
		t.Errorf("public should allow any viewer, got %v", err)
	}
}

func TestDecidePrivateBlocksOthers(t *testing.T) {
	s := newTestStore(t)
	s.Set("target", Private)
	if err := s.Decide("viewer", "target", alwaysMutual{}); err == nil {
		t.Error("private should block non-self viewer even with mutual trust")
	}
}

func TestDecidePrivateAllowsSelf(t *testing.T) {
	s := newTestStore(t)
	s.Set("target", Private)
	if err := s.Decide("target", "target", neverMutual{}); err != nil {
		t.Errorf("private should allow self, got %v", err)
	}
}

func TestDecideMutualOnlyWithMutual(t *testing.T) {
	s := newTestStore(t)
	s.Set("target", MutualOnly)
	if err := s.Decide("viewer", "target", alwaysMutual{}); err != nil {
		t.Errorf("mutual_only with mutual trust should allow, got %v", err)
	}
}

func TestDecideMutualOnlyWithoutMutual(t *testing.T) {
	s := newTestStore(t)
	s.Set("target", MutualOnly)
	if err := s.Decide("viewer", "target", neverMutual{}); err == nil {
		t.Error("mutual_only without mutual should block")
	}
}

func TestDecideMutualOnlyNilCheckerBlocks(t *testing.T) {
	s := newTestStore(t)
	s.Set("target", MutualOnly)
	if err := s.Decide("viewer", "target", nil); err == nil {
		t.Error("mutual_only with nil checker should default to deny")
	}
}

func TestErrHiddenMessage(t *testing.T) {
	err := ErrHidden{Mode: Private}
	if err.Error() == "" {
		t.Error("error message should not be empty")
	}
}

func TestPersistence(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "vis")
	s, _ := NewStore(dir)
	s.Set("did:a", Private)
	s.Set("did:b", MutualOnly)
	s.Close()

	s2, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if got := s2.Get("did:a"); got != Private {
		t.Errorf("private not persisted: %v", got)
	}
	if got := s2.Get("did:b"); got != MutualOnly {
		t.Errorf("mutual_only not persisted: %v", got)
	}
}
