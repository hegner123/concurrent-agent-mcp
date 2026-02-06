package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"
)

// DB wraps the database connection and sqlc queries
type DB struct {
	conn    *sql.DB
	queries *Queries
}

// NewDB creates a new database connection and runs migrations
func NewDB(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// WAL allows concurrent readers but only one writer.
	// Keep pool small: pragmas are per-connection and must be applied to each.
	// With MaxOpenConns(1), all queries use the single connection that has pragmas set.
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	conn.SetConnMaxLifetime(0)

	// Configure SQLite for concurrent access
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
		"PRAGMA temp_store=MEMORY",
		"PRAGMA cache_size=-64000",
	}

	for _, pragma := range pragmas {
		if _, err := conn.Exec(pragma); err != nil {
			pragmaErr := err
			if closeErr := conn.Close(); closeErr != nil {
				return nil, fmt.Errorf("set pragma %s: %w (close error: %v)", pragma, pragmaErr, closeErr)
			}
			return nil, fmt.Errorf("set pragma %s: %w", pragma, pragmaErr)
		}
	}

	db := &DB{
		conn:    conn,
		queries: New(conn),
	}

	// Run migrations
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

// migrate runs database migrations
func (db *DB) migrate() error {
	// Check if schema_migrations table exists
	var count int
	err := db.conn.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name='schema_migrations'
	`).Scan(&count)
	if err != nil {
		return fmt.Errorf("check migrations table: %w", err)
	}

	// If schema_migrations exists, check if migration is already applied
	if count > 0 {
		var v1 int
		err = db.conn.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 1`).Scan(&v1)
		if err != nil {
			return fmt.Errorf("check migration version: %w", err)
		}
		if v1 > 0 {
			return db.migrateV2()
		}
	}

	// Execute initial migration
	initialSchema := `
		-- Initial schema for concurrent agent coordination
		PRAGMA foreign_keys = ON;

		CREATE TABLE IF NOT EXISTS projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			base_commit TEXT,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'completed', 'aborted')),
			priority INTEGER NOT NULL DEFAULT 0
		);

		CREATE INDEX IF NOT EXISTS idx_projects_status ON projects(status);
		CREATE INDEX IF NOT EXISTS idx_projects_priority ON projects(priority DESC);

		CREATE TABLE IF NOT EXISTS steps (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL,
			step_num INTEGER NOT NULL,
			branch TEXT NOT NULL,
			scope TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'not_started' CHECK(status IN ('not_started', 'claimed', 'in_progress', 'completed', 'failed', 'merged')),
			worktree TEXT,
			agent_id TEXT,
			claimed_at TEXT,
			started_at TEXT,
			completed_at TEXT,
			last_heartbeat TEXT,
			last_commit TEXT,
			files_modified TEXT,
			notes TEXT,
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE,
			UNIQUE(project_id, step_num)
		);

		CREATE INDEX IF NOT EXISTS idx_steps_project_id ON steps(project_id);
		CREATE INDEX IF NOT EXISTS idx_steps_status ON steps(status);
		CREATE INDEX IF NOT EXISTS idx_steps_agent_id ON steps(agent_id);
		CREATE INDEX IF NOT EXISTS idx_steps_scope ON steps(scope);
		CREATE INDEX IF NOT EXISTS idx_steps_last_heartbeat ON steps(last_heartbeat);

		CREATE TABLE IF NOT EXISTS dependencies (
			step_id INTEGER NOT NULL,
			depends_on_step_id INTEGER NOT NULL,
			FOREIGN KEY(step_id) REFERENCES steps(id) ON DELETE CASCADE,
			FOREIGN KEY(depends_on_step_id) REFERENCES steps(id) ON DELETE CASCADE,
			PRIMARY KEY(step_id, depends_on_step_id)
		);

		CREATE INDEX IF NOT EXISTS idx_dependencies_step_id ON dependencies(step_id);
		CREATE INDEX IF NOT EXISTS idx_dependencies_depends_on ON dependencies(depends_on_step_id);

		CREATE TABLE IF NOT EXISTS agent_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_id TEXT NOT NULL,
			project_id INTEGER,
			step_id INTEGER,
			event_type TEXT NOT NULL CHECK(event_type IN ('claimed', 'started', 'heartbeat', 'completed', 'failed', 'recovered')),
			timestamp TEXT NOT NULL DEFAULT (datetime('now')),
			metadata TEXT,
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE SET NULL,
			FOREIGN KEY(step_id) REFERENCES steps(id) ON DELETE SET NULL
		);

		CREATE INDEX IF NOT EXISTS idx_agent_events_timestamp ON agent_events(timestamp DESC);
		CREATE INDEX IF NOT EXISTS idx_agent_events_agent_id ON agent_events(agent_id);
		CREATE INDEX IF NOT EXISTS idx_agent_events_step_id ON agent_events(step_id);

		CREATE TABLE IF NOT EXISTS failed_steps_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			original_step_id INTEGER NOT NULL,
			project_id INTEGER NOT NULL,
			step_num INTEGER NOT NULL,
			branch TEXT NOT NULL,
			scope TEXT NOT NULL,
			worktree TEXT,
			agent_id TEXT,
			claimed_at TEXT,
			started_at TEXT,
			failed_at TEXT,
			last_commit TEXT,
			files_modified TEXT,
			notes TEXT,
			archived_at TEXT NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_failed_steps_history_original_step_id ON failed_steps_history(original_step_id);
		CREATE INDEX IF NOT EXISTS idx_failed_steps_history_project_id ON failed_steps_history(project_id);
		CREATE INDEX IF NOT EXISTS idx_failed_steps_history_archived_at ON failed_steps_history(archived_at DESC);

		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		);

		INSERT OR IGNORE INTO schema_migrations (version) VALUES (1);
	`

	if _, err := db.conn.Exec(initialSchema); err != nil {
		return fmt.Errorf("execute migration: %w", err)
	}

	return db.migrateV2()
}

