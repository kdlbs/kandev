# Getting Started with Kandev Backend Development

> **Note:** This guide reflects the current unified binary implementation.
> The original microservices architecture is documented in IMPLEMENTATION_PLAN.md for future reference.

## Prerequisites

### Required Software

1. **Go 1.21 or higher**
   ```bash
   # Check version
   go version

   # Install from https://go.dev/dl/
   ```

2. **Docker**
   ```bash
   # Check version
   docker --version

   # Install from https://docs.docker.com/get-docker/
   ```

3. **Augment CLI Credentials**
   ```bash
   # Verify Augment session exists
   cat ~/.augment/session.json
   ```

4. **Git with SSH Keys Configured** (for repository access)
   ```bash
   git --version

   # Verify SSH keys exist
   ls -la ~/.ssh/
   ```

---

## Quick Start

### Step 1: Build the Backend

```bash
cd apps/backend

# Build the binary
make build

# This creates ./bin/kandev
```

### Step 2: Run the Server

```bash
# Run with default settings (creates kandev.db in current directory)
./bin/kandev

# Or specify a custom database path
KANDEV_DB_PATH=/path/to/kandev.db ./bin/kandev
```

The server starts on port 8080 by default.

### Step 3: Verify It's Running

The backend uses a WebSocket-first architecture. Connect to `ws://localhost:8080/ws` to interact with the API.

#### Using websocat (Command Line)

```bash
# Install websocat
# macOS: brew install websocat
# Linux: cargo install websocat

# Health check
echo '{"id":"1","type":"request","action":"health.check","payload":{}}' | websocat ws://localhost:8080/ws

# Should return a response with status "ok"
```

#### Using JavaScript

```javascript
const ws = new WebSocket('ws://localhost:8080/ws');

ws.onopen = () => {
  ws.send(JSON.stringify({
    id: '1',
    type: 'request',
    action: 'health.check',
    payload: {}
  }));
};

ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  console.log('Health:', msg); // { id: '1', type: 'response', action: 'health.check', payload: { status: 'ok' }, ... }
};
```

---

## WebSocket Message Protocol

### Connection

Connect to: `ws://localhost:8080/ws`

### Message Envelope

All messages use this envelope format:

```json
{
  "id": "correlation-uuid",
  "type": "request|response|notification|error",
  "action": "action.name",
  "payload": {},
  "timestamp": "2026-01-10T10:30:00Z"
}
```

### Message Types

- `request` - Client request to the server
- `response` - Server response to a request
- `notification` - Server push notification (e.g., agent status updates)
- `error` - Error response

---

## Basic Usage

### Create a Board

#### websocat

```bash
echo '{"id":"1","type":"request","action":"board.create","payload":{"name":"My Project","description":"Project tasks"}}' | websocat ws://localhost:8080/ws
```

#### JavaScript

```javascript
ws.send(JSON.stringify({
  id: '1',
  type: 'request',
  action: 'board.create',
  payload: { name: 'My Project', description: 'Project tasks' }
}));
```

### List Boards

#### websocat

```bash
echo '{"id":"2","type":"request","action":"board.list","payload":{}}' | websocat ws://localhost:8080/ws
```

#### JavaScript

```javascript
ws.send(JSON.stringify({
  id: '2',
  type: 'request',
  action: 'board.list',
  payload: {}
}));
```

### Create a Column

#### websocat

```bash
echo '{"id":"3","type":"request","action":"column.create","payload":{"board_id":"<board_id>","name":"To Do"}}' | websocat ws://localhost:8080/ws
```

#### JavaScript

```javascript
ws.send(JSON.stringify({
  id: '3',
  type: 'request',
  action: 'column.create',
  payload: { board_id: '<board_id>', name: 'To Do' }
}));
```

### Create a Task

#### websocat

```bash
echo '{"id":"4","type":"request","action":"task.create","payload":{"board_id":"<board_id>","column_id":"<column_id>","title":"Fix login bug","description":"Users cannot login"}}' | websocat ws://localhost:8080/ws
```

#### JavaScript

```javascript
ws.send(JSON.stringify({
  id: '4',
  type: 'request',
  action: 'task.create',
  payload: {
    board_id: '<board_id>',
    column_id: '<column_id>',
    title: 'Fix login bug',
    description: 'Users cannot login'
  }
}));
```

### Launch an Agent for a Task

#### websocat

```bash
echo '{"id":"5","type":"request","action":"agent.launch","payload":{"task_id":"<task_id>","agent_type":"augment-agent","repository_url":"/path/to/your/project"}}' | websocat ws://localhost:8080/ws
```

#### JavaScript

```javascript
ws.send(JSON.stringify({
  id: '5',
  type: 'request',
  action: 'agent.launch',
  payload: {
    task_id: '<task_id>',
    agent_type: 'augment-agent',
    repository_url: '/path/to/your/project'
  }
}));
```

