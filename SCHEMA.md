# Database Schema

SQLite database schema for concurrent agent coordination.

## Overview

**Database location:** `~/.claude/agent-coordination.db`
**Journal mode:** WAL (Write-Ahead Logging)
**Foreign keys:** Enabled
**Concurrent access:** Multiple readers + single writer via WAL

## Tables

### projects

Top-level projects containing multiple steps.

**Columns:**
- `id` INTEGER PRIMARY KEY - Auto-increment project ID
- `name` TEXT UNIQUE NOT NULL - Project name (e.g., "myapp-auth-feature")
- `base_commit` TEXT - Git commit hash all worktrees are based on
- `created_at` TEXT NOT NULL - ISO 8601 timestamp
- `status` TEXT NOT NULL - Project status: `active`, `completed`, `aborted`
- `priority` INTEGER DEFAULT 0 - Priority for work assignment (higher = more urgent)

**Indexes:**
- `idx_projects_status` - For filtering by status
- `idx_projects_priority` - For priority-based work assignment

**Example:**
```sql
INSERT INTO projects (name, base_commit, created_at, status, priority)
VALUES ('myapp-auth-feature', 'abc123def456', '2026-01-18T10:00:00Z', 'active', 5);
```

### steps

Individual work items within a project.

**Columns:**
- `id` INTEGER PRIMARY KEY - Auto-increment step ID
- `project_id` INTEGER NOT NULL - FK to projects(id)
- `step_num` INTEGER NOT NULL - Step number (1, 2, 3...)
- `branch` TEXT NOT NULL - Git branch name (e.g., "step1/auth-add-jwt-utils")
- `scope` TEXT NOT NULL - Scope category (api, ui, db, auth, tests, docs, etc)
- `status` TEXT NOT NULL - Step status (see below)
- `worktree` TEXT - Worktree directory path
- `agent_id` TEXT - Agent currently working on this step
- `claimed_at` TEXT - When step was claimed
- `started_at` TEXT - When work actually started
- `completed_at` TEXT - When work finished
- `last_heartbeat` TEXT - Last heartbeat timestamp
- `last_commit` TEXT - Final git commit hash
- `files_modified` TEXT - JSON array of modified file paths
- `notes` TEXT - Free-form notes

**Constraints:**
- `UNIQUE(project_id, step_num)` - Step numbers unique within project
- `FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE`

**Status values:**
- `not_started` - Initial state, ready to be claimed
- `claimed` - Claimed by an agent but work not started
- `in_progress` - Work actively happening
- `completed` - Work finished successfully
- `failed` - Work failed (can be recovered)
- `merged` - Merged to main branch (terminal state)

**Indexes:**
- `idx_steps_project_id` - For project queries
- `idx_steps_status` - For status filtering
- `idx_steps_agent_id` - For agent queries
- `idx_steps_scope` - For scope-based work assignment
- `idx_steps_last_heartbeat` - For stale work detection

**State transitions:**
```
not_started → claimed → in_progress → completed → merged
                          ↓
                       failed → not_started (recovered)
```

**Example:**
```sql
INSERT INTO steps (project_id, step_num, branch, scope, status)
VALUES (1, 1, 'step1/auth-add-jwt-utils', 'auth', 'not_started');
```

### dependencies

Defines dependencies between steps (step A must complete before step B can start).

**Columns:**
- `step_id` INTEGER NOT NULL - Step that has the dependency
- `depends_on_step_id` INTEGER NOT NULL - Step that must complete first

**Constraints:**
- `PRIMARY KEY(step_id, depends_on_step_id)`
- `FOREIGN KEY(step_id) REFERENCES steps(id) ON DELETE CASCADE`
- `FOREIGN KEY(depends_on_step_id) REFERENCES steps(id) ON DELETE CASCADE`

**Indexes:**
- `idx_dependencies_step_id` - For checking if step can start
- `idx_dependencies_depends_on` - For finding blocked steps

**Example:**
```sql
-- Step 2 depends on step 1
INSERT INTO dependencies (step_id, depends_on_step_id)
VALUES (2, 1);

-- Step 3 depends on both step 1 and step 2
INSERT INTO dependencies (step_id, depends_on_step_id)
VALUES (3, 1), (3, 2);
```

