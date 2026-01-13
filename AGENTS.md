# Kandev Agent Protocol

> **For Agent Developers**: This document explains how to build agents that integrate with Kandev using the Agent Communication Protocol (ACP).
>
> **For System Architecture**: See [ARCHITECTURE.md](ARCHITECTURE.md)
> **For WebSocket API**: See [docs/asyncapi.yaml](docs/asyncapi.yaml)

## Overview

Kandev uses a **WebSocket-only architecture** for all client-backend communication. Agents run in Docker containers with **agentctl** as a sidecar process. The agent communicates via **ACP (Agent Communication Protocol)** - a JSON-RPC 2.0 protocol over stdin/stdout. ACP messages are streamed to clients via WebSocket notifications.

```
Client (WS)              Backend (kandev)              agentctl (HTTP)           Agent Process
    │                         │                             │                          │
    │  orchestrator.start     │                             │                          │
    ├────────────────────────►│  POST /api/v1/start         │                          │
    │                         ├────────────────────────────►│  spawn auggie --acp      │
    │                         │                             ├─────────────────────────►│
    │                         │  POST /api/v1/acp/call      │  initialize              │
    │                         ├────────────────────────────►├─────────────────────────►│
    │                         │◄────────────────────────────┤◄─────────────────────────┤
    │  acp.progress           │  WS /api/v1/acp/stream      │  session/update          │
    │◄────────────────────────┤◄────────────────────────────┤◄─────────────────────────┤
    │                         │                             │                          │
```

## Architecture

### WebSocket Gateway

All client communication uses a single WebSocket endpoint at `ws://localhost:8080/ws`. The backend:

1. **Receives orchestrator requests** (`orchestrator.start`, `orchestrator.stop`, `orchestrator.prompt`)
2. **Manages agent containers** via Docker and agentctl HTTP API
3. **Streams ACP messages** to subscribed clients as WebSocket notifications
4. **Publishes events** via internal event bus for real-time updates

### agentctl

`agentctl` is an HTTP server that runs inside each agent container as a sidecar process. It:

1. **Manages the agent subprocess** (e.g., `auggie --acp`)
2. **Provides HTTP API** for control operations (start, stop, status)
3. **Relays ACP messages** between the backend and agent's stdin/stdout
4. **Streams output** via WebSocket for real-time monitoring

### Communication Flow

1. Client sends `orchestrator.start` via WebSocket
2. Backend creates container with agentctl as entrypoint
3. agentctl starts HTTP server on port 9999
4. Backend waits for agentctl to be ready (`GET /health`)
5. Backend starts agent process (`POST /api/v1/start`)
6. Backend connects to agentctl WebSocket (`GET /api/v1/acp/stream`)
7. Backend sends ACP messages via HTTP (`POST /api/v1/acp/call`)
8. ACP updates are streamed to subscribed clients as `acp.*` notifications

### agentctl HTTP API (Internal)

These endpoints are used by the backend to communicate with agent containers:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/api/v1/status` | GET | Get agent status |
| `/api/v1/start` | POST | Start agent process |
| `/api/v1/stop` | POST | Stop agent process |
| `/api/v1/acp/send` | POST | Send ACP message (fire and forget) |
| `/api/v1/acp/call` | POST | Send ACP request, wait for response |
| `/api/v1/acp/stream` | GET | WebSocket for ACP message streaming |
| `/api/v1/output` | GET | Get buffered output |
| `/api/v1/output/stream` | GET | WebSocket for real-time output |

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `AGENTCTL_PORT` | 9999 | HTTP server port |
| `AGENTCTL_AGENT_COMMAND` | `auggie --acp` | Command to run agent |
| `AGENTCTL_AUTO_START` | false | Auto-start agent on startup |
| `AGENTCTL_LOG_LEVEL` | info | Logging level |

---

## Agent Communication Protocol (ACP)

### Protocol Basics

- **Transport**: JSON-RPC 2.0 over stdin/stdout
- **Format**: Newline-delimited JSON
- **Direction**: Bidirectional (request/response + notifications)

### Message Types

**Backend → Agent (Requests):**
- `initialize` - Establish connection and capabilities
- `session/new` - Create new session
- `session/load` - Resume existing session
- `session/prompt` - Send task/prompt to execute
- `session/cancel` - Cancel current operation

**Agent → Backend (Notifications):**
- `session/update` - Progress updates, content, tool calls, completion

---

## Message Specifications

### 1. Initialize Connection

**Backend → Agent:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "protocolVersion": "1.0",
    "clientInfo": {
      "name": "kandev",
      "version": "0.1.0"
    },
    "capabilities": {
      "streaming": true
    }
  }
}
```

