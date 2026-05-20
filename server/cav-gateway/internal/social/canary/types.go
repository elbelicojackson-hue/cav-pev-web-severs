// Package canary implements the entry-exam (probation) subsystem from
// cav-social-trust §R3.
//
// New citizens enter `probation` state, complete 3-5 canary tasks (questions
// with known ground truth in the agent's declared capability domains), get
// graded, and either advance to `active` or fall to `restricted` (with a
// 24h retry cooldown). Active citizens get one calibration task every 30
// days to keep reputation honest.
//
// CRITICAL invariant (design §11 / I5): canary task ground truth never leaves
// this package's process. The `groundTruth` field on CanaryTask is unexported
// and JSON-tagged "-" so it cannot be serialized. The Sanitize helper
// produces a client-safe copy.

package canary

import "time"

// === Tasks ===

// CanaryTask is one entry-exam item. Ground truth is private to the grader.
type CanaryTask struct {
	ID            string    `json:"id"`
	Domain        string    `json:"domain"`        // e.g. "crypto", "ml", "forensics"
	Capabilities  []string  `json:"capabilities"`  // capability tags this task covers
	Prompt        string    `json:"prompt"`        // human-readable task statement
	EvidencePack  string    `json:"evidence_pack"` // optional context blob
	Difficulty    float64   `json:"difficulty"`    // [0, 1]
	GeneratedFrom string    `json:"generated_from"`// "seed" or praxon ID
	CreatedAt     time.Time `json:"created_at"`

	// groundTruth is intentionally unexported and JSON-tag "-" so it never
	// crosses the wire. Sanitize() is the client-facing accessor.
	groundTruth GroundTruth `json:"-"`
}

// GroundTruth is the answer key for a CanaryTask.
//
// Stored fields:
//   Conclusion           — the canonical answer string
//   AcceptedAlternatives — equivalent phrasings / wordings that should also pass
//   RequiredMethodology  — methodology constraints the submission must respect
//   RequiredGroundingTags — grounding tags the submission must include
type GroundTruth struct {
	Conclusion            string                 `json:"conclusion"`
	AcceptedAlternatives  []string               `json:"accepted_alternatives,omitempty"`
	RequiredMethodology   MethodologyExpectations `json:"required_methodology,omitempty"`
	RequiredGroundingTags []string               `json:"required_grounding_tags,omitempty"`
}

// MethodologyExpectations is what the grader looks for in submission.methodology.
type MethodologyExpectations struct {
	PriorSourceTags    []string `json:"prior_source_tags,omitempty"`
	InferenceMethodTags []string `json:"inference_method_tags,omitempty"`
}

// GroundTruth returns a defensive copy of the task's answer key. Visible only
// inside this package (other callers cannot import a private accessor).
func (t *CanaryTask) GroundTruthRef() GroundTruth {
	return t.groundTruth
}

// SetGroundTruth stores the answer key. Internal builder helper used by Pool
// loaders; not exposed via JSON.
func (t *CanaryTask) SetGroundTruth(gt GroundTruth) {
	t.groundTruth = gt
}

// Sanitize returns a copy of the task safe for client serialization.
// (groundTruth is already JSON-tag "-", but this helper documents the
// intent and removes any future ambiguity.)
func Sanitize(t *CanaryTask) *CanaryTask {
	if t == nil {
		return nil
	}
	cp := *t
	cp.groundTruth = GroundTruth{} // belt-and-suspenders
	return &cp
}

// === Submissions / Results ===

// Submission is what an agent submits to claim completion of a canary task.
//
// Phase 1 expects a praxon-shaped object encoded as plain Go fields. We
// deliberately don't import the cav-node praxon package here — the grader
// does its work on the structural fields it actually needs.
type Submission struct {
	TaskID            string    `json:"task_id"`
	SubmittedAt       time.Time `json:"submitted_at"`
	Conclusion        string    `json:"conclusion"`
	PriorSourceTag    string    `json:"prior_source_tag"`
	InferenceMethodTag string   `json:"inference_method_tag"`
	GroundingTags     []string  `json:"grounding_tags"`
	HasMethodology    bool      `json:"has_methodology"`
	HasGrounding      bool      `json:"has_grounding"`
	HasFalsifiability bool      `json:"has_falsifiability"`
}

// TaskScores is the four-dimension grade.
type TaskScores struct {
	GroundTruthAlignment float64 `json:"ground_truth_alignment"`
	MethodologyQuality   float64 `json:"methodology_quality"`
	ResponseTimePattern  float64 `json:"response_time_pattern"`
	GroundingQuality     float64 `json:"grounding_quality"`
}

// TaskResult is the full grading record persisted under p:result:*.
type TaskResult struct {
	TaskID      string     `json:"task_id"`
	DID         string     `json:"did"`
	AssignedAt  time.Time  `json:"assigned_at"`
	SubmittedAt time.Time  `json:"submitted_at"`
	Scores      TaskScores `json:"scores"`
	Passed      bool       `json:"passed"`
	Reason      string     `json:"reason,omitempty"`
}

// === Citizen status ===

// CitizenStatus tracks a single agent's probation lifecycle.
//
// Stored under p:status:<did>. State here mirrors citizen.ProbationState but
// with canary-specific bookkeeping (assigned tasks, results, retry timestamp).
// The authoritative state for routing/auth decisions stays on the citizen
// registry (citizen.Citizen.State); this struct is the canary subsystem's
// local view.
type CitizenStatus struct {
	DID               string       `json:"did"`
	State             string       `json:"state"`          // mirrors citizen.ProbationState
	AssignedTasks     []string     `json:"assigned_tasks"` // task IDs
	CompletedResults  []TaskResult `json:"completed_results,omitempty"`
	NextRetryAt       *time.Time   `json:"next_retry_at,omitempty"`
	LastCalibrationAt time.Time    `json:"last_calibration_at,omitempty"`
}
