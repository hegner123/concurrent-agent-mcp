# Architecture

System design for concurrent agent coordination via MCP.

## Overview

The concurrent-agent-mcp server provides a coordination layer for multiple AI agents working in parallel. It solves the fundamental problem of concurrent task assignment, dependency management, and crash recovery.

## Design Principles

1. **Atomic operations** - No race conditions in task claiming
2. **Crash resilience** - Detect and recover from agent failures
3. **Simple deployment** - Single binary, single database file
4. **Standards-based** - MCP protocol for interoperability
5. **Performance** - Sub-10ms operation latency
6. **Transparency** - Human-readable database, queryable state

## System Components

```
┌─────────────────────────────────────────────────────────────┐
│                       Claude Instances                       │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐   │
│  │ Agent 1  │  │ Agent 2  │  │ Agent 3  │  │ Agent N  │   │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘   │
└───────┼─────────────┼─────────────┼─────────────┼──────────┘
        │             │             │             │
        └─────────────┴─────────────┴─────────────┘
                      │
                   MCP stdio
                      │
        ┌─────────────▼──────────────┐
        │  concurrent-agent-mcp      │
        │  (Go binary)                │
        │                             │
        │  ┌────────────────────┐    │
        │  │  MCP Server        │    │
        │  │  - Tool handlers   │    │
        │  │  - Validation      │    │
        │  └────────┬───────────┘    │
        │           │                 │
        │  ┌────────▼───────────┐    │
        │  │  Database Layer    │    │
        │  │  - Queries         │    │
        │  │  - Transactions    │    │
        │  └────────┬───────────┘    │
        └───────────┼─────────────────┘
                    │
        ┌───────────▼───────────┐
        │  SQLite Database      │
        │  ~/.claude/           │
        │  agent-coordination.db│
        │  (WAL mode)           │
        └───────────────────────┘
```

## Data Flow

### Claim Step Flow

```
Agent → claim_step(project, agent_id)
         ↓
    MCP Handler validates inputs
         ↓
    Begin transaction
         ↓
    SELECT next available step (atomic)
    WHERE status = 'not_started'
    AND dependencies satisfied
         ↓
    UPDATE step SET claimed
    INSERT agent_event
         ↓
    Commit transaction
         ↓
    Return step to agent
```

**Key insight:** The `UPDATE ... WHERE id = (SELECT ...)` pattern is atomic in SQLite. Only one agent can claim a given step.

### Dependency Resolution

```
Step can start if:
  NOT EXISTS (
    Dependencies WHERE dependency.status NOT IN ('completed', 'merged')
  )
```

This ensures steps only become available when all dependencies are satisfied.

### Heartbeat Monitoring

```
Agent working → heartbeat() every 30-60s
                     ↓
              UPDATE last_heartbeat
                     ↓
              Monitor process checks
              WHERE last_heartbeat < 15min ago
                     ↓
              Stale work detected
                     ↓
              recover_step() resets to not_started
```

## Concurrency Model

**SQLite WAL mode enables:**
- Multiple concurrent readers (agents querying available work)
- Single writer at a time (queued via locks)
- Readers never block writers
- Writers never block readers
- Atomic commits via transactions

**Busy timeout:**
- Set to 5 seconds
- If write lock held, wait up to 5s
- Prevents immediate failure on contention

**Connection pool:**
- Max 25 open connections
- Handles multiple agent requests concurrently
- Each request gets own connection from pool

## State Machine

**Step lifecycle:**
```
not_started
    ↓ claim_step()
claimed
    ↓ start_step()
in_progress
    ↓ complete_step() or fail_step()
completed/failed
    ↓ (external merge)
merged
```

**Project lifecycle:**
```
active → working on steps
    ↓ all steps completed
completed → all work done
    ↓ optional
archived → exported to separate DB
```

## Failure Modes

### Agent Crashes

**Detection:**
- Heartbeats stop
- `detect_stale_work()` finds step with old heartbeat

**Recovery:**
- `recover_step()` resets step to `not_started`
- Next agent can claim it
- Original work not lost (commits still in git)

### Database Corruption

**Prevention:**
- WAL mode (crash-safe)
- PRAGMA synchronous=NORMAL (fsync on commits)
- Foreign key constraints (data integrity)

**Recovery:**
- SQLite has built-in recovery
- Backup via `.backup` command
- Export completed projects

### Network Partition (N/A)

Not applicable - local database, no network.

### Concurrent Claim Attempts

**Scenario:** Two agents try to claim same step simultaneously

**Result:**
- Both execute `UPDATE ... WHERE id = (SELECT ...)`
- SQLite serializes the UPDATEs
- First UPDATE succeeds, claims step
- Second UPDATE finds no matching row (step already claimed)
- Second agent gets `null`, tries again

