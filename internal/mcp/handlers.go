package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hegner123/hq/internal/db"
	"github.com/mark3labs/mcp-go/mcp"
)

const handlerTimeout = 30 * time.Second

// parseInt extracts an int from arguments by key, handling int, float64, and string formats
func parseInt(arguments map[string]any, key string) (int, bool) {
	switch v := arguments[key].(type) {
	case int:
		return v, true
	case float64:
		return int(v), true
	case string:
		s := strings.Trim(v, "\"")
		n, err := strconv.Atoi(s)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

// parseString extracts a string from arguments by key, handling string, int, and float64 formats
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

func parseString(arguments map[string]any, key string) (string, bool) {
	switch v := arguments[key].(type) {
	case string:
		return v, true
	case int:
		return strconv.Itoa(v), true
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	default:
		return "", false
	}
}

// parseStepID extracts step_id from arguments
func parseStepID(arguments map[string]any) (int, bool) {
	return parseInt(arguments, "step_id")
}

// Handlers contains MCP tool handlers
type Handlers struct {
	db *db.DB
}

// NewHandlers creates new handlers
func NewHandlers(database *db.DB) *Handlers {
	return &Handlers{db: database}
}

// newContext creates a context with the standard handler timeout
func newContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), handlerTimeout)
}

// CreateProject handles create_project tool
func (h *Handlers) CreateProject(arguments map[string]any) (*mcp.CallToolResult, error) {
	ctx, cancel := newContext()
	defer cancel()

	name, ok := parseString(arguments, "name")
	if !ok {
		return mcp.NewToolResultError("name is required"), nil
	}

	baseCommit, ok := parseString(arguments, "base_commit")
	if !ok {
		return mcp.NewToolResultError("base_commit is required"), nil
	}

	// Handle steps - can be either []any or string (JSON)
	var stepsRaw []any
	switch v := arguments["steps"].(type) {
	case []any:
		stepsRaw = v
	case string:
		// Parse JSON string
		if err := json.Unmarshal([]byte(v), &stepsRaw); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("steps must be a valid JSON array: %v", err)), nil
		}
	default:
		return mcp.NewToolResultError("steps must be an array or JSON string"), nil
	}

	// Parse steps
	var steps []db.StepInput
	for _, stepRaw := range stepsRaw {
		stepMap, ok := stepRaw.(map[string]any)
		if !ok {
			return mcp.NewToolResultError("each step must be an object"), nil
		}

		stepNum, ok := parseInt(stepMap, "step_num")
		if !ok {
			return mcp.NewToolResultError("step_num must be a number"), nil
		}

		branch, ok := stepMap["branch"].(string)
		if !ok {
			return mcp.NewToolResultError("branch must be a string"), nil
		}

		scope, ok := stepMap["scope"].(string)
		if !ok {
			return mcp.NewToolResultError("scope must be a string"), nil
		}

		var dependsOn []int
		if depsRaw, ok := stepMap["depends_on"].([]any); ok {
			for _, depRaw := range depsRaw {
				dep, ok := toInt(depRaw)
				if !ok {
					return mcp.NewToolResultError("depends_on must be array of numbers"), nil
				}
				dependsOn = append(dependsOn, dep)
			}
		}

		steps = append(steps, db.StepInput{
			StepNum:   int(stepNum),
			Branch:    branch,
			Scope:     scope,
			DependsOn: dependsOn,
		})
	}

	project, err := h.db.CreateProject(ctx, name, baseCommit, steps)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("create project: %v", err)), nil
	}

	result, err := json.Marshal(project)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(result)), nil
}

// GetProject handles get_project tool
func (h *Handlers) GetProject(arguments map[string]any) (*mcp.CallToolResult, error) {
	ctx, cancel := newContext()
	defer cancel()

	name, ok := parseString(arguments, "name")
	if !ok {
		return mcp.NewToolResultError("name is required"), nil
	}

	project, err := h.db.GetProject(ctx, name)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("get project: %v", err)), nil
	}

	result, err := json.Marshal(project)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(result)), nil
}

// ListProjects handles list_projects tool
func (h *Handlers) ListProjects(arguments map[string]any) (*mcp.CallToolResult, error) {
	ctx, cancel := newContext()
	defer cancel()

	var status *string
	if statusArg, ok := parseString(arguments, "status"); ok {
		status = &statusArg
	}

	projects, err := h.db.ListProjects(ctx, status)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("list projects: %v", err)), nil
	}

	result, err := json.Marshal(projects)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(result)), nil
}

