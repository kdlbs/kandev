# Kandev - Kanban Board with AI Agent Orchestration

A Kanban board system that integrates AI agent orchestration capabilities with **real-time Agent Communication Protocol (ACP) streaming**, allowing tasks to be automatically executed by AI agents running in isolated Docker containers.

## Overview

Kandev combines traditional Kanban task management with automated AI agent execution and real-time feedback streaming. Users create tasks on a Kanban board, and when configured with an AI agent type, the system automatically launches Docker containers to execute those tasks against specified code repositories. **Real-time progress, logs, and results are streamed from agents to the frontend via WebSocket connections.**

### Key Features

- **Kanban Board Management**: Create and manage tasks across multiple boards with columns
- **AI Agent Orchestration**: Launch AI agents (Augment Agent) to execute tasks
- **Real-Time ACP Streaming**: Live progress updates, logs, and results streamed from agents
- **Session Resumption**: Agents can resume previous conversations using session IDs
- **Docker Isolation**: Each agent runs in an isolated Docker container with resource limits
- **Repository Integration**: Agents can work on code repositories with host credentials
- **Host Credential Mounting**: Secure read-only mounting of SSH keys and Git credentials
- **SQLite Persistence**: Lightweight file-based database for task and board storage
- **Local Deployment**: Designed for client machines with minimal dependencies

## Architecture

The system runs as a **unified Go binary** that includes:

- **Task Service**: Manages tasks, boards, and columns with SQLite persistence
- **Agent Manager**: Manages Docker container lifecycle and ACP message streaming
- **Orchestrator**: Coordinates agent launches and monitors execution
- **WebSocket API**: Real-time bidirectional communication for all operations (Port 8080)

### Agent Communication Protocol (ACP)

ACP is a JSON-based streaming protocol that enables real-time communication between AI agents and the backend:

```
Agent Container (stdout) → Agent Manager → Event Bus → API
```

**Message Types**: `progress`, `log`, `result`, `error`, `status`, `heartbeat`

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for detailed architecture documentation.

## Supported Agent Types

1. **Augment Agent** (`augment-agent`)
   - Uses Auggie CLI for code analysis, generation, debugging, refactoring
   - Native ACP support via stdout JSON streaming
   - Session resumption for multi-turn conversations

## Technology Stack

- **Backend**: Go 1.21+
- **Database**: SQLite (with WAL mode)
- **Container Runtime**: Docker
- **Web Framework**: Gin
- **Logging**: Zap (structured logging)
- **Deployment**: Single binary for client machines

## Quick Start

### Prerequisites

- Go 1.21 or higher
- Docker
- Augment CLI credentials (`~/.augment/session.json`)

### Build and Run

#### NPX Launcher (recommended)

```bash
npx kandev
```

This downloads prebuilt backend + frontend bundles and starts them locally.

#### From Source

```bash
cd apps/backend

# Build the binary
make build

# Run the server (creates kandev.db in current directory)
./bin/kandev

# Or specify a custom database path
KANDEV_DB_PATH=/path/to/kandev.db ./bin/kandev
```

### WebSocket API Usage

**Connection**: `ws://localhost:8080/ws`

#### Message Envelope

```json
{
  "id": "correlation-uuid",
  "type": "request|response|notification|error",
  "action": "action.name",
  "payload": {},
  "timestamp": "2026-01-10T10:30:00Z"
}
```

#### Key Message Actions

- `board.create` - Create board
- `board.list` - List boards
- `column.create` - Create column
- `task.create` - Create task
- `agent.launch` - Launch agent
- `agent.status` - Get agent status
- `orchestrator.start` - Start task execution

#### Example Usage (JavaScript)

```javascript
const ws = new WebSocket('ws://localhost:8080/ws');

// Create a board
ws.send(JSON.stringify({
  id: crypto.randomUUID(),
  type: 'request',
  action: 'board.create',
  payload: { name: 'My Project', description: 'Project tasks' }
}));

// Create a task
ws.send(JSON.stringify({
  id: crypto.randomUUID(),
  type: 'request',
  action: 'task.create',
  payload: {
    board_id: '{board_id}',
    column_id: '{column_id}',
    title: 'Fix login bug',
    description: 'Users cannot login'
  }
}));

// Launch an agent
ws.send(JSON.stringify({
  id: crypto.randomUUID(),
  type: 'request',
  action: 'agent.launch',
  payload: {
    task_id: '{task_id}',
    agent_type: 'augment-agent',
    workspace_path: '/path/to/project',
    env: {
      AUGMENT_SESSION_AUTH: '...',
      TASK_DESCRIPTION: 'What is 2+2?'
    }
  }
}));

// Handle responses and notifications
ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  if (msg.type === 'response') {
    console.log('Response:', msg.payload);
  } else if (msg.type === 'notification') {
    // Real-time updates: acp.progress, acp.log, task.updated, etc.
    console.log('Notification:', msg.action, msg.payload);
  }
};
```

See [apps/backend/NEXT_STEPS.md](apps/backend/NEXT_STEPS.md) for detailed API reference and next steps.

## Project Structure

```
kandev/
├── apps/
│   ├── backend/              # Go backend services
│   │   ├── cmd/kandev/       # Unified binary entry point
│   │   ├── internal/
│   │   │   ├── agent/        # Agent manager (docker, lifecycle, registry, api)
│   │   │   ├── task/         # Task service (models, repository, service, api)
│   │   │   ├── orchestrator/ # Orchestrator service
│   │   │   ├── events/       # In-memory event bus
│   │   │   └── common/       # Shared utilities (logger, config)
│   │   ├── pkg/acp/          # ACP protocol types
│   │   ├── dockerfiles/      # Agent container Dockerfiles
│   │   └── Makefile
│   └── web/                  # Frontend application
├── docs/                     # Architecture and planning docs
├── migrations/               # Database migrations (future)
├── deployments/              # Deployment configs
└── scripts/                  # Utility scripts (including e2e-test.sh)
```

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `KANDEV_DB_PATH` | `./kandev.db` | SQLite database file path |
| `PORT` | `8080` | HTTP server port |

## Development

```bash
cd apps/backend

# Build
make build

# Run tests
make test

# Format code
go fmt ./...

# Lint
go vet ./...
```

## Pre-commit Hooks

We use `pre-commit` to run the backend tests, web tests, and web lint before commits.

### Install

```bash
# Install pre-commit (https://pre-commit.com/#install)
pipx install pre-commit

# Install git hooks
pre-commit install
```

### Run Manually

```bash
# Run all hooks once
pre-commit run --all-files
```

### Hooks Config

The hooks are defined in `.pre-commit-config.yaml` and run these Makefile targets:

- `make test-backend`
- `make test-web`
- `make lint-web`

## Project Status

✅ **Core Features Working**

- [x] SQLite persistence for boards, columns, and tasks
- [x] Agent lifecycle management with Docker
- [x] ACP message streaming from containers
- [x] Session resumption for multi-turn agent conversations
- [x] WebSocket-first API for all operations
- [x] Real-time bidirectional communication

🚧 **In Progress**

- [ ] Authentication and authorization
- [ ] Frontend UI

See [apps/backend/NEXT_STEPS.md](apps/backend/NEXT_STEPS.md) for the complete roadmap.

## License

[To be determined]

## Acknowledgments

Built with Go and inspired by modern DevOps practices and AI-assisted development workflows.
