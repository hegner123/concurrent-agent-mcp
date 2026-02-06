package mcp

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/hegner123/hq/internal/db"
	"github.com/mark3labs/mcp-go/mcp"
)

// newTestHandlers creates handlers with a temporary database
func newTestHandlers(t *testing.T) *Handlers {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "hq-handler-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	err = tmpFile.Close()
	if err != nil {
		t.Fatal(err)
		return nil
	}
	t.Cleanup(func() {
		err := os.Remove(tmpFile.Name())
		if err != nil {
			return
		}
	})

	database, err := db.NewDB(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		err := database.Close()
		if err != nil {
			return
		}
	})

	return NewHandlers(database)
}

// getResultText extracts the text from an MCP CallToolResult
func getResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	textContent, ok := mcp.AsTextContent(result.Content[0])
	if !ok {
		t.Fatalf("content is not TextContent: %T", result.Content[0])
	}
	return textContent.Text
}

// --- Argument Validation Tests ---

func TestCreateProject_MissingName(t *testing.T) {
	h := newTestHandlers(t)

	args := map[string]any{
		"base_commit": "abc123",
		"steps":       "[]",
	}

	result, err := h.CreateProject(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing name")
	}
	text := getResultText(t, result)
	if !strings.Contains(text, "name is required") {
		t.Errorf("unexpected error message: %s", text)
	}
}

func TestCreateProject_MissingBaseCommit(t *testing.T) {
	h := newTestHandlers(t)

	args := map[string]any{
		"name":  "test-project",
		"steps": "[]",
	}

	result, err := h.CreateProject(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing base_commit")
	}
}

