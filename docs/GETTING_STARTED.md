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
cd backend

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

```bash
# Health check
curl http://localhost:8080/health

# Should return: {"status":"ok"}
```

---

## Basic Usage

### Create a Board

```bash
curl -X POST http://localhost:8080/api/v1/boards \
  -H "Content-Type: application/json" \
  -d '{"name": "My Project", "description": "Project tasks"}'
```

### Create a Column

```bash
curl -X POST http://localhost:8080/api/v1/boards/{board_id}/columns \
  -H "Content-Type: application/json" \
  -d '{"name": "To Do"}'
```

### Create a Task

```bash
curl -X POST http://localhost:8080/api/v1/boards/{board_id}/columns/{column_id}/tasks \
  -H "Content-Type: application/json" \
  -d '{"title": "Fix login bug", "description": "Users cannot login"}'
```

### Create a Task with Agent

```bash
# Create a task that will be executed by an AI agent
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Analyze codebase",
    "description": "Provide an overview of the project structure",
    "board_id": "{board_id}",
    "column_id": "{column_id}",
    "agent_type": "augment-agent",
    "repository_url": "/path/to/your/project"
  }'
```

### Start Task via Orchestrator

```bash
# Start the task - this launches the agent container
curl -X POST http://localhost:8080/api/v1/orchestrator/tasks/{task_id}/start \
  -H "Content-Type: application/json"
```

### Check Agent Status

```bash
curl http://localhost:8080/api/v1/agents/{agent_id}/status

# List all agents
curl http://localhost:8080/api/v1/agents
```

---

## Running the End-to-End Test

The project includes a comprehensive E2E test script:

```bash
# Build the agent Docker image first
cd backend/dockerfiles/augment-agent
docker build -t kandev/augment-agent:latest .

# Run the E2E test
cd /path/to/kandev
./scripts/e2e-test.sh
```

The E2E test:
1. Starts the server with a fresh database
2. Creates a board, column, and task
3. Starts the task via the orchestrator
4. Verifies the agent container starts
5. Checks that ACP session is created
6. Verifies permission requests are handled
7. Waits for agent to complete

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
cd backend
make build
```

### Run Tests

```bash
cd backend
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
sqlite3 backend/kandev.db

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
backend/
├── cmd/kandev/           # Main entry point
│   └── main.go
├── internal/
│   ├── agent/            # Agent management
│   │   ├── acp/          # ACP session management
│   │   ├── api/          # Agent HTTP handlers
│   │   ├── credentials/  # Credential providers
│   │   ├── docker/       # Docker client
│   │   ├── lifecycle/    # Container lifecycle
│   │   └── registry/     # Agent type registry
│   ├── task/             # Task management
│   │   ├── api/          # Task HTTP handlers
│   │   ├── models/       # Data models
│   │   ├── repository/   # SQLite/Memory storage
│   │   └── service/      # Business logic
│   ├── orchestrator/     # Orchestration logic
│   ├── events/           # In-memory event bus
│   └── common/           # Shared utilities
│       └── logger/       # Zap logger
├── pkg/acp/              # ACP protocol types
│   └── jsonrpc/          # JSON-RPC 2.0 client
├── dockerfiles/          # Agent Dockerfiles
│   └── augment-agent/
├── bin/                  # Built binaries (gitignored)
├── Makefile
└── NEXT_STEPS.md         # Development roadmap
scripts/
└── e2e-test.sh           # End-to-end test script
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
rm backend/kandev.db
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

See [NEXT_STEPS.md](../backend/NEXT_STEPS.md) for the development roadmap and immediate next steps.
