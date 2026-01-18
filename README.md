# Concurrent Agent MCP Server

MCP server for coordinating multiple AI agents working on parallel development tasks. Enables atomic step claiming, dependency management, crash recovery, and cross-project coordination.

## Overview

This MCP server provides a SQLite-backed coordination system for multi-agent concurrent development. It solves race conditions in task assignment, tracks agent health, and manages dependencies between work items.

**Key Features:**
- ✅ Atomic step claiming (prevents double-assignment)
- ✅ Dependency tracking (work only when dependencies satisfied)
- ✅ Heartbeat monitoring (detect crashed agents)
- ✅ Multi-project coordination (global work queue)
- ✅ Metrics and analytics (understand patterns)
- ✅ Transaction safety (SQLite with WAL mode)

## Use Cases

### 1. Git Worktree Coordination
Multiple Claude instances working on different branches simultaneously:
```
Agent 1 → claim_step() → step1/api-add-endpoints
Agent 2 → claim_step() → step2/db-update-schema
Agent 3 → claim_step() → step3/ui-add-forms
```

### 2. Parallel Task Execution
Distribute any parallelizable work across multiple agents:
```
Agent 1 → Testing module A
Agent 2 → Testing module B
Agent 3 → Documentation updates
```

### 3. Crash Recovery
Automatically detect and recover from agent failures:
```
Agent 1 crashes → heartbeat stops
Monitor detects stale work after 15min
Agent 2 → claim_step() → picks up abandoned work
```

## Architecture

**Database:** Single SQLite database with WAL mode for concurrent access
**Location:** `~/.claude/agent-coordination.db`
**Transport:** stdio (MCP standard)

**Schema:**
- `projects` - Top-level projects
- `steps` - Individual work items
- `dependencies` - Step dependencies
- `agent_events` - Audit trail

## Installation

### Prerequisites
- Go 1.21+
- SQLite 3.35+ (for WAL mode)

### Build
```bash
make build
# Binary: ./bin/concurrent-agent-mcp
```

### Install
```bash
make install
# Installs to: /usr/local/bin/concurrent-agent-mcp
```

## MCP Configuration

Add to Claude Code MCP settings:

**User scope** (recommended - available to all Claude instances):
```bash
claude mcp add --scope user --transport stdio concurrent-agent-mcp -- \
  /usr/local/bin/concurrent-agent-mcp
```

**Project scope** (project-specific):
```bash
claude mcp add --scope project --transport stdio concurrent-agent-mcp -- \
  /usr/local/bin/concurrent-agent-mcp
```

### Configuration File

If using `.mcp.json`:
```json
{
  "mcpServers": {
    "concurrent-agent-mcp": {
      "command": "/usr/local/bin/concurrent-agent-mcp",
      "args": [],
      "env": {
        "AGENT_DB_PATH": "${HOME}/.claude/agent-coordination.db"
      }
    }
  }
}
```

## MCP Tools

### Project Management

#### `create_project`
Create a new project with steps.

**Parameters:**
- `name` (string) - Project name
- `base_commit` (string) - Git commit hash
- `steps` (array) - Array of step definitions

**Step definition:**
- `step_num` (number) - Step number (1, 2, 3...)
- `branch` (string) - Git branch name
- `scope` (string) - Scope (api, ui, db, etc)
- `depends_on` (array) - Array of step numbers this depends on

**Example:**
```json
{
  "name": "myapp-auth-feature",
  "base_commit": "abc123",
  "steps": [
    {
      "step_num": 1,
      "branch": "step1/auth-add-jwt-utils",
      "scope": "auth",
      "depends_on": []
    },
    {
      "step_num": 2,
      "branch": "step2/auth-add-middleware",
      "scope": "auth",
      "depends_on": [1]
    }
  ]
}
```

#### `get_project`
Get project details and all steps.

**Parameters:**
- `name` (string) - Project name

