-- Schema for concurrent agent coordination

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

CREATE INDEX IF NOT EXISTS idx_steps_project_status_stepnum ON steps(project_id, status, step_num);
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
