# HQ Codebase Review Results

**Date:** 2026-02-06
**Commit:** 4f14817
**Project:** hq-codebase-review (6 coordinated reviews)

---

## Cross-Review Consensus

Issues flagged independently by 3+ reviewers:

| Issue | Flagged By | Severity |
|-------|-----------|----------|
| JSON injection in FailStep (`fmt.Sprintf` for metadata) | backend, spec, database, security, orchestrator | Critical |
| Pragma error silently lost (err reassigned to Close result) | backend, spec, database, orchestrator | Critical |
| CreateDependency silent failure (0 rows inserted = no error) | backend, database, tests, orchestrator | Critical |
| ResetFailedStep no WHERE status guard (TOCTOU race) | backend, database, tests, security, orchestrator | High |
| Connection pool 25 is wrong for SQLite | backend, spec, database, orchestrator | High |
| context.Background() everywhere | backend, spec, security, orchestrator | Medium |
| os.Exit(0) bypasses defer cleanup | backend, spec, security, orchestrator | Medium |
| Heartbeat swallows event insert error | backend, spec, database, orchestrator | Medium |

---

## 1. Go Backend Review

**Agent:** go-backend-reviewer
**Focus:** Transaction patterns, connection pooling, graceful shutdown, error propagation

### Assessment

MCP server coordinating concurrent AI agents through SQLite with atomic step claiming, dependency resolution, heartbeat monitoring, and crash recovery. For its scope (local dev tool, ~100 agents), it works. The atomic claiming query is the right approach. The codebase is readable and the sqlc integration is clean.

That said, there are several issues ranging from "will cause a production incident" to "will cause confusion during debugging."

### Critical Issues

**C1. Graceful shutdown is not graceful -- it is `os.Exit(0)` in a goroutine.**

`main.go:29-32`:
```go
go func() {
    <-sigChan
    os.Exit(0)
}()
```

This kills the process immediately. `defer database.Close()` on line 64 never runs. SQLite WAL mode means in-flight writes can leave the WAL file in a dirty state. The `-wal` and `-shm` files may not be checkpointed. For a coordination database, this means steps could be left in inconsistent states -- claimed but the claim transaction's RETURNING never made it back to the caller, so the agent doesn't know it owns the step.

The fix: cancel a context, let `server.ServeStdio` (or its replacement) return, and let the deferred `Close()` run. If the MCP library doesn't support context cancellation, at minimum run `database.Close()` in the signal handler before exiting.

**C2. Pragma error handling silently loses the original error.**

`internal/db/store.go:42-48`:
```go
if _, err := conn.Exec(pragma); err != nil {
    err = conn.Close()
    if err != nil {
        return nil, fmt.Errorf("connection closed error %s,%w", pragma, err)
    }
    return nil, fmt.Errorf("set pragma %s: %w", pragma, err)
}
```

When `conn.Exec(pragma)` fails, `err` is reassigned to `conn.Close()`. If `Close()` succeeds (returns nil), the final `return` wraps a nil error: `fmt.Errorf("set pragma %s: %w", pragma, nil)`. The caller gets a message like `"set pragma PRAGMA journal_mode=WAL: <nil>"`. The actual pragma failure reason is gone.

**C3. JSON injection in `FailStep` metadata.**

`internal/db/store.go:497`:
```go
metadata := fmt.Sprintf(`{"reason": "%s"}`, reason)
```

If `reason` contains a double quote or backslash, this produces malformed JSON. If `reason` contains `"}, "injected": "value`, this produces unintended JSON structure. Use `json.Marshal` for the map or escape properly.

**C4. `ResetFailedStep` query has no status guard.**

`internal/db/query.sql:110-122`:
```sql
UPDATE steps
SET status = 'not_started', ...
WHERE id = ?;
```

No `AND status = 'failed'` guard. The `RecoverFailedStep` Go function does a `GetFailedStep` check first, but between that SELECT and this UPDATE, another agent could have changed the step's status. The archive-then-reset is not atomic with respect to concurrent status changes. A completed step could be reset to not_started if the timing is wrong. Add `AND status = 'failed'` to the WHERE clause.

**C5. `CreateDependency` silently succeeds when the dependency target doesn't exist.**