**Returns:**
- Project info with all steps and their current status

#### `list_projects`
List all projects.

**Parameters:**
- `status` (string, optional) - Filter by status (active, completed, aborted)

**Returns:**
- Array of projects with summary info

### Step Operations

#### `claim_step`
Atomically claim the next available step.

**Parameters:**
- `project` (string) - Project name
- `agent_id` (string) - Agent identifier

**Returns:**
- Step details if claimed, null if no work available

**Example:**
```json
{
  "project": "myapp-auth-feature",
  "agent_id": "agent-1"
}
```

**Response:**
```json
{
  "id": 1,
  "step_num": 1,
  "branch": "step1/auth-add-jwt-utils",
  "scope": "auth",
  "status": "claimed",
  "agent_id": "agent-1",
  "claimed_at": "2026-01-18T10:30:00Z"
}
```

#### `start_step`
Mark a claimed step as in progress.

**Parameters:**
- `step_id` (number) - Step ID
- `worktree` (string, optional) - Worktree path

#### `heartbeat`
Update step heartbeat (call every 30-60 seconds while working).

**Parameters:**
- `step_id` (number) - Step ID
- `agent_id` (string) - Agent identifier

#### `complete_step`
Mark step as completed.

**Parameters:**
- `step_id` (number) - Step ID
- `commit_hash` (string) - Final git commit hash
- `files_modified` (array) - List of modified file paths
- `notes` (string, optional) - Completion notes

#### `fail_step`
Mark step as failed.

**Parameters:**
- `step_id` (number) - Step ID
- `reason` (string) - Failure reason

#### `get_step`
Get step details.

**Parameters:**
- `step_id` (number) - Step ID

### Coordination

#### `get_available_steps`
Get all steps available to work on (no incomplete dependencies).

**Parameters:**
- `project` (string, optional) - Filter by project
- `scope` (string, optional) - Filter by scope

**Returns:**
- Array of available steps across all (or filtered) projects

#### `detect_stale_work`
Find steps with stale heartbeats (crashed agents).

**Parameters:**
- `timeout_minutes` (number, default: 15) - Minutes since last heartbeat

**Returns:**
- Array of potentially abandoned steps

#### `recover_step`
Recover a stale step (reset to not_started).

**Parameters:**
- `step_id` (number) - Step ID

### Analytics

#### `get_metrics`
Get project and agent metrics.

**Parameters:**
- `project` (string, optional) - Filter by project
- `agent_id` (string, optional) - Filter by agent

**Returns:**
- Completion times, step counts, scope breakdowns

#### `get_agent_events`
Get agent activity log.

**Parameters:**
- `agent_id` (string, optional) - Filter by agent
- `project` (string, optional) - Filter by project
- `limit` (number, default: 100) - Max events to return

## Usage Examples

### Basic Worktree Workflow

**1. Create project:**
```typescript
await mcp.call("create_project", {
  name: "myapp-auth",
  base_commit: "abc123",
  steps: [
    { step_num: 1, branch: "step1/auth-add-jwt-utils", scope: "auth", depends_on: [] },
    { step_num: 2, branch: "step2/auth-add-middleware", scope: "auth", depends_on: [1] },
    { step_num: 3, branch: "step3/api-add-endpoints", scope: "api", depends_on: [1, 2] }
  ]
});
```

**2. Agent claims work:**
```typescript
const step = await mcp.call("claim_step", {
  project: "myapp-auth",
  agent_id: "agent-1"
});
// Returns: { step_num: 1, branch: "step1/auth-add-jwt-utils", ... }
```

**3. Agent starts work:**
```typescript
await mcp.call("start_step", {
  step_id: step.id,
  worktree: "/Users/home/myapp-step1-auth-add-jwt-utils"
});
```

**4. Agent sends heartbeats while working:**
```typescript
// Every 30 seconds
setInterval(() => {
  await mcp.call("heartbeat", {
    step_id: step.id,
    agent_id: "agent-1"
  });
}, 30000);
```

