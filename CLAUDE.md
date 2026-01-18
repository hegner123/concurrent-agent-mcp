# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

This is a Go-based MCP (Model Context Protocol) server for coordinating multiple AI agents working concurrently on development tasks. It provides atomic step claiming, dependency management, heartbeat monitoring, and crash recovery through a SQLite database with WAL mode. Agents can work on independent tasks simultaneously, with automatic coordination ensuring only one agent claims each step.

**Key Architecture:**
- Single Go binary serving MCP protocol over stdio
- SQLite database at `~/.claude/agent-coordination.db` (WAL mode for concurrency)
- Database layer in `internal/db/db.go` with transaction-safe operations
- MCP tool handlers in `internal/mcp/handlers.go`
- Schema migrations embedded in code (internal/db/db.go:migrate)

## Build and Development Commands

```bash
# Build binary
make build              # Builds to bin/concurrent-agent-mcp

# Install system-wide
make install            # Installs to /usr/local/bin/concurrent-agent-mcp

# Run tests
make test

# Run in development mode (with logging)
make dev

# Format code
make fmt

# Clean build artifacts and database
make clean              # Removes bin/ and database files

# Database management
make db-shell           # Opens sqlite3 shell
make db-reset           # Deletes database (recreated on next run)
make db-backup          # Creates timestamped backup
```

## Code Architecture

### Concurrency Model

**Critical:** All step claiming uses atomic operations via SQLite transactions. The `ClaimStep` operation uses `UPDATE ... WHERE id = (SELECT ...)` pattern which is atomic in SQLite - only one agent can claim a given step even with concurrent attempts.

**WAL Mode Configuration (internal/db/db.go:33-40):**
- `journal_mode=WAL` - Enables Write-Ahead Logging for concurrent readers/writers
- `busy_timeout=5000` - 5 second wait if write lock is held
- `foreign_keys=ON` - Enforces referential integrity
- Connection pool: max 25 open connections, 5 idle

### Database Layer (internal/db/)

**Key types:**
- `Project` - Top-level project with base commit hash
- `Step` - Individual work item with status, agent assignment, heartbeat, optional workspace path
- `StepInput` - Input for creating steps with dependencies

**Transaction pattern:** All state-changing operations use `tx.Begin()`, defer `tx.Rollback()`, explicit `tx.Commit()`. This ensures atomicity even with concurrent access.

**Dependency resolution:** Steps only become available when all dependencies have status `completed` or `merged`. The availability query uses `NOT EXISTS` to check unsatisfied dependencies.

### MCP Handler Layer (internal/mcp/)

Handlers receive `map[string]interface{}` arguments, perform type validation, call database methods, and return `*mcp.CallToolResult` (or error).

**Handler pattern:**
1. Type-check and extract arguments from map
2. Call database method(s)
3. Return `mcp.NewToolResultText(json)` for success
4. Return `mcp.NewToolResultError(message)` for errors (non-nil error should not be returned)

### State Machine

**Step lifecycle:**
```
not_started → claim_step() → claimed → start_step() → in_progress
→ complete_step() or fail_step() → completed/failed → merged
```

**Heartbeat requirement:** Agents must call `heartbeat()` every 30-60 seconds while working on a step. Steps with heartbeats older than 15 minutes (default) are detected by `detect_stale_work()` and can be recovered.

## Testing Patterns

**Testing the server locally:**
1. Run `make build` to create binary
2. Execute `./bin/concurrent-agent-mcp` directly (connects stdin/stdout)
3. The server will create the database and run migrations
4. Use Claude Code or any MCP client to test tools

**Database inspection:**
```bash
sqlite3 ~/.claude/agent-coordination.db

# Check active projects
SELECT * FROM projects WHERE status = 'active';

# See available steps (with dependency checking)
SELECT s.* FROM steps s
LEFT JOIN dependencies d ON s.id = d.step_id
LEFT JOIN steps ds ON d.depends_on_step_id = ds.id
WHERE s.status = 'not_started'
AND (ds.status = 'completed' OR ds.id IS NULL);

# View recent agent activity
SELECT * FROM agent_events ORDER BY timestamp DESC LIMIT 20;
```

## Go-Specific Conventions

- Use `any` not `interface{}`
- All database fields that can be NULL are Go pointer types (`*string`, `*time.Time`)
- JSON arguments from MCP come as `float64` for numbers (cast to `int` or `int64`)
- Database uses `?` placeholders (not named parameters)
- SQL timestamps use SQLite's `datetime('now')` function

## Adding New Tools

1. Add tool registration in `main.go:registerTools()` with schema
2. Add handler method to `internal/mcp/handlers.go` following the handler pattern
3. Add database method to `internal/db/db.go` if needed
4. Update README.md with tool documentation

## Adding Database Fields

1. Update the schema in `internal/db/db.go:migrate()` function
2. Update corresponding struct in `internal/db/db.go`
3. Update queries to include new field
4. Consider migration strategy for existing databases (schema version tracking exists)

## Performance Characteristics

**Operation latencies (local SQLite):**
- `claim_step`: 3-5ms (atomic SELECT + UPDATE + INSERT)
- `heartbeat`: 1-2ms (simple UPDATE)
- `get_available_steps`: 5-10ms (complex query with joins)

**Scalability:** Excellent for 100 concurrent agents, good for 1000, degrades beyond that due to write lock contention.

## MCP Configuration

**User scope (recommended for global access):**
```bash
claude mcp add --scope user --transport stdio concurrent-agent-mcp -- \
  /usr/local/bin/concurrent-agent-mcp
```

**Environment variable:** Set `AGENT_DB_PATH` to override default database location.

**Usage:** See global `~/.claude/CLAUDE.md` for complete usage examples and workflow patterns.

## Documentation Files

- `README.md` - Complete API documentation and usage examples
- `ARCHITECTURE.md` - System design, data flow, failure modes, extensibility
- `QUICKSTART.md` - Step-by-step setup and testing guide
- `SCHEMA.md` - Database schema details
- `TODO.md` - Outstanding implementation tasks
