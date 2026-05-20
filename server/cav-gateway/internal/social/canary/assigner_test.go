package canary

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/anthropic-cav/cav-gateway/internal/social/reputation"
)

type harness struct {
	pool   *Pool
	rep    *reputation.Store
	a      *Assigner
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()

	pool, err := NewPool(filepath.Join(dir, "pool"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close() })
	if _, err := LoadDefaultSeeds(pool); err != nil {
		t.Fatal(err)
	}
	rep, err := reputation.NewStore(filepath.Join(dir, "rep"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rep.Close() })
	a, err := NewAssigner(filepath.Join(dir, "probation"), pool, NewGrader(), rep)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	return &harness{pool: pool, rep: rep, a: a}
}

func goodSubmissionFor(task *CanaryTask, submittedAt time.Time) *Submission {
	gt := task.GroundTruthRef()
	prior := "tool"
	if len(gt.RequiredMethodology.PriorSourceTags) > 0 {
		prior = gt.RequiredMethodology.PriorSourceTags[0]
	}
	inf := "deductive"
	if len(gt.RequiredMethodology.InferenceMethodTags) > 0 {
		inf = gt.RequiredMethodology.InferenceMethodTags[0]
	}
	return &Submission{
		TaskID:             task.ID,
		Conclusion:         gt.Conclusion,
		PriorSourceTag:     prior,
		InferenceMethodTag: inf,
		HasMethodology:     true,
		HasGrounding:       true,
		HasFalsifiability:  true,
		GroundingTags:      append([]string{}, gt.RequiredGroundingTags...),
		SubmittedAt:        submittedAt,
	}
}

func badSubmissionFor(task *CanaryTask, submittedAt time.Time) *Submission {
	return &Submission{
		TaskID:         task.ID,
		Conclusion:     "totally wrong answer",
		HasMethodology: true,
		HasGrounding:   true,
		SubmittedAt:    submittedAt,
	}
}

func TestAssignTasksPicksRequestedNumber(t *testing.T) {
	h := newHarness(t)
	tasks, err := h.a.AssignTasks("did:cav:a", []string{"crypto", "ml"}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(tasks))
	}
	for _, tk := range tasks {
		if tk.GroundTruthRef().Conclusion != "" {
			t.Errorf("assigned task must be sanitized, got ground truth: %+v", tk.GroundTruthRef())
		}
	}
	st := h.a.Status("did:cav:a")
	if st.State != "probation" {
		t.Errorf("expected state=probation, got %q", st.State)
	}
	if len(st.AssignedTasks) != 3 {
		t.Errorf("status should record 3 assigned tasks, got %d", len(st.AssignedTasks))
	}
}