**Outcome:** Race-free claiming

## Performance Characteristics

**Operation latencies (M1 Mac):**
- `claim_step`: 3-5ms (includes SELECT + UPDATE + INSERT)
- `heartbeat`: 1-2ms (simple UPDATE)
- `complete_step`: 3-5ms (UPDATE + INSERT event)
- `get_available_steps`: 5-10ms (complex query with joins)
- `detect_stale_work`: 5-10ms (full table scan with time comparison)

**Scalability limits:**
- 100 concurrent agents: excellent
- 1000 concurrent agents: good (some queuing)
- 10,000 concurrent agents: poor (excessive lock contention)

**Database size:**
- 100 projects × 10 steps: ~100KB
- 1000 projects × 10 steps: ~1MB
- 10,000 projects × 10 steps: ~10MB
- Events grow over time, consider pruning

## Security

**Threat model:**
- **Local user access** - Database stored in `~/.claude/`
- **No network exposure** - stdio transport only
- **No authentication** - Agents trusted (same user)
- **SQLite injection** - Use parameterized queries (safe)

**Mitigations:**
- File permissions: 0600 on database
- Parameterized queries (all queries use `?` placeholders)
- Input validation in handlers
- No eval or dynamic SQL

## Extensibility

### Adding New Tools

1. Add tool to `registerTools()` in `main.go`
2. Implement handler in `internal/mcp/handlers.go`
3. Add database method if needed in `internal/db/db.go`
4. Update README.md with tool documentation

### Adding New Fields

1. Create migration SQL in `migrations/`
2. Update schema in `internal/db/db.go` (struct fields)
3. Update queries to include new fields
4. Increment migration version

### Supporting New Workflows

Example: Adding support for task priorities

1. Add `priority` column to `steps` table
2. Modify `claim_step` to `ORDER BY priority DESC, step_num ASC`
3. Add `priority` parameter to `create_project` tool
4. Update documentation

## Monitoring

**Health checks:**
- Database connection: `PRAGMA integrity_check`
- Lock contention: `PRAGMA wal_checkpoint(PASSIVE)` status
- Active work: `SELECT COUNT(*) FROM steps WHERE status='in_progress'`

**Metrics to track:**
- Steps claimed per minute
- Average time to complete by scope
- Stale work detection rate
- Database size growth

**Debugging:**
```sql
-- Recent activity
SELECT * FROM agent_events ORDER BY timestamp DESC LIMIT 20;

-- Stuck steps
SELECT * FROM steps WHERE status='in_progress' AND last_heartbeat < datetime('now', '-1 hour');

-- Project status
SELECT p.name, COUNT(*) as total, SUM(CASE WHEN s.status='completed' THEN 1 ELSE 0 END) as done
FROM projects p JOIN steps s ON p.id=s.project_id GROUP BY p.id;
```

## Future Enhancements

**Potential improvements:**

1. **Step priorities** - High-priority steps claimed first
2. **Agent capabilities** - Match steps to agent skills
3. **Resource limits** - Max concurrent steps per agent
4. **Step estimates** - Track and predict completion times
5. **Webhooks** - Notify external systems on events
6. **Multi-database** - Shard across multiple DBs for scale
7. **Replication** - Sync state across machines
8. **Web UI** - Dashboard for monitoring

## Comparison to Alternatives

**vs File-based coordination (JSON):**
- ✅ Atomic operations (files have race conditions)
- ✅ Rich queries (files need custom parsing)
- ✅ Transactions (files are not transactional)
- ✅ Performance (indexed queries vs linear scan)

**vs PostgreSQL:**
- ✅ Zero-config deployment (no server needed)
- ✅ Single-file portability (easy backup/archive)
- ✅ Lower overhead (no network, no process)
- ❌ Horizontal scalability (single-machine only)
- ❌ Concurrent writers (queued vs parallel)

**vs Redis:**
- ✅ Persistent (Redis is in-memory first)
- ✅ SQL queries (Redis has limited querying)
- ✅ No daemon (Redis needs server)
- ❌ Pub/sub (Redis excels here)
- ❌ Real-time (Redis lower latency)

**Verdict:** SQLite is optimal for local agent coordination. PostgreSQL or Redis only needed for distributed multi-machine setups.

## References

- [SQLite WAL Mode](https://www.sqlite.org/wal.html)
- [MCP Specification](https://modelcontextprotocol.io/)
- [Go sqlite driver](https://gitlab.com/cznic/sqlite)
- [Concurrent SQLite](https://www.sqlite.org/threadsafe.html)
