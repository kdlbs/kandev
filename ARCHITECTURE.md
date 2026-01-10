# Kandev Architecture

> Quick reference for developers and AI agents working on this codebase.

## System Overview

Kandev is a kanban board with AI agent orchestration. Three main components:

```
┌──────────────────────────────────────────────────────────┐
│  Web UI (Next.js 16 + React 19)                         │
│  http://localhost:3000                                   │
└───────────────────────┬──────────────────────────────────┘
                        │ HTTP/WebSocket
┌───────────────────────▼──────────────────────────────────┐
│  Backend (Go + Gin + SQLite)                             │
│  http://localhost:8080                                   │
│                                                           │
│  [Task Service] [Agent Manager] [Orchestrator]          │
│  [Event Bus: In-Memory/NATS]                            │
└───────────────────────┬──────────────────────────────────┘
                        │ ACP (JSON-RPC over stdout)
┌───────────────────────▼──────────────────────────────────┐
│  AI Agents (Docker)                                      │
│  auggie --acp                                            │
└──────────────────────────────────────────────────────────┘
```

## Quick Start

```bash
# Install dependencies
make install

# Terminal 1: Backend
make dev-backend

# Terminal 2: Web app
make dev-web

# Build everything
make build

# Run tests
make test
```

## Tech Stack

### Web App (`apps/web/`)
- **Framework**: Next.js 16 (App Router), React 19
- **Styling**: Tailwind CSS 4, shadcn/ui
- **Language**: TypeScript
- **Port**: 3000

### Backend (`backend/`)
- **Language**: Go 1.21+
- **Framework**: Gin (HTTP), Gorilla WebSocket
- **Database**: SQLite (WAL mode)
- **Container**: Docker client
- **Logging**: Zap
- **Port**: 8080

### Agents
- **Runtime**: Docker containers
- **Protocol**: ACP (JSON-RPC 2.0 over stdout)
- **Current**: Augment Agent (`auggie --acp`)

## Project Structure

```
.
├── Makefile               # Root build orchestration
├── ARCHITECTURE.md        # This file
├── AGENTS.md             # Agent protocol details
├── apps/
│   └── web/              # Next.js web application
│       ├── app/          # Pages (App Router)
│       ├── components/   # React components
│       └── lib/          # Utilities
├── backend/
│   ├── cmd/kandev/       # Main entry point
│   ├── internal/
│   │   ├── agent/        # Agent manager (Docker, ACP, lifecycle)
│   │   ├── task/         # Task service (CRUD, SQLite)
│   │   ├── orchestrator/ # Task execution coordinator
│   │   ├── events/       # Event bus (in-memory/NATS)
│   │   ├── gateway/      # WebSocket gateway
│   │   └── common/       # Shared utilities (config, logger, db)
│   └── pkg/
│       ├── acp/          # ACP protocol types
│       └── api/v1/       # API models
└── docs/
    └── openapi.yaml      # REST API specification
```

## Data Flow

1. User creates task in web UI
2. POST `/api/v1/tasks` → SQLite
3. User launches agent for task
4. Backend spawns Docker container
5. Agent streams progress via ACP
6. Events published to event bus
7. WebSocket pushes updates to UI

## Backend Services

### Task Service
- **Responsibility**: Manage boards, columns, tasks
- **Storage**: SQLite (`kandev.db`)
- **Tables**: `boards`, `columns`, `tasks`

### Agent Manager
- **Responsibility**: Docker lifecycle, ACP streaming
- **Key Components**:
  - Docker client (container management)
  - ACP session manager (JSON-RPC handler)
  - Streaming reader (parse stdout)
  - Credentials manager (mount secrets)

### Orchestrator
- **Responsibility**: Queue tasks, coordinate execution
- **Features**: Task queue, agent launch, monitoring

## API Endpoints

**Base**: `http://localhost:8080/api/v1`

| Endpoint | Purpose |
|----------|---------|
| `GET /health` | Health check |
| `/boards` | Board CRUD |
| `/columns` | Column CRUD |
| `/tasks` | Task CRUD, move, state changes |
| `/agents/launch` | Launch agent for task |
| `/agents/:id/status` | Agent status |
| `/agents/:id/logs` | Agent logs |
| `/agents/:id/prompt` | Send prompt (ACP) |
| `/agents/types` | List agent types |
| `/orchestrator/*` | Queue, trigger, status |

**Full API**: See `docs/openapi.yaml`

## Agent Communication Protocol (ACP)

Agents use JSON-RPC 2.0 over stdout for bidirectional communication.

### Key Methods

**Backend → Agent:**
- `initialize` - Start connection
- `session/new` - Create session
- `session/load` - Resume session
- `session/prompt` - Send task
- `session/cancel` - Cancel operation

**Agent → Backend:**
- `session/update` - Progress notifications

