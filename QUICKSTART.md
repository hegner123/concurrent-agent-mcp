# Quick Start Guide

Get the hq server running in 5 minutes.

## 1. Build

```bash
cd ~/Documents/Code/hq
make deps
make build
```

This creates `bin/hq`.

## 2. Test Locally

**Terminal 1 - Start the server:**
```bash
./bin/hq
```

The server:
- Creates database at `~/.claude/agent-coordination.db`
- Runs migrations
- Listens on stdin/stdout for MCP protocol

## 3. Install System-Wide

```bash
make install
```

This installs to `/usr/local/bin/hq`.

## 4. Configure Claude Code

Add to your MCP configuration (user scope for global access):

```bash
claude mcp add --scope user --transport stdio hq -- \
  /usr/local/bin/hq
```

Verify it's installed:
```bash
claude mcp list
```

You should see `hq` in the list.

## 5. Test from Claude

**Check MCP tools are available:**
```
/mcp
```

You should see all the hq tools listed.

**Create a test project:**
```
Use the create_project MCP tool to create a project named "test-project" with base commit "abc123" and these steps:
- step 1: branch "step1/api-add-endpoints", scope "api", no dependencies
- step 2: branch "step2/db-update-schema", scope "db", no dependencies
- step 3: branch "step3/ui-add-forms", scope "ui", depends on steps 1 and 2
```

**Claim a step:**
```
Use claim_step to claim the next available step from "test-project" for agent "agent-1"
```

You should get step 1 or 2 (both available since no dependencies).

## 6. Database Inspection

View the database directly:
```bash
sqlite3 ~/.claude/agent-coordination.db
```

**Useful queries:**
```sql
-- See all projects
SELECT * FROM projects;

-- See all steps
SELECT * FROM steps;

-- See recent events
SELECT * FROM agent_events ORDER BY timestamp DESC LIMIT 10;

-- Check WAL mode
PRAGMA journal_mode;  -- Should return 'wal'
```

## 7. Development Workflow

**Edit code:**
```bash
# Make changes to internal/db/db.go or internal/mcp/handlers.go
```

**Rebuild:**
```bash
make build
```

**Test:**
```bash
make test
```

**Clean database (start fresh):**
```bash
make db-reset
```

## 8. Example Multi-Agent Scenario

**Terminal 1 - Agent 1:**
```
Use claim_step for project "myapp" as agent "agent-1"
→ Gets step 1

Start working, send heartbeats every 30s:
Use heartbeat for step_id 1 and agent_id "agent-1"

Complete the step:
Use complete_step for step_id 1 with commit_hash "def456"
```

**Terminal 2 - Agent 2 (simultaneously):**
```
Use claim_step for project "myapp" as agent "agent-2"
→ Gets step 2 (different step, no conflict!)

Send heartbeats while working
Complete when done
```

**Both agents work in parallel without conflicts.**

## 9. Worktree Integration

**Full worktree workflow:**

1. **Create project from plan:**
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

2. **Agent claims step:**
```typescript
const step = await mcp.call("claim_step", {
  project: "myapp-auth",
  agent_id: "agent-1"
});
// { step_num: 1, branch: "step1/auth-add-jwt-utils", ... }
```

3. **Create worktree:**
```bash
git worktree add -b step1/auth-add-jwt-utils ../myapp-step1-auth-add-jwt-utils
```

4. **Start step and work:**
```typescript
await mcp.call("start_step", {
  step_id: step.id,
  worktree: "/Users/home/myapp-step1-auth-add-jwt-utils"
});

// Work in the worktree
cd ../myapp-step1-auth-add-jwt-utils
// ... make changes ...
git commit -m "Add JWT utilities"
git push

// Send heartbeats while working
setInterval(() => {
  await mcp.call("heartbeat", { step_id: step.id, agent_id: "agent-1" });
}, 30000);
```

5. **Complete step:**
```typescript
await mcp.call("complete_step", {
  step_id: step.id,
  commit_hash: "def456",
  files_modified: ["src/utils/jwt.go", "src/utils/jwt_test.go"],
  notes: "JWT signing and validation complete"
});
```

6. **Next agent claims step 2:**
```typescript
const nextStep = await mcp.call("claim_step", {
  project: "myapp-auth",
  agent_id: "agent-2"
});
// { step_num: 2, branch: "step2/auth-add-middleware", ... }
// Dependencies satisfied automatically
```

## 10. Monitoring

**Check active work:**
```bash
sqlite3 ~/.claude/agent-coordination.db "SELECT * FROM steps WHERE status='in_progress';"
```

**Find stale work:**
```typescript
const stale = await mcp.call("detect_stale_work", {
  timeout_minutes: 15
});
```

**Get metrics:**
```typescript
const metrics = await mcp.call("get_metrics", {
  project: "myapp-auth"
});
```

## 11. Troubleshooting

**Database locked:**
```bash
# Check WAL mode
sqlite3 ~/.claude/agent-coordination.db "PRAGMA journal_mode;"

# Should return 'wal'
# If not, server didn't start properly
```

**Server not responding:**
```bash
# Check if server is running
ps aux | grep hq

# Check for errors
# Server logs to stderr if there are issues
```

**Steps not claiming:**
```bash
# Check dependencies
sqlite3 ~/.claude/agent-coordination.db "
  SELECT s.step_num, s.status, ds.step_num as depends_on, ds.status as dep_status
  FROM steps s
  JOIN dependencies d ON s.id = d.step_id
  JOIN steps ds ON d.depends_on_step_id = ds.id
  WHERE s.project_id = 1;
"
```

**Reset everything:**
```bash
make clean
make build
make install
```

## 12. Next Steps

- Read [README.md](README.md) for full tool documentation
- Read [SCHEMA.md](SCHEMA.md) for database schema details
- Read [ARCHITECTURE.md](ARCHITECTURE.md) for system design
- See [~/.claude/WORKTREE.md](~/.claude/WORKTREE.md) for git worktree workflows
- Implement remaining TODOs in `internal/mcp/handlers.go`

## Common Patterns

**Pattern: Global work queue**
```typescript
// Get any available work from any project
const available = await mcp.call("get_available_steps", {});

// Prefer UI work
const uiWork = await mcp.call("get_available_steps", { scope: "ui" });

// Specific project
const projectWork = await mcp.call("get_available_steps", { project: "myapp" });
```

**Pattern: Crash recovery**
```typescript
// Detect and recover stale work
const stale = await mcp.call("detect_stale_work", { timeout_minutes: 15 });

for (const step of stale) {
  await mcp.call("recover_step", { step_id: step.id });
}
```

**Pattern: Progress tracking**
```typescript
// Project completion percentage
const project = await mcp.call("get_project", { name: "myapp" });
const metrics = await mcp.call("get_metrics", { project: "myapp" });

console.log(`${metrics.completed_steps}/${metrics.total_steps} steps complete`);
```

You're ready to go! Create projects, claim steps, and coordinate multiple agents.