`internal/db/query.sql:33-35`:
```sql
INSERT INTO dependencies (step_id, depends_on_step_id)
SELECT ?, id FROM steps
WHERE project_id = ? AND step_num = ?;
```

If `step_num` doesn't exist, the SELECT returns zero rows, the INSERT inserts nothing, and `ExecContext` returns nil error. The caller in `store.go:246` won't know the dependency wasn't created. This is a silent data corruption path -- the step appears to have no dependencies when it should.

Check `RowsAffected()` or use `:one` / `RETURNING` to detect this.

### Improvements

**I1. Connection pool size is wrong for SQLite.**

`internal/db/store.go:27-29`:
```go
conn.SetMaxOpenConns(25)
conn.SetMaxIdleConns(5)
conn.SetConnMaxLifetime(time.Hour)
```

SQLite is a single-writer database. 25 open connections means 24 of them will block on every write waiting for `busy_timeout`. `SetMaxOpenConns(1)` is the standard recommendation for SQLite. 25 connections is PostgreSQL thinking applied to SQLite.

**I2. `context.Background()` everywhere -- no cancellation propagation.**

Every method in `store.go` creates `context.Background()`. If the MCP server is shutting down or a request times out, there's no way to cancel in-flight database operations. Each method should accept a `context.Context` parameter.

**I3. `Heartbeat` swallows the event insertion error.**

`internal/db/store.go:349-353`:
```go
// Record event (silently fail if error)
db.queries.InsertAgentEvent(ctx, InsertAgentEventParams{...})
```

The return value is discarded. This is the only place in the codebase that does this. Heartbeat events are the primary forensic tool for debugging stale step detection.

**I4. `ListProjects` doesn't validate the status filter.**

`internal/db/store.go:528-529`:
```go
return db.queries.ListProjectsByStatus(ctx, ProjectStatus(*status))
```

The `ProjectStatus` type has a `Valid()` method that is never called anywhere in the codebase.

**I5. `DetectStaleWork` uses `fmt.Sprintf` for the query instead of a parameter.**

`internal/db/store.go:560-567`:
```go
query := fmt.Sprintf(`...
    AND (last_heartbeat IS NULL OR last_heartbeat < datetime('now', '-%d minutes'))
`, timeoutMinutes)
```

`timeoutMinutes` is an `int` so injection is impossible here. However, `fmt.Sprintf` in SQL is a pattern that will be cargo-culted into less safe contexts. Consider computing the cutoff time in Go and passing it as a parameter.

**I6. Handler signatures don't match `mcp-go` v0.26+.**

`internal/mcp/handlers.go:73`:
```go
func (h *Handlers) CreateProject(arguments map[string]any) (*mcp.CallToolResult, error) {
```

Recent `mcp-go` releases changed to `(context.Context, mcp.CallToolRequest)`. When you upgrade the library, every handler breaks.

**I7. Migration runs multi-statement DDL without a transaction.**

`internal/db/store.go:196`:
```go
if _, err := db.conn.Exec(initialSchema); err != nil {
    return fmt.Errorf("execute migration: %w", err)
}
```

If the migration fails partway through, the database is left in a partial state. Wrap it in a transaction.

### Testing Notes

- **ClaimStep atomicity** needs a concurrent test: N goroutines all calling `ClaimStep` for the same project simultaneously.
- **ResetFailedStep TOCTOU**: test where one goroutine completes a step while another is in `RecoverFailedStep`.
- **CreateDependency silent failure**: test creating a step with `depends_on: [999]` where step_num 999 doesn't exist.
- **Heartbeat under connection contention**: with `MaxOpenConns(25)`, fire 50 concurrent heartbeats.

### Strengths

- The `ClaimStep` SQL query is the correct atomic pattern for SQLite work queues.
- sqlc for query generation with custom types gives real type safety.
- The `defer tx.Rollback()` pattern is used consistently and correctly.
- Handler layer is thin. No business logic leaking into the MCP layer.

---

## 2. Go Spec Review

**Agent:** go-spec-reviewer
**Focus:** Idiomatic patterns, simplicity, Go philosophy alignment

### Summary

