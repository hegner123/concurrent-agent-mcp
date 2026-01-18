package db

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteTime wraps time.Time to handle SQLite TEXT datetime format
type SQLiteTime struct {
	time.Time
}

// Scan implements sql.Scanner for SQLite TEXT datetime
func (st *SQLiteTime) Scan(value interface{}) error {
	if value == nil {
		st.Time = time.Time{}
		return nil
	}

	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("cannot scan %T into SQLiteTime", value)
	}

	// SQLite datetime format: "2006-01-02 15:04:05"
	t, err := time.Parse("2006-01-02 15:04:05", str)
	if err != nil {
		return err
	}

	st.Time = t
	return nil
}

// MarshalJSON implements json.Marshaler for SQLiteTime
func (st SQLiteTime) MarshalJSON() ([]byte, error) {
	return st.Time.MarshalJSON()
}

// NullSQLiteTime wraps sql.NullTime to handle SQLite TEXT datetime format
type NullSQLiteTime struct {
	Time  time.Time
	Valid bool
}

// Scan implements sql.Scanner for nullable SQLite TEXT datetime
func (nst *NullSQLiteTime) Scan(value interface{}) error {
	if value == nil {
		nst.Time = time.Time{}
		nst.Valid = false
		return nil
	}

	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("cannot scan %T into NullSQLiteTime", value)
	}

	t, err := time.Parse("2006-01-02 15:04:05", str)
	if err != nil {
		return err
	}

	nst.Time = t
	nst.Valid = true
	return nil
}

// MarshalJSON implements json.Marshaler for NullSQLiteTime
func (nst NullSQLiteTime) MarshalJSON() ([]byte, error) {
	if !nst.Valid {
		return []byte("null"), nil
	}
	return nst.Time.MarshalJSON()
}

// Value implements driver.Valuer for NullSQLiteTime
func (nst NullSQLiteTime) Value() (driver.Value, error) {
	if !nst.Valid {
		return nil, nil
	}
	return nst.Time.Format("2006-01-02 15:04:05"), nil
}


// DB wraps the database connection
type DB struct {
	conn *sql.DB
}

