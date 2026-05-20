package thread

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type fakeRelay struct {
	mu        sync.Mutex
	published []*CrystallizedPraxon
}

func (f *fakeRelay) Publish(ctx context.Context, p *CrystallizedPraxon) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.published = append(f.published, p)
	return nil
}

func (f *fakeRelay) Count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.published)
}

func (f *fakeRelay) Last() *CrystallizedPraxon {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.published) == 0 {
		return nil
	}
	return f.published[len(f.published)-1]
}

type fakeRetractor struct {
	mu      sync.Mutex
	retracts []struct{ from, to CrystallizationLevel }
}

func (r *fakeRetractor) EmitRetraction(ctx context.Context, threadID, praxonID string, from, to CrystallizationLevel) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.retracts = append(r.retracts, struct{ from, to CrystallizationLevel }{from, to})
	return nil
}

func TestThreadIssuerDIDFormat(t *testing.T) {
	got := ThreadIssuerDID("abc123")
	if got != "did:cav:thread:abc123" {
		t.Errorf("unexpected issuer DID format: %q", got)
	}
}

func TestLevelRankOrder(t *testing.T) {
	if levelRank(LevelNone) >= levelRank(LevelDraft) {
		t.Error("None must rank below Draft")
	}
	if levelRank(LevelDraft) >= levelRank(LevelProvisional) {
		t.Error("Draft must rank below Provisional")
	}
	if levelRank(LevelProvisional) >= levelRank(LevelCanonical) {
		t.Error("Provisional must rank below Canonical")
	}
}

func TestCrystallizerPublishesOnUpgrade(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "thread")
	tr, _ := NewTracker(dir)
	defer tr.Close()
	relay := &fakeRelay{}
	if _, err := NewCrystallizer(tr, relay, nil); err != nil {
		t.Fatal(err)
	}

	// Send a few signals from a small network with a long-running thread —
	// readiness should cross at least one threshold.
	threadStart := time.Now().Add(-2 * time.Hour)
	for i, sender := range []string{"a", "b", "c", "d"} {
		tr.OnSignal(SignalRef{
			Snapshot: SignalSnapshot{
				ID: "s" + string(rune('0'+i)), From: sender,
				Position: "endorse", Confidence: 0.9, Reputation: 0.85,
				IssuedAt: threadStart.Add(time.Duration(i) * time.Minute),
				Tags:     []string{string(rune('a' + i))},
			},
			ParentID:    "s0",
			NetworkSize: 10,
		})
	}
	if relay.Count() == 0 {
		t.Error("crystallizer should have published at least one Praxon")
	}
	last := relay.Last()
	if last == nil {
		t.Fatal("nil last published praxon")
	}
	if last.Issuer == "" || last.Issuer[:len("did:cav:thread:")] != "did:cav:thread:" {
		t.Errorf("issuer should be did:cav:thread:*, got %q", last.Issuer)
	}
	if last.Provenance.ConsensusEpisode == "" {
		t.Error("provenance.consensus_episode must be set")
	}
	if len(last.Provenance.DerivedFrom) < 2 {
		t.Errorf("derived_from should reference signal IDs, got %v", last.Provenance.DerivedFrom)
	}
}

func TestCrystallizerErrorsOnNilArgs(t *testing.T) {
	if _, err := NewCrystallizer(nil, &fakeRelay{}, nil); err == nil {
		t.Error("nil tracker should error")
	}
	dir := filepath.Join(t.TempDir(), "thread")
	tr, _ := NewTracker(dir)
	defer tr.Close()
	if _, err := NewCrystallizer(tr, nil, nil); err == nil {
		t.Error("nil relay should error")
	}
}

func TestBuildPraxonReusesExistingID(t *testing.T) {
	th := &Thread{
		ID:                   "thr1",
		StartedAt:            time.Now().Add(-time.Hour),
		SignalIDs:            []string{"a", "b", "c"},
		CrystallizedPraxonID: "px_existing",
	}
	change := LevelChange{
		ThreadID:           "thr1",
		To:                 LevelProvisional,
		DominantPosition:   "endorse",
		DominantConfidence: 0.8,
	}
	p := buildPraxon(th, change)
	if p.ID != "px_existing" {
		t.Errorf("expected reuse of existing ID, got %q", p.ID)
	}
	if p.Level != LevelProvisional {
		t.Errorf("level not propagated: %v", p.Level)
	}
}

func TestBuildPraxonGeneratesIDOnFirstCrystal(t *testing.T) {
	th := &Thread{
		ID:        "thr2",
		StartedAt: time.Now().Add(-time.Hour),
		SignalIDs: []string{"a", "b"},
	}
	change := LevelChange{ThreadID: "thr2", To: LevelDraft}
	p := buildPraxon(th, change)
	if p.ID == "" {
		t.Error("first emission should mint an ID")
	}
}
