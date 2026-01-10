# Kandev Agent Protocol

> **For Agent Developers**: This document explains how to build agents that integrate with Kandev using the Agent Communication Protocol (ACP).
>
> **For System Architecture**: See [ARCHITECTURE.md](ARCHITECTURE.md)
> **For REST API**: See [docs/openapi.yaml](docs/openapi.yaml)

## Overview

Kandev agents run in Docker containers and communicate via **ACP (Agent Communication Protocol)** - a JSON-RPC 2.0 protocol over stdin/stdout. This enables bidirectional communication for task execution with real-time progress updates.

```
Backend                          Agent Container
  │                                    │
  │  initialize                        │
  ├──────────────────────────────────► │
  │◄──────────────────────────────────┤
  │  session/prompt                    │
  ├──────────────────────────────────► │
  │  session/update (progress)         │
  │◄──────────────────────────────────┤
  │  session/update (complete)         │
  │◄──────────────────────────────────┤
```

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

Primary AI coding agent powered by Auggie CLI.

**Docker Image**: `kandev/augment-agent:latest`

**Specification:**
- **Working Directory**: `/workspace`
- **Memory**: 4096 MB
- **CPU**: 2.0 cores
- **Timeout**: 1 hour
- **Capabilities**: code_generation, code_review, refactoring, testing, shell_execution

**Required Environment Variables:**
```bash
AUGMENT_SESSION_AUTH  # From ~/.augment/session.json
TASK_DESCRIPTION      # The task to execute
```

**Optional Environment Variables:**
```bash
AUGGIE_SESSION_ID     # Resume previous session
```

**Mounted Volumes:**
- `{workspace}` → `/workspace` - Project directory
- `{augment_sessions}` → `/root/.augment/sessions` - Session persistence

**Dockerfile**: `backend/dockerfiles/augment-agent/Dockerfile`

---

## Creating Custom Agents

### Step 1: Create Dockerfile

Create `backend/dockerfiles/your-agent/Dockerfile`:

```dockerfile
FROM ubuntu:22.04

# Install dependencies
RUN apt-get update && apt-get install -y \
    curl jq git \
    && rm -rf /var/lib/apt/lists/*

# Copy agent script
COPY agent.sh /usr/local/bin/agent
RUN chmod +x /usr/local/bin/agent

WORKDIR /workspace

ENTRYPOINT ["/usr/local/bin/agent"]
```

### Step 2: Implement ACP Protocol

Create `backend/dockerfiles/your-agent/agent.sh`:

```bash
#!/bin/bash
set -e

# ACP mode - JSON-RPC 2.0
if [ "$1" = "--acp" ]; then
    # Initialize response
    echo '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"1.0","serverInfo":{"name":"your-agent","version":"1.0.0"},"capabilities":{"streaming":true}}}'

    # Read requests from stdin
    while IFS= read -r line; do
        METHOD=$(echo "$line" | jq -r '.method')
        ID=$(echo "$line" | jq -r '.id')

        case "$METHOD" in
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
                # Handle cancellation
                exit 0
                ;;
        esac
    done
fi
```

### Step 3: Register Agent Type

Add to `backend/internal/agent/registry/defaults.go`:

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
cd backend/dockerfiles/your-agent
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

## REST API Integration

### Launch Agent

```bash
POST /api/v1/agents/launch
```

**Request:**
```json
{
  "task_id": "uuid",
  "agent_type": "augment-agent",
  "workspace_path": "/absolute/path/to/project",
  "env": {
    "AUGMENT_SESSION_AUTH": "{...}",
    "TASK_DESCRIPTION": "Fix the bug in auth/login.go"
  },
  "priority": 1,
  "metadata": {}
}
```

**Response:**
```json
{
  "id": "instance-uuid",
  "task_id": "uuid",
  "agent_type": "augment-agent",
  "container_id": "docker-id",
  "status": "starting",
  "started_at": "2026-01-10T12:00:00Z"
}
```

### Send Prompt (Mid-Execution)

```bash
POST /api/v1/agents/:instanceId/prompt
```

**Request:**
```json
{
  "message": "Please also add unit tests"
}
```

### Cancel Agent

```bash
POST /api/v1/agents/:instanceId/cancel
```

**Request:**
```json
{
  "reason": "User requested cancellation"
}
```

### Get Agent Status

```bash
GET /api/v1/agents/:instanceId/status
```

**Response:**
```json
{
  "id": "instance-uuid",
  "status": "running",
  "progress": 50,
  "started_at": "2026-01-10T12:00:00Z",
  "finished_at": null
}
```

Statuses: `starting`, `running`, `completed`, `failed`, `stopped`

### Get Agent Logs

```bash
GET /api/v1/agents/:instanceId/logs?tail=100
```

### Get Session Info

```bash
GET /api/v1/agents/:instanceId/session
```

**Response:**
```json
{
  "instance_id": "instance-uuid",
  "task_id": "task-uuid",
  "session_id": "abc123",
  "status": "running"
}
```

---

## Session Resumption

Agents can resume previous sessions for follow-up conversations.

### How It Works

1. First task completes, session ID stored in task metadata
2. Retrieve session ID from task: `GET /tasks/:taskId` → `metadata.auggie_session_id`
3. Launch new task with `AUGGIE_SESSION_ID` environment variable

### Example

```bash
# Get session ID from completed task
SESSION_ID=$(curl -s http://localhost:8080/api/v1/tasks/$TASK_ID | \
  jq -r '.metadata.auggie_session_id')

# Launch with resumption
curl -X POST http://localhost:8080/api/v1/agents/launch \
  -H "Content-Type: application/json" \
  -d '{
    "task_id": "'$NEW_TASK_ID'",
    "agent_type": "augment-agent",
    "workspace_path": "/path/to/project",
    "env": {
      "AUGMENT_SESSION_AUTH": "'"$(cat ~/.augment/session.json | jq -c)"'",
      "AUGGIE_SESSION_ID": "'$SESSION_ID'",
      "TASK_DESCRIPTION": "What changes did you make in the previous task?"
    }
  }'
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

### Message Flow

```
1. Backend starts container
2. Backend sends initialize
3. Agent responds with capabilities
4. Backend sends session/new
5. Agent responds with sessionId
6. Backend sends session/prompt
7. Agent responds immediately (accepted: true)
8. Agent sends multiple session/update notifications
9. Agent sends final session/update (type: complete)
10. Container exits
```

### Testing Your Agent

```bash
# Test initialize
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1.0"}}' | \
  docker run -i kandev/your-agent:latest --acp

# Test full flow
(
  echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1.0"}}'
  echo '{"jsonrpc":"2.0","id":2,"method":"session/new","params":{}}'
  echo '{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"message":"test"}}'
) | docker run -i kandev/your-agent:latest --acp
```

---

## Reference Implementation

See `backend/dockerfiles/augment-agent/` for a complete working example:

- **Dockerfile** - Container setup
- **agent.sh** - Entry point and ACP handler
- **README.md** - Usage instructions

---

## Additional Resources

- **System Architecture**: [ARCHITECTURE.md](ARCHITECTURE.md)
- **REST API Specification**: [docs/openapi.yaml](docs/openapi.yaml)
- **Agent Registry**: `backend/internal/agent/registry/defaults.go`
- **ACP Implementation**: `backend/internal/agent/acp/`
- **JSON-RPC 2.0 Spec**: https://www.jsonrpc.org/specification

---

**Last Updated**: 2026-01-10
