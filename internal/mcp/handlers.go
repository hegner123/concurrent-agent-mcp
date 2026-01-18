package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/yourusername/concurrent-agent-mcp/internal/db"
)

// Handlers contains MCP tool handlers
type Handlers struct {
	db *db.DB
}

// NewHandlers creates new handlers
func NewHandlers(database *db.DB) *Handlers {
	return &Handlers{db: database}
}

// CreateProject handles create_project tool
func (h *Handlers) CreateProject(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	name, ok := arguments["name"].(string)
	if !ok {
		return mcp.NewToolResultError("name must be a string"), nil
	}

	baseCommit, ok := arguments["base_commit"].(string)
	if !ok {
		return mcp.NewToolResultError("base_commit must be a string"), nil
	}

	// Handle steps - can be either []interface{} or string (JSON)
	var stepsRaw []interface{}
	switch v := arguments["steps"].(type) {
	case []interface{}:
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
		stepMap, ok := stepRaw.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("each step must be an object"), nil
		}

		stepNum, ok := stepMap["step_num"].(float64)
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
		if depsRaw, ok := stepMap["depends_on"].([]interface{}); ok {
			for _, depRaw := range depsRaw {
				dep, ok := depRaw.(float64)
				if !ok {
					return mcp.NewToolResultError("depends_on must be array of numbers"), nil
				}
				dependsOn = append(dependsOn, int(dep))
			}
		}

		steps = append(steps, db.StepInput{
			StepNum:   int(stepNum),
			Branch:    branch,
			Scope:     scope,
			DependsOn: dependsOn,
		})
	}

	project, err := h.db.CreateProject(name, baseCommit, steps)
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
func (h *Handlers) GetProject(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	name, ok := arguments["name"].(string)
	if !ok {
		return mcp.NewToolResultError("name must be a string"), nil
	}

	project, err := h.db.GetProject(name)
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
func (h *Handlers) ListProjects(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	var status *string
	if statusArg, ok := arguments["status"].(string); ok {
		status = &statusArg
	}

	projects, err := h.db.ListProjects(status)
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
func (h *Handlers) ClaimStep(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	project, ok := arguments["project"].(string)
	if !ok {
		return mcp.NewToolResultError("project must be a string"), nil
	}

	agentID, ok := arguments["agent_id"].(string)
	if !ok {
		return mcp.NewToolResultError("agent_id must be a string"), nil
	}

	step, err := h.db.ClaimStep(project, agentID)
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
func (h *Handlers) StartStep(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	stepID, ok := arguments["step_id"].(float64)
	if !ok {
		return mcp.NewToolResultError("step_id must be a number"), nil
	}

	var worktree *string
	if worktreeArg, ok := arguments["worktree"].(string); ok {
		worktree = &worktreeArg
	}

	if err := h.db.StartStep(int64(stepID), worktree); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("start step: %v", err)), nil
	}

	return mcp.NewToolResultText("{\"ok\":true}"), nil
}

// Heartbeat handles heartbeat tool
func (h *Handlers) Heartbeat(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	stepID, ok := arguments["step_id"].(float64)
	if !ok {
		return mcp.NewToolResultError("step_id must be a number"), nil
	}

	agentID, ok := arguments["agent_id"].(string)
	if !ok {
		return mcp.NewToolResultError("agent_id must be a string"), nil
	}

	if err := h.db.Heartbeat(int64(stepID), agentID); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("heartbeat: %v", err)), nil
	}

	return mcp.NewToolResultText("{\"ok\":true}"), nil
}

// CompleteStep handles complete_step tool
func (h *Handlers) CompleteStep(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	stepID, ok := arguments["step_id"].(float64)
	if !ok {
		return mcp.NewToolResultError("step_id must be a number"), nil
	}

	commitHash, ok := arguments["commit_hash"].(string)
	if !ok {
		return mcp.NewToolResultError("commit_hash must be a string"), nil
	}

	// Handle files_modified - can be either []interface{} or string (JSON)
	var filesModified []string
	if filesArg, ok := arguments["files_modified"]; ok && filesArg != nil {
		switch v := filesArg.(type) {
		case []interface{}:
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
	if notesArg, ok := arguments["notes"].(string); ok {
		notes = &notesArg
	}

	if err := h.db.CompleteStep(int64(stepID), commitHash, filesModified, notes); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("complete step: %v", err)), nil
	}

	return mcp.NewToolResultText("{\"ok\":true}"), nil
}

// FailStep handles fail_step tool
func (h *Handlers) FailStep(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	stepID, ok := arguments["step_id"].(float64)
	if !ok {
		return mcp.NewToolResultError("step_id must be a number"), nil
	}

	reason, ok := arguments["reason"].(string)
	if !ok {
		return mcp.NewToolResultError("reason must be a string"), nil
	}

	if err := h.db.FailStep(int64(stepID), reason); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("fail step: %v", err)), nil
	}

	return mcp.NewToolResultText("{\"ok\":true}"), nil
}

// GetStep handles get_step tool
func (h *Handlers) GetStep(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	stepID, ok := arguments["step_id"].(float64)
	if !ok {
		return mcp.NewToolResultError("step_id must be a number"), nil
	}

	step, err := h.db.GetStep(int64(stepID))
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
func (h *Handlers) GetAvailableSteps(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	var projectName *string
	if projectArg, ok := arguments["project"].(string); ok {
		projectName = &projectArg
	}

	var scope *string
	if scopeArg, ok := arguments["scope"].(string); ok {
		scope = &scopeArg
	}

	steps, err := h.db.GetAvailableSteps(projectName, scope)
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
func (h *Handlers) DetectStaleWork(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	timeoutMinutes := 15
	if timeoutArg, ok := arguments["timeout_minutes"].(float64); ok {
		timeoutMinutes = int(timeoutArg)
	}

	steps, err := h.db.DetectStaleWork(timeoutMinutes)
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
func (h *Handlers) RecoverStep(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	stepID, ok := arguments["step_id"].(float64)
	if !ok {
		return mcp.NewToolResultError("step_id must be a number"), nil
	}

	if err := h.db.RecoverStep(int64(stepID)); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("recover step: %v", err)), nil
	}

	return mcp.NewToolResultText("{\"ok\":true}"), nil
}

// GetMetrics handles get_metrics tool
func (h *Handlers) GetMetrics(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	var projectName *string
	if projectArg, ok := arguments["project"].(string); ok {
		projectName = &projectArg
	}

	var agentID *string
	if agentArg, ok := arguments["agent_id"].(string); ok {
		agentID = &agentArg
	}

	metrics, err := h.db.GetMetrics(projectName, agentID)
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
func (h *Handlers) GetAgentEvents(arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	var agentID *string
	if agentArg, ok := arguments["agent_id"].(string); ok {
		agentID = &agentArg
	}

	var projectName *string
	if projectArg, ok := arguments["project"].(string); ok {
		projectName = &projectArg
	}

	limit := 100
	if limitArg, ok := arguments["limit"].(float64); ok {
		limit = int(limitArg)
	}

	events, err := h.db.GetAgentEvents(agentID, projectName, limit)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("get agent events: %v", err)), nil
	}

	result, err := json.Marshal(events)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(result)), nil
}