// New creates a new database connection and runs migrations
func New(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Configure connection pool
	conn.SetMaxOpenConns(25)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(time.Hour)

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
			conn.Close()
			return nil, fmt.Errorf("set pragma %s: %w", pragma, err)
		}
	}

	db := &DB{conn: conn}

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
		var version int
		err = db.conn.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 1`).Scan(&version)
		if err != nil {
			return fmt.Errorf("check migration version: %w", err)
		}
		if version > 0 {
			// Migration already applied
			return nil
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

	return nil
}

// Project represents a project
type Project struct {
	ID         int64       `json:"id"`
	Name       string      `json:"name"`
	BaseCommit string      `json:"base_commit"`
	CreatedAt  SQLiteTime  `json:"created_at"`
	Status     string      `json:"status"`
	Priority   int         `json:"priority"`
}

// Step represents a work item
type Step struct {
	ID            int64           `json:"id"`
	ProjectID     int64           `json:"project_id"`
	StepNum       int             `json:"step_num"`
	Branch        string          `json:"branch"`
	Scope         string          `json:"scope"`
	Status        string          `json:"status"`
	Worktree      *string         `json:"worktree,omitempty"`
	AgentID       *string         `json:"agent_id,omitempty"`
	ClaimedAt     NullSQLiteTime  `json:"claimed_at,omitempty"`
	StartedAt     NullSQLiteTime  `json:"started_at,omitempty"`
	CompletedAt   NullSQLiteTime  `json:"completed_at,omitempty"`
	LastHeartbeat NullSQLiteTime  `json:"last_heartbeat,omitempty"`
	LastCommit    *string         `json:"last_commit,omitempty"`
	FilesModified *string         `json:"files_modified,omitempty"`
	Notes         *string         `json:"notes,omitempty"`
}

// CreateProject creates a new project with steps
func (db *DB) CreateProject(name, baseCommit string, steps []StepInput) (*Project, error) {
	tx, err := db.conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert project
	var projectID int64
	err = tx.QueryRow(`
		INSERT INTO projects (name, base_commit, status)
		VALUES (?, ?, 'active')
		RETURNING id
	`, name, baseCommit).Scan(&projectID)
	if err != nil {
		return nil, fmt.Errorf("insert project: %w", err)
	}

	// Insert steps
	for _, step := range steps {
		var stepID int64
		err = tx.QueryRow(`
			INSERT INTO steps (project_id, step_num, branch, scope, status)
			VALUES (?, ?, ?, ?, 'not_started')
			RETURNING id
		`, projectID, step.StepNum, step.Branch, step.Scope).Scan(&stepID)
		if err != nil {
			return nil, fmt.Errorf("insert step %d: %w", step.StepNum, err)
		}

		// Insert dependencies
		for _, depStepNum := range step.DependsOn {
			_, err = tx.Exec(`
				INSERT INTO dependencies (step_id, depends_on_step_id)
				SELECT ?, id FROM steps
				WHERE project_id = ? AND step_num = ?
			`, stepID, projectID, depStepNum)
			if err != nil {
				return nil, fmt.Errorf("insert dependency %d->%d: %w", step.StepNum, depStepNum, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	// Return created project
	return db.GetProject(name)
}

// GetProject gets a project by name
func (db *DB) GetProject(name string) (*Project, error) {
	var p Project
	err := db.conn.QueryRow(`
		SELECT id, name, base_commit, created_at, status, priority
		FROM projects
		WHERE name = ?
	`, name).Scan(&p.ID, &p.Name, &p.BaseCommit, &p.CreatedAt, &p.Status, &p.Priority)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ClaimStep atomically claims the next available step
func (db *DB) ClaimStep(projectName, agentID string) (*Step, error) {
	tx, err := db.conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	var step Step
	err = tx.QueryRow(`
		UPDATE steps
		SET status = 'claimed',
		    agent_id = ?,
		    claimed_at = datetime('now'),
		    last_heartbeat = datetime('now')
		WHERE id = (
			SELECT s.id FROM steps s
			WHERE s.project_id = (SELECT id FROM projects WHERE name = ?)
			AND s.status = 'not_started'
			AND NOT EXISTS (
				SELECT 1 FROM dependencies d
				JOIN steps ds ON d.depends_on_step_id = ds.id
				WHERE d.step_id = s.id
				AND ds.status NOT IN ('completed', 'merged')
			)
			ORDER BY s.step_num ASC
			LIMIT 1
		)
		RETURNING id, project_id, step_num, branch, scope, status, agent_id, claimed_at, last_heartbeat
	`, agentID, projectName).Scan(
		&step.ID, &step.ProjectID, &step.StepNum, &step.Branch,
		&step.Scope, &step.Status, &step.AgentID, &step.ClaimedAt, &step.LastHeartbeat,
	)
	if err == sql.ErrNoRows {
		return nil, nil // No work available
	}
	if err != nil {
		return nil, fmt.Errorf("claim step: %w", err)
	}

	// Record event
	_, err = tx.Exec(`
		INSERT INTO agent_events (agent_id, project_id, step_id, event_type)
		VALUES (?, ?, ?, 'claimed')
	`, agentID, step.ProjectID, step.ID)
	if err != nil {
		return nil, fmt.Errorf("record event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &step, nil
}

// Heartbeat updates step heartbeat
func (db *DB) Heartbeat(stepID int64, agentID string) error {
	result, err := db.conn.Exec(`
		UPDATE steps
		SET last_heartbeat = datetime('now')
		WHERE id = ? AND agent_id = ? AND status IN ('claimed', 'in_progress')
	`, stepID, agentID)
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

	// Record event (silently fail if error)
	db.conn.Exec(`
		INSERT INTO agent_events (agent_id, step_id, event_type)
		VALUES (?, ?, 'heartbeat')
	`, agentID, stepID)

	return nil
}

// StepInput is input for creating a step
type StepInput struct {
	StepNum   int    `json:"step_num"`
	Branch    string `json:"branch"`
	Scope     string `json:"scope"`
	DependsOn []int  `json:"depends_on"`
}

// StartStep marks a claimed step as in progress
func (db *DB) StartStep(stepID int64, worktree *string) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
		UPDATE steps
		SET status = 'in_progress',
		    started_at = datetime('now'),
		    last_heartbeat = datetime('now'),
		    worktree = ?
		WHERE id = ? AND status = 'claimed'
	`, worktree, stepID)
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
	_, err = tx.Exec(`
		INSERT INTO agent_events (agent_id, step_id, event_type)
		SELECT agent_id, id, 'started' FROM steps WHERE id = ?
	`, stepID)
	if err != nil {
		return fmt.Errorf("record event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

// CompleteStep marks a step as completed
func (db *DB) CompleteStep(stepID int64, commitHash string, filesModified []string, notes *string) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Convert files to JSON
	var filesJSON *string
	if len(filesModified) > 0 {
		bytes, err := json.Marshal(filesModified)
		if err != nil {
			return fmt.Errorf("marshal files: %w", err)
		}
		str := string(bytes)
		filesJSON = &str
	}

	result, err := tx.Exec(`
		UPDATE steps
		SET status = 'completed',
		    completed_at = datetime('now'),
		    last_commit = ?,
		    files_modified = ?,
		    notes = ?
		WHERE id = ? AND status = 'in_progress'
	`, commitHash, filesJSON, notes, stepID)
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
	_, err = tx.Exec(`
		INSERT INTO agent_events (agent_id, step_id, event_type)
		SELECT agent_id, id, 'completed' FROM steps WHERE id = ?
	`, stepID)
	if err != nil {
		return fmt.Errorf("record event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

// FailStep marks a step as failed
func (db *DB) FailStep(stepID int64, reason string) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
		UPDATE steps
		SET status = 'failed',
		    notes = ?
		WHERE id = ? AND status IN ('claimed', 'in_progress')
	`, reason, stepID)
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
	metadata := fmt.Sprintf(`{"reason": "%s"}`, reason)
	_, err = tx.Exec(`
		INSERT INTO agent_events (agent_id, step_id, event_type, metadata)
		SELECT agent_id, id, 'failed', ? FROM steps WHERE id = ?
	`, metadata, stepID)
	if err != nil {
		return fmt.Errorf("record event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

// GetStep gets a step by ID
func (db *DB) GetStep(stepID int64) (*Step, error) {
	var s Step
	err := db.conn.QueryRow(`
		SELECT id, project_id, step_num, branch, scope, status,
		       worktree, agent_id, claimed_at, started_at, completed_at,
		       last_heartbeat, last_commit, files_modified, notes
		FROM steps
		WHERE id = ?
	`, stepID).Scan(
		&s.ID, &s.ProjectID, &s.StepNum, &s.Branch, &s.Scope, &s.Status,
		&s.Worktree, &s.AgentID, &s.ClaimedAt, &s.StartedAt, &s.CompletedAt,
		&s.LastHeartbeat, &s.LastCommit, &s.FilesModified, &s.Notes,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ListProjects lists projects with optional status filter
func (db *DB) ListProjects(status *string) ([]Project, error) {
	query := `
		SELECT id, name, base_commit, created_at, status, priority
		FROM projects
	`
	args := []any{}

	if status != nil {
		query += " WHERE status = ?"
		args = append(args, *status)
	}

	query += " ORDER BY priority DESC, created_at DESC"

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query projects: %w", err)
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		err := rows.Scan(&p.ID, &p.Name, &p.BaseCommit, &p.CreatedAt, &p.Status, &p.Priority)
		if err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return projects, nil
}

// GetAvailableSteps gets all steps available to work on
func (db *DB) GetAvailableSteps(projectName *string, scope *string) ([]Step, error) {
	query := `
		SELECT s.id, s.project_id, s.step_num, s.branch, s.scope, s.status,
		       s.worktree, s.agent_id, s.claimed_at, s.started_at, s.completed_at,
		       s.last_heartbeat, s.last_commit, s.files_modified, s.notes
		FROM steps s
		JOIN projects p ON s.project_id = p.id
		WHERE s.status = 'not_started'
		AND p.status = 'active'
		AND NOT EXISTS (
			SELECT 1 FROM dependencies d
			JOIN steps ds ON d.depends_on_step_id = ds.id
			WHERE d.step_id = s.id
			AND ds.status NOT IN ('completed', 'merged')
		)
	`
	args := []any{}

	if projectName != nil {
		query += " AND p.name = ?"
		args = append(args, *projectName)
	}

	if scope != nil {
		query += " AND s.scope = ?"
		args = append(args, *scope)
	}

	query += " ORDER BY p.priority DESC, s.step_num ASC"

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query steps: %w", err)
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

// DetectStaleWork finds steps with stale heartbeats
func (db *DB) DetectStaleWork(timeoutMinutes int) ([]Step, error) {
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

	rows, err := db.conn.Query(query)
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
func (db *DB) RecoverStep(stepID int64) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
		UPDATE steps
		SET status = 'not_started',
		    agent_id = NULL,
		    claimed_at = NULL,
		    started_at = NULL,
		    last_heartbeat = NULL,
		    worktree = NULL
		WHERE id = ? AND status IN ('claimed', 'in_progress')
	`, stepID)
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
	_, err = tx.Exec(`
		INSERT INTO agent_events (agent_id, step_id, event_type)
		VALUES ('system', ?, 'recovered')
	`, stepID)
	if err != nil {
		return fmt.Errorf("record event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

// RecoverFailedStep archives a failed step to history and resets it to not_started
func (db *DB) RecoverFailedStep(stepID int64) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get the failed step
	var step Step
	err = tx.QueryRow(`
		SELECT id, project_id, step_num, branch, scope,
		       worktree, agent_id, claimed_at, started_at,
		       last_commit, files_modified, notes
		FROM steps
		WHERE id = ? AND status = 'failed'
	`, stepID).Scan(
		&step.ID, &step.ProjectID, &step.StepNum, &step.Branch, &step.Scope,
		&step.Worktree, &step.AgentID, &step.ClaimedAt, &step.StartedAt,
		&step.LastCommit, &step.FilesModified, &step.Notes,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("step not found or not in failed state")
		}
		return fmt.Errorf("get failed step: %w", err)
	}

	// Archive the failed step to history
	_, err = tx.Exec(`
		INSERT INTO failed_steps_history
		(original_step_id, project_id, step_num, branch, scope,
		 worktree, agent_id, claimed_at, started_at, failed_at,
		 last_commit, files_modified, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), ?, ?, ?)
	`, step.ID, step.ProjectID, step.StepNum, step.Branch, step.Scope,
		step.Worktree, step.AgentID, step.ClaimedAt, step.StartedAt,
		step.LastCommit, step.FilesModified, step.Notes)
	if err != nil {
		return fmt.Errorf("archive failed step: %w", err)
	}

	// Reset the step to not_started
	result, err := tx.Exec(`
		UPDATE steps
		SET status = 'not_started',
		    agent_id = NULL,
		    claimed_at = NULL,
		    started_at = NULL,
		    completed_at = NULL,
		    last_heartbeat = NULL,
		    last_commit = NULL,
		    files_modified = NULL,
		    worktree = NULL,
		    notes = NULL
		WHERE id = ?
	`, stepID)
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
	_, err = tx.Exec(`
		INSERT INTO agent_events (agent_id, project_id, step_id, event_type, metadata)
		VALUES ('system', ?, ?, 'recovered', '{"from_status":"failed"}')
	`, step.ProjectID, stepID)
	if err != nil {
		return fmt.Errorf("record event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

// Metrics holds metrics data
type Metrics struct {
	TotalSteps      int                `json:"total_steps"`
	CompletedSteps  int                `json:"completed_steps"`
	FailedSteps     int                `json:"failed_steps"`
	InProgressSteps int                `json:"in_progress_steps"`
	AvgTimeHours    float64            `json:"avg_time_hours,omitempty"`
	ScopeBreakdown  map[string]int     `json:"scope_breakdown,omitempty"`
}

// GetMetrics calculates project and agent metrics
func (db *DB) GetMetrics(projectName *string, agentID *string) (*Metrics, error) {
	query := `
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN s.status IN ('completed', 'merged') THEN 1 ELSE 0 END) as completed,
			SUM(CASE WHEN s.status = 'failed' THEN 1 ELSE 0 END) as failed,
			SUM(CASE WHEN s.status IN ('claimed', 'in_progress') THEN 1 ELSE 0 END) as in_progress,
			AVG(CASE
				WHEN s.status IN ('completed', 'merged') AND s.started_at IS NOT NULL AND s.completed_at IS NOT NULL
				THEN (julianday(s.completed_at) - julianday(s.started_at)) * 24
				ELSE NULL
			END) as avg_hours
		FROM steps s
		JOIN projects p ON s.project_id = p.id
		WHERE 1=1
	`
	args := []any{}

	if projectName != nil {
		query += " AND p.name = ?"
		args = append(args, *projectName)
	}

	if agentID != nil {
		query += " AND s.agent_id = ?"
		args = append(args, *agentID)
	}

	var m Metrics
	var avgHours sql.NullFloat64
	err := db.conn.QueryRow(query, args...).Scan(
		&m.TotalSteps, &m.CompletedSteps, &m.FailedSteps, &m.InProgressSteps, &avgHours,
	)
	if err != nil {
		return nil, fmt.Errorf("query metrics: %w", err)
	}

	if avgHours.Valid {
		m.AvgTimeHours = avgHours.Float64
	}

	// Get scope breakdown
	scopeQuery := `
		SELECT scope, COUNT(*) as count
		FROM steps s
		JOIN projects p ON s.project_id = p.id
		WHERE 1=1
	`
	scopeArgs := []any{}

	if projectName != nil {
		scopeQuery += " AND p.name = ?"
		scopeArgs = append(scopeArgs, *projectName)
	}

	if agentID != nil {
		scopeQuery += " AND s.agent_id = ?"
		scopeArgs = append(scopeArgs, *agentID)
	}

	scopeQuery += " GROUP BY scope"

	rows, err := db.conn.Query(scopeQuery, scopeArgs...)
	if err != nil {
		return nil, fmt.Errorf("query scope breakdown: %w", err)
	}
	defer rows.Close()

	m.ScopeBreakdown = make(map[string]int)
	for rows.Next() {
		var scope string
		var count int
		if err := rows.Scan(&scope, &count); err != nil {
			return nil, fmt.Errorf("scan scope: %w", err)
		}
		m.ScopeBreakdown[scope] = count
	}

	return &m, nil
}

// AgentEvent represents an event in the audit trail
type AgentEvent struct {
	ID        int64      `json:"id"`
	AgentID   string     `json:"agent_id"`
	ProjectID *int64     `json:"project_id,omitempty"`
	StepID    *int64     `json:"step_id,omitempty"`
	EventType string     `json:"event_type"`
	Timestamp SQLiteTime `json:"timestamp"`
	Metadata  *string    `json:"metadata,omitempty"`
}

// GetAgentEvents gets agent activity log
func (db *DB) GetAgentEvents(agentID *string, projectName *string, limit int) ([]AgentEvent, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT ae.id, ae.agent_id, ae.project_id, ae.step_id,
		       ae.event_type, ae.timestamp, ae.metadata
		FROM agent_events ae
		LEFT JOIN projects p ON ae.project_id = p.id
		WHERE 1=1
	`
	args := []any{}

	if agentID != nil {
		query += " AND ae.agent_id = ?"
		args = append(args, *agentID)
	}

	if projectName != nil {
		query += " AND p.name = ?"
		args = append(args, *projectName)
	}

	query += " ORDER BY ae.timestamp DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []AgentEvent
	for rows.Next() {
		var e AgentEvent
		err := rows.Scan(&e.ID, &e.AgentID, &e.ProjectID, &e.StepID,
			&e.EventType, &e.Timestamp, &e.Metadata)
		if err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return events, nil
}