// ClaimStep handles claim_step tool
func (h *Handlers) ClaimStep(arguments map[string]any) (*mcp.CallToolResult, error) {
	ctx, cancel := newContext()
	defer cancel()

	project, ok := parseString(arguments, "project")
	if !ok {
		return mcp.NewToolResultError("project is required"), nil
	}

	agentID, ok := parseString(arguments, "agent_id")
	if !ok {
		return mcp.NewToolResultError("agent_id is required"), nil
	}

	step, err := h.db.ClaimStep(ctx, project, agentID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("claim step: %v", err)), nil
	}

	if step == nil {
		return mcp.NewToolResultText("null"), nil
	}

	resultBytes, err := json.Marshal(step)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(resultBytes)), nil
}

// StartStep handles start_step tool
func (h *Handlers) StartStep(arguments map[string]any) (*mcp.CallToolResult, error) {
	ctx, cancel := newContext()
	defer cancel()

	stepID, ok := parseStepID(arguments)
	if !ok {
		return mcp.NewToolResultError("step_id is required"), nil
	}

	var worktree *string
	if worktreeArg, ok := parseString(arguments, "worktree"); ok {
		worktree = &worktreeArg
	}

	if err := h.db.StartStep(ctx, db.StepID(stepID), worktree); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("start step: %v", err)), nil
	}

	return mcp.NewToolResultText("{\"ok\":true}"), nil
}

// Heartbeat handles heartbeat tool
func (h *Handlers) Heartbeat(arguments map[string]any) (*mcp.CallToolResult, error) {
	ctx, cancel := newContext()
	defer cancel()

	stepID, ok := parseStepID(arguments)
	if !ok {
		return mcp.NewToolResultError("step_id is required"), nil
	}

	agentID, ok := parseString(arguments, "agent_id")
	if !ok {
		return mcp.NewToolResultError("agent_id is required"), nil
	}

	if err := h.db.Heartbeat(ctx, db.StepID(stepID), agentID); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("heartbeat: %v", err)), nil
	}

	return mcp.NewToolResultText("{\"ok\":true}"), nil
}

// CompleteStep handles complete_step tool
func (h *Handlers) CompleteStep(arguments map[string]any) (*mcp.CallToolResult, error) {
	ctx, cancel := newContext()
	defer cancel()

	stepID, ok := parseStepID(arguments)
	if !ok {
		return mcp.NewToolResultError("step_id is required"), nil
	}

	commitHash, ok := parseString(arguments, "commit_hash")
	if !ok {
		return mcp.NewToolResultError("commit_hash is required"), nil
	}

	// Handle files_modified - can be either []any or string (JSON)
	var filesModified []string
	if filesArg, ok := arguments["files_modified"]; ok && filesArg != nil {
		switch v := filesArg.(type) {
		case []any:
			for _, fileRaw := range v {
				if file, ok := fileRaw.(string); ok {
					filesModified = append(filesModified, file)
				}
			}
		case string:
			// Parse JSON string
			if err := json.Unmarshal([]byte(v), &filesModified); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("files_modified must be a valid JSON array: %v", err)), nil
			}
		}
	}

	var notes *string
	if notesArg, ok := parseString(arguments, "notes"); ok {
		notes = &notesArg
	}

	if err := h.db.CompleteStep(ctx, db.StepID(stepID), commitHash, filesModified, notes); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("complete step: %v", err)), nil
	}

	return mcp.NewToolResultText("{\"ok\":true}"), nil
}

// FailStep handles fail_step tool
func (h *Handlers) FailStep(arguments map[string]any) (*mcp.CallToolResult, error) {
	ctx, cancel := newContext()
	defer cancel()

	stepID, ok := parseStepID(arguments)
	if !ok {
		return mcp.NewToolResultError("step_id is required"), nil
	}

	reason, ok := parseString(arguments, "reason")
	if !ok {
		return mcp.NewToolResultError("reason is required"), nil
	}

	if err := h.db.FailStep(ctx, db.StepID(stepID), reason); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("fail step: %v", err)), nil
	}

	return mcp.NewToolResultText("{\"ok\":true}"), nil
}