Well-structured Go MCP server with strong foundational choices: typed IDs, enum types with validation, sqlc-generated query code, and atomic transactions. The worst severity found is a **correctness bug** in the pragma error-handling path plus a **JSON injection vulnerability** in `FailStep`. The code generally follows Go conventions and avoids premature abstraction.

### Critical Issues

1. **Silent error loss in pragma setup** (`store.go:42-48`) -- `err` reassigned to `conn.Close()` result, original pragma error lost when Close succeeds.

2. **JSON injection in FailStep metadata** (`store.go:497`) -- `fmt.Sprintf` without escaping. Compare with `CompleteStep` which correctly uses `json.Marshal`.

3. **Heartbeat silently discards event insertion error** (`store.go:349-353`) -- Error return completely discarded. Per Go convention, ignored errors must have a comment stating the specific reason recovery is impossible.

4. **`ClaimStep` uses `err == sql.ErrNoRows` instead of `errors.Is`** (`store.go:290`) -- Direct equality comparison instead of idiomatic `errors.Is(err, sql.ErrNoRows)`. Same at line 653.

5. **`context.Background()` used everywhere** -- Every public method on `DB` creates its own `context.Background()`. Should accept `context.Context` as first parameter.

### Important Issues

1. **`GetMetrics` has excessive combinatorial branching** (`store.go:722-813`) -- 4-way `if/else if/else if/else` block for metrics, then near-identical 4-way block for scope breakdown.

2. **`Handlers` methods all accept `map[string]any`** -- Framework-dictated. The `error` return is dead code (every error path returns `mcp.NewToolResultError(...), nil`).

3. **`SQLiteTime` does not implement `driver.Valuer`** (`types.go`) -- `NullSQLiteTime` implements both `Scan` and `Value`, but `SQLiteTime` only implements `Scan` and `MarshalJSON`. Asymmetry suggests oversight.

4. **`parseString` function comment is wrong** (`handlers.go:32`) -- Comment says `parseString` but the function is `toInt`. Copy-paste leftover.

5. **Signal handler calls `os.Exit(0)` without cleanup** (`main.go:29-32`) -- Bypasses `defer database.Close()`.

### Minor Issues

1. **`NullProjectID` and `NullStepID` are structurally identical** -- Consider generic `Null[T]` if more nullable ID types are added.

2. **`StepInput.DependsOn` uses `[]int` instead of typed IDs** (`store.go:208`) -- Inconsistent with the rest of the typed-ID discipline.

3. **`DetectStaleWork` uses `fmt.Sprintf` for query construction** (`store.go:560-567`) -- Safe since `%d` format, but sets a cargo-cultable pattern.

4. **`FailedStepsHistory.ID` is plain `int64`** (`models.go:28`) -- Every other table's PK has a typed ID. Fix belongs in `sqlc.yaml` type overrides.

5. **`migrate()` runs PRAGMA inside the migration string** (`store.go:98`) -- Foreign keys already enabled in pragma setup loop. Redundant.

### Positive Observations

1. **Excellent type discipline.** Typed IDs with nullable wrappers prevent parameter-swapping bugs at compile time.
2. **Clean transaction discipline.** Correct `Begin()` / `defer Rollback()` / `Commit()` pattern throughout.
3. **Pragmatic architecture.** No unnecessary interfaces, no DI frameworks. Right level of abstraction for the problem size.

---

## 3. Database Review

**Agent:** database-specialist
**Focus:** Index strategy, transaction boundaries, foreign key cascades, migration readiness

### Assessment

SQLite-backed coordination database for concurrent AI agents. Clean schema, well-structured for its use case, solid understanding of SQLite's concurrency model. The sqlc integration is well-configured with custom type overrides.

### Critical Issues

**C1. JSON injection in FailStep metadata** (`store.go:497`)

`fmt.Sprintf` without escaping. Corrupts `agent_events.metadata` column with invalid JSON.

**C2. ResetFailedStep has no status guard** (`query.sql:121-122`)

No `AND status = 'failed'` guard. The SQL is semantically wrong -- it should express its precondition.

**C3. CreateDependency silently succeeds when target doesn't exist** (`query.sql:33-35`)

`INSERT ... SELECT` inserts 0 rows silently. Change to `:execresult` and check `RowsAffected() == 0`.

**C4. Pragma error masking** (`store.go:42-48`)

