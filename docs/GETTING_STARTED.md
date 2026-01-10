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

### Launch an Agent

```bash
# Get your Augment session auth
SESSION_AUTH=$(cat ~/.augment/session.json)

# Launch an agent
curl -X POST http://localhost:8080/api/v1/agents/launch \
  -H "Content-Type: application/json" \
  -d '{
    "task_id": "{task_id}",
    "agent_type": "augment-agent",
    "workspace_path": "/path/to/your/project",
    "env": {
      "AUGMENT_SESSION_AUTH": "'$SESSION_AUTH'",
      "TASK_DESCRIPTION": "Analyze this codebase"
    }
  }'
```

### Check Agent Status

```bash
curl http://localhost:8080/api/v1/agents/{agent_id}/status
```

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
│   │   ├── api/          # Agent HTTP handlers
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
├── dockerfiles/          # Agent Dockerfiles
│   └── augment-agent/
├── bin/                  # Built binaries (gitignored)
├── Makefile
└── NEXT_STEPS.md         # Development roadmap
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