### Start Task via Orchestrator

#### websocat

```bash
echo '{"id":"6","type":"request","action":"orchestrator.start","payload":{"task_id":"<task_id>"}}' | websocat ws://localhost:8080/ws
```

#### JavaScript

```javascript
ws.send(JSON.stringify({
  id: '6',
  type: 'request',
  action: 'orchestrator.start',
  payload: { task_id: '<task_id>' }
}));
```

### Check Agent Status

#### websocat

```bash
echo '{"id":"7","type":"request","action":"agent.status","payload":{"agent_id":"<agent_id>"}}' | websocat ws://localhost:8080/ws
```

#### JavaScript

```javascript
ws.send(JSON.stringify({
  id: '7',
  type: 'request',
  action: 'agent.status',
  payload: { agent_id: '<agent_id>' }
}));
```

### Interactive Mode

For interactive testing, connect in interactive mode:

```bash
websocat ws://localhost:8080/ws
# Then type messages manually, one per line:
{"id":"1","type":"request","action":"health.check","payload":{}}
{"id":"2","type":"request","action":"board.list","payload":{}}
```

---

## Running the End-to-End Test

The project includes a comprehensive E2E test script:

```bash
# Build the agent Docker image first
cd apps/backend/dockerfiles/augment-agent
docker build -t kandev/augment-agent:latest .

# Run the E2E test
cd /path/to/kandev
./scripts/e2e-test.sh
```

The E2E test uses WebSocket connections to:
1. Start the server with a fresh database
2. Connect via WebSocket to `ws://localhost:8080/ws`
3. Create a board, column, and task using WebSocket messages
4. Start the task via the orchestrator (`orchestrator.start`)
5. Verify the agent container starts
6. Check that ACP session is created
7. Receive real-time agent status notifications
8. Wait for agent to complete

---

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `KANDEV_DB_PATH` | `./kandev.db` | SQLite database file path |
| `PORT` | `8080` | HTTP server port |

---

## Development Workflow

### Build

```bash
cd apps/backend
make build
```

### Run Tests

```bash
cd apps/backend
make test

# Or with verbose output
go test -v ./...
```

### Code Formatting

```bash
go fmt ./...
go vet ./...
```

### View Database

```bash
# SQLite CLI
sqlite3 apps/backend/kandev.db

# List tables
.tables

# View boards
SELECT * FROM boards;

# View tasks
SELECT * FROM tasks;
```

---

## Project Structure

```
kandev/
├── apps/
│   ├── backend/              # Go backend services
│   │   ├── cmd/kandev/       # Main entry point
│   │   │   └── main.go
│   │   ├── internal/
│   │   │   ├── agent/        # Agent management
│   │   │   │   ├── acp/      # ACP session management
│   │   │   │   ├── api/      # Agent HTTP handlers
│   │   │   │   ├── credentials/  # Credential providers
│   │   │   │   ├── docker/   # Docker client
│   │   │   │   ├── lifecycle/# Container lifecycle
│   │   │   │   └── registry/ # Agent type registry
│   │   │   ├── task/         # Task management
│   │   │   │   ├── api/      # Task HTTP handlers
│   │   │   │   ├── models/   # Data models
│   │   │   │   ├── repository/  # SQLite/Memory storage
│   │   │   │   └── service/  # Business logic
│   │   │   ├── orchestrator/ # Orchestration logic
│   │   │   ├── events/       # In-memory event bus
│   │   │   └── common/       # Shared utilities
│   │   │       └── logger/   # Zap logger
│   │   ├── pkg/acp/          # ACP protocol types
│   │   │   └── jsonrpc/      # JSON-RPC 2.0 client
│   │   ├── dockerfiles/      # Agent Dockerfiles
│   │   │   └── augment-agent/
│   │   ├── bin/              # Built binaries (gitignored)
│   │   └── Makefile
│   └── web/                  # Frontend application
├── scripts/
│   └── e2e-test.sh           # End-to-end test script
└── docs/                     # Documentation
```

---

## Troubleshooting

### Docker Permission Issues

```bash
# Add user to docker group (Linux)
sudo usermod -aG docker $USER
newgrp docker
```

### Port Already in Use

```bash
# Find process using port 8080
lsof -i :8080

# Kill it
kill -9 <PID>
```

### Database Issues

```bash
# Delete and recreate database
rm apps/backend/kandev.db
./bin/kandev  # Will create fresh database
```

### Agent Container Issues

```bash
# List running containers
docker ps -a --filter "label=kandev.managed=true"

# View container logs
docker logs <container_id>

# Clean up old containers
docker rm $(docker ps -a -q --filter "label=kandev.managed=true")
```

---

## Next Steps

See [NEXT_STEPS.md](../apps/backend/NEXT_STEPS.md) for the development roadmap and immediate next steps.