Variable `err` reassigned to `conn.Close()` result, original pragma error lost.

### Improvements

**I1. Missing composite index for ClaimStep hot path.**

The `ClaimStep` query is the most contention-sensitive. Current indexes are single-column. Add:
```sql
CREATE INDEX IF NOT EXISTS idx_steps_claim_lookup
ON steps(project_id, status, step_num);
```
Existing `idx_steps_project_id` and `idx_steps_status` become redundant.

**I2. DetectStaleWork heartbeat query not using index optimally.**

OR condition prevents efficient index usage. Add partial index:
```sql
CREATE INDEX IF NOT EXISTS idx_steps_stale_detection
ON steps(status, last_heartbeat)
WHERE status IN ('claimed', 'in_progress');
```

**I3. Redundant indexes on dependencies table.**

`idx_dependencies_step_id` on `(step_id)` is redundant -- PK `(step_id, depends_on_step_id)` already covers it. Keep `idx_dependencies_depends_on`.

**I4. Heartbeat fires event insert outside transaction.**

Only state-changing method that does NOT use a transaction. Inconsistency worth documenting or fixing.

**I5. Connection pool inappropriate for SQLite.**

25 connections create unnecessary resource overhead. PRAGMAs set in `NewDB` won't apply to new pool connections. Recommend `SetMaxOpenConns(1)` or setting pragmas via DSN.

**I6. agent_events table will grow unbounded.**

With 100 agents heartbeating every 30-60 seconds: ~100,000+ rows per day. No retention policy exists.

**I7. ListProjectsAll has no LIMIT.**

Returns all projects ever created with no pagination.

### Recommendations

**R1. Migration strategy needs forward planning.** Current "check version, skip if applied" doesn't compose. Adopt a migration runner pattern with versioned migrations array.

**R2. Foreign key cascades deserve scrutiny.** Deleting a project destroys `failed_steps_history` (CASCADE) but orphans `agent_events` (SET NULL). Inconsistent. Consider `ON DELETE RESTRICT` on `steps.project_id`.

**R3. No CHECK constraint on self-referential dependencies.** `step_id = depends_on_step_id` creates permanent deadlock. Add `CHECK(step_id != depends_on_step_id)`.

**R4. No cross-project dependency validation.** FK constraints only reference `steps.id`, not `steps.project_id`. A direct INSERT could create cross-project dependencies.

**R5. Consider `updated_at` column on steps.** Single authoritative "when was this last touched?" answer.

**R6. TEXT timestamps vs INTEGER.** TEXT works but consumes more storage. Consider integer Unix epoch for v2.

**R7. Scope breakdown queries join projects unnecessarily.** `GetScopeBreakdownAll` and `GetScopeBreakdownByAgent` never filter on projects table.

### Summary Table

| Severity | Count |
|----------|-------|
| Critical | 4 |
| Improvement | 7 |
| Recommendation | 7 |

Highest-priority: C1 (data corruption), C3 (dependency graph corruption), I5 (pragma loss on new pool connections).

---

## 4. Orchestrator Architecture Review

**Agent:** orchestrator-consultant
**Focus:** Coordination architecture, failure handling, bottlenecks, deadlock scenarios

### Summary

HQ is a well-scoped, pragmatically designed coordination server for local multi-agent development workflows. The core design decision -- SQLite as a coordination primitive over stdio MCP -- is sound for its target environment. The codebase demonstrates good engineering taste. However, the system has several architectural gaps that would cause failures under real production pressure: no automated stale-work sweeper, silent dependency creation failures, no mechanism to release a claim, and `GetProject` doesn't return its steps.

### Probability Assessment

**Success at stated scale (100 concurrent agents): 60-70%**

| Risk | Impact | Probability |
|------|--------|-------------|
| No automated stale-work recovery | High | Near-certain |
| Silent dependency creation failure | High | Moderate |
| ResetFailedStep TOCTOU | Medium | Moderate |
| Connection pool = 25 for SQLite | Medium | Likely |
| No graceful shutdown | Low-Medium | Certain on SIGINT |
| No agent release/unclaim mechanism | Medium | Moderate |

**Confidence level:** Medium-High (codebase small enough to fully audit).

### Strengths

