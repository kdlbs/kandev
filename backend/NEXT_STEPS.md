# Kandev Backend - Next Steps

## Current State (2026-01-10)

The backend is a unified Go binary (`cmd/kandev/main.go`) that runs:
- **Task Service**: Kanban board/column/task management (SQLite persistence)
- **Agent Manager**: Docker container lifecycle for AI agents
- **Orchestrator**: Coordinates task execution with agents

### Working Features

1. **SQLite Persistence**
   - Database file: `./kandev.db` (configurable via `KANDEV_DB_PATH`)
   - Stores boards, columns, and tasks with metadata
   - WAL mode enabled for concurrent reads
   - Data survives server restarts

2. **Augment Agent Integration**
   - Docker image: `kandev/augment-agent:latest`
   - Uses Auggie CLI with `--print --output-format json`
   - ACP (Agent Communication Protocol) message streaming
   - Session ID extraction and storage in task metadata

3. **Session Resumption**
   - Sessions persist in `~/.augment/sessions/` (mounted into containers)
   - Task metadata stores `auggie_session_id`
   - Follow-up agents can resume with `AUGGIE_SESSION_ID` env var

4. **Real-time Streaming**
   - Container logs parsed for ACP JSON messages
   - Events published to in-memory event bus
   - WebSocket endpoints available for frontend

## Immediate Next Steps

### 1. Agent Run History
Track all agent executions:
- [ ] Add `agent_runs` table to SQLite schema
- [ ] Store task_id, session_id, started_at, completed_at, exit_code
- [ ] Link runs to tasks for history viewing
- [ ] Store ACP messages for replay

### 2. Frontend Integration
- [ ] Create React/Vue frontend for Kanban board
- [ ] WebSocket client for real-time agent progress
- [ ] Task detail view with agent history and session resumption

### 3. Automatic Task Orchestration
- [ ] Configure tasks with `agent_type` field
- [ ] Auto-launch agents when tasks move to specific columns
- [ ] Handle agent failures with retry logic

## API Reference

### Task Management
```
POST   /api/v1/boards                    - Create board
GET    /api/v1/boards                    - List boards
POST   /api/v1/boards/:id/columns        - Create column
POST   /api/v1/tasks                     - Create task (requires board_id, column_id, title)
GET    /api/v1/tasks/:id                 - Get task (includes metadata.auggie_session_id)
```

### Agent Management
```
POST   /api/v1/agents/launch             - Launch agent
GET    /api/v1/agents/:id/status         - Get agent status
GET    /api/v1/agents/:id/logs           - Get agent logs
DELETE /api/v1/agents/:id                - Stop agent
```

### Launch Agent Request
```json
{
  "task_id": "uuid",
  "agent_type": "augment-agent",
  "workspace_path": "/path/to/project",
  "env": {
    "AUGMENT_SESSION_AUTH": "{...session json...}",
    "TASK_DESCRIPTION": "Your task here",
    "AUGGIE_SESSION_ID": "optional-for-resumption"
  }
}
```

## Development Commands

```bash
# Build
cd backend && make build

# Run server
./bin/kandev

# Build Docker image
cd dockerfiles/augment-agent && docker build -t kandev/augment-agent:latest .

# Get Augment session auth
cat ~/.augment/session.json
```

## Key Files

| File | Purpose |
|------|---------|
| `cmd/kandev/main.go` | Unified entry point, ACP handlers |
| `internal/task/repository/interface.go` | Repository interface definition |
| `internal/task/repository/sqlite.go` | SQLite implementation with schema |
| `internal/task/repository/memory.go` | In-memory implementation (for testing) |
| `internal/agent/lifecycle/manager.go` | Container lifecycle, mount templates |
| `internal/agent/registry/defaults.go` | Agent type definitions |
| `internal/agent/streaming/reader.go` | ACP message parsing from logs |
| `dockerfiles/augment-agent/agent.sh` | Container entrypoint script |
| `dockerfiles/augment-agent/Dockerfile` | Agent Docker image |

## Architecture Decisions

1. **SQLite with WAL mode** for simple, file-based persistence
2. **In-memory event bus** instead of NATS for simpler development
3. **One agent per task** constraint (prevents conflicts)
4. **Session files mounted** from host for persistence across container runs
5. **ACP protocol** for structured agent-to-server communication
6. **Repository interface** allows swapping storage backends (SQLite, Memory, PostgreSQL)

## Configuration

Environment variables:
- `KANDEV_DB_PATH` - SQLite database file path (default: `./kandev.db`)
- `KANDEV_LOGGING_LEVEL` - Log level: debug, info, warn, error (default: info)
- `KANDEV_CREDENTIALS_FILE` - Path to credentials file for agents
