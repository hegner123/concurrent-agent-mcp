# Concurrent Agent MCP - Getting Started

## Overview

This is a Go-based MCP (Model Context Protocol) server for coordinating multiple AI agents working concurrently on development tasks. It provides atomic step claiming, dependency management, heartbeat monitoring, and crash recovery through a SQLite database with WAL mode.

## Onboarding

Welcome to the hq project. Here's what you need to know:

**Key Directories:**
- `internal/db/` - Database layer with transaction-safe operations
- `internal/mcp/` - MCP tool handlers
- `migrations/` - Database schema migrations (if using separate migration files)
- `bin/` - Build output directory

**Key Files:**
- `main.go` - Entry point, tool registration
- `internal/db/db.go` - Database schema, migrations, core operations
- `internal/mcp/handlers.go` - MCP tool handlers
- `Makefile` - Build and development commands
- `CLAUDE.md` - Development guidance for AI assistants
- `ARCHITECTURE.md` - System design and architecture
- `README.md` - Complete API documentation
- `QUICKSTART.md` - Step-by-step setup guide
- `SCHEMA.md` - Database schema details
- `TODO.md` - Outstanding implementation tasks

**Database Location:**
- Default: `~/.claude/agent-coordination.db`
- Override with `AGENT_DB_PATH` environment variable

**Quick Start Commands:**
```bash
make build      # Build the binary
make install    # Install to /usr/local/bin
make dev        # Run in development mode with logging
make test       # Run tests
make db-shell   # Open SQLite shell for inspection
```

**Architecture Highlights:**
- Single Go binary serving MCP protocol over stdio
- SQLite with WAL mode for concurrent access
- Atomic step claiming via SQL transactions
- Heartbeat monitoring for crash detection
- Dependency resolution for task ordering

**Next Steps:**
1. Read QUICKSTART.md for setup instructions
2. Review ARCHITECTURE.md to understand the system design
3. Check SCHEMA.md for database structure
4. Look at TODO.md for current work items
