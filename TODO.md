# Implementation TODO

Tasks for the MCP agent coordination server.

## Completed (MVP)

- [x] Database schema with migrations (v1, v2)
- [x] CreateProject with steps and dependencies
- [x] ClaimStep (atomic, concurrent-safe)
- [x] StartStep, CompleteStep, FailStep
- [x] GetStep, GetProject (with steps), ListProjects
- [x] Heartbeat (transactional - update + event in single tx)
- [x] GetAvailableSteps (with dependency resolution, project/scope filters)
- [x] DetectStaleWork, RecoverStep, RecoverFailedStep
- [x] AutoRecover (background goroutine, returns RecoveryResult)
- [x] GetMetrics (by project, agent, scope breakdown)
- [x] GetAgentEvents (with filters)
- [x] MCP handlers for all tools (14 tools)
- [x] Type-safe IDs (ProjectID, StepID, AgentEventID)
- [x] Status enum types with validation
- [x] Context propagation in all database methods
- [x] errors.Is for sql.ErrNoRows checks
- [x] Input validation (name, steps, step_num, branch, scope)
- [x] Dependency cycle detection (DFS)
- [x] WAL checkpoint on Close
- [x] Graceful shutdown (signal handling, WaitGroup)
- [x] Context timeout in handlers (30s)
- [x] Composite index for ClaimStep performance
- [x] Single-connection pool (pragma safety)
- [x] Unit tests (55 tests across db and mcp packages)
- [x] Concurrent claim test

## Post-MVP

### Testing

- [ ] Integration tests for full multi-agent workflows
- [ ] Benchmark tests (claim_step, concurrent contention, large projects)

### Error Handling

- [ ] Structured logging (slog)
- [ ] Better error messages for dependency failures
- [ ] Transaction retry on SQLITE_BUSY

### Features

- [ ] UpdateProject (change priority, status)
- [ ] DeleteProject (cascade, safety checks)
- [ ] Reorder dependencies (plan adjustments)
- [ ] Step priorities (high-priority claimed first)

## Nice to Have

- [ ] Agent capabilities matching
- [ ] Resource limits (max concurrent steps per agent)
- [ ] Pagination on list operations
- [ ] Health check endpoint
- [ ] Metrics export (Prometheus format)
- [ ] CLI subcommands for debugging
- [ ] Web UI dashboard

## Known Issues

- [ ] No automatic retry on SQLITE_BUSY (relies on busy_timeout pragma)
- [ ] Large result sets have no pagination
- [ ] No pruning of old agent events
