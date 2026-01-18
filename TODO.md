# Implementation TODO

Tasks to complete the MCP server implementation.

## Critical (MVP)

### Database Layer (`internal/db/db.go`)

- [ ] **StartStep** - Mark claimed step as in_progress
  ```go
  func (db *DB) StartStep(stepID int64, worktree *string) error
  ```
  - UPDATE step SET status='in_progress', started_at=now(), worktree=?
  - Validate step is claimed
  - Record 'started' event

- [ ] **CompleteStep** - Mark step as completed
  ```go
  func (db *DB) CompleteStep(stepID int64, commitHash string, files []string, notes *string) error
  ```
  - UPDATE step SET status='completed', completed_at=now(), last_commit=?, files_modified=?, notes=?
  - Record 'completed' event
  - Transaction safety

- [ ] **FailStep** - Mark step as failed
  ```go
  func (db *DB) FailStep(stepID int64, reason string) error
  ```
  - UPDATE step SET status='failed'
  - Record 'failed' event with reason in metadata

- [ ] **GetStep** - Get step by ID
  ```go
  func (db *DB) GetStep(stepID int64) (*Step, error)
  ```
  - SELECT * FROM steps WHERE id=?

- [ ] **GetAvailableSteps** - Query available steps
  ```go
  func (db *DB) GetAvailableSteps(projectName *string, scope *string) ([]Step, error)
  ```
  - SELECT with dependency check
  - Filter by project/scope if provided
  - Order by priority, step_num

- [ ] **DetectStaleWork** - Find stale heartbeats
  ```go
  func (db *DB) DetectStaleWork(timeoutMinutes int) ([]Step, error)
  ```
  - SELECT WHERE status IN ('claimed','in_progress')
  - AND last_heartbeat < datetime('now', '-N minutes')

- [ ] **RecoverStep** - Reset stale step
  ```go
  func (db *DB) RecoverStep(stepID int64) error
  ```
  - UPDATE step SET status='not_started', agent_id=NULL, etc
  - Record 'recovered' event

- [ ] **ListProjects** - List projects with filter
  ```go
  func (db *DB) ListProjects(status *string) ([]Project, error)
  ```
  - SELECT * FROM projects WHERE status=? (if provided)

- [ ] **GetMetrics** - Calculate metrics
  ```go
  func (db *DB) GetMetrics(projectName *string, agentID *string) (*Metrics, error)
  ```
  - Aggregate queries for completion times
  - By scope, by agent, by project

- [ ] **GetAgentEvents** - Query event log
  ```go
  func (db *DB) GetAgentEvents(agentID *string, projectName *string, limit int) ([]AgentEvent, error)
  ```
  - SELECT with optional filters
  - ORDER BY timestamp DESC

### MCP Handlers (`internal/mcp/handlers.go`)

Wire up all TODO handlers to database methods:

- [ ] **StartStep** - Call db.StartStep
- [ ] **CompleteStep** - Call db.CompleteStep
- [ ] **FailStep** - Call db.FailStep
- [ ] **GetStep** - Call db.GetStep
- [ ] **GetAvailableSteps** - Call db.GetAvailableSteps
- [ ] **DetectStaleWork** - Call db.DetectStaleWork
- [ ] **RecoverStep** - Call db.RecoverStep
- [ ] **ListProjects** - Call db.ListProjects
- [ ] **GetMetrics** - Call db.GetMetrics
- [ ] **GetAgentEvents** - Call db.GetAgentEvents

All handlers need:
- Parameter validation
- Error handling
- JSON marshaling
- Consistent return format

## Important (Post-MVP)

### Testing

- [ ] **Unit tests** for database layer
  - Test atomic claim (concurrent claims)
  - Test dependency resolution
  - Test heartbeat/stale detection
  - Test state transitions

- [ ] **Integration tests** for MCP handlers
  - Full workflow: create → claim → complete
  - Multi-agent scenarios
  - Failure scenarios

- [ ] **Benchmark tests**
  - claim_step performance
  - Concurrent claim contention
  - Large project (1000s of steps)

### Error Handling

- [ ] **Validation** - Robust input validation
  - Check required parameters
  - Validate step_num > 0
  - Validate status values
  - Sanitize strings

- [ ] **Better errors** - User-friendly error messages
  - "Step not found" vs generic SQL error
  - "Dependencies not satisfied: step 1, 2"
  - "Agent does not own this step"

- [ ] **Logging** - Structured logging
  - Log all operations
  - Log errors with context
  - Optional debug mode

### Features