// migrateV2 adds composite index for ClaimStep performance
func (db *DB) migrateV2() error {
	var v2 int
	err := db.conn.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 2`).Scan(&v2)
	if err != nil {
		return fmt.Errorf("check migration v2: %w", err)
	}
	if v2 > 0 {
		return nil
	}

	v2Schema := `
		CREATE INDEX IF NOT EXISTS idx_steps_project_status_stepnum ON steps(project_id, status, step_num);
		DROP INDEX IF EXISTS idx_steps_project_id;
		INSERT OR IGNORE INTO schema_migrations (version) VALUES (2);
	`
	if _, err := db.conn.Exec(v2Schema); err != nil {
		return fmt.Errorf("execute migration v2: %w", err)
	}

	return nil
}

// StepInput is input for creating a step
type StepInput struct {
	StepNum   int    `json:"step_num"`
	Branch    string `json:"branch"`
	Scope     string `json:"scope"`
	DependsOn []int  `json:"depends_on"`
}

// CreateProject creates a new project with steps
func (db *DB) CreateProject(ctx context.Context, name, baseCommit string, steps []StepInput) (*Project, error) {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := db.queries.WithTx(tx)

	// Insert project
	project, err := qtx.CreateProject(ctx, CreateProjectParams{
		Name:       name,
		BaseCommit: sql.NullString{String: baseCommit, Valid: baseCommit != ""},
	})
	if err != nil {
		return nil, fmt.Errorf("insert project: %w", err)
	}

	// Insert steps
	for _, step := range steps {
		stepID, err := qtx.CreateStep(ctx, CreateStepParams{
			ProjectID: project.ID,
			StepNum:   int64(step.StepNum),
			Branch:    step.Branch,
			Scope:     step.Scope,
		})
		if err != nil {
			return nil, fmt.Errorf("insert step %d: %w", step.StepNum, err)
		}

		// Insert dependencies
		for _, depStepNum := range step.DependsOn {
			result, err := qtx.CreateDependency(ctx, CreateDependencyParams{
				StepID:    stepID,
				ProjectID: project.ID,
				StepNum:   int64(depStepNum),
			})
			if err != nil {
				return nil, fmt.Errorf("insert dependency %d->%d: %w", step.StepNum, depStepNum, err)
			}
			rows, err := result.RowsAffected()
			if err != nil {
				return nil, fmt.Errorf("check dependency rows %d->%d: %w", step.StepNum, depStepNum, err)
			}
			if rows == 0 {
				return nil, fmt.Errorf("dependency target step_num %d not found in project", depStepNum)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return &project, nil
}

// ProjectWithSteps wraps a project with its steps for API responses
type ProjectWithSteps struct {
	Project Project `json:"project"`
	Steps   []Step  `json:"steps"`
}

// GetProject gets a project by name with all its steps
func (db *DB) GetProject(ctx context.Context, name string) (*ProjectWithSteps, error) {
	p, err := db.queries.GetProject(ctx, name)
	if err != nil {
		return nil, err
	}
	steps, err := db.queries.GetStepsByProjectID(ctx, p.ID)
	if err != nil {
		return nil, fmt.Errorf("get steps: %w", err)
	}
	return &ProjectWithSteps{Project: p, Steps: steps}, nil
}

// ClaimStep atomically claims the next available step
func (db *DB) ClaimStep(ctx context.Context, projectName, agentID string) (*Step, error) {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := db.queries.WithTx(tx)

	row, err := qtx.ClaimStep(ctx, ClaimStepParams{
		AgentID: sql.NullString{String: agentID, Valid: true},
		Name:    projectName,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil // No work available
	}
	if err != nil {
		return nil, fmt.Errorf("claim step: %w", err)
	}

	// Record event
	err = qtx.InsertAgentEvent(ctx, InsertAgentEventParams{
		AgentID:   agentID,
		ProjectID: NullProjectID{ProjectID: row.ProjectID, Valid: true},
		StepID:    NullStepID{StepID: row.ID, Valid: true},
		EventType: EventTypeClaimed,
	})
	if err != nil {
		return nil, fmt.Errorf("record event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// Convert ClaimStepRow to Step
	step := &Step{
		ID:            row.ID,
		ProjectID:     row.ProjectID,
		StepNum:       row.StepNum,
		Branch:        row.Branch,
		Scope:         row.Scope,
		Status:        row.Status,
		AgentID:       row.AgentID,
		ClaimedAt:     row.ClaimedAt,
		LastHeartbeat: row.LastHeartbeat,
	}

	return step, nil
}

// Heartbeat updates step heartbeat
func (db *DB) Heartbeat(ctx context.Context, stepID StepID, agentID string) error {
	result, err := db.queries.UpdateHeartbeat(ctx, UpdateHeartbeatParams{
		ID:      stepID,
		AgentID: sql.NullString{String: agentID, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("update heartbeat: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("step not found or not owned by agent")
	}

	if evtErr := db.queries.InsertAgentEvent(ctx, InsertAgentEventParams{
		AgentID:   agentID,
		StepID:    NullStepID{StepID: stepID, Valid: true},
		EventType: EventTypeHeartbeat,
	}); evtErr != nil {
		return fmt.Errorf("heartbeat succeeded but event insert failed: %w", evtErr)
	}

	return nil
}

// StartStep marks a claimed step as in progress
func (db *DB) StartStep(ctx context.Context, stepID StepID, worktree *string) error {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := db.queries.WithTx(tx)

	var worktreeNull sql.NullString
	if worktree != nil {
		worktreeNull = sql.NullString{String: *worktree, Valid: true}
	}

	result, err := qtx.StartStep(ctx, StartStepParams{
		Worktree: worktreeNull,
		ID:       stepID,
	})
	if err != nil {
		return fmt.Errorf("update step: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("step not found or not in claimed state")
	}

	// Record event
	err = qtx.InsertAgentEventFromStep(ctx, InsertAgentEventFromStepParams{
		EventType: EventTypeStarted,
		ID:        stepID,
	})
	if err != nil {
		return fmt.Errorf("record event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

// CompleteStep marks a step as completed
func (db *DB) CompleteStep(ctx context.Context, stepID StepID, commitHash string, filesModified []string, notes *string) error {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := db.queries.WithTx(tx)

	// Convert files to JSON
	var filesJSON sql.NullString
	if len(filesModified) > 0 {
		bytes, err := json.Marshal(filesModified)
		if err != nil {
			return fmt.Errorf("marshal files: %w", err)
		}
		filesJSON = sql.NullString{String: string(bytes), Valid: true}
	}

	var notesNull sql.NullString
	if notes != nil {
		notesNull = sql.NullString{String: *notes, Valid: true}
	}

	result, err := qtx.CompleteStep(ctx, CompleteStepParams{
		LastCommit:    sql.NullString{String: commitHash, Valid: commitHash != ""},
		FilesModified: filesJSON,
		Notes:         notesNull,
		ID:            stepID,
	})
	if err != nil {
		return fmt.Errorf("update step: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("step not found or not in progress")
	}

	// Record event
	err = qtx.InsertAgentEventFromStep(ctx, InsertAgentEventFromStepParams{
		EventType: EventTypeCompleted,
		ID:        stepID,
	})
	if err != nil {
		return fmt.Errorf("record event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

// FailStep marks a step as failed
func (db *DB) FailStep(ctx context.Context, stepID StepID, reason string) error {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := db.queries.WithTx(tx)

	result, err := qtx.FailStep(ctx, FailStepParams{
		Notes: sql.NullString{String: reason, Valid: reason != ""},
		ID:    stepID,
	})
	if err != nil {
		return fmt.Errorf("update step: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("step not found or not in active state")
	}

	// Record event with reason in metadata
	metadataMap := map[string]string{"reason": reason}
	metadataBytes, err := json.Marshal(metadataMap)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	metadata := string(metadataBytes)
	err = qtx.InsertAgentEventFromStepWithMetadata(ctx, InsertAgentEventFromStepWithMetadataParams{
		EventType: EventTypeFailed,
		Metadata:  sql.NullString{String: metadata, Valid: true},
		ID:        stepID,
	})
	if err != nil {
		return fmt.Errorf("record event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

// GetStep gets a step by ID
func (db *DB) GetStep(ctx context.Context, stepID StepID) (*Step, error) {
	s, err := db.queries.GetStep(ctx, stepID)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ListProjects lists projects with optional status filter
func (db *DB) ListProjects(ctx context.Context, status *string) ([]Project, error) {
	if status != nil {
		ps := ProjectStatus(*status)
		if !ps.Valid() {
			return nil, fmt.Errorf("invalid project status: %q", *status)
		}
		return db.queries.ListProjectsByStatus(ctx, ps)
	}
	return db.queries.ListProjectsAll(ctx)
}

// GetAvailableSteps gets all steps available to work on
func (db *DB) GetAvailableSteps(ctx context.Context, projectName *string, scope *string) ([]Step, error) {
	if projectName != nil && scope != nil {
		return db.queries.GetAvailableStepsByProjectAndScope(ctx, GetAvailableStepsByProjectAndScopeParams{
			Name:  *projectName,
			Scope: *scope,
		})
	}
	if projectName != nil {
		return db.queries.GetAvailableStepsByProject(ctx, *projectName)
	}
	if scope != nil {
		return db.queries.GetAvailableStepsByScope(ctx, *scope)
	}
	return db.queries.GetAvailableStepsAll(ctx)
}

// DetectStaleWork finds steps with stale heartbeats
// This method uses raw SQL because sqlc doesn't support parameterized time intervals
func (db *DB) DetectStaleWork(ctx context.Context, timeoutMinutes int) ([]Step, error) {
	if timeoutMinutes <= 0 {
		timeoutMinutes = 15
	}

	query := fmt.Sprintf(`
		SELECT id, project_id, step_num, branch, scope, status,
		       worktree, agent_id, claimed_at, started_at, completed_at,
		       last_heartbeat, last_commit, files_modified, notes
		FROM steps
		WHERE status IN ('claimed', 'in_progress')
		AND (last_heartbeat IS NULL OR last_heartbeat < datetime('now', '-%d minutes'))
	`, timeoutMinutes)

	rows, err := db.conn.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query stale steps: %w", err)
	}
	defer rows.Close()

	var steps []Step
	for rows.Next() {
		var s Step
		err := rows.Scan(
			&s.ID, &s.ProjectID, &s.StepNum, &s.Branch, &s.Scope, &s.Status,
			&s.Worktree, &s.AgentID, &s.ClaimedAt, &s.StartedAt, &s.CompletedAt,
			&s.LastHeartbeat, &s.LastCommit, &s.FilesModified, &s.Notes,
		)
		if err != nil {
			return nil, fmt.Errorf("scan step: %w", err)
		}
		steps = append(steps, s)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return steps, nil
}

// RecoverStep resets a stale step to not_started
func (db *DB) RecoverStep(ctx context.Context, stepID StepID) error {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := db.queries.WithTx(tx)

	result, err := qtx.RecoverStep(ctx, stepID)
	if err != nil {
		return fmt.Errorf("update step: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("step not found or not in recoverable state")
	}

	// Record event
	err = qtx.InsertAgentEvent(ctx, InsertAgentEventParams{
		AgentID:   "system",
		StepID:    NullStepID{StepID: stepID, Valid: true},
		EventType: EventTypeRecovered,
	})
	if err != nil {
		return fmt.Errorf("record event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

// RecoverFailedStep archives a failed step to history and resets it to not_started
func (db *DB) RecoverFailedStep(ctx context.Context, stepID StepID) error {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := db.queries.WithTx(tx)

	// Get the failed step
	step, err := qtx.GetFailedStep(ctx, stepID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("step not found or not in failed state")
		}
		return fmt.Errorf("get failed step: %w", err)
	}

	// Archive the failed step to history
	err = qtx.ArchiveFailedStep(ctx, ArchiveFailedStepParams{
		OriginalStepID: step.ID,
		ProjectID:      step.ProjectID,
		StepNum:        step.StepNum,
		Branch:         step.Branch,
		Scope:          step.Scope,
		Worktree:       step.Worktree,
		AgentID:        step.AgentID,
		ClaimedAt:      step.ClaimedAt,
		StartedAt:      step.StartedAt,
		LastCommit:     step.LastCommit,
		FilesModified:  step.FilesModified,
		Notes:          step.Notes,
	})
	if err != nil {
		return fmt.Errorf("archive failed step: %w", err)
	}

	// Reset the step to not_started
	result, err := qtx.ResetFailedStep(ctx, stepID)
	if err != nil {
		return fmt.Errorf("reset step: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("step update failed")
	}

	// Record recovery event
	err = qtx.InsertAgentEventWithMetadata(ctx, InsertAgentEventWithMetadataParams{
		AgentID:   "system",
		ProjectID: NullProjectID{ProjectID: step.ProjectID, Valid: true},
		StepID:    NullStepID{StepID: stepID, Valid: true},
		EventType: EventTypeRecovered,
		Metadata:  sql.NullString{String: `{"from_status":"failed"}`, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("record event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

// AutoRecover detects stale work and recovers it automatically
func (db *DB) AutoRecover(ctx context.Context, timeoutMinutes int) (int, error) {
	stale, err := db.DetectStaleWork(ctx, timeoutMinutes)
	if err != nil {
		return 0, fmt.Errorf("detect stale work: %w", err)
	}
	recovered := 0
	for _, step := range stale {
		if err := db.RecoverStep(ctx, step.ID); err != nil {
			continue // Step may have been recovered by another path
		}
		recovered++
	}
	return recovered, nil
}

// Metrics holds metrics data
type Metrics struct {
	TotalSteps      int            `json:"total_steps"`
	CompletedSteps  int            `json:"completed_steps"`
	FailedSteps     int            `json:"failed_steps"`
	InProgressSteps int            `json:"in_progress_steps"`
	AvgTimeHours    float64        `json:"avg_time_hours,omitempty"`
	ScopeBreakdown  map[string]int `json:"scope_breakdown,omitempty"`
}

// GetMetrics calculates project and agent metrics
func (db *DB) GetMetrics(ctx context.Context, projectName *string, agentID *string) (*Metrics, error) {
	var m Metrics
	var total int64
	var completed, failed, inProgress, avgHours sql.NullFloat64

	if projectName != nil && agentID != nil {
		row, err := db.queries.GetMetricsByProjectAndAgent(ctx, GetMetricsByProjectAndAgentParams{
			Name:    *projectName,
			AgentID: sql.NullString{String: *agentID, Valid: true},
		})
		if err != nil {
			return nil, fmt.Errorf("query metrics: %w", err)
		}
		total, completed, failed, inProgress, avgHours = row.Total, row.Completed, row.Failed, row.InProgress, row.AvgHours
	} else if projectName != nil {
		row, err := db.queries.GetMetricsByProject(ctx, *projectName)
		if err != nil {
			return nil, fmt.Errorf("query metrics: %w", err)
		}
		total, completed, failed, inProgress, avgHours = row.Total, row.Completed, row.Failed, row.InProgress, row.AvgHours
	} else if agentID != nil {
		row, err := db.queries.GetMetricsByAgent(ctx, sql.NullString{String: *agentID, Valid: true})
		if err != nil {
			return nil, fmt.Errorf("query metrics: %w", err)
		}
		total, completed, failed, inProgress, avgHours = row.Total, row.Completed, row.Failed, row.InProgress, row.AvgHours
	} else {
		row, err := db.queries.GetMetricsAll(ctx)
		if err != nil {
			return nil, fmt.Errorf("query metrics: %w", err)
		}
		total, completed, failed, inProgress, avgHours = row.Total, row.Completed, row.Failed, row.InProgress, row.AvgHours
	}

	m.TotalSteps = int(total)
	if completed.Valid {
		m.CompletedSteps = int(completed.Float64)
	}
	if failed.Valid {
		m.FailedSteps = int(failed.Float64)
	}
	if inProgress.Valid {
		m.InProgressSteps = int(inProgress.Float64)
	}
	if avgHours.Valid {
		m.AvgTimeHours = avgHours.Float64
	}

	// Get scope breakdown
	m.ScopeBreakdown = make(map[string]int)

	if projectName != nil && agentID != nil {
		rows, err := db.queries.GetScopeBreakdownByProjectAndAgent(ctx, GetScopeBreakdownByProjectAndAgentParams{
			Name:    *projectName,
			AgentID: sql.NullString{String: *agentID, Valid: true},
		})
		if err != nil {
			return nil, fmt.Errorf("query scope breakdown: %w", err)
		}
		for _, r := range rows {
			m.ScopeBreakdown[r.Scope] = int(r.Count)
		}
	} else if projectName != nil {
		rows, err := db.queries.GetScopeBreakdownByProject(ctx, *projectName)
		if err != nil {
			return nil, fmt.Errorf("query scope breakdown: %w", err)
		}
		for _, r := range rows {
			m.ScopeBreakdown[r.Scope] = int(r.Count)
		}
	} else if agentID != nil {
		rows, err := db.queries.GetScopeBreakdownByAgent(ctx, sql.NullString{String: *agentID, Valid: true})
		if err != nil {
			return nil, fmt.Errorf("query scope breakdown: %w", err)
		}
		for _, r := range rows {
			m.ScopeBreakdown[r.Scope] = int(r.Count)
		}
	} else {
		rows, err := db.queries.GetScopeBreakdownAll(ctx)
		if err != nil {
			return nil, fmt.Errorf("query scope breakdown: %w", err)
		}
		for _, r := range rows {
			m.ScopeBreakdown[r.Scope] = int(r.Count)
		}
	}

	return &m, nil
}

// AgentEventResult represents an event in the audit trail (for API compatibility)
type AgentEventResult struct {
	ID        AgentEventID `json:"id"`
	AgentID   string       `json:"agent_id"`
	ProjectID *ProjectID   `json:"project_id,omitempty"`
	StepID    *StepID      `json:"step_id,omitempty"`
	EventType EventType    `json:"event_type"`
	Timestamp SQLiteTime   `json:"timestamp"`
	Metadata  *string      `json:"metadata,omitempty"`
}

// GetAgentEvents gets agent activity log
func (db *DB) GetAgentEvents(ctx context.Context, agentID *string, projectName *string, limit int) ([]AgentEventResult, error) {
	if limit <= 0 {
		limit = 100
	}

	var events []AgentEvent
	var err error

	if agentID != nil && projectName != nil {
		events, err = db.queries.GetAgentEventsByAgentAndProject(ctx, GetAgentEventsByAgentAndProjectParams{
			AgentID: *agentID,
			Name:    *projectName,
			Limit:   int64(limit),
		})
	} else if agentID != nil {
		events, err = db.queries.GetAgentEventsByAgent(ctx, GetAgentEventsByAgentParams{
			AgentID: *agentID,
			Limit:   int64(limit),
		})
	} else if projectName != nil {
		events, err = db.queries.GetAgentEventsByProject(ctx, GetAgentEventsByProjectParams{
			Name:  *projectName,
			Limit: int64(limit),
		})
	} else {
		events, err = db.queries.GetAgentEventsAll(ctx, int64(limit))
	}

	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}

	// Convert to result type
	results := make([]AgentEventResult, len(events))
	for i, e := range events {
		results[i] = AgentEventResult{
			ID:        e.ID,
			AgentID:   e.AgentID,
			EventType: e.EventType,
			Timestamp: e.Timestamp,
		}
		if e.ProjectID.Valid {
			results[i].ProjectID = &e.ProjectID.ProjectID
		}
		if e.StepID.Valid {
			results[i].StepID = &e.StepID.StepID
		}
		if e.Metadata.Valid {
			results[i].Metadata = &e.Metadata.String
		}
	}

	return results, nil
}
