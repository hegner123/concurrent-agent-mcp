# Known Issues and Limitations

A candid assessment of areas needing improvement in the hq multi-agent coordination server.

## Critical Gaps

### 1. Zero Automated Tests

No `_test.go` files exist. Manual testing has been done (documented in TEST_RESULTS.md) but isn't reproducible or automated. For a concurrency-heavy system, this is the most significant gap.

**Impact:** Regressions can slip through. Race conditions may exist but go undetected without `go test -race`.

**To fix:** Add unit tests for db layer, integration tests for handlers, and run with race detector in CI.

### 2. Weak Input Validation

- No length limits on strings (project names, agent IDs, notes)
- No format validation (commit hashes should be hex)
- Empty strings accepted where they shouldn't be (empty project name, empty agent_id)
- Non-string items in arrays silently dropped instead of rejected
- Error messages expose raw SQL errors ("sql: no rows") instead of user-friendly messages

**Location:** `internal/mcp/handlers.go` - all handlers lack comprehensive validation

**To fix:** Add validation layer before database calls. Return specific error messages.

### 3. No Observability

- No structured logging
- No Prometheus metrics
- No health check endpoint
- Silent operation makes troubleshooting difficult

**Impact:** When coordination fails in production, there's no way to diagnose without direct database inspection.

**To fix:** Add `log/slog` structured logging, expose metrics for step counts/latencies/errors.

## Code Quality Issues

### 4. Unsafe JSON String Construction

```go
// internal/mcp/handlers.go:588
metadata := fmt.Sprintf(`{"reason": "%s"}`, reason)
```

If `reason` contains quotes or special characters, this produces invalid JSON or enables injection.

**To fix:** Use `json.Marshal` for all JSON construction.

### 5. Magic Strings Instead of Constants

Status values ("not_started", "claimed", "in_progress", "completed", "failed", "merged") are raw strings throughout the codebase.

**Impact:** Typos won't be caught at compile time. Refactoring is error-prone.

**To fix:** Define typed constants:
```go
const (
    StatusNotStarted  = "not_started"
    StatusClaimed     = "claimed"
    StatusInProgress  = "in_progress"
    StatusCompleted   = "completed"
    StatusFailed      = "failed"
    StatusMerged      = "merged"
)
```

### 6. Large Functions

Several functions exceed 50 lines and handle multiple concerns:
- `CreateProject` (48 lines) - validation + transaction + loop
- `ClaimStep` (55 lines) - complex query + transaction
- `RecoverFailedStep` (83 lines) - archive + reset + events

**To fix:** Extract helpers for common patterns (transaction wrapper, event logging).

## Design Limitations

### 7. Circular Dependencies Not Validated

Steps can be created with circular dependencies (A depends on B, B depends on A). The system won't detect this at creation time—it will simply deadlock at runtime with those steps never becoming available.

**To fix:** Add cycle detection in `CreateProject` before inserting dependencies.

### 8. No Pagination

List operations return all results:
- `GetAgentEvents` has a limit but no offset/cursor
- `GetAvailableSteps` returns all matching steps
- `ListProjects` returns all projects

**Impact:** Could OOM with large datasets.

**To fix:** Add `limit` and `offset` parameters to all list operations.

### 9. Single-Machine Only

SQLite file-based storage means:
- No horizontal scaling
- No high availability
- Backup requires stopping writes or using SQLite backup API

**This is a conscious design choice, not necessarily a bug.** For the intended use case (coordinating agents on a single developer machine), it's appropriate.

## Security Considerations

### 10. No Authentication

The server trusts all callers. Security relies entirely on:
- Filesystem permissions on the Unix socket / stdio
- MCP client authentication (handled externally)

**For internal use:** Acceptable.
**For exposed deployment:** Would need auth layer.

### 11. No Encryption at Rest

Database file is unencrypted. Agent events, project names, and step details are stored in plaintext.

**For sensitive workloads:** Consider SQLCipher or application-level encryption.

---

## Priority Order for Fixes

1. **Add automated tests** - highest impact on reliability
2. **Input validation** - prevents garbage data and improves error messages
3. **Circular dependency detection** - prevents silent deadlocks
4. **Constants for status values** - low effort, prevents bugs
5. **Structured logging** - essential for production debugging
6. **Safe JSON construction** - security fix
7. **Pagination** - needed before large-scale use
