# Kandev Agents Guide

> ⚠️ **IMPORTANT**: API documentation in this file must be updated whenever the API changes.
> When modifying any endpoint, request/response types, or ACP messages, update this document accordingly.

## Overview

Kandev uses Docker containers to run AI agents that execute tasks from the Kanban board. Agents communicate with the backend using the **Agent Communication Protocol (ACP)**, a JSON-based streaming protocol.

## Architecture

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│   HTTP Client   │────▶│  Kandev Backend  │────▶│ Docker Container│
│  (Frontend/CLI) │     │   (Port 8080)    │     │   (AI Agent)    │
└─────────────────┘     └────────┬─────────┘     └────────┬────────┘
                                 │                        │
                                 │◀───────────────────────┘
                                 │    ACP Messages (stdout)
                                 ▼
                        ┌──────────────────┐
                        │   Event Bus      │
                        │ (In-Memory)      │
                        └──────────────────┘
```

## Available Agent Types

### Augment Agent (`augment-agent`)

The primary AI coding agent powered by Auggie CLI.

| Property | Value |
|----------|-------|
| Image | `kandev/augment-agent:latest` |
| Working Directory | `/workspace` |
| Memory Limit | 4096 MB |
| CPU Cores | 2.0 |
| Timeout | 1 hour |
| Capabilities | code_generation, code_review, refactoring, testing, shell_execution |

**Required Environment Variables:**
- `AUGMENT_SESSION_AUTH` - Augment CLI session authentication (from `~/.augment/session.json`)
- `TASK_DESCRIPTION` - The task for the agent to execute

**Optional Environment Variables:**
- `AUGGIE_SESSION_ID` - Resume a previous session (for follow-up tasks)

**Mounted Volumes:**
- `{workspace}` → `/workspace` - The project directory
- `{augment_sessions}` → `/root/.augment/sessions` - Session persistence

---

## Agent Communication Protocol (ACP)

Agents communicate by writing JSON messages to stdout. Each line must be a complete JSON object.

### Message Format

```json
{
  "type": "progress",
  "agent_id": "uuid",
  "task_id": "uuid",
  "timestamp": "2026-01-10T12:00:00Z",
  "data": { ... }
}
```

### Message Types

#### `progress` - Progress Updates
```json
{
  "type": "progress",
  "data": {
    "progress": 50,
    "message": "Analyzing codebase...",
    "current_file": "src/main.go",
    "files_processed": 10,
    "total_files": 20
  }
}
```

#### `log` - Log Messages
```json
{
  "type": "log",
  "data": {
    "level": "info",
    "message": "Starting code review",
    "metadata": { "file": "main.go" }
  }
}
```
Levels: `debug`, `info`, `warn`, `error`

#### `result` - Task Completion
```json
{
  "type": "result",
  "data": {
    "status": "completed",
    "summary": "Task completed successfully",
    "artifacts": [
      { "type": "report", "path": "/workspace/report.md" }
    ]
  }
}
```
Status: `completed`, `failed`, `cancelled`

#### `error` - Error Messages
```json
{
  "type": "error",
  "data": {
    "error": "Failed to parse file",
    "file": "broken.go",
    "details": "syntax error on line 42"
  }
}
```

#### `status` - Agent Status Changes
```json
{
  "type": "status",
  "data": {
    "status": "running",
    "message": "Agent initialized"
  }
}
```
Status: `started`, `running`, `paused`, `stopped`

#### `heartbeat` - Keep-Alive
```json
{
  "type": "heartbeat",
  "data": {}
}
```

---

## REST API Reference

Base URL: `http://localhost:8080/api/v1`

### Health Check

```
GET /health
```

Response:
```json
{
  "status": "ok"
}
```

---

## Boards API

### Create Board
```
POST /boards
```

Request:
```json
{
  "name": "My Project",
  "description": "Project tasks"
}
```

Response: `201 Created`
```json
{
  "id": "uuid",
  "name": "My Project",
  "description": "Project tasks",
  "created_at": "2026-01-10T12:00:00Z",
  "updated_at": "2026-01-10T12:00:00Z"
}
```

### List Boards
```
GET /boards
```

Response:
```json
{
  "boards": [...],
  "total": 5
}
```

### Get Board
```
GET /boards/:boardId
```

### Update Board
```
PUT /boards/:boardId
```