**Checking if step can start:**
```sql
-- Step is ready if it has no incomplete dependencies
SELECT COUNT(*) = 0 as ready
FROM dependencies d
JOIN steps s ON d.depends_on_step_id = s.id
WHERE d.step_id = ?
AND s.status NOT IN ('completed', 'merged');
```

### agent_events

Audit trail of all agent actions.

**Columns:**
- `id` INTEGER PRIMARY KEY - Auto-increment event ID
- `agent_id` TEXT NOT NULL - Agent identifier
- `project_id` INTEGER - FK to projects(id), nullable
- `step_id` INTEGER - FK to steps(id), nullable
- `event_type` TEXT NOT NULL - Event type (see below)
- `timestamp` TEXT NOT NULL - ISO 8601 timestamp
- `metadata` TEXT - JSON metadata (optional)

**Event types:**
- `claimed` - Agent claimed a step
- `started` - Agent started work on a step
- `heartbeat` - Agent sent heartbeat
- `completed` - Agent completed a step
- `failed` - Agent marked step as failed
- `recovered` - Step recovered from stale state

**Constraints:**
- `FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE SET NULL`
- `FOREIGN KEY(step_id) REFERENCES steps(id) ON DELETE SET NULL`

**Indexes:**
- `idx_agent_events_timestamp` - For recent activity queries
- `idx_agent_events_agent_id` - For agent-specific queries
- `idx_agent_events_step_id` - For step history

**Example:**
```sql
INSERT INTO agent_events (agent_id, project_id, step_id, event_type, timestamp, metadata)
VALUES ('agent-1', 1, 1, 'claimed', '2026-01-18T10:30:00Z', '{"worktree": "/path/to/worktree"}');
```

## Common Queries

### Find next available step (respecting dependencies)

```sql
SELECT s.*
FROM steps s
WHERE s.project_id = ?
AND s.status = 'not_started'
AND NOT EXISTS (
  SELECT 1
  FROM dependencies d
  JOIN steps ds ON d.depends_on_step_id = ds.id
  WHERE d.step_id = s.id
  AND ds.status NOT IN ('completed', 'merged')
)
ORDER BY s.step_num ASC
LIMIT 1;
```

### Atomic claim operation

```sql
UPDATE steps
SET status = 'claimed',
    agent_id = ?,
    claimed_at = datetime('now')
WHERE id = (
  SELECT id FROM steps
  WHERE project_id = (SELECT id FROM projects WHERE name = ?)
  AND status = 'not_started'
  AND NOT EXISTS (
    SELECT 1 FROM dependencies d
    JOIN steps ds ON d.depends_on_step_id = ds.id
    WHERE d.step_id = steps.id
    AND ds.status NOT IN ('completed', 'merged')
  )
  ORDER BY step_num ASC
  LIMIT 1
)
RETURNING *;
```

### Find stale work (crashed agents)

```sql
SELECT s.*
FROM steps s
WHERE s.status IN ('claimed', 'in_progress')
AND s.last_heartbeat < datetime('now', '-15 minutes');
```

### Project completion percentage

```sql
SELECT
  p.name,
  COUNT(*) as total_steps,
  SUM(CASE WHEN s.status IN ('completed', 'merged') THEN 1 ELSE 0 END) as completed_steps,
  ROUND(100.0 * SUM(CASE WHEN s.status IN ('completed', 'merged') THEN 1 ELSE 0 END) / COUNT(*), 1) as completion_pct
FROM projects p
JOIN steps s ON p.id = s.project_id
WHERE p.id = ?
GROUP BY p.id, p.name;
```

### Agent productivity metrics

```sql
SELECT
  agent_id,
  COUNT(*) as steps_completed,
  AVG(julianday(completed_at) - julianday(started_at)) * 24 as avg_hours,
  MIN(julianday(completed_at) - julianday(started_at)) * 24 as min_hours,
  MAX(julianday(completed_at) - julianday(started_at)) * 24 as max_hours
FROM steps
WHERE agent_id IS NOT NULL
AND completed_at IS NOT NULL
AND started_at IS NOT NULL
GROUP BY agent_id;
```

### Scope breakdown by project

```sql
SELECT
  p.name as project,
  s.scope,
  COUNT(*) as total,
  SUM(CASE WHEN s.status IN ('completed', 'merged') THEN 1 ELSE 0 END) as completed
FROM projects p
JOIN steps s ON p.id = s.project_id
GROUP BY p.id, p.name, s.scope
ORDER BY p.name, s.scope;
```

