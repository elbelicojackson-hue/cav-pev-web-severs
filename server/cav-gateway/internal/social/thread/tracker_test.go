package thread

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestTracker(t *testing.T) *Tracker {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "thread")
	tr, err := NewTracker(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tr.Close() })
	return tr
}

func TestOnSignalCreatesNewThread(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()
	th, err := tr.OnSignal(SignalRef{
		Snapshot: SignalSnapshot{
			ID: "sig1", From: "a", Position: "endorse",
			Confidence: 1.0, Reputation: 0.8, IssuedAt: now,
		},
		ParentID:    "",
		NetworkSize: 10,
	})
	if err != nil {
		t.Fatalf("OnSignal: %v", err)
	}
	if th.ID != "sig1" {
		t.Errorf("root signal should become thread ID, got %q", th.ID)
	}
	if len(th.SignalIDs) != 1 || len(th.Participants) != 1 {
		t.Errorf("expected 1 signal + 1 participant, got %+v", th)
	}
}

func TestOnSignalAppendsToExistingThread(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()
	tr.OnSignal(SignalRef{
		Snapshot: SignalSnapshot{
			ID: "root", From: "a", Position: "endorse",
			Confidence: 1.0, Reputation: 0.8, IssuedAt: now,
		},
		NetworkSize: 10,
	})
	th, err := tr.OnSignal(SignalRef{
		Snapshot: SignalSnapshot{
			ID: "child", From: "b", Position: "endorse",
			Confidence: 1.0, Reputation: 0.8, IssuedAt: now.Add(time.Minute),
		},
		ParentID:    "root",
		NetworkSize: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(th.SignalIDs) != 2 {
		t.Errorf("expected 2 signals after second OnSignal, got %d", len(th.SignalIDs))
	}
	if len(th.Participants) != 2 {
		t.Errorf("expected 2 participants, got %d", len(th.Participants))
	}
}

func TestOnSignalIdempotentForSameID(t *testing.T) {
	tr := newTestTracker(t)
	ref := SignalRef{
		Snapshot: SignalSnapshot{
			ID: "x", From: "a", Position: "endorse",
			Confidence: 1.0, Reputation: 0.8, IssuedAt: time.Now(),
		},
		NetworkSize: 10,
	}
	tr.OnSignal(ref)
	th, _ := tr.OnSignal(ref)
	if len(th.SignalIDs) != 1 {
		t.Errorf("duplicate signal should not be re-added, got %d", len(th.SignalIDs))
	}
}

func TestSubscriberFiresOnLevelChange(t *testing.T) {
	tr := newTestTracker(t)
	var fired []LevelChange
	tr.Subscribe(func(c LevelChange) { fired = append(fired, c) })

	now := time.Now()
	// Start a tiny network thread that should hit Draft quickly.
	threadStart := now.Add(-1 * time.Hour) // mature for small networks
	for i, sender := range []string{"a", "b", "c"} {
		tr.OnSignal(SignalRef{
			Snapshot: SignalSnapshot{
				ID: "s" + string(rune('0'+i)), From: sender,
				Position: "endorse", Confidence: 0.9, Reputation: 0.8,
				IssuedAt: threadStart.Add(time.Duration(i) * time.Minute),
				Tags:     []string{string(rune('a' + i))},
			},
			ParentID:    "s0",
			NetworkSize: 10,
		})
	}
	// At least one level transition should have fired (None → Draft or higher).
	if len(fired) == 0 {
		t.Error("expected at least one level change to fire")
	}
	for _, c := range fired {
		if c.From == c.To {
			t.Errorf("LevelChange should not fire for unchanged level: %+v", c)
		}
	}
}

func TestActiveFiltering(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()
	tr.OnSignal(SignalRef{
		Snapshot: SignalSnapshot{
			ID: "thr1", From: "a", Position: "endorse",
			Confidence: 1.0, Reputation: 0.8, IssuedAt: now,
		},
		NetworkSize: 10,
	})
	all := tr.Active(LevelNone)
	if len(all) == 0 {
		t.Error("expected at least one thread")
	}
	// Most threads with a single signal won't crystallize.
	canonical := tr.Active(LevelCanonical)
	if len(canonical) != 0 {
		t.Errorf("single-signal thread should not be canonical, got %d", len(canonical))
	}
}

func TestSetCrystallizedPraxonID(t *testing.T) {
	tr := newTestTracker(t)
	tr.OnSignal(SignalRef{
		Snapshot: SignalSnapshot{
			ID: "t1", From: "a", Position: "endorse",
			Confidence: 1.0, Reputation: 0.8, IssuedAt: time.Now(),
		},
		NetworkSize: 10,
	})
	if err := tr.SetCrystallizedPraxonID("t1", "praxon_xyz"); err != nil {
		t.Fatal(err)
	}
	th := tr.Get("t1")
	if th.CrystallizedPraxonID != "praxon_xyz" {
		t.Errorf("praxon ID not stored: %q", th.CrystallizedPraxonID)
	}
	if err := tr.SetCrystallizedPraxonID("nonexistent", "x"); err != ErrUnknownThread {
		t.Errorf("expected ErrUnknownThread, got %v", err)
	}
}

func TestPersistenceRoundtrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "thread")
	tr, _ := NewTracker(dir)
	now := time.Now()
	tr.OnSignal(SignalRef{
		Snapshot: SignalSnapshot{
			ID: "p1", From: "a", Position: "endorse",
			Confidence: 1.0, Reputation: 0.8, IssuedAt: now,
		},
		NetworkSize: 10,
	})
	tr.Close()

	tr2, err := NewTracker(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer tr2.Close()
	th := tr2.Get("p1")
	if th == nil {
		t.Fatal("thread lost after restart")
	}
	if len(th.SignalIDs) != 1 {
		t.Errorf("signal list lost: %+v", th.SignalIDs)
	}
}