- **Atomic claiming is correct.** `UPDATE ... WHERE id = (SELECT ... LIMIT 1) RETURNING ...` is genuinely atomic in SQLite. The single most important thing to get right and it is correct.
- **Type safety through the stack.** Typed IDs, enum types, custom timestamp handling.
- **Transaction discipline.** Consistent `Begin()` / `defer Rollback()` / `WithTx()` / `Commit()` pattern.
- **Dependency resolution query.** `NOT EXISTS` pattern correctly handles full dependency graph including diamond dependencies.
- **State machine enforcement in SQL.** Illegal transitions impossible at database level.
- **Zero-infrastructure deployment.** Single binary, single file database, stdio transport.

### Concerns

**Critical:**
- **No automated stale-work recovery.** `detect_stale_work` is passive/tool-invoked. If no agent calls it, stale steps block downstream forever. Real scenario: agent crashes at 3am, entire downstream graph permanently stuck.
- **CreateDependency silently succeeds when target doesn't exist.** Typo in `depends_on` silently creates broken dependency graph.

**High:**
- ResetFailedStep TOCTOU race (no status guard)
- Connection pool of 25 wrong for SQLite
- `GetProject` returns only project row, not steps (insufficient for dashboards)

**Medium:**
- No mechanism to release/unclaim a step (agent stuck with unwanted claim)
- No priority within claims (first-come-first-served only, no capability matching)
- Heartbeat swallows event insertion error
- JSON injection in FailStep metadata
- Pragma error handling loses original error

**Low:**
- `context.Background()` everywhere
- No project lifecycle management (no `complete_project` or `abort_project`)
- `agent_events` table grows unbounded

### Recommendations

1. **(Critical)** Add background stale-work recovery goroutine (every 60s, auto-recover stale steps)
2. **(Critical)** Validate dependency targets exist during project creation
3. **(High)** Add `AND status = 'failed'` to `ResetFailedStep`
4. **(High)** Reduce connection pool to 1-3
5. **(Medium)** Add `release_step` tool for voluntary unclaim
6. **(Medium)** Add `GetStepsByProject` query, expose steps in `GetProject`
7. **(Medium)** Fix signal handler to use context cancellation
8. **(Medium)** Use `json.Marshal` for FailStep metadata
9. **(Low)** Fix pragma error handling
10. **(Low)** Add project lifecycle transitions

### Open Questions

1. Who calls `detect_stale_work`? Is manual-only intentional for v0.1.0?
2. Is the `merged` status used? No tool transitions steps to `merged`.
3. What is the expected heartbeat architecture? Each heartbeat consumes a conversation turn.
4. Multi-machine intention? If near-term, abstract SQLite-specific patterns behind an interface now.
5. Why is `scope` a free-form string rather than an enum?

---

## 5. Test Suite Review

**Agent:** test-reviewer
**Focus:** Concurrency testing gaps, transaction edge cases, heartbeat scenarios, dependency graph coverage

### Summary

The test suite covers basic happy-path operations and input validation reasonably well, but has **significant gaps in the areas that matter most**: atomicity under contention, dependency graph edge cases, heartbeat-based failure detection, and state machine boundary enforcement.

### Critical Issues

1. **Concurrent Claim Test has a soundness problem** (`db_test.go:362-406`) -- Asserts `claimed > 1` is failure, but silently accepts `claimed == 0`. Test can pass even if no agent ever successfully claims.

2. **Concurrent Claim with Multiple Available Steps is missing** -- Only races for 1 step. The dangerous scenario (3 agents, 2 steps) is never tested.

3. **ResetFailedStep has no WHERE status guard** -- No test attempts `RecoverFailedStep` on a non-failed step or races it against `CompleteStep`.

4. **Heartbeat silently swallows event insert errors** -- No test verifies this behavior or its implications.

5. **No test for heartbeat timing boundary conditions** -- No test for heartbeat at exactly the timeout threshold.

### Coverage Gaps