Request:
```json
{
  "name": "Updated Name",
  "description": "Updated description"
}
```

### Delete Board
```
DELETE /boards/:boardId
```

---

## Columns API

### Create Column
```
POST /boards/:boardId/columns
```

Request:
```json
{
  "name": "To Do",
  "position": 0,
  "state": "TODO"
}
```

Response: `201 Created`
```json
{
  "id": "uuid",
  "board_id": "uuid",
  "name": "To Do",
  "position": 0,
  "state": "TODO",
  "created_at": "2026-01-10T12:00:00Z"
}
```

### List Columns
```
GET /boards/:boardId/columns
```

### Get Column
```
GET /columns/:columnId
```

---

## Tasks API

### Create Task
```
POST /tasks
```

Request:
```json
{
  "board_id": "uuid",
  "column_id": "uuid",
  "title": "Fix login bug",
  "description": "Users cannot login with email",
  "priority": 1,
  "agent_type": "augment-agent",
  "metadata": {}
}
```

Response: `201 Created`
```json
{
  "id": "uuid",
  "board_id": "uuid",
  "column_id": "uuid",
  "title": "Fix login bug",
  "description": "Users cannot login with email",
  "state": "TODO",
  "priority": 1,
  "agent_type": "augment-agent",
  "position": 0,
  "created_at": "2026-01-10T12:00:00Z",
  "updated_at": "2026-01-10T12:00:00Z",
  "metadata": {}
}
```

### Get Task
```
GET /tasks/:taskId
```

### Update Task
```
PUT /tasks/:taskId
```

Request (all fields optional):
```json
{
  "title": "Updated title",
  "description": "Updated description",
  "priority": 2,
  "metadata": { "key": "value" }
}
```

### Delete Task
```
DELETE /tasks/:taskId
```

### Update Task State
```
PUT /tasks/:taskId/state
```

Request:
```json
{
  "state": "IN_PROGRESS"
}
```

States: `TODO`, `IN_PROGRESS`, `BLOCKED`, `COMPLETED`, `FAILED`, `CANCELLED`

### Move Task
```
PUT /tasks/:taskId/move
```

Request:
```json
{
  "column_id": "uuid",
  "position": 0
}
```

### List Tasks in Board
```
GET /boards/:boardId/tasks
```

---

## Agents API

### Launch Agent
```
POST /agents/launch
```

Request:
```json
{
  "task_id": "uuid",
  "agent_type": "augment-agent",
  "workspace_path": "/path/to/project",
  "env": {
    "AUGMENT_SESSION_AUTH": "...",
    "TASK_DESCRIPTION": "Analyze this codebase"
  },
  "priority": 1,
  "metadata": {}
}
```

Response: `201 Created`
```json
{
  "id": "uuid",
  "task_id": "uuid",
  "agent_type": "augment-agent",
  "container_id": "docker-container-id",
  "status": "starting",
  "progress": 0,
  "started_at": "2026-01-10T12:00:00Z"
}
```

### List Agents
```
GET /agents
```

Response:
```json
{
  "agents": [...],
  "total": 3
}
```

### Get Agent Status
```
GET /agents/:instanceId/status
```

Response:
```json
{
  "id": "uuid",
  "task_id": "uuid",
  "agent_type": "augment-agent",
  "container_id": "docker-container-id",
  "status": "running",
  "progress": 50,
  "started_at": "2026-01-10T12:00:00Z",
  "finished_at": null,
  "exit_code": null,
  "error_message": ""
}
```

Agent statuses: `starting`, `running`, `completed`, `failed`, `stopped`

### Get Agent Logs
```
GET /agents/:instanceId/logs?tail=100
```

Response:
```json
{
  "logs": [
    {
      "timestamp": "2026-01-10T12:00:00Z",
      "message": "Starting agent...",
      "stream": "stdout"
    }
  ],
  "total": 50
}
```

### Stop Agent
```
DELETE /agents/:instanceId
```

Request (optional body):
```json
{
  "force": false,
  "reason": "User requested stop"
}
```

### List Agent Types
```
GET /agents/types
```

Response:
```json
{
  "types": [
    {
      "id": "augment-agent",
      "name": "Augment Coding Agent",
      "description": "Auggie CLI-powered autonomous coding agent",
      "image": "kandev/augment-agent",
      "capabilities": ["code_generation", "code_review", "refactoring", "testing", "shell_execution"],
      "enabled": true
    }
  ],
  "total": 1
}
```