**Agent → Backend:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "protocolVersion": "1.0",
    "serverInfo": {
      "name": "augment-agent",
      "version": "1.0.0"
    },
    "capabilities": {
      "streaming": true,
      "sessionResumption": true
    }
  }
}
```

### 2. Create New Session

**Backend → Agent:**
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "session/new",
  "params": {}
}
```

**Agent → Backend:**
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "sessionId": "abc123def456"
  }
}
```

### 3. Resume Existing Session

**Backend → Agent:**
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "session/load",
  "params": {
    "sessionId": "abc123def456"
  }
}
```

**Agent → Backend:**
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "sessionId": "abc123def456",
    "loaded": true
  }
}
```

### 4. Send Prompt

**Backend → Agent:**
```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "method": "session/prompt",
  "params": {
    "message": "Fix the null pointer bug in the login function"
  }
}
```

**Agent → Backend (immediate response):**
```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "result": {
    "accepted": true
  }
}
```

### 5. Progress Updates (Notifications)

**Content Update:**
```json
{
  "jsonrpc": "2.0",
  "method": "session/update",
  "params": {
    "type": "content",
    "data": {
      "text": "I'm analyzing the code in auth/login.go..."
    }
  }
}
```

**Tool Call:**
```json
{
  "jsonrpc": "2.0",
  "method": "session/update",
  "params": {
    "type": "toolCall",
    "data": {
      "toolName": "file_edit",
      "status": "running",
      "args": {
        "file": "auth/login.go",
        "line": 42
      }
    }
  }
}
```

**Completion:**
```json
{
  "jsonrpc": "2.0",
  "method": "session/update",
  "params": {
    "type": "complete",
    "data": {
      "sessionId": "abc123def456",
      "success": true,
      "summary": "Fixed null pointer by adding nil check"
    }
  }
}
```

**Error:**
```json
{
  "jsonrpc": "2.0",
  "method": "session/update",
  "params": {
    "type": "error",
    "data": {
      "message": "Failed to compile after changes",
      "details": "syntax error on line 45"
    }
  }
}
```

### 6. Cancel Operation

**Backend → Agent (notification, no response expected):**
```json
{
  "jsonrpc": "2.0",
  "method": "session/cancel",
  "params": {
    "reason": "User requested cancellation"
  }
}
```

---

## Available Agent Types

### Augment Agent (`augment-agent`)

Primary AI coding agent powered by Auggie CLI with agentctl sidecar.

**Docker Image**: `kandev/augment-agent:latest`

**Architecture:**
```
Container
├── agentctl (HTTP server on port 9999)
│   └── manages → auggie --acp (subprocess)
└── /workspace (mounted project directory)
```

**Specification:**
- **Working Directory**: `/workspace`
- **Memory**: 4096 MB
- **CPU**: 2.0 cores
- **Timeout**: 1 hour
- **Exposed Port**: 9999 (agentctl HTTP API)
- **Capabilities**: code_generation, code_review, refactoring, testing, shell_execution

**Required Environment Variables:**
```bash
AUGMENT_SESSION_AUTH  # From ~/.augment/session.json
TASK_DESCRIPTION      # The task to execute
```

**Optional Environment Variables:**
```bash
AUGGIE_SESSION_ID     # Resume previous session
AGENTCTL_AUTO_START   # Auto-start agent on container start
```

**Mounted Volumes:**
- `{workspace}` → `/workspace` - Project directory
- `{augment_sessions}` → `/root/.augment/sessions` - Session persistence

**Dockerfile**: `apps/backend/dockerfiles/augment-agent/Dockerfile`

**Building the Image:**
```bash
cd apps/backend
docker build -t kandev/augment-agent:latest -f dockerfiles/augment-agent/Dockerfile .
```

---

## Creating Custom Agents

Custom agents can be created in two ways:

### Option A: With agentctl (Recommended)

Use the multi-stage Dockerfile pattern to include agentctl:

```dockerfile
# Stage 1: Build agentctl
FROM golang:1.23-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o agentctl ./cmd/agentctl

# Stage 2: Final image
FROM ubuntu:22.04