| Missing Test | Risk |
|-------------|------|
| Concurrent claim with multiple steps | Critical |
| Recover-during-complete race | Critical |
| CreateProject rollback on dependency failure | High |
| Concurrent claim across multiple projects | High |
| Dependency chain longer than 2 | High |
| Diamond dependencies | High |
| Circular dependency prevention | Medium |
| Fail from claimed state | Medium |
| Recover step from claimed vs in_progress | Medium |
| Heartbeat after recover lifecycle | Medium |
| FailStep metadata JSON injection | Medium |
| Empty project (zero steps) | Low |
| DetectStaleWork with zero timeout | Low |
| ListProjects with invalid status | Low |

### Improvement Opportunities

1. `TestClaimStep_Concurrent` should verify post-race database state (exactly 1 owner, correct status, non-null timestamps)
2. Handler tests should verify response content, not just shape
3. `TestHandlers_FullWorkflow` should be table-driven
4. `TestStartStep_WrongState` doesn't test the unclaimed case
5. Error message assertions too loose (only check `IsError == true`)

### Praise

1. **ClaimStep SQL atomicity design** -- `UPDATE ... WHERE id = (SELECT ... LIMIT 1) RETURNING ...` is the correct approach.
2. **Dependency resolution logic** -- `NOT EXISTS` pattern correctly handles lifecycle states.
3. **Type safety with custom IDs** -- Prevents ID type confusion at compile time.
4. **Handler argument validation coverage** -- Thorough edge case testing for MCP input parsing.
5. **`TestClaimStep_RespectsDependencies`** -- Tests WHAT (dependency blocking) not HOW.

### Context-Specific Notes

- WAL mode behavior under concurrent writes is barely tested
- `Heartbeat` is the only state-changing operation without a transaction -- race with `RecoverStep` is untested
- `merged` status is never tested despite appearing in dependency resolution
- No tests verify graceful handling of database initialization failures

---

## 6. Security Review

**Agent:** security-reviewer
**Focus:** Input validation, SQL injection, path sanitization, environment handling

### Summary

The codebase is **generally security-conscious** for its threat model (local coordination server for trusted AI agents over stdio). Strong use of sqlc parameterized queries. Two issues merit attention -- one high-severity and one medium -- along with several defense-in-depth recommendations.

### High Issues

**H1: JSON Injection in FailStep Metadata** (`store.go:497`)

```go
metadata := fmt.Sprintf(`{"reason": "%s"}`, reason)
```

Agent-supplied `reason` interpolated without escaping. Produces malformed JSON or allows key injection. Fix: use `json.Marshal`.

### Medium Issues

**M1: SQL Injection via Integer Interpolation Precedent** (`store.go:560-567`)

```go
query := fmt.Sprintf(`... datetime('now', '-%d minutes') ...`, timeoutMinutes)
```

`%d` prevents injection, but sets a bad precedent. Fix: compute cutoff time in Go, pass as `?` parameter.

**M2: No Input Length Validation** (all handlers)

All agent-supplied strings are unbounded. A misbehaving agent could supply megabytes for any field, causing database bloat and memory pressure.

**M3: No Status Filter Validation** (`store.go:525-532`)

`Valid()` methods on enum types exist but are never called in any code path.

### Low Issues / Hardening

- **L1:** ResetFailedStep no status guard (defense in depth)
- **L2:** No worktree path validation (stored as-is, no traversal check)
- **L3:** Database directory created with 0755 (world-readable on shared systems)
- **L4:** Heartbeat error silently ignored
- **L5:** Signal handler calls os.Exit(0) without cleanup
- **L6:** No upper bound on GetAgentEvents limit (agent can request millions of rows)
- **L7:** No context.Context propagation (slow/locked DB blocks indefinitely)

### Positive Security Observations

1. Parameterized queries via sqlc for the vast majority of SQL
2. Atomic step claiming prevents race conditions
3. State machine enforcement in SQL WHERE clauses
4. Typed IDs prevent accidental mixing
5. Foreign key enforcement with `PRAGMA foreign_keys=ON`
6. WAL mode with busy timeout for concurrent access
7. Error handling returns `NewToolResultError()` not Go errors (no stack traces leaked)

### Threat Model

| Trust Boundary | Risk Level | Notes |
|---|---|---|
| MCP tool arguments (from agents) | Medium | Semi-trusted but can misbehave |
| AGENT_DB_PATH env var | Low | Set by user/admin |
| SQLite database file | Low | Local, but permissions could be tighter |
| stdio transport | Low | No network exposure |