### Get Agent Type
```
GET /agents/types/:typeId
```

---

## Orchestrator API

Base: `/api/v1/orchestrator`

### Get Orchestrator Status
```
GET /orchestrator/status
```

### Get Task Queue
```
GET /orchestrator/queue
```

Response:
```json
{
  "tasks": [
    {
      "task_id": "uuid",
      "priority": 1,
      "queued_at": "2026-01-10T12:00:00Z"
    }
  ],
  "total": 5
}
```

### Trigger Task Execution
```
POST /orchestrator/trigger
```

Request:
```json
{
  "task_id": "uuid",
  "force": false
}
```

### Start Task
```
POST /orchestrator/tasks/:taskId/start
```

### Stop Task
```
POST /orchestrator/tasks/:taskId/stop
```

### Pause Task
```
POST /orchestrator/tasks/:taskId/pause
```

### Resume Task
```
POST /orchestrator/tasks/:taskId/resume
```

### Get Task Execution Status
```
GET /orchestrator/tasks/:taskId/status
```

### Get Task Logs
```
GET /orchestrator/tasks/:taskId/logs
```

### Get Task Artifacts
```
GET /orchestrator/tasks/:taskId/artifacts
```

---

## Session Resumption

Agents can resume previous sessions for follow-up tasks:

1. **First task** - Launch agent normally, session ID stored in task metadata
2. **Get session ID** from task: `GET /tasks/:taskId` → `metadata.auggie_session_id`
3. **Follow-up task** - Launch with `AUGGIE_SESSION_ID` env var

Example:
```bash
# Get session ID from completed task
SESSION_ID=$(curl -s http://localhost:8080/api/v1/tasks/$TASK_ID | jq -r '.metadata.auggie_session_id')

# Launch follow-up with session resumption
curl -X POST http://localhost:8080/api/v1/agents/launch \
  -H "Content-Type: application/json" \
  -d '{
    "task_id": "'$NEW_TASK_ID'",
    "agent_type": "augment-agent",
    "workspace_path": "/path/to/project",
    "env": {
      "AUGMENT_SESSION_AUTH": "...",
      "AUGGIE_SESSION_ID": "'$SESSION_ID'",
      "TASK_DESCRIPTION": "What was my previous question?"
    }
  }'
```

---

## Error Responses

All error responses follow this format:

```json
{
  "error": "error_code",
  "message": "Human readable message",
  "details": "Additional details",
  "http_status": 400
}
```

Common error codes:
- `bad_request` (400) - Invalid request body or parameters
- `not_found` (404) - Resource not found
- `validation_error` (400) - Validation failed
- `internal_error` (500) - Server error

---

## Creating Custom Agents

To create a new agent type:

1. **Create Dockerfile** in `backend/dockerfiles/your-agent/`
2. **Implement ACP** - Write JSON messages to stdout
3. **Register agent** in `backend/internal/agent/registry/defaults.go`
4. **Build image**: `docker build -t kandev/your-agent:latest .`

Example agent script:
```bash
#!/bin/bash
echo '{"type":"status","agent_id":"'$AGENT_ID'","task_id":"'$TASK_ID'","timestamp":"'$(date -u +%Y-%m-%dT%H:%M:%SZ)'","data":{"status":"started"}}'
echo '{"type":"progress","agent_id":"'$AGENT_ID'","task_id":"'$TASK_ID'","timestamp":"'$(date -u +%Y-%m-%dT%H:%M:%SZ)'","data":{"progress":50,"message":"Working..."}}'
# Do work here
echo '{"type":"result","agent_id":"'$AGENT_ID'","task_id":"'$TASK_ID'","timestamp":"'$(date -u +%Y-%m-%dT%H:%M:%SZ)'","data":{"status":"completed","summary":"Done!"}}'
```

---

## Development Notes

### Running Locally
```bash
cd backend
make build
./bin/kandev
```

### Running Tests
```bash
cd backend
go test ./...
```

### Environment Variables
| Variable | Default | Description |
|----------|---------|-------------|
| `KANDEV_DB_PATH` | `./kandev.db` | SQLite database path |
| `PORT` | `8080` | HTTP server port |

---

**Last Updated**: 2026-01-10