# Install your agent dependencies
RUN apt-get update && apt-get install -y curl jq git && rm -rf /var/lib/apt/lists/*

# Copy agentctl from builder
COPY --from=builder /build/agentctl /usr/local/bin/agentctl

# Copy your agent script
COPY agent.sh /usr/local/bin/agent
RUN chmod +x /usr/local/bin/agent

WORKDIR /workspace
EXPOSE 9999

# Set agentctl as entrypoint
ENV AGENTCTL_AGENT_COMMAND="/usr/local/bin/agent --acp"
ENTRYPOINT ["/usr/local/bin/agentctl"]
```

### Option B: Standalone ACP Agent

For simpler agents that don't need HTTP control:

```dockerfile
FROM ubuntu:22.04

RUN apt-get update && apt-get install -y curl jq git && rm -rf /var/lib/apt/lists/*

COPY agent.sh /usr/local/bin/agent
RUN chmod +x /usr/local/bin/agent

WORKDIR /workspace
ENTRYPOINT ["/usr/local/bin/agent", "--acp"]
```

### Implementing ACP Protocol

Create `agent.sh` that implements JSON-RPC 2.0:

```bash
#!/bin/bash
set -e

# ACP mode - JSON-RPC 2.0
if [ "$1" = "--acp" ]; then
    # Read requests from stdin
    while IFS= read -r line; do
        METHOD=$(echo "$line" | jq -r '.method')
        ID=$(echo "$line" | jq -r '.id')

        case "$METHOD" in
            "initialize")
                echo '{"jsonrpc":"2.0","id":'$ID',"result":{"protocolVersion":"1.0","serverInfo":{"name":"your-agent","version":"1.0.0"},"capabilities":{"streaming":true}}}'
                ;;

            "session/new")
                SESSION_ID=$(uuidgen)
                echo '{"jsonrpc":"2.0","id":'$ID',"result":{"sessionId":"'$SESSION_ID'"}}'
                ;;

            "session/prompt")
                MESSAGE=$(echo "$line" | jq -r '.params.message')

                # Accept prompt
                echo '{"jsonrpc":"2.0","id":'$ID',"result":{"accepted":true}}'

                # Send progress
                echo '{"jsonrpc":"2.0","method":"session/update","params":{"type":"content","data":{"text":"Processing: '$MESSAGE'"}}}'

                # Do work here
                sleep 2

                # Send completion
                echo '{"jsonrpc":"2.0","method":"session/update","params":{"type":"complete","data":{"sessionId":"'$SESSION_ID'","success":true}}}'
                ;;

            "session/cancel")
                exit 0
                ;;
        esac
    done
fi
```

### Register Agent Type

Add to `apps/backend/internal/agent/registry/defaults.go`:

```go
{
    ID:          "your-agent",
    Name:        "Your Custom Agent",
    Description: "Description of what your agent does",
    Image:       "kandev/your-agent:latest",
    WorkingDir:  "/workspace",
    MemoryMB:    2048,
    CPUCores:    1.0,
    Timeout:     30 * time.Minute,
    Capabilities: []string{"custom_capability"},
    Enabled:     true,
}
```

### Step 4: Build and Test

```bash
# Build image
cd apps/backend/dockerfiles/your-agent
docker build -t kandev/your-agent:latest .

# Test locally
docker run -it --rm \
  -v $(pwd):/workspace \
  -e TASK_DESCRIPTION="Test task" \
  kandev/your-agent:latest --acp
```

Test by sending JSON-RPC messages via stdin:

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1.0"}}' | \
  docker run -i --rm kandev/your-agent:latest --acp
```

---

## WebSocket API Integration

All client communication uses a single WebSocket endpoint: `ws://localhost:8080/ws`

### Message Format

```json
{
  "id": "uuid",
  "type": "request|response|notification|error",
  "action": "action.name",
  "payload": { ... },
  "timestamp": "2026-01-10T12:00:00Z"
}
```

### Orchestrator Actions

| Action | Description |
|--------|-------------|
| `orchestrator.status` | Get orchestrator status |
| `orchestrator.queue` | Get queued tasks |
| `orchestrator.start` | Start task execution |
| `orchestrator.stop` | Stop running task |
| `orchestrator.prompt` | Send follow-up prompt |
| `orchestrator.complete` | Mark task complete |
| `orchestrator.trigger` | Queue task for execution |

### Start Task Execution

**Action**: `orchestrator.start`

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "orchestrator.start",
  "payload": {
    "task_id": "uuid",
    "agent_type": "augment-agent",
    "priority": 2
  }
}
```

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "orchestrator.start",
  "payload": {
    "success": true,
    "task_id": "uuid",
    "agent_instance_id": "instance-uuid"
  }
}
```

### Send Follow-up Prompt

**Action**: `orchestrator.prompt`

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "orchestrator.prompt",
  "payload": {
    "task_id": "uuid",
    "prompt": "Please also add unit tests"
  }
}
```

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "orchestrator.prompt",
  "payload": {
    "success": true,
    "task_id": "uuid"
  }
}
```

### Stop Task Execution

**Action**: `orchestrator.stop`

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "orchestrator.stop",
  "payload": {
    "task_id": "uuid",
    "reason": "User requested cancellation",
    "force": false
  }
}
```

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "orchestrator.stop",
  "payload": {
    "success": true,
    "task_id": "uuid"
  }
}
```

### Complete Task

**Action**: `orchestrator.complete`

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "orchestrator.complete",
  "payload": {
    "task_id": "uuid"
  }
}
```

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "orchestrator.complete",
  "payload": {
    "success": true,
    "task_id": "uuid"
  }
}
```

### Get Orchestrator Status

**Action**: `orchestrator.status`

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "orchestrator.status",
  "payload": {}
}
```

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "orchestrator.status",
  "payload": {
    "running": true,
    "active_agents": 2,
    "queued_tasks": 5,
    "max_concurrent": 3
  }
}
```

### ACP Notifications

When subscribed to a task via `task.subscribe`, clients receive real-time ACP notifications:

**Progress Notification:**
```json
{
  "id": "uuid",
  "type": "notification",
  "action": "acp.progress",
  "payload": {
    "task_id": "uuid",
    "type": "progress",
    "data": {
      "progress": 50,
      "stage": "executing",
      "message": "Processing files..."
    },
    "timestamp": "2026-01-10T12:00:00Z"
  }
}
```

**Log Notification:**
```json
{
  "id": "uuid",
  "type": "notification",
  "action": "acp.log",
  "payload": {
    "task_id": "uuid",
    "type": "log",
    "data": {
      "level": "info",
      "message": "Found 5 files to modify"
    },
    "timestamp": "2026-01-10T12:00:00Z"
  }
}
```

**Task Subscription:**

**Action**: `task.subscribe`

```json
{
  "id": "uuid",
  "type": "request",
  "action": "task.subscribe",
  "payload": {
    "task_id": "uuid"
  }
}
```

---

## Session Resumption

Agents can resume previous sessions for follow-up conversations.

### How It Works

1. First task completes, session ID stored in task metadata
2. Retrieve session ID via WebSocket `task.get` action → `metadata.auggie_session_id`
3. Launch new task with `AUGGIE_SESSION_ID` environment variable

### Example

```javascript
// WebSocket connection
const ws = new WebSocket('ws://localhost:8080/ws');

// Get session ID from completed task
ws.send(JSON.stringify({
  id: crypto.randomUUID(),
  type: 'request',
  action: 'task.get',
  payload: { id: previousTaskId }
}));

// Handle response and start with session resumption
ws.onmessage = (event) => {
  const response = JSON.parse(event.data);

  if (response.action === 'task.get') {
    const sessionId = response.payload.metadata.auggie_session_id;

    // Create new task with session ID in metadata
    ws.send(JSON.stringify({
      id: crypto.randomUUID(),
      type: 'request',
      action: 'task.create',
      payload: {
        board_id: boardId,
        column_id: columnId,
        title: 'Follow-up task',
        description: 'What changes did you make in the previous task?',
        agent_type: 'augment-agent',
        metadata: {
          auggie_session_id: sessionId
        }
      }
    }));
  }
};
```

The agent will load the previous context and continue the conversation.

---

## Legacy Message Format

Prior versions used a simpler JSON format (not JSON-RPC). For compatibility reference:

```json
{
  "type": "progress|log|result|error|status|heartbeat",
  "agent_id": "uuid",
  "task_id": "uuid",
  "timestamp": "2026-01-10T12:00:00Z",
  "data": { ... }
}
```

**Types:**
- `progress` - Progress percentage, current file
- `log` - Log messages (debug, info, warn, error)
- `result` - Task completion (completed, failed, cancelled)
- `error` - Error messages
- `status` - Status changes (started, running, paused, stopped)
- `heartbeat` - Keep-alive

**Current agents should use JSON-RPC 2.0 (ACP)**, but the backend may support legacy format for backward compatibility.

---

## Error Handling

### JSON-RPC Errors

**Error Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "error": {
    "code": -32600,
    "message": "Invalid request",
    "data": "Missing required parameter: message"
  }
}
```

**Standard Error Codes:**
- `-32700` - Parse error
- `-32600` - Invalid request
- `-32601` - Method not found
- `-32602` - Invalid params
- `-32603` - Internal error

### HTTP API Errors

```json
{
  "error": "error_code",
  "message": "Human readable message",
  "details": "Additional context",
  "http_status": 400
}
```

**Common Codes:**
- `bad_request` (400)
- `not_found` (404)
- `validation_error` (400)
- `internal_error` (500)

---

## Best Practices

### For Agent Developers

1. **Respond quickly to initialize** - Send capabilities within 1 second
2. **Stream progress frequently** - Update every 1-5 seconds during work
3. **Handle cancellation gracefully** - Clean up and exit when receiving `session/cancel`
4. **Use proper session IDs** - Generate unique, persistent IDs for resumption
5. **Log to stderr** - Use stdout exclusively for ACP messages
6. **Validate messages** - Check for required fields before processing
7. **Set exit codes** - 0 for success, non-zero for errors

## Local Agent Discovery

Kandev checks a bundled discovery config to detect locally installed agents. Each agent entry lists OS-specific installation paths (Linux, Windows, macOS). At runtime, the backend selects the paths for the current OS, expands environment variables and `~`, and marks the agent as available if any configured file or directory exists.

### Message Flow

```
1. Client sends orchestrator.start via WebSocket
2. Backend creates container with agentctl
3. agentctl starts HTTP server on port 9999
4. Backend waits for agentctl health check (GET /health)
5. Backend calls POST /api/v1/start
6. agentctl spawns agent subprocess
7. Backend calls POST /api/v1/acp/call with initialize
8. Agent responds with capabilities
9. Backend calls POST /api/v1/acp/call with session/new
10. Agent responds with sessionId
11. Backend connects to agentctl WS /api/v1/acp/stream
12. Backend calls POST /api/v1/acp/call with session/prompt
13. Agent sends session/update notifications (streamed to client via WebSocket)
14. Agent sends final session/update (type: complete)
15. Client sends orchestrator.complete or backend calls POST /api/v1/stop
```

### Testing Your Agent

**With agentctl:**
```bash
# Build and run container
docker run -d -p 9999:9999 kandev/your-agent:latest

# Check health
curl http://localhost:9999/health

# Start agent
curl -X POST http://localhost:9999/api/v1/start

# Send ACP message
curl -X POST http://localhost:9999/api/v1/acp/call \
  -H "Content-Type: application/json" \
  -d '{"method":"initialize","params":{"protocolVersion":"1.0"}}'
```

**Direct ACP testing:**
```bash
# Test initialize
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1.0"}}' | \
  docker run -i kandev/your-agent:latest /usr/local/bin/agent --acp

# Test full flow
(
  echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1.0"}}'
  echo '{"jsonrpc":"2.0","id":2,"method":"session/new","params":{}}'
  echo '{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"message":"test"}}'
) | docker run -i kandev/your-agent:latest /usr/local/bin/agent --acp
```

---

## Reference Implementation

See `apps/backend/` for complete working examples:

**agentctl:**
- `cmd/agentctl/main.go` - Entry point
- `internal/agentctl/` - HTTP server, process manager, ACP relay

**Augment Agent:**
- `dockerfiles/augment-agent/Dockerfile` - Multi-stage build with agentctl
- `dockerfiles/augment-agent/entrypoint.sh` - Container entrypoint

**Backend Integration:**
- `internal/agent/agentctl/client.go` - HTTP client for agentctl
- `internal/agent/lifecycle/manager.go` - Container lifecycle management

---

## Additional Resources

- **System Architecture**: [ARCHITECTURE.md](ARCHITECTURE.md)
- **WebSocket API Specification**: [docs/asyncapi.yaml](docs/asyncapi.yaml)
- **Agent Registry**: `apps/backend/internal/agent/registry/defaults.go`
- **agentctl Implementation**: `apps/backend/internal/agentctl/`
- **ACP Protocol Types**: `apps/backend/pkg/acp/jsonrpc/`
- **JSON-RPC 2.0 Spec**: https://www.jsonrpc.org/specification

---

**Last Updated**: 2026-01-11