### Identify bottlenecks (slowest steps)

```sql
SELECT
  p.name as project,
  s.step_num,
  s.branch,
  s.scope,
  s.agent_id,
  julianday(s.completed_at) - julianday(s.started_at) * 24 as hours_taken
FROM steps s
JOIN projects p ON s.project_id = p.id
WHERE s.completed_at IS NOT NULL
AND s.started_at IS NOT NULL
ORDER BY hours_taken DESC
LIMIT 10;
```

### Cross-project work queue

```sql
SELECT
  p.name as project,
  p.priority,
  s.step_num,
  s.branch,
  s.scope
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
ORDER BY p.priority DESC, s.step_num ASC;
```

### Recent agent activity

```sql
SELECT
  ae.timestamp,
  ae.agent_id,
  ae.event_type,
  p.name as project,
  s.branch,
  ae.metadata
FROM agent_events ae
LEFT JOIN projects p ON ae.project_id = p.id
LEFT JOIN steps s ON ae.step_id = s.id
ORDER BY ae.timestamp DESC
LIMIT 50;
```

## Database Configuration

**Pragmas (set on connection):**
```sql
PRAGMA journal_mode=WAL;              -- Enable Write-Ahead Logging
PRAGMA synchronous=NORMAL;            -- Balance safety/performance
PRAGMA busy_timeout=5000;             -- Wait 5s for lock
PRAGMA foreign_keys=ON;               -- Enforce referential integrity
PRAGMA temp_store=MEMORY;             -- Faster temp tables
PRAGMA cache_size=-64000;             -- 64MB cache
```

**WAL checkpointing:**
```sql
-- Checkpoint WAL every 1000 pages or at idle
PRAGMA wal_autocheckpoint=1000;

-- Manual checkpoint
PRAGMA wal_checkpoint(TRUNCATE);
```

## Schema Migrations

Migrations are in `migrations/` directory, numbered sequentially:
- `001_initial_schema.sql` - Create all tables
- `002_add_priority.sql` - Add priority column to projects
- etc.

**Apply migration:**
```sql
-- Track applied migrations
CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL
);

-- Check current version
SELECT MAX(version) FROM schema_migrations;
```

## Data Retention

**Active projects:**
- Keep all data while project is active

**Completed projects:**
- Option 1: Keep forever (small DB footprint)
- Option 2: Export and delete after 90 days
- Option 3: Archive to separate DB file

**Agent events:**
- Optionally prune events older than 30 days
- Keep completed/failed events for metrics

**Cleanup query:**
```sql
-- Delete events older than 30 days for completed projects
DELETE FROM agent_events
WHERE timestamp < datetime('now', '-30 days')
AND project_id IN (
  SELECT id FROM projects WHERE status IN ('completed', 'aborted')
);

-- Vacuum to reclaim space
VACUUM;
```

## Backup

**Simple backup:**
```bash
sqlite3 ~/.claude/agent-coordination.db ".backup /path/to/backup.db"
```

**Export project to separate DB:**
```bash
sqlite3 ~/.claude/agent-coordination.db <<EOF
ATTACH DATABASE '/path/to/archive.db' AS archive;
CREATE TABLE archive.projects AS SELECT * FROM projects WHERE id = 1;
CREATE TABLE archive.steps AS SELECT * FROM steps WHERE project_id = 1;
CREATE TABLE archive.dependencies AS SELECT * FROM dependencies WHERE step_id IN (SELECT id FROM steps WHERE project_id = 1);
CREATE TABLE archive.agent_events AS SELECT * FROM agent_events WHERE project_id = 1;
DETACH DATABASE archive;
EOF
```

## Performance Notes

**Indexes cover common queries:**
- claim_step (project + status + dependencies)
- detect_stale_work (status + last_heartbeat)
- get_available_steps (status + dependencies)
- agent queries (agent_id)

**Expected performance (M1 Mac):**
- claim_step: <5ms
- heartbeat: <2ms
- complete_step: <5ms
- detect_stale_work: <10ms
- Complex analytics: <50ms

**Scaling limits:**
- 100 projects: excellent
- 10,000 steps: excellent
- 100,000 events: good
- 1,000,000 events: consider pruning

**WAL mode benefits:**
- Multiple agents read simultaneously
- Writers queue but don't block readers
- Crash-safe (atomic commits)
- Better performance than DELETE/ROLLBACK journal
