# Kandev Architecture

> Quick reference for developers and AI agents working on this codebase.

## System Overview

Kandev is a kanban board with AI agent orchestration. Three main components:

```
┌──────────────────────────────────────────────────────────┐
│  Web UI (Next.js 16 + React 19)                         │
│  http://localhost:3000                                   │
└───────────────────────┬──────────────────────────────────┘
                        │ WebSocket (ws://localhost:8080/ws)
┌───────────────────────▼──────────────────────────────────┐
│  Backend (Go + Gin + SQLite)                             │
│  ws://localhost:8080/ws                                  │
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

### Backend (`apps/backend/`)
- **Language**: Go 1.21+
- **Framework**: Gin, Gorilla WebSocket
- **Protocol**: WebSocket (message-based API)
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
│   ├── web/              # Next.js web application
│   │   ├── app/          # Pages (App Router)
│   │   ├── components/   # React components
│   │   └── lib/          # Utilities
│   └── backend/          # Go backend services
│       ├── cmd/kandev/   # Main entry point
│       ├── internal/
│       │   ├── agent/    # Agent manager (Docker, ACP, lifecycle)
│       │   ├── task/     # Task service (CRUD, SQLite)
│       │   ├── orchestrator/  # Task execution coordinator
│   │   ├── events/       # Event bus (in-memory/NATS)
│   │   ├── gateway/      # WebSocket gateway
│   │   └── common/       # Shared utilities (config, logger, db)
│   └── pkg/
│       ├── acp/          # ACP protocol types
│       └── api/v1/       # API models
└── docs/
    └── asyncapi.yaml     # WebSocket API specification
```

## Data Flow

1. User creates task in web UI
2. WebSocket `task.create` message → SQLite
3. User launches agent for task
4. WebSocket `agent.launch` message → Backend spawns Docker container
5. Agent streams progress via ACP
6. Events published to event bus
7. WebSocket notifications pushed to UI (`task.updated`, `agent.updated`)

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

## WebSocket Message Types

**Connection**: `ws://localhost:8080/ws`

### Message Envelope
```json
{
  "id": "correlation-uuid",
  "type": "request|response|notification|error",
  "action": "action.name",
  "payload": {},
  "timestamp": "2026-01-10T10:30:00Z"
}
```

### Request/Response Pattern
- Requests include `id` for correlation
- Responses include matching `id`
- Notifications have no `id` (one-way)

### Message Actions

| Action | Description |
|--------|-------------|
| `health.check` | Health check |
| `board.list` | List boards |
| `board.create` | Create board |
| `board.get` | Get board |
| `board.update` | Update board |
| `board.delete` | Delete board |
| `column.list` | List columns |
| `column.create` | Create column |
| `task.list` | List tasks |
| `task.create` | Create task |
| `task.get` | Get task |
| `task.update` | Update task |
| `task.delete` | Delete task |
| `task.move` | Move task |
| `task.state` | Update state |
| `task.subscribe` | Subscribe to task updates |
| `task.unsubscribe` | Unsubscribe from task updates |
| `agent.list` | List agents |
| `agent.launch` | Launch agent |
| `agent.status` | Get status |
| `agent.logs` | Get logs |
| `agent.stop` | Stop agent |
| `agent.prompt` | Send prompt |
| `agent.cancel` | Cancel agent |
| `agent.session` | Get session |
| `agent.types` | List types |
| `orchestrator.status` | Get status |
| `orchestrator.queue` | Get queue |
| `orchestrator.trigger` | Trigger task |
| `orchestrator.start` | Start task |
| `orchestrator.stop` | Stop task |
| `orchestrator.prompt` | Send prompt |
| `orchestrator.complete` | Complete task |

### Notification Types (Server → Client)

| Action | Description |
|--------|-------------|
| `acp.progress` | Agent progress update |
| `acp.log` | Agent log message |
| `acp.result` | Agent result |
| `acp.error` | Agent error |
| `acp.status` | Agent status change |
| `acp.heartbeat` | Keep-alive |
| `task.updated` | Task was updated |
| `agent.updated` | Agent status changed |

### Error Response Format
```json
{
  "id": "correlation-uuid",
  "type": "error",
  "action": "original.action",
  "payload": {
    "code": "ERROR_CODE",
    "message": "Human readable message",
    "details": {}
  },
  "timestamp": "2026-01-10T10:30:00Z"
}
```

**Full API**: See `docs/asyncapi.yaml`

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

### WebSocket Usage Example (JavaScript)

```javascript
const ws = new WebSocket('ws://localhost:8080/ws');

// Send request
ws.send(JSON.stringify({
  id: crypto.randomUUID(),
  type: 'request',
  action: 'board.create',
  payload: { name: 'My Project' }
}));

// Handle messages
ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  if (msg.type === 'response') {
    console.log('Response:', msg.payload);
  } else if (msg.type === 'notification') {
    console.log('Notification:', msg.action, msg.payload);
  }
};
```

### Create and Launch Agent

```javascript
// Create board
ws.send(JSON.stringify({
  id: crypto.randomUUID(),
  type: 'request',
  action: 'board.create',
  payload: { name: 'My Project' }
}));

// Create task (after receiving board.create response)
ws.send(JSON.stringify({
  id: crypto.randomUUID(),
  type: 'request',
  action: 'task.create',
  payload: { board_id: BOARD_ID, title: 'Fix bug' }
}));

// Launch agent (after receiving task.create response)
ws.send(JSON.stringify({
  id: crypto.randomUUID(),
  type: 'request',
  action: 'agent.launch',
  payload: {
    task_id: TASK_ID,
    agent_type: 'augment-agent',
    workspace_path: '/path/to/project',
    env: {
      AUGMENT_SESSION_AUTH: sessionAuth,
      TASK_DESCRIPTION: 'Fix the bug'
    }
  }
}));
```

### Session Resumption

```javascript
// Get session ID from completed task
ws.send(JSON.stringify({
  id: crypto.randomUUID(),
  type: 'request',
  action: 'agent.session',
  payload: { task_id: TASK_ID }
}));

// Launch with resumption (after receiving session response)
ws.send(JSON.stringify({
  id: crypto.randomUUID(),
  type: 'request',
  action: 'agent.launch',
  payload: {
    task_id: NEW_TASK_ID,
    agent_type: 'augment-agent',
    workspace_path: '/path/to/project',
    env: {
      AUGMENT_SESSION_AUTH: sessionAuth,
      AUGGIE_SESSION_ID: SESSION_ID,
      TASK_DESCRIPTION: 'Follow-up question'
    }
  }
}));
```

## Database

**Location**: `./kandev.db` (SQLite with WAL mode)

**Schema** (`apps/backend/internal/task/repository/sqlite.go`):
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
| `docs/asyncapi.yaml` | WebSocket API spec |
| `apps/backend/cmd/kandev/main.go` | Entry point |
| `apps/backend/internal/agent/registry/defaults.go` | Agent type registry |
| `apps/backend/internal/task/repository/sqlite.go` | Database schema |
| `apps/web/app/page.tsx` | Web app home |

## Contributing

1. Update docs when changing API (AGENTS.md + asyncapi.yaml)
2. Run tests: `make test`
3. Format code: `make fmt`
4. Test full stack: `make dev-backend` + `make dev-web`

---

**For Agent Protocol Details**: See `AGENTS.md`
**For API Reference**: See `docs/asyncapi.yaml`
**Last Updated**: 2026-01-10