func TestCreateProject_InvalidSteps(t *testing.T) {
	h := newTestHandlers(t)

	t.Run("invalid JSON string", func(t *testing.T) {
		args := map[string]any{
			"name":        "test-project",
			"base_commit": "abc123",
			"steps":       "not valid json",
		}

		result, err := h.CreateProject(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for invalid steps JSON")
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		args := map[string]any{
			"name":        "test-project",
			"base_commit": "abc123",
			"steps":       12345,
		}

		result, err := h.CreateProject(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for wrong steps type")
		}
	})
}

func TestCreateProject_InvalidStepFields(t *testing.T) {
	h := newTestHandlers(t)

	t.Run("missing step_num", func(t *testing.T) {
		args := map[string]any{
			"name":        "test-project",
			"base_commit": "abc123",
			"steps": []any{
				map[string]any{
					"branch": "feature/test",
					"scope":  "backend",
				},
			},
		}

		result, err := h.CreateProject(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for missing step_num")
		}
	})

	t.Run("missing branch", func(t *testing.T) {
		args := map[string]any{
			"name":        "test-project",
			"base_commit": "abc123",
			"steps": []any{
				map[string]any{
					"step_num": int(1),
					"scope":    "backend",
				},
			},
		}

		result, err := h.CreateProject(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for missing branch")
		}
	})
}

func TestClaimStep_MissingProject(t *testing.T) {
	h := newTestHandlers(t)

	args := map[string]any{
		"agent_id": "agent-1",
	}

	result, err := h.ClaimStep(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing project")
	}
}

func TestClaimStep_MissingAgentID(t *testing.T) {
	h := newTestHandlers(t)

	args := map[string]any{
		"project": "test-project",
	}

	result, err := h.ClaimStep(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing agent_id")
	}
}

func TestStartStep_InvalidStepID(t *testing.T) {
	h := newTestHandlers(t)

	t.Run("missing step_id", func(t *testing.T) {
		args := map[string]any{}

		result, err := h.StartStep(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for missing step_id")
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		args := map[string]any{
			"step_id": "not a number",
		}

		result, err := h.StartStep(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for wrong step_id type")
		}
	})
}

func TestHeartbeat_MissingArgs(t *testing.T) {
	h := newTestHandlers(t)

	t.Run("missing step_id", func(t *testing.T) {
		args := map[string]any{
			"agent_id": "agent-1",
		}

		result, err := h.Heartbeat(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for missing step_id")
		}
	})

	t.Run("missing agent_id", func(t *testing.T) {
		args := map[string]any{
			"step_id": int(1),
		}

		result, err := h.Heartbeat(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for missing agent_id")
		}
	})
}

func TestCompleteStep_MissingArgs(t *testing.T) {
	h := newTestHandlers(t)

	t.Run("missing step_id", func(t *testing.T) {
		args := map[string]any{
			"commit_hash": "abc123",
		}

		result, err := h.CompleteStep(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for missing step_id")
		}
	})

	t.Run("missing commit_hash", func(t *testing.T) {
		args := map[string]any{
			"step_id": int(1),
		}

		result, err := h.CompleteStep(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for missing commit_hash")
		}
	})
}

func TestFailStep_MissingArgs(t *testing.T) {
	h := newTestHandlers(t)

	t.Run("missing step_id", func(t *testing.T) {
		args := map[string]any{
			"reason": "test failed",
		}

		result, err := h.FailStep(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for missing step_id")
		}
	})

	t.Run("missing reason", func(t *testing.T) {
		args := map[string]any{
			"step_id": int(1),
		}

		result, err := h.FailStep(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for missing reason")
		}
	})
}

func TestGetStep_InvalidStepID(t *testing.T) {
	h := newTestHandlers(t)

	args := map[string]any{
		"step_id": "invalid",
	}

	result, err := h.GetStep(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for invalid step_id")
	}
}

func TestRecoverStep_InvalidStepID(t *testing.T) {
	h := newTestHandlers(t)

	args := map[string]any{}

	result, err := h.RecoverStep(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing step_id")
	}
}

func TestRecoverFailedStep_InvalidStepID(t *testing.T) {
	h := newTestHandlers(t)

	args := map[string]any{
		"step_id": "not a number",
	}

	result, err := h.RecoverFailedStep(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for invalid step_id")
	}
}

func TestGetProject_MissingName(t *testing.T) {
	h := newTestHandlers(t)

	args := map[string]any{}

	result, err := h.GetProject(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing name")
	}
}

// --- Integration Tests ---

func TestHandlers_FullWorkflow(t *testing.T) {
	h := newTestHandlers(t)

	// 1. Create project
	createArgs := map[string]any{
		"name":        "workflow-test",
		"base_commit": "abc123",
		"steps": []any{
			map[string]any{
				"step_num":   int(1),
				"branch":     "feature/step-1",
				"scope":      "backend",
				"depends_on": []any{},
			},
			map[string]any{
				"step_num":   int(2),
				"branch":     "feature/step-2",
				"scope":      "frontend",
				"depends_on": []any{int(1)},
			},
		},
	}

	result, err := h.CreateProject(createArgs)
	if err != nil {
		t.Fatalf("CreateProject error: %v", err)
	}
	if result.IsError {
		t.Fatalf("CreateProject failed: %s", getResultText(t, result))
	}

	// 2. Get project to verify
	getArgs := map[string]any{
		"name": "workflow-test",
	}
	result, err = h.GetProject(getArgs)
	if err != nil {
		t.Fatalf("GetProject error: %v", err)
	}
	if result.IsError {
		t.Fatalf("GetProject failed: %s", getResultText(t, result))
	}

	// 3. Claim step
	claimArgs := map[string]any{
		"project":  "workflow-test",
		"agent_id": "agent-1",
	}
	result, err = h.ClaimStep(claimArgs)
	if err != nil {
		t.Fatalf("ClaimStep error: %v", err)
	}
	if result.IsError {
		t.Fatalf("ClaimStep failed: %s", getResultText(t, result))
	}

	// Parse claimed step
	var claimedStep struct {
		ID      int64  `json:"id"`
		StepNum int    `json:"step_num"`
		Status  string `json:"status"`
	}
	text := getResultText(t, result)
	if err := json.Unmarshal([]byte(text), &claimedStep); err != nil {
		t.Fatalf("failed to parse claimed step: %v", err)
	}
	if claimedStep.StepNum != 1 {
		t.Errorf("expected step 1, got step %d", claimedStep.StepNum)
	}

	// 4. Start step
	startArgs := map[string]any{
		"step_id":  int(claimedStep.ID),
		"worktree": "/tmp/worktree",
	}
	result, err = h.StartStep(startArgs)
	if err != nil {
		t.Fatalf("StartStep error: %v", err)
	}
	if result.IsError {
		t.Fatalf("StartStep failed: %s", getResultText(t, result))
	}

	// 5. Heartbeat
	heartbeatArgs := map[string]any{
		"step_id":  int(claimedStep.ID),
		"agent_id": "agent-1",
	}
	result, err = h.Heartbeat(heartbeatArgs)
	if err != nil {
		t.Fatalf("Heartbeat error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Heartbeat failed: %s", getResultText(t, result))
	}

	// 6. Complete step
	completeArgs := map[string]any{
		"step_id":        int(claimedStep.ID),
		"commit_hash":    "def456",
		"files_modified": []any{"src/main.go"},
		"notes":          "Completed successfully",
	}
	result, err = h.CompleteStep(completeArgs)
	if err != nil {
		t.Fatalf("CompleteStep error: %v", err)
	}
	if result.IsError {
		t.Fatalf("CompleteStep failed: %s", getResultText(t, result))
	}

	// 7. Verify step 2 is now available
	availableArgs := map[string]any{
		"project": "workflow-test",
	}
	result, err = h.GetAvailableSteps(availableArgs)
	if err != nil {
		t.Fatalf("GetAvailableSteps error: %v", err)
	}
	if result.IsError {
		t.Fatalf("GetAvailableSteps failed: %s", getResultText(t, result))
	}

	var availableSteps []struct {
		StepNum int `json:"step_num"`
	}
	text = getResultText(t, result)
	if err := json.Unmarshal([]byte(text), &availableSteps); err != nil {
		t.Fatalf("failed to parse available steps: %v", err)
	}
	if len(availableSteps) != 1 || availableSteps[0].StepNum != 2 {
		t.Errorf("expected step 2 to be available, got %v", availableSteps)
	}
}

func TestHandlers_ClaimReturnsNull(t *testing.T) {
	h := newTestHandlers(t)

	// Create project with 1 step
	createArgs := map[string]any{
		"name":        "null-test",
		"base_commit": "abc123",
		"steps": []any{
			map[string]any{
				"step_num": int(1),
				"branch":   "feature/only-step",
				"scope":    "backend",
			},
		},
	}
	_, err := h.CreateProject(createArgs)
	if err != nil {
		t.Fatal(err)
	}

	// Claim the only step
	claimArgs := map[string]any{
		"project":  "null-test",
		"agent_id": "agent-1",
	}
	_, err = h.ClaimStep(claimArgs)
	if err != nil {
		t.Fatal(err)
	}

	// Try to claim again - should return null
	result, err := h.ClaimStep(map[string]any{
		"project":  "null-test",
		"agent_id": "agent-2",
	})
	if err != nil {
		t.Fatalf("ClaimStep error: %v", err)
	}
	if result.IsError {
		t.Fatalf("ClaimStep returned error: %s", getResultText(t, result))
	}

	text := getResultText(t, result)
	if text != "null" {
		t.Errorf("expected 'null', got %q", text)
	}
}

func TestHandlers_ListProjects(t *testing.T) {
	h := newTestHandlers(t)

	// Create some projects
	for _, name := range []string{"list-1", "list-2"} {
		args := map[string]any{
			"name":        name,
			"base_commit": "abc",
			"steps": []any{
				map[string]any{
					"step_num": int(1),
					"branch":   "b",
					"scope":    "s",
				},
			},
		}
		_, err := h.CreateProject(args)
		if err != nil {
			t.Fatal(err)
		}
	}

	// List all
	result, err := h.ListProjects(map[string]any{})
	if err != nil {
		t.Fatalf("ListProjects error: %v", err)
	}
	if result.IsError {
		t.Fatalf("ListProjects failed: %s", getResultText(t, result))
	}

	var projects []struct {
		Name string `json:"name"`
	}
	text := getResultText(t, result)
	if err := json.Unmarshal([]byte(text), &projects); err != nil {
		t.Fatalf("failed to parse projects: %v", err)
	}
	if len(projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(projects))
	}
}

func TestHandlers_GetMetrics(t *testing.T) {
	h := newTestHandlers(t)

	// Create project
	args := map[string]any{
		"name":        "metrics-test",
		"base_commit": "abc",
		"steps": []any{
			map[string]any{
				"step_num": int(1),
				"branch":   "b1",
				"scope":    "backend",
			},
			map[string]any{
				"step_num": int(2),
				"branch":   "b2",
				"scope":    "frontend",
			},
		},
	}
	_, err := h.CreateProject(args)
	if err != nil {
		t.Fatal(err)
	}

	// Get metrics
	result, err := h.GetMetrics(map[string]any{
		"project": "metrics-test",
	})
	if err != nil {
		t.Fatalf("GetMetrics error: %v", err)
	}
	if result.IsError {
		t.Fatalf("GetMetrics failed: %s", getResultText(t, result))
	}

	var metrics struct {
		TotalSteps     int            `json:"total_steps"`
		ScopeBreakdown map[string]int `json:"scope_breakdown"`
	}
	text := getResultText(t, result)
	if err := json.Unmarshal([]byte(text), &metrics); err != nil {
		t.Fatalf("failed to parse metrics: %v", err)
	}
	if metrics.TotalSteps != 2 {
		t.Errorf("expected 2 total steps, got %d", metrics.TotalSteps)
	}
}

func TestHandlers_DetectStaleWork(t *testing.T) {
	h := newTestHandlers(t)

	// Create project and claim a step
	args := map[string]any{
		"name":        "stale-test",
		"base_commit": "abc",
		"steps": []any{
			map[string]any{
				"step_num": int(1),
				"branch":   "b",
				"scope":    "s",
			},
		},
	}
	_, err := h.CreateProject(args)
	if err != nil {
		t.Fatal(err)
	}

	_, err = h.ClaimStep(map[string]any{
		"project":  "stale-test",
		"agent_id": "agent-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	// DetectStaleWork - with fresh heartbeat, should return empty
	result, err := h.DetectStaleWork(map[string]any{
		"timeout_minutes": int(15),
	})
	if err != nil {
		t.Fatalf("DetectStaleWork error: %v", err)
	}
	if result.IsError {
		t.Fatalf("DetectStaleWork failed: %s", getResultText(t, result))
	}

	// Just verify it returns valid JSON (empty array or with steps)
	var staleSteps []struct {
		ID int64 `json:"id"`
	}
	text := getResultText(t, result)
	if err := json.Unmarshal([]byte(text), &staleSteps); err != nil {
		t.Fatalf("failed to parse stale steps: %v", err)
	}
}

func TestHandlers_GetAgentEvents(t *testing.T) {
	h := newTestHandlers(t)

	// Create project and do some operations
	args := map[string]any{
		"name":        "events-test",
		"base_commit": "abc",
		"steps": []any{
			map[string]any{
				"step_num": int(1),
				"branch":   "b",
				"scope":    "s",
			},
		},
	}
	_, err := h.CreateProject(args)
	if err != nil {
		t.Fatal(err)
	}

	_, err = h.ClaimStep(map[string]any{
		"project":  "events-test",
		"agent_id": "test-agent",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Get events
	result, err := h.GetAgentEvents(map[string]any{
		"agent_id": "test-agent",
		"limit":    int(10),
	})
	if err != nil {
		t.Fatalf("GetAgentEvents error: %v", err)
	}
	if result.IsError {
		t.Fatalf("GetAgentEvents failed: %s", getResultText(t, result))
	}

	var events []struct {
		EventType string `json:"event_type"`
		AgentID   string `json:"agent_id"`
	}
	text := getResultText(t, result)
	if err := json.Unmarshal([]byte(text), &events); err != nil {
		t.Fatalf("failed to parse events: %v", err)
	}
	if len(events) == 0 {
		t.Error("expected at least one event")
	}
	if events[0].AgentID != "test-agent" {
		t.Errorf("expected agent_id 'test-agent', got %q", events[0].AgentID)
	}
}

func TestHandlers_FailAndRecoverStep(t *testing.T) {
	h := newTestHandlers(t)

	// Create project
	args := map[string]any{
		"name":        "fail-recover-test",
		"base_commit": "abc",
		"steps": []any{
			map[string]any{
				"step_num": int(1),
				"branch":   "b",
				"scope":    "s",
			},
		},
	}
	_, err := h.CreateProject(args)
	if err != nil {
		t.Fatal(err)
	}

	// Claim and start
	claimResult, err := h.ClaimStep(map[string]any{
		"project":  "fail-recover-test",
		"agent_id": "agent-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	var step struct {
		ID int64 `json:"id"`
	}
	text := getResultText(t, claimResult)
	if err := json.Unmarshal([]byte(text), &step); err != nil {
		t.Fatalf("failed to parse step: %v", err)
	}

	_, err = h.StartStep(map[string]any{
		"step_id": int(step.ID),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Fail the step
	result, err := h.FailStep(map[string]any{
		"step_id": int(step.ID),
		"reason":  "test failure",
	})
	if err != nil {
		t.Fatalf("FailStep error: %v", err)
	}
	if result.IsError {
		t.Fatalf("FailStep failed: %s", getResultText(t, result))
	}

	// Recover the failed step
	result, err = h.RecoverFailedStep(map[string]any{
		"step_id": int(step.ID),
	})
	if err != nil {
		t.Fatalf("RecoverFailedStep error: %v", err)
	}
	if result.IsError {
		t.Fatalf("RecoverFailedStep failed: %s", getResultText(t, result))
	}

	// Verify step is available again
	getResult, err := h.GetStep(map[string]any{
		"step_id": int(step.ID),
	})
	if err != nil {
		t.Fatal(err)
	}

	var recoveredStep struct {
		Status string `json:"status"`
	}
	text = getResultText(t, getResult)
	if err := json.Unmarshal([]byte(text), &recoveredStep); err != nil {
		t.Fatalf("failed to parse recovered step: %v", err)
	}
	if recoveredStep.Status != "not_started" {
		t.Errorf("expected status 'not_started', got %q", recoveredStep.Status)
	}
}

func TestHandlers_CreateProjectWithArraySteps(t *testing.T) {
	h := newTestHandlers(t)

	// Test with steps as []any (as it would come from MCP)
	args := map[string]any{
		"name":        "array-steps-test",
		"base_commit": "abc123",
		"steps": []any{
			map[string]any{
				"step_num":   int(1),
				"branch":     "feature/test",
				"scope":      "backend",
				"depends_on": []any{},
			},
		},
	}

	result, err := h.CreateProject(args)
	if err != nil {
		t.Fatalf("CreateProject error: %v", err)
	}
	if result.IsError {
		t.Fatalf("CreateProject failed: %s", getResultText(t, result))
	}

	// Verify project was created
	getResult, err := h.GetProject(map[string]any{"name": "array-steps-test"})
	if err != nil {
		t.Fatal(err)
	}
	if getResult.IsError {
		t.Fatalf("GetProject failed: %s", getResultText(t, getResult))
	}
}

func TestHandlers_CompleteStepWithFilesJSON(t *testing.T) {
	h := newTestHandlers(t)

	// Create and claim step
	args := map[string]any{
		"name":        "files-json-test",
		"base_commit": "abc",
		"steps": []any{
			map[string]any{
				"step_num": int(1),
				"branch":   "b",
				"scope":    "s",
			},
		},
	}
	if _, err := h.CreateProject(args); err != nil {
		t.Fatal(err)
	}

	claimResult, err := h.ClaimStep(map[string]any{
		"project":  "files-json-test",
		"agent_id": "agent-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	var step struct {
		ID int64 `json:"id"`
	}
	text := getResultText(t, claimResult)
	if err := json.Unmarshal([]byte(text), &step); err != nil {
		t.Fatalf("failed to parse step: %v", err)
	}

	if _, err := h.StartStep(map[string]any{"step_id": int(step.ID)}); err != nil {
		t.Fatal(err)
	}

	// Complete with JSON string for files_modified
	result, err := h.CompleteStep(map[string]any{
		"step_id":        int(step.ID),
		"commit_hash":    "abc123",
		"files_modified": `["src/main.go", "src/util.go"]`,
	})
	if err != nil {
		t.Fatalf("CompleteStep error: %v", err)
	}
	if result.IsError {
		t.Fatalf("CompleteStep failed: %s", getResultText(t, result))
	}
}
