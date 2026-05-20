package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// CanaryTask represents a probation task assigned by the gateway.
// Ground truth is never exposed via the API.
type CanaryTask struct {
	ID           string   `json:"id"`
	Domain       string   `json:"domain"`
	Capabilities []string `json:"capabilities"`
	Prompt       string   `json:"prompt"`
	Difficulty   float64  `json:"difficulty"`
}

// TaskResult is the grading result returned after canary submission.
type TaskResult struct {
	TaskID      string     `json:"task_id"`
	SubmittedAt string     `json:"submitted_at"`
	PraxonID    string     `json:"praxon_id"`
	Scores      TaskScores `json:"scores"`
	Passed      bool       `json:"passed"`
}

// TaskScores contains the individual grading dimensions.
type TaskScores struct {
	GroundTruthAlignment float64 `json:"ground_truth_alignment"`
	MethodologyQuality   float64 `json:"methodology_quality"`
	ResponseTimePattern  float64 `json:"response_time_pattern"`
	GroundingQuality     float64 `json:"grounding_quality"`
}

// CanaryClient handles canary/probation API interactions.
type CanaryClient struct {
	gw *GatewayClient
}

// NewCanaryClient creates a canary client.
func NewCanaryClient(gw *GatewayClient) *CanaryClient {
	return &CanaryClient{gw: gw}
}

// FetchTasks retrieves assigned canary tasks from GET /v1/social/canary/tasks.
func (c *CanaryClient) FetchTasks(ctx context.Context) ([]CanaryTask, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.gw.BaseURL+"/v1/social/canary/tasks", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.gw.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("canary: fetch tasks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// No tasks assigned (probation already passed)
		return nil, nil
	}
	if resp.StatusCode >= 400 {
		return nil, parseHTTPError(resp)
	}

	var tasks []CanaryTask
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		return nil, fmt.Errorf("canary: decode tasks: %w", err)
	}
	return tasks, nil
}

// SubmitTask submits a canary task answer via POST /v1/social/canary/submit.
// The answer is a Praxon-format JSON object produced by the LLM.
func (c *CanaryClient) SubmitTask(ctx context.Context, taskID string, praxonAnswer any) (*TaskResult, error) {
	payload := map[string]any{
		"task_id": taskID,
		"praxon":  praxonAnswer,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("canary: marshal submission: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.gw.BaseURL+"/v1/social/canary/submit", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.gw.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("canary: submit request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, parseHTTPError(resp)
	}

	var result TaskResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("canary: decode result: %w", err)
	}
	return &result, nil
}
