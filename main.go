package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/yourusername/concurrent-agent-mcp/internal/db"
	mcphandlers "github.com/yourusername/concurrent-agent-mcp/internal/mcp"
)

const (
	serverName    = "concurrent-agent-mcp"
	serverVersion = "0.1.0"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// Get database path from env or use default
	dbPath := os.Getenv("AGENT_DB_PATH")
	if dbPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("get home dir: %w", err)
		}
		dbPath = filepath.Join(homeDir, ".claude", "agent-coordination.db")
	}

	// Ensure database directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return fmt.Errorf("create db directory: %w", err)
	}

	// Initialize database
	database, err := db.New(dbPath)
	if err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}
	defer database.Close()

	// Create MCP server
	s := server.NewMCPServer(
		serverName,
		serverVersion,
	)

	// Initialize handlers
	handlers := mcphandlers.NewHandlers(database)

	// Register tools
	registerTools(s, handlers)

	// Run server via stdio
	if err := server.ServeStdio(s); err != nil {
		return fmt.Errorf("serve: %w", err)
	}

	return nil
}

func registerTools(s *server.MCPServer, handlers *mcphandlers.Handlers) {
	// Project management
	s.AddTool(mcp.NewTool("create_project",
		mcp.WithDescription("Create a new project with steps. Steps parameter should be a JSON array of objects with step_num, branch, scope, and depends_on fields."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Project name")),
		mcp.WithString("base_commit",
			mcp.Required(),
			mcp.Description("Git commit hash all worktrees are based on")),
		mcp.WithString("steps",
			mcp.Required(),
			mcp.Description("JSON string containing array of step definitions")),
	), handlers.CreateProject)

	s.AddTool(mcp.NewTool("get_project",
		mcp.WithDescription("Get project details and all steps"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Project name")),
	), handlers.GetProject)

	s.AddTool(mcp.NewTool("list_projects",
		mcp.WithDescription("List all projects"),
		mcp.WithString("status",
			mcp.Description("Filter by status (active, completed, aborted)")),
	), handlers.ListProjects)

	// Step operations
	s.AddTool(mcp.NewTool("claim_step",
		mcp.WithDescription("Atomically claim the next available step"),
		mcp.WithString("project",
			mcp.Required(),
			mcp.Description("Project name")),
		mcp.WithString("agent_id",
			mcp.Required(),
			mcp.Description("Agent identifier")),
	), handlers.ClaimStep)

	s.AddTool(mcp.NewTool("start_step",
		mcp.WithDescription("Mark a claimed step as in progress"),
		mcp.WithNumber("step_id",
			mcp.Required(),
			mcp.Description("Step ID")),
		mcp.WithString("worktree",
			mcp.Description("Worktree path")),
	), handlers.StartStep)

	s.AddTool(mcp.NewTool("heartbeat",
		mcp.WithDescription("Update step heartbeat (call every 30-60 seconds while working)"),
		mcp.WithNumber("step_id",
			mcp.Required(),
			mcp.Description("Step ID")),
		mcp.WithString("agent_id",
			mcp.Required(),
			mcp.Description("Agent identifier")),
	), handlers.Heartbeat)

	s.AddTool(mcp.NewTool("complete_step",
		mcp.WithDescription("Mark step as completed"),
		mcp.WithNumber("step_id",
			mcp.Required(),
			mcp.Description("Step ID")),
		mcp.WithString("commit_hash",
			mcp.Required(),
			mcp.Description("Final git commit hash")),
		mcp.WithString("files_modified",
			mcp.Description("JSON array string of modified file paths")),
		mcp.WithString("notes",
			mcp.Description("Completion notes")),
	), handlers.CompleteStep)

	s.AddTool(mcp.NewTool("fail_step",
		mcp.WithDescription("Mark step as failed"),
		mcp.WithNumber("step_id",
			mcp.Required(),
			mcp.Description("Step ID")),
		mcp.WithString("reason",
			mcp.Required(),
			mcp.Description("Failure reason")),
	), handlers.FailStep)

	s.AddTool(mcp.NewTool("get_step",
		mcp.WithDescription("Get step details"),
		mcp.WithNumber("step_id",
			mcp.Required(),
			mcp.Description("Step ID")),
	), handlers.GetStep)

	// Coordination
	s.AddTool(mcp.NewTool("get_available_steps",
		mcp.WithDescription("Get all steps available to work on (no incomplete dependencies)"),
		mcp.WithString("project",
			mcp.Description("Filter by project name")),
		mcp.WithString("scope",
			mcp.Description("Filter by scope (api, ui, db, etc)")),
	), handlers.GetAvailableSteps)

	s.AddTool(mcp.NewTool("detect_stale_work",
		mcp.WithDescription("Find steps with stale heartbeats (crashed agents)"),
		mcp.WithNumber("timeout_minutes",
			mcp.Description("Minutes since last heartbeat (default: 15)")),
	), handlers.DetectStaleWork)

	s.AddTool(mcp.NewTool("recover_step",
		mcp.WithDescription("Recover a stale step (reset to not_started)"),
		mcp.WithNumber("step_id",
			mcp.Required(),
			mcp.Description("Step ID")),
	), handlers.RecoverStep)

	// Analytics
	s.AddTool(mcp.NewTool("get_metrics",
		mcp.WithDescription("Get project and agent metrics"),
		mcp.WithString("project",
			mcp.Description("Filter by project name")),
		mcp.WithString("agent_id",
			mcp.Description("Filter by agent ID")),
	), handlers.GetMetrics)

	s.AddTool(mcp.NewTool("get_agent_events",
		mcp.WithDescription("Get agent activity log"),
		mcp.WithString("agent_id",
			mcp.Description("Filter by agent ID")),
		mcp.WithString("project",
			mcp.Description("Filter by project name")),
		mcp.WithNumber("limit",
			mcp.Description("Max events to return (default: 100)")),
	), handlers.GetAgentEvents)
}
