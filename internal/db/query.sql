-- name: CreateProject :one
INSERT INTO projects (name, base_commit, status)
VALUES (?, ?, 'active')
RETURNING id, name, base_commit, created_at, status, priority;

-- name: GetProject :one
SELECT id, name, base_commit, created_at, status, priority
FROM projects
WHERE name = ?;

-- name: GetProjectByID :one
SELECT id, name, base_commit, created_at, status, priority
FROM projects
WHERE id = ?;

-- name: ListProjectsAll :many
SELECT id, name, base_commit, created_at, status, priority
FROM projects
ORDER BY priority DESC, created_at DESC;

-- name: ListProjectsByStatus :many
SELECT id, name, base_commit, created_at, status, priority
FROM projects
WHERE status = ?
ORDER BY priority DESC, created_at DESC;

-- name: CreateStep :one
INSERT INTO steps (project_id, step_num, branch, scope, status)
VALUES (?, ?, ?, ?, 'not_started')
RETURNING id;

-- name: CreateDependency :execresult
INSERT INTO dependencies (step_id, depends_on_step_id)
SELECT ?, id FROM steps
WHERE project_id = ? AND step_num = ?;

-- name: GetStep :one
SELECT id, project_id, step_num, branch, scope, status,
       worktree, agent_id, claimed_at, started_at, completed_at,
       last_heartbeat, last_commit, files_modified, notes
FROM steps
WHERE id = ?;

-- name: GetFailedStep :one
SELECT id, project_id, step_num, branch, scope,
       worktree, agent_id, claimed_at, started_at,
       last_commit, files_modified, notes
FROM steps
WHERE id = ? AND status = 'failed';

-- name: ClaimStep :one
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
RETURNING id, project_id, step_num, branch, scope, status, agent_id, claimed_at, last_heartbeat;

-- name: UpdateHeartbeat :execresult
UPDATE steps
SET last_heartbeat = datetime('now')
WHERE id = ? AND agent_id = ? AND status IN ('claimed', 'in_progress');

-- name: StartStep :execresult
UPDATE steps
SET status = 'in_progress',
    started_at = datetime('now'),
    last_heartbeat = datetime('now'),
    worktree = ?
WHERE id = ? AND status = 'claimed';

-- name: CompleteStep :execresult
UPDATE steps
SET status = 'completed',
    completed_at = datetime('now'),
    last_commit = ?,
    files_modified = ?,
    notes = ?
WHERE id = ? AND status = 'in_progress';

-- name: FailStep :execresult
UPDATE steps
SET status = 'failed',
    notes = ?
WHERE id = ? AND status IN ('claimed', 'in_progress');

-- name: RecoverStep :execresult
UPDATE steps
SET status = 'not_started',
    agent_id = NULL,
    claimed_at = NULL,
    started_at = NULL,
    last_heartbeat = NULL,
    worktree = NULL
WHERE id = ? AND status IN ('claimed', 'in_progress');

-- name: ResetFailedStep :execresult
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
WHERE id = ? AND status = 'failed';

-- name: ArchiveFailedStep :exec
INSERT INTO failed_steps_history
(original_step_id, project_id, step_num, branch, scope,
 worktree, agent_id, claimed_at, started_at, failed_at,
 last_commit, files_modified, notes)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), ?, ?, ?);

-- name: InsertAgentEvent :exec
INSERT INTO agent_events (agent_id, project_id, step_id, event_type)
VALUES (?, ?, ?, ?);

-- name: InsertAgentEventWithMetadata :exec
INSERT INTO agent_events (agent_id, project_id, step_id, event_type, metadata)
VALUES (?, ?, ?, ?, ?);

-- name: InsertAgentEventFromStep :exec
INSERT INTO agent_events (agent_id, step_id, event_type)
SELECT s.agent_id, s.id, ? FROM steps s WHERE s.id = ?;

-- name: InsertAgentEventFromStepWithMetadata :exec
INSERT INTO agent_events (agent_id, step_id, event_type, metadata)
SELECT s.agent_id, s.id, ?, ? FROM steps s WHERE s.id = ?;

-- name: GetAvailableStepsAll :many
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
ORDER BY p.priority DESC, s.step_num ASC;

-- name: GetAvailableStepsByProject :many
SELECT s.id, s.project_id, s.step_num, s.branch, s.scope, s.status,
       s.worktree, s.agent_id, s.claimed_at, s.started_at, s.completed_at,
       s.last_heartbeat, s.last_commit, s.files_modified, s.notes