### Example Flow

```json
// Backend sends
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{...}}

// Agent responds
{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"1.0",...}}

// Backend sends task
{"jsonrpc":"2.0","id":2,"method":"session/prompt","params":{"message":"Fix bug"}}

// Agent streams updates
{"jsonrpc":"2.0","method":"session/update","params":{"type":"content","data":{...}}}
{"jsonrpc":"2.0","method":"session/update","params":{"type":"complete","data":{...}}}
```

**Details**: See `AGENTS.md`

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `KANDEV_DB_PATH` | `./kandev.db` | SQLite database |
| `PORT` | `8080` | HTTP port |
| `KANDEV_LOGGING_LEVEL` | `info` | Log level |
| `KANDEV_CREDENTIALS_FILE` | - | Agent credentials |

### Agent Types

**Augment Agent** (`augment-agent`):
- Image: `kandev/augment-agent:latest`
- Memory: 4GB, CPU: 2 cores, Timeout: 1 hour
- Required env: `AUGMENT_SESSION_AUTH`, `TASK_DESCRIPTION`
- Optional env: `AUGGIE_SESSION_ID` (for resumption)

## Development Workflow

### Create and Launch Agent

```bash
# Create board
BOARD_ID=$(curl -s -X POST http://localhost:8080/api/v1/boards \
  -H "Content-Type: application/json" \
  -d '{"name":"My Project"}' | jq -r '.id')

# Create task
TASK_ID=$(curl -s -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{"board_id":"'$BOARD_ID'","title":"Fix bug"}' | jq -r '.id')

# Launch agent
curl -X POST http://localhost:8080/api/v1/agents/launch \
  -H "Content-Type: application/json" \
  -d '{
    "task_id":"'$TASK_ID'",
    "agent_type":"augment-agent",
    "workspace_path":"/path/to/project",
    "env":{
      "AUGMENT_SESSION_AUTH":"'"$(cat ~/.augment/session.json | jq -c)"'",
      "TASK_DESCRIPTION":"Fix the bug"
    }
  }'
```

### Session Resumption

```bash
# Get session ID from completed task
SESSION_ID=$(curl -s http://localhost:8080/api/v1/tasks/$TASK_ID | \
  jq -r '.metadata.auggie_session_id')

# Launch with resumption
curl -X POST http://localhost:8080/api/v1/agents/launch \
  -H "Content-Type: application/json" \
  -d '{
    "task_id":"'$NEW_TASK_ID'",
    "agent_type":"augment-agent",
    "workspace_path":"/path/to/project",
    "env":{
      "AUGMENT_SESSION_AUTH":"'"$(cat ~/.augment/session.json | jq -c)"'",
      "AUGGIE_SESSION_ID":"'$SESSION_ID'",
      "TASK_DESCRIPTION":"Follow-up question"
    }
  }'
```

## Database

**Location**: `./kandev.db` (SQLite with WAL mode)

**Schema** (`backend/internal/task/repository/sqlite.go`):
- `boards` - Board records
- `columns` - Column definitions with state mapping
- `tasks` - Task records with JSON metadata

**Inspect**:
```bash
sqlite3 kandev.db
> .tables
> SELECT * FROM tasks;
```

**Backup**:
```bash
cp kandev.db kandev.db.backup
```

## Testing

```bash
# All tests
make test

# Backend only (216 tests)
make test-backend

# Web only
make test-web
```

## Troubleshooting

### Backend won't start
- Port 8080 in use: `lsof -i :8080`
- Docker not running: `docker ps`
- DB permissions: `ls -la kandev.db`

### Web app won't start
- Port 3000 in use: `lsof -i :3000`
- Missing deps: `cd apps/web && npm install`
- Node version: `node --version` (need 18+)

### Agents won't launch
- Docker accessible: `docker info`
- Image exists: `docker images | grep kandev`
- Credentials: `cat ~/.augment/session.json`
- Container logs: `docker logs <container-id>`

## Key Files

| File | Purpose |
|------|---------|
| `Makefile` | Build orchestration |
| `ARCHITECTURE.md` | This file |
| `AGENTS.md` | Agent protocol details |
| `docs/openapi.yaml` | REST API spec |
| `backend/cmd/kandev/main.go` | Entry point |
| `backend/internal/agent/registry/defaults.go` | Agent type registry |
| `backend/internal/task/repository/sqlite.go` | Database schema |
| `apps/web/app/page.tsx` | Web app home |

## Contributing

1. Update docs when changing API (AGENTS.md + openapi.yaml)
2. Run tests: `make test`
3. Format code: `make fmt`
4. Test full stack: `make dev-backend` + `make dev-web`

---

**For Agent Protocol Details**: See `AGENTS.md`
**For API Reference**: See `docs/openapi.yaml`
**Last Updated**: 2026-01-10