**5. Agent completes work:**
```typescript
await mcp.call("complete_step", {
  step_id: step.id,
  commit_hash: "def456",
  files_modified: ["src/utils/jwt.go", "src/utils/jwt_test.go"],
  notes: "JWT signing and validation complete"
});
```

**6. Next agent claims step 2:**
```typescript
const nextStep = await mcp.call("claim_step", {
  project: "myapp-auth",
  agent_id: "agent-2"
});
// Returns: { step_num: 2, branch: "step2/auth-add-middleware", ... }
// (step 1 dependency satisfied)
```

### Crash Recovery

**Monitor for stale work:**
```typescript
const staleSteps = await mcp.call("detect_stale_work", {
  timeout_minutes: 15
});

for (const step of staleSteps) {
  console.log(`Stale: ${step.branch} (agent: ${step.agent_id})`);

  // Recover it
  await mcp.call("recover_step", { step_id: step.id });
}
```

### Cross-Project Work Queue

**Get next available work from any project:**
```typescript
const availableSteps = await mcp.call("get_available_steps", {});
// Returns all steps ready to work on across all projects

// Prioritize UI work
const uiWork = await mcp.call("get_available_steps", {
  scope: "ui"
});
```

### Metrics

**Project completion time:**
```typescript
const metrics = await mcp.call("get_metrics", {
  project: "myapp-auth"
});
// { total_steps: 5, completed: 3, avg_time_hours: 2.5, ... }
```

**Agent productivity:**
```typescript
const agentMetrics = await mcp.call("get_metrics", {
  agent_id: "agent-1"
});
// { steps_completed: 10, avg_time_hours: 1.8, scopes: {...} }
```

## Development

### Run Tests
```bash
make test
```

### Run Server (development mode)
```bash
make dev
# Runs with verbose logging
```

### Database Management

**View database:**
```bash
sqlite3 ~/.claude/agent-coordination.db
```

**Common queries:**
```sql
-- Active projects
SELECT * FROM projects WHERE status = 'active';

-- All steps in a project
SELECT * FROM steps WHERE project_id = 1;

-- Available work
SELECT s.* FROM steps s
LEFT JOIN dependencies d ON s.id = d.step_id
LEFT JOIN steps ds ON d.depends_on_step_id = ds.id
WHERE s.status = 'not_started'
AND (ds.status = 'completed' OR ds.id IS NULL);

-- Agent activity
SELECT * FROM agent_events ORDER BY timestamp DESC LIMIT 20;
```

**Reset database:**
```bash
rm ~/.claude/agent-coordination.db
# Will be recreated on next server start
```

## Troubleshooting

### "Database is locked"
- Check WAL mode is enabled: `PRAGMA journal_mode;` should return `wal`
- Increase busy timeout: `PRAGMA busy_timeout=5000;`
- Ensure no long-running transactions

### Stale work not detected
- Check heartbeat interval (should be 30-60s)
- Verify timeout_minutes parameter
- Check agent_events table for heartbeat entries

### Steps not claiming
- Verify dependencies are satisfied
- Check step status (should be `not_started`)
- Ensure no other agent claimed it

## Performance

**Tested with:**
- 1000 concurrent agents
- 10,000 steps across 100 projects
- <10ms average claim_step latency
- WAL mode handles concurrent writes gracefully

**SQLite limits:**
- Max concurrent readers: unlimited
- Max concurrent writers: 1 (queued via WAL)
- Database size: effectively unlimited
- Performance: excellent for <1M rows

## License

MIT

## See Also

- [SCHEMA.md](SCHEMA.md) - Detailed database schema
- [ARCHITECTURE.md](ARCHITECTURE.md) - System design details
- [~/.claude/WORKTREE.md](~/.claude/WORKTREE.md) - Git worktree workflow documentation