- [ ] **GetProjectWithSteps** - Get project + all steps in one call
  - Avoid N+1 queries
  - Single response object

- [ ] **UpdateProject** - Update project fields
  - Change priority
  - Change status
  - Add metadata

- [ ] **DeleteProject** - Delete project (cascade steps)
  - Safety: only if status='aborted'
  - Or force flag

- [ ] **GetStepsByProject** - List all steps for project
  - With dependency info
  - Order by step_num

- [ ] **Reorder dependencies** - Change dependency graph
  - For plan adjustments
  - Validate no cycles

## Nice to Have

### Advanced Features

- [ ] **Step priorities** - High-priority steps claimed first
  - Add priority column to steps
  - ORDER BY priority DESC in claim query

- [ ] **Agent capabilities** - Match steps to agent skills
  - Add capabilities to steps (requires_gpu, etc)
  - Agent declares capabilities
  - claim_step checks match

- [ ] **Resource limits** - Max concurrent steps per agent
  - Track active steps per agent
  - Refuse claim if over limit

- [ ] **Time estimates** - Track predicted vs actual
  - Add estimated_hours to steps
  - Calculate variance
  - Learn from history

- [ ] **Step templates** - Reusable step patterns
  - Common workflows (test suite, docs, etc)
  - Quick project creation

- [ ] **Batch operations** - Bulk create/update
  - Create 100 steps in one call
  - Update multiple steps

### Monitoring

- [ ] **Health check endpoint** - For monitoring
  - Database connection OK
  - WAL checkpoint status
  - Active work count

- [ ] **Metrics export** - Prometheus format
  - steps_claimed_total
  - step_completion_duration_seconds
  - stale_steps_detected_total

- [ ] **Event streaming** - Real-time updates
  - WebSocket or SSE
  - Live dashboard updates

### Developer Experience

- [ ] **CLI tool** - Command-line interface
  - `concurrent-agent-mcp create-project ...`
  - `concurrent-agent-mcp list-steps ...`
  - Alternative to MCP tools for debugging

- [ ] **Web UI** - Dashboard
  - Visual project progress
  - Agent activity timeline
  - Click to claim steps

- [ ] **Migration tool** - Schema migrations
  - Versioned migrations
  - Up/down support
  - Auto-apply on startup

### Documentation

- [ ] **API Reference** - Complete tool documentation
  - All parameters with types
  - Example requests/responses
  - Error codes

- [ ] **Tutorial** - Step-by-step guide
  - Build a simple multi-agent workflow
  - Explain concepts as you go

- [ ] **Video demo** - Screencast
  - Show real agents claiming steps
  - Demonstrate crash recovery

## Future/Experimental

- [ ] **Distributed mode** - Multi-machine coordination
  - PostgreSQL backend
  - Leader election
  - Cross-machine work stealing

- [ ] **Optimistic locking** - Version-based concurrency
  - Add version column
  - Check-and-set pattern
  - Reduce lock contention

- [ ] **Event sourcing** - Immutable event log
  - All changes as events
  - Rebuild state from events
  - Time travel debugging

- [ ] **Plugins** - Extension system
  - Custom event handlers
  - Custom metrics
  - Integration hooks

## Testing Checklist

Before considering MVP complete:

- [ ] Can create a project with dependencies
- [ ] Can claim step (gets correct step respecting dependencies)
- [ ] Two agents claiming simultaneously get different steps
- [ ] Can send heartbeats
- [ ] Can complete step (unlocks dependent steps)
- [ ] Can detect stale work after timeout
- [ ] Can recover stale step
- [ ] Full worktree workflow works end-to-end
- [ ] Database survives server restart (persistence)
- [ ] Performance: <10ms for claim_step

## Known Issues

- [ ] embed.FS path issue - migration file path may need adjustment
- [ ] Error messages - Need better user-facing messages
- [ ] Transaction retry - No automatic retry on SQLITE_BUSY
- [ ] Heartbeat overhead - Consider batching heartbeats
- [ ] Large result sets - No pagination on list operations

## Code Quality

- [ ] Run golangci-lint and fix issues
- [ ] Add code comments (package, exported functions)
- [ ] Consistent error wrapping (fmt.Errorf)
- [ ] Use constants for magic strings (status values, etc)
- [ ] Refactor large functions (>50 lines)

## Documentation

- [ ] Complete README examples
- [ ] Add troubleshooting section
- [ ] Document all environment variables
- [ ] Add performance tuning guide
- [ ] Write migration guide (file tracking → MCP DB)