// GetStep handles get_step tool
func (h *Handlers) GetStep(arguments map[string]any) (*mcp.CallToolResult, error) {
	ctx, cancel := newContext()
	defer cancel()

	stepID, ok := parseStepID(arguments)
	if !ok {
		return mcp.NewToolResultError("step_id is required"), nil
	}

	step, err := h.db.GetStep(ctx, db.StepID(stepID))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("get step: %v", err)), nil
	}

	result, err := json.Marshal(step)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(result)), nil
}

// GetAvailableSteps handles get_available_steps tool
func (h *Handlers) GetAvailableSteps(arguments map[string]any) (*mcp.CallToolResult, error) {
	ctx, cancel := newContext()
	defer cancel()

	var projectName *string
	if projectArg, ok := parseString(arguments, "project"); ok {
		projectName = &projectArg
	}

	var scope *string
	if scopeArg, ok := parseString(arguments, "scope"); ok {
		scope = &scopeArg
	}

	steps, err := h.db.GetAvailableSteps(ctx, projectName, scope)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("get available steps: %v", err)), nil
	}

	result, err := json.Marshal(steps)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(result)), nil
}

// DetectStaleWork handles detect_stale_work tool
func (h *Handlers) DetectStaleWork(arguments map[string]any) (*mcp.CallToolResult, error) {
	ctx, cancel := newContext()
	defer cancel()

	timeoutMinutes := 15
	if v, ok := parseInt(arguments, "timeout_minutes"); ok {
		timeoutMinutes = v
	}

	steps, err := h.db.DetectStaleWork(ctx, timeoutMinutes)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("detect stale work: %v", err)), nil
	}

	result, err := json.Marshal(steps)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(result)), nil
}

// RecoverStep handles recover_step tool
func (h *Handlers) RecoverStep(arguments map[string]any) (*mcp.CallToolResult, error) {
	ctx, cancel := newContext()
	defer cancel()

	stepID, ok := parseStepID(arguments)
	if !ok {
		return mcp.NewToolResultError("step_id is required"), nil
	}

	if err := h.db.RecoverStep(ctx, db.StepID(stepID)); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("recover step: %v", err)), nil
	}

	return mcp.NewToolResultText("{\"ok\":true}"), nil
}

// RecoverFailedStep handles recover_failed_step tool
func (h *Handlers) RecoverFailedStep(arguments map[string]any) (*mcp.CallToolResult, error) {
	ctx, cancel := newContext()
	defer cancel()

	stepID, ok := parseStepID(arguments)
	if !ok {
		return mcp.NewToolResultError("step_id is required"), nil
	}

	if err := h.db.RecoverFailedStep(ctx, db.StepID(stepID)); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("recover failed step: %v", err)), nil
	}

	return mcp.NewToolResultText("{\"ok\":true}"), nil
}

// GetMetrics handles get_metrics tool
func (h *Handlers) GetMetrics(arguments map[string]any) (*mcp.CallToolResult, error) {
	ctx, cancel := newContext()
	defer cancel()

	var projectName *string
	if projectArg, ok := parseString(arguments, "project"); ok {
		projectName = &projectArg
	}

	var agentID *string
	if agentArg, ok := parseString(arguments, "agent_id"); ok {
		agentID = &agentArg
	}

	metrics, err := h.db.GetMetrics(ctx, projectName, agentID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("get metrics: %v", err)), nil
	}

	result, err := json.Marshal(metrics)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(result)), nil
}

// GetAgentEvents handles get_agent_events tool
func (h *Handlers) GetAgentEvents(arguments map[string]any) (*mcp.CallToolResult, error) {
	ctx, cancel := newContext()
	defer cancel()

	var agentID *string
	if agentArg, ok := parseString(arguments, "agent_id"); ok {
		agentID = &agentArg
	}

	var projectName *string
	if projectArg, ok := parseString(arguments, "project"); ok {
		projectName = &projectArg
	}

	limit := 100
	if v, ok := parseInt(arguments, "limit"); ok {
		limit = v
	}

	events, err := h.db.GetAgentEvents(ctx, agentID, projectName, limit)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("get agent events: %v", err)), nil
	}

	result, err := json.Marshal(events)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(result)), nil
}