func TestAssignTwiceFailsBeforeCompletion(t *testing.T) {
	h := newHarness(t)
	if _, err := h.a.AssignTasks("did:cav:a", []string{"crypto"}, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := h.a.AssignTasks("did:cav:a", []string{"crypto"}, 2); err != ErrAlreadyAssigned {
		t.Errorf("expected ErrAlreadyAssigned, got %v", err)
	}
}

func TestSubmitGoodTasksLeadsToActive(t *testing.T) {
	h := newHarness(t)
	tasks, _ := h.a.AssignTasks("did:cav:winner", []string{"crypto"}, 3)
	now := time.Now()
	var lastOutcome *SubmitOutcome
	for _, tk := range tasks {
		// The returned tasks are sanitized; we need the original for the
		// answer key when crafting a "good" submission.
		full, _ := h.pool.Get(tk.ID)
		out, err := h.a.Submit("did:cav:winner", goodSubmissionFor(full, now))
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
		lastOutcome = out
	}
	if lastOutcome == nil || !lastOutcome.Complete {
		t.Fatalf("last submit should signal completion, got %+v", lastOutcome)
	}
	if lastOutcome.Desired == nil || lastOutcome.Desired.State != "active" {
		t.Errorf("good probation should desire active, got %+v", lastOutcome.Desired)
	}

	// Reputation must be seeded for crypto.
	v := h.rep.Get("did:cav:winner")
	d, ok := v.Operational.Domains["crypto"]
	if !ok {
		t.Fatalf("expected crypto domain seeded in reputation: %+v", v.Operational.Domains)
	}
	if d.Score <= 0 {
		t.Errorf("reputation score should be > 0 after passing canary, got %v", d.Score)
	}
	if d.SampleSize == 0 {
		t.Errorf("reputation sample size should be > 0 after canary event")
	}
}

func TestSubmitOneBadTaskLeadsToRestricted(t *testing.T) {
	h := newHarness(t)
	tasks, _ := h.a.AssignTasks("did:cav:loser", []string{"crypto"}, 3)
	now := time.Now()
	for i, tk := range tasks {
		full, _ := h.pool.Get(tk.ID)
		var sub *Submission
		if i == 0 {
			sub = badSubmissionFor(full, now)
		} else {
			sub = goodSubmissionFor(full, now)
		}
		_, err := h.a.Submit("did:cav:loser", sub)
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
	}
	st := h.a.Status("did:cav:loser")
	if st.State != "restricted" {
		t.Errorf("one failure should restrict, got state=%q", st.State)
	}
	if st.NextRetryAt == nil {
		t.Error("restricted state should have next_retry_at set")
	}
	// And no reputation should be seeded.
	v := h.rep.Get("did:cav:loser")
	if _, ok := v.Operational.Domains["crypto"]; ok {
		t.Errorf("reputation should NOT be seeded on failure: %+v", v.Operational.Domains)
	}
}

func TestSubmitUnassignedTaskRejected(t *testing.T) {
	h := newHarness(t)
	if _, err := h.a.Submit("did:cav:nobody", &Submission{TaskID: "seed_crypto_001"}); err != ErrTaskNotAssigned {
		t.Errorf("expected ErrTaskNotAssigned, got %v", err)
	}
}

func TestSubmitDuplicateRejected(t *testing.T) {
	h := newHarness(t)
	tasks, _ := h.a.AssignTasks("did:cav:dup", []string{"crypto"}, 3)
	full, _ := h.pool.Get(tasks[0].ID)
	if _, err := h.a.Submit("did:cav:dup", goodSubmissionFor(full, time.Now())); err != nil {
		t.Fatalf("first submit: %v", err)
	}
	if _, err := h.a.Submit("did:cav:dup", goodSubmissionFor(full, time.Now())); err != ErrAlreadySubmitted {
		t.Errorf("expected ErrAlreadySubmitted, got %v", err)
	}
}

func TestReassignAfterCompletionAllowed(t *testing.T) {
	// After a citizen completes (passes or fails) a probation cycle, they
	// should be eligible for a new round (e.g. an admin retriggers).
	h := newHarness(t)
	tasks, _ := h.a.AssignTasks("did:cav:cycle", []string{"crypto"}, 2)
	for _, tk := range tasks {
		full, _ := h.pool.Get(tk.ID)
		h.a.Submit("did:cav:cycle", goodSubmissionFor(full, time.Now()))
	}
	// All assigned tasks completed → reassignment must succeed.
	if _, err := h.a.AssignTasks("did:cav:cycle", []string{"ml"}, 2); err != nil {
		t.Errorf("reassign after completion should succeed, got %v", err)
	}
}

func TestRestrictedCooldownBlocksReassign(t *testing.T) {
	h := newHarness(t)
	tasks, _ := h.a.AssignTasks("did:cav:cooldown", []string{"crypto"}, 1)
	full, _ := h.pool.Get(tasks[0].ID)
	h.a.Submit("did:cav:cooldown", badSubmissionFor(full, time.Now()))

	// Citizen is now restricted with a future retry timestamp; reassignment
	// must be rejected.
	if _, err := h.a.AssignTasks("did:cav:cooldown", []string{"crypto"}, 1); err != ErrCooldownActive {
		t.Errorf("expected ErrCooldownActive, got %v", err)
	}
}

func TestAssignedTasksReturnsSanitized(t *testing.T) {
	h := newHarness(t)
	if _, err := h.a.AssignTasks("did:cav:a", []string{"crypto"}, 2); err != nil {
		t.Fatal(err)
	}
	tasks, err := h.a.AssignedTasks("did:cav:a")
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range tasks {
		if tk.GroundTruthRef().Conclusion != "" {
			t.Errorf("assigned tasks must be sanitized: %+v", tk.GroundTruthRef())
		}
	}
}

func TestStatusPersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	pool, _ := NewPool(filepath.Join(dir, "pool"))
	LoadDefaultSeeds(pool)
	rep, _ := reputation.NewStore(filepath.Join(dir, "rep"))
	a, _ := NewAssigner(filepath.Join(dir, "probation"), pool, NewGrader(), rep)

	a.AssignTasks("did:cav:persist", []string{"crypto"}, 2)
	a.Close()

	a2, err := NewAssigner(filepath.Join(dir, "probation"), pool, NewGrader(), rep)
	if err != nil {
		t.Fatal(err)
	}
	defer a2.Close()
	defer pool.Close()
	defer rep.Close()

	st := a2.Status("did:cav:persist")
	if len(st.AssignedTasks) != 2 {
		t.Errorf("expected 2 assigned tasks after restart, got %d", len(st.AssignedTasks))
	}
}