FROM steps s
JOIN projects p ON s.project_id = p.id
WHERE s.status = 'not_started'
AND p.status = 'active'
AND p.name = ?
AND NOT EXISTS (
    SELECT 1 FROM dependencies d
    JOIN steps ds ON d.depends_on_step_id = ds.id
    WHERE d.step_id = s.id
    AND ds.status NOT IN ('completed', 'merged')
)
ORDER BY p.priority DESC, s.step_num ASC;

-- name: GetAvailableStepsByScope :many
SELECT s.id, s.project_id, s.step_num, s.branch, s.scope, s.status,
       s.worktree, s.agent_id, s.claimed_at, s.started_at, s.completed_at,
       s.last_heartbeat, s.last_commit, s.files_modified, s.notes
FROM steps s
JOIN projects p ON s.project_id = p.id
WHERE s.status = 'not_started'
AND p.status = 'active'
AND s.scope = ?
AND NOT EXISTS (
    SELECT 1 FROM dependencies d
    JOIN steps ds ON d.depends_on_step_id = ds.id
    WHERE d.step_id = s.id
    AND ds.status NOT IN ('completed', 'merged')
)
ORDER BY p.priority DESC, s.step_num ASC;

-- name: GetAvailableStepsByProjectAndScope :many
SELECT s.id, s.project_id, s.step_num, s.branch, s.scope, s.status,
       s.worktree, s.agent_id, s.claimed_at, s.started_at, s.completed_at,
       s.last_heartbeat, s.last_commit, s.files_modified, s.notes
FROM steps s
JOIN projects p ON s.project_id = p.id
WHERE s.status = 'not_started'
AND p.status = 'active'
AND p.name = ?
AND s.scope = ?
AND NOT EXISTS (
    SELECT 1 FROM dependencies d
    JOIN steps ds ON d.depends_on_step_id = ds.id
    WHERE d.step_id = s.id
    AND ds.status NOT IN ('completed', 'merged')
)
ORDER BY p.priority DESC, s.step_num ASC;

-- name: GetAgentEventsAll :many
SELECT ae.id, ae.agent_id, ae.project_id, ae.step_id,
       ae.event_type, ae.timestamp, ae.metadata
FROM agent_events ae
ORDER BY ae.timestamp DESC
LIMIT ?;

-- name: GetAgentEventsByAgent :many
SELECT ae.id, ae.agent_id, ae.project_id, ae.step_id,
       ae.event_type, ae.timestamp, ae.metadata
FROM agent_events ae
WHERE ae.agent_id = ?
ORDER BY ae.timestamp DESC
LIMIT ?;

-- name: GetAgentEventsByProject :many
SELECT ae.id, ae.agent_id, ae.project_id, ae.step_id,
       ae.event_type, ae.timestamp, ae.metadata
FROM agent_events ae
LEFT JOIN projects p ON ae.project_id = p.id
WHERE p.name = ?
ORDER BY ae.timestamp DESC
LIMIT ?;

-- name: GetAgentEventsByAgentAndProject :many
SELECT ae.id, ae.agent_id, ae.project_id, ae.step_id,
       ae.event_type, ae.timestamp, ae.metadata
FROM agent_events ae
LEFT JOIN projects p ON ae.project_id = p.id
WHERE ae.agent_id = ? AND p.name = ?
ORDER BY ae.timestamp DESC
LIMIT ?;

-- name: GetMetricsAll :one
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
JOIN projects p ON s.project_id = p.id;

-- name: GetMetricsByProject :one
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
WHERE p.name = ?;

-- name: GetMetricsByAgent :one
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
WHERE s.agent_id = ?;

-- name: GetMetricsByProjectAndAgent :one
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
WHERE p.name = ? AND s.agent_id = ?;

-- name: GetScopeBreakdownAll :many
SELECT scope, COUNT(*) as count
FROM steps s
JOIN projects p ON s.project_id = p.id
GROUP BY scope;

-- name: GetScopeBreakdownByProject :many
SELECT scope, COUNT(*) as count
FROM steps s
JOIN projects p ON s.project_id = p.id
WHERE p.name = ?
GROUP BY scope;

-- name: GetScopeBreakdownByAgent :many
SELECT scope, COUNT(*) as count
FROM steps s
JOIN projects p ON s.project_id = p.id
WHERE s.agent_id = ?
GROUP BY scope;

-- name: GetScopeBreakdownByProjectAndAgent :many
SELECT scope, COUNT(*) as count
FROM steps s
JOIN projects p ON s.project_id = p.id
WHERE p.name = ? AND s.agent_id = ?
GROUP BY scope;
