# Kandev WebSocket API Reference

This document provides a complete reference for all WebSocket API actions, their payloads, responses, and usage patterns.

## Overview

Kandev uses a **single WebSocket connection** for all API operations and real-time streaming. This eliminates the need for separate REST endpoints and enables full-duplex communication.

**Endpoint**: `ws://localhost:8080/ws`

### Message Envelope

All messages follow this JSON envelope format:

```json
{
  "id": "uuid",
  "type": "request|response|notification|error",
  "action": "action.name",
  "payload": {},
  "timestamp": "2026-01-10T12:00:00Z"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | UUID for request/response correlation. Omit for notifications. |
| `type` | string | Message type: `request`, `response`, `notification`, or `error` |
| `action` | string | The action to perform or that was performed |
| `payload` | object | Action-specific data |
| `timestamp` | string | ISO 8601 timestamp (optional) |

### Error Response Format

```json
{
  "id": "correlation-uuid",
  "type": "error",
  "action": "original.action",
  "payload": {
    "code": "ERROR_CODE",
    "message": "Human-readable message",
    "details": {}
  }
}
```

**Error Codes:**
- `BAD_REQUEST` - Invalid message format
- `VALIDATION_ERROR` - Missing or invalid fields
- `NOT_FOUND` - Resource not found
- `INTERNAL_ERROR` - Server error
- `UNKNOWN_ACTION` - Action not recognized

---

## System Flow

```
┌─────────────┐     WebSocket      ┌─────────────────────────────────────────────┐
│   Client    │◄──────────────────►│               Kandev Server                 │
│  (Browser)  │  Single Connection │                                             │
└─────────────┘                    │  ┌─────────┐ ┌─────────┐ ┌──────────────┐   │
                                   │  │  Task   │ │  Agent  │ │ Orchestrator │   │
                                   │  │ Service │ │ Manager │ │   Service    │   │
                                   │  └─────────┘ └─────────┘ └──────────────┘   │
                                   │                   │                          │
                                   │            ┌──────▼──────┐                   │
                                   │            │   Docker    │                   │
                                   │            │  Containers │                   │
                                   │            └─────────────┘                   │
                                   └─────────────────────────────────────────────┘
```

### Typical Workflow

1. **Connect** to `ws://localhost:8080/ws`
2. **Create Board** → `board.create`
3. **Create Column** → `column.create`
4. **Create Task** → `task.create`
5. **Subscribe to Task** → `task.subscribe` (receive real-time updates)
6. **Start Execution** → `orchestrator.start` (launches agent)
7. **Receive ACP Notifications** ← `acp.progress`, `acp.log`, etc.
8. **Send Follow-up Prompts** → `orchestrator.prompt` (multi-turn)
9. **Complete Task** → `orchestrator.complete`
10. **Unsubscribe** → `task.unsubscribe`

---

## Health Check

### `health.check`

**Purpose:** Verify the WebSocket connection and server status.

**Flow Position:** Can be called at any time to verify connectivity.

**Request:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "type": "request",
  "action": "health.check",
  "payload": {}
}
```

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "type": "response",
  "action": "health.check",
  "payload": {
    "status": "ok",
    "service": "kandev",
    "mode": "unified"
  }
}
```

| Response Field | Type | Description |
|---------------|------|-------------|
| `status` | string | Server health status (`ok`) |
| `service` | string | Service name |
| `mode` | string | API mode (`unified` = WebSocket-only) |

---

## Board Actions

Boards are the top-level containers that organize columns and tasks.

### `board.create`

**Purpose:** Create a new Kanban board.

**Flow Position:** First step - create a board before adding columns and tasks.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "board.create",
  "payload": {
    "name": "My Project",
    "description": "Project description"
  }
}
```

| Payload Field | Type | Required | Description |
|--------------|------|----------|-------------|
| `name` | string | ✅ | Board name |
| `description` | string | ❌ | Optional description |

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "board.create",
  "payload": {
    "id": "board-uuid",
    "name": "My Project",
    "description": "Project description",
    "created_at": "2026-01-10T12:00:00Z",
    "updated_at": "2026-01-10T12:00:00Z"
  }
}
```

### `board.list`

**Purpose:** List all boards.

**Flow Position:** Use to display available boards to the user.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "board.list",
  "payload": {}
}
```

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "board.list",
  "payload": {
    "boards": [
      {
        "id": "board-uuid",
        "name": "My Project",
        "description": "...",
        "created_at": "2026-01-10T12:00:00Z",
        "updated_at": "2026-01-10T12:00:00Z"
      }
    ],
    "total": 1
  }
}
```

### `board.get`

**Purpose:** Get a specific board by ID.

**Flow Position:** Use to load board details before displaying columns and tasks.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "board.get",
  "payload": {
    "id": "board-uuid"
  }
}
```

| Payload Field | Type | Required | Description |
|--------------|------|----------|-------------|
| `id` | string | ✅ | Board ID |

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "board.get",
  "payload": {
    "id": "board-uuid",
    "name": "My Project",
    "description": "Project description",
    "created_at": "2026-01-10T12:00:00Z",
    "updated_at": "2026-01-10T12:00:00Z"
  }
}
```

### `board.update`

**Purpose:** Update an existing board's properties.

**Flow Position:** Use when user edits board name or description.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "board.update",
  "payload": {
    "id": "board-uuid",
    "name": "Updated Name",
    "description": "Updated description"
  }
}
```

| Payload Field | Type | Required | Description |
|--------------|------|----------|-------------|
| `id` | string | ✅ | Board ID |
| `name` | string | ❌ | New name (optional) |
| `description` | string | ❌ | New description (optional) |

**Response:** Same as `board.get`

### `board.delete`

**Purpose:** Delete a board and all its columns and tasks.

**Flow Position:** Use with caution - this is destructive.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "board.delete",
  "payload": {
    "id": "board-uuid"
  }
}
```

| Payload Field | Type | Required | Description |
|--------------|------|----------|-------------|
| `id` | string | ✅ | Board ID |

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "board.delete",
  "payload": {
    "success": true
  }
}
```

---

## Column Actions

Columns organize tasks within a board and represent workflow states.

### `column.create`

**Purpose:** Create a new column in a board.

**Flow Position:** After creating a board, create columns for workflow stages.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "column.create",
  "payload": {
    "board_id": "board-uuid",
    "name": "To Do",
    "position": 0,
    "state": "TODO"
  }
}
```

| Payload Field | Type | Required | Description |
|--------------|------|----------|-------------|
| `board_id` | string | ✅ | Parent board ID |
| `name` | string | ✅ | Column name |
| `position` | int | ❌ | Display order (0-indexed) |
| `state` | string | ❌ | Task state: `TODO`, `IN_PROGRESS`, `DONE` |

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "column.create",
  "payload": {
    "id": "column-uuid",
    "board_id": "board-uuid",
    "name": "To Do",
    "position": 0,
    "state": "TODO",
    "created_at": "2026-01-10T12:00:00Z"
  }
}
```

### `column.list`

**Purpose:** List all columns in a board.

**Flow Position:** After loading a board, get its columns.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "column.list",
  "payload": {
    "board_id": "board-uuid"
  }
}
```

| Payload Field | Type | Required | Description |
|--------------|------|----------|-------------|
| `board_id` | string | ✅ | Board ID |

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "column.list",
  "payload": {
    "columns": [
      {
        "id": "column-uuid",
        "board_id": "board-uuid",
        "name": "To Do",
        "position": 0,
        "state": "TODO",
        "created_at": "2026-01-10T12:00:00Z"
      }
    ],
    "total": 1
  }
}
```

### `column.get`

**Purpose:** Get a specific column by ID.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "column.get",
  "payload": {
    "id": "column-uuid"
  }
}
```

| Payload Field | Type | Required | Description |
|--------------|------|----------|-------------|
| `id` | string | ✅ | Column ID |

**Response:** Same structure as items in `column.list`

---

## Task Actions

Tasks are work items that can be executed by AI agents.

### `task.create`

**Purpose:** Create a new task in a column.

**Flow Position:** Create tasks that agents will work on.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "task.create",
  "payload": {
    "board_id": "board-uuid",
    "column_id": "column-uuid",
    "title": "Implement feature X",
    "description": "Detailed requirements...",
    "priority": 1,
    "agent_type": "auggie",
    "repository_url": "https://github.com/org/repo",
    "branch": "main",
    "metadata": {
      "labels": ["feature", "priority-high"]
    }
  }
}
```

| Payload Field | Type | Required | Description |
|--------------|------|----------|-------------|
| `board_id` | string | ✅ | Board ID |
| `column_id` | string | ✅ | Column ID |
| `title` | string | ✅ | Task title |
| `description` | string | ❌ | Detailed description |
| `priority` | int | ❌ | Priority level (higher = more important) |
| `agent_type` | string | ❌ | Agent to use: `auggie`, `gemini` |
| `repository_url` | string | ❌ | Git repository URL |
| `branch` | string | ❌ | Git branch |
| `metadata` | object | ❌ | Custom key-value data |

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "task.create",
  "payload": {
    "id": "task-uuid",
    "board_id": "board-uuid",
    "column_id": "column-uuid",
    "title": "Implement feature X",
    "description": "Detailed requirements...",
    "state": "TODO",
    "priority": 1,
    "agent_type": "auggie",
    "repository_url": "https://github.com/org/repo",
    "branch": "main",
    "position": 0,
    "created_at": "2026-01-10T12:00:00Z",
    "updated_at": "2026-01-10T12:00:00Z",
    "metadata": {}
  }
}
```

| Response Field | Type | Description |
|---------------|------|-------------|
| `id` | string | Unique task ID |
| `state` | string | `TODO`, `IN_PROGRESS`, `BLOCKED`, `COMPLETED`, `FAILED`, `CANCELLED` |
| `position` | int | Position within column |

### `task.list`

**Purpose:** List all tasks in a board.

**Flow Position:** Load tasks to display on the Kanban board.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "task.list",
  "payload": {
    "board_id": "board-uuid"
  }
}
```

| Payload Field | Type | Required | Description |
|--------------|------|----------|-------------|
| `board_id` | string | ✅ | Board ID |

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "task.list",
  "payload": {
    "tasks": [ /* array of task objects */ ],
    "total": 5
  }
}
```

### `task.get`

**Purpose:** Get a specific task by ID.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "task.get",
  "payload": {
    "id": "task-uuid"
  }
}
```

| Payload Field | Type | Required | Description |
|--------------|------|----------|-------------|
| `id` | string | ✅ | Task ID |

**Response:** Same structure as `task.create` response

### `task.update`

**Purpose:** Update task properties (not state or position).

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "task.update",
  "payload": {
    "id": "task-uuid",
    "title": "Updated title",
    "description": "Updated description",
    "priority": 2,
    "metadata": { "key": "value" }
  }
}
```

| Payload Field | Type | Required | Description |
|--------------|------|----------|-------------|
| `id` | string | ✅ | Task ID |
| `title` | string | ❌ | New title |
| `description` | string | ❌ | New description |
| `priority` | int | ❌ | New priority |
| `metadata` | object | ❌ | Updated metadata |

**Response:** Updated task object

### `task.delete`

**Purpose:** Delete a task.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "task.delete",
  "payload": {
    "id": "task-uuid"
  }
}
```

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "task.delete",
  "payload": {
    "success": true
  }
}
```

### `task.move`

**Purpose:** Move a task to a different column and/or position.

**Flow Position:** Use for drag-and-drop on Kanban board.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "task.move",
  "payload": {
    "id": "task-uuid",
    "column_id": "target-column-uuid",
    "position": 0
  }
}
```

| Payload Field | Type | Required | Description |
|--------------|------|----------|-------------|
| `id` | string | ✅ | Task ID |
| `column_id` | string | ✅ | Target column ID |
| `position` | int | ❌ | Position in column (0-indexed) |

**Response:** Updated task object with new `column_id` and `position`

### `task.state`

**Purpose:** Update a task's state directly.

**Flow Position:** Used by orchestrator to update task state during execution.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "task.state",
  "payload": {
    "id": "task-uuid",
    "state": "IN_PROGRESS"
  }
}
```

| Payload Field | Type | Required | Description |
|--------------|------|----------|-------------|
| `id` | string | ✅ | Task ID |
| `state` | string | ✅ | New state: `TODO`, `IN_PROGRESS`, `BLOCKED`, `COMPLETED`, `FAILED`, `CANCELLED` |

**Response:** Updated task object


---

## Task Subscription Actions

Subscriptions enable real-time streaming of agent output (ACP) to specific clients.

### `task.subscribe`

**Purpose:** Subscribe to receive real-time notifications for a specific task.

**Flow Position:** Call before `orchestrator.start` to receive agent output.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "task.subscribe",
  "payload": {
    "task_id": "task-uuid"
  }
}
```

| Payload Field | Type | Required | Description |
|--------------|------|----------|-------------|
| `task_id` | string | ✅ | Task ID to subscribe to |

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "task.subscribe",
  "payload": {
    "success": true,
    "task_id": "task-uuid"
  }
}
```

**Effect:** After subscribing, you will receive notifications for this task:
- `acp.progress` - Progress updates
- `acp.log` - Agent log messages
- `acp.result` - Agent results
- `acp.error` - Agent errors
- `task.updated` - Task state changes

### `task.unsubscribe`

**Purpose:** Stop receiving notifications for a task.

**Flow Position:** Call after task completes or when navigating away.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "task.unsubscribe",
  "payload": {
    "task_id": "task-uuid"
  }
}
```

| Payload Field | Type | Required | Description |
|--------------|------|----------|-------------|
| `task_id` | string | ✅ | Task ID to unsubscribe from |

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "task.unsubscribe",
  "payload": {
    "success": true,
    "task_id": "task-uuid"
  }
}
```

---

## Agent Actions

Agents are Docker containers running AI coding assistants.

### `agent.list`

**Purpose:** List all agent instances (running and completed).

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "agent.list",
  "payload": {}
}
```

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "agent.list",
  "payload": {
    "agents": [
      {
        "id": "agent-uuid",
        "task_id": "task-uuid",
        "agent_type": "auggie",
        "container_id": "docker-container-id",
        "status": "running",
        "progress": 45,
        "started_at": "2026-01-10T12:00:00Z",
        "finished_at": null,
        "exit_code": null,
        "error": null
      }
    ],
    "total": 1
  }
}
```

| Response Field | Type | Description |
|---------------|------|-------------|
| `status` | string | `starting`, `running`, `completed`, `failed`, `stopped`, `READY` |
| `progress` | int | 0-100 percentage |
| `exit_code` | int | Container exit code (when completed) |
| `error` | string | Error message (if failed) |

### `agent.launch`

**Purpose:** Launch an agent container directly (low-level).

**Flow Position:** Typically use `orchestrator.start` instead, which handles this automatically.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "agent.launch",
  "payload": {
    "task_id": "task-uuid",
    "agent_type": "auggie",
    "workspace_path": "/path/to/workspace",
    "env": {
      "CUSTOM_VAR": "value"
    }
  }
}
```

| Payload Field | Type | Required | Description |
|--------------|------|----------|-------------|
| `task_id` | string | ✅ | Task ID this agent is for |
| `agent_type` | string | ✅ | Agent type: `auggie`, `gemini` |
| `workspace_path` | string | ✅ | Path to mount in container |
| `env` | object | ❌ | Additional environment variables |

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "agent.launch",
  "payload": {
    "success": true,
    "agent_id": "agent-uuid",
    "task_id": "task-uuid"
  }
}
```

### `agent.status`

**Purpose:** Get detailed status of a specific agent.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "agent.status",
  "payload": {
    "agent_id": "agent-uuid"
  }
}
```

| Payload Field | Type | Required | Description |
|--------------|------|----------|-------------|
| `agent_id` | string | ✅ | Agent instance ID |

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "agent.status",
  "payload": {
    "id": "agent-uuid",
    "task_id": "task-uuid",
    "agent_type": "auggie",
    "container_id": "docker-container-id",
    "status": "running",
    "progress": 67,
    "started_at": "2026-01-10T12:00:00Z",
    "finished_at": null,
    "exit_code": null,
    "error": null
  }
}
```

### `agent.logs`

**Purpose:** Get agent container logs.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "agent.logs",
  "payload": {
    "agent_id": "agent-uuid"
  }
}
```

| Payload Field | Type | Required | Description |
|--------------|------|----------|-------------|
| `agent_id` | string | ✅ | Agent instance ID |

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "agent.logs",
  "payload": {
    "agent_id": "agent-uuid",
    "logs": ["log line 1", "log line 2"],
    "message": "..."
  }
}
```

### `agent.stop`

**Purpose:** Stop a running agent container.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "agent.stop",
  "payload": {
    "agent_id": "agent-uuid"
  }
}
```

| Payload Field | Type | Required | Description |
|--------------|------|----------|-------------|
| `agent_id` | string | ✅ | Agent instance ID |

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "agent.stop",
  "payload": {
    "success": true
  }
}
```

### `agent.types`

**Purpose:** List available agent types.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "agent.types",
  "payload": {}
}
```

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "agent.types",
  "payload": {
    "types": [
      {
        "id": "auggie",
        "name": "Auggie CLI Agent",
        "description": "Augment Code CLI-based coding agent",
        "image": "ghcr.io/kandev/auggie-agent:latest",
        "capabilities": ["code-generation", "refactoring", "testing"],
        "enabled": true
      },
      {
        "id": "gemini",
        "name": "Gemini Agent",
        "description": "Google Gemini-based coding agent",
        "image": "ghcr.io/kandev/gemini-agent:latest",
        "capabilities": ["code-generation", "analysis"],
        "enabled": true
      }
    ],
    "total": 2
  }
}
```

---

## Orchestrator Actions

The orchestrator coordinates task execution, managing the workflow between tasks and agents.

### `orchestrator.status`

**Purpose:** Get overall orchestrator status.

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
    "active_tasks": 2,
    "queued_tasks": 5,
    "max_concurrent": 3
  }
}
```

### `orchestrator.queue`

**Purpose:** Get the current task queue.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "orchestrator.queue",
  "payload": {}
}
```

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "orchestrator.queue",
  "payload": {
    "tasks": [
      {
        "task_id": "task-uuid",
        "priority": 1,
        "queued_at": "2026-01-10T12:00:00Z"
      }
    ],
    "total": 1
  }
}
```

### `orchestrator.trigger`

**Purpose:** Trigger a task for execution (adds to queue).

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "orchestrator.trigger",
  "payload": {
    "task_id": "task-uuid"
  }
}
```

| Payload Field | Type | Required | Description |
|--------------|------|----------|-------------|
| `task_id` | string | ✅ | Task ID to trigger |

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "orchestrator.trigger",
  "payload": {
    "success": true,
    "message": "Task triggered",
    "task_id": "task-uuid"
  }
}
```

### `orchestrator.start`

**Purpose:** Start executing a task immediately with an agent.

**Flow Position:** Main action to begin agent work on a task.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "orchestrator.start",
  "payload": {
    "task_id": "task-uuid",
    "agent_type": "auggie",
    "priority": 1
  }
}
```

| Payload Field | Type | Required | Description |
|--------------|------|----------|-------------|
| `task_id` | string | ✅ | Task ID to start |
| `agent_type` | string | ❌ | Override default agent type |
| `priority` | int | ❌ | Execution priority |

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "orchestrator.start",
  "payload": {
    "success": true,
    "task_id": "task-uuid",
    "agent_instance_id": "agent-uuid",
    "status": "running"
  }
}
```

**Effect:**
1. Task state changes to `IN_PROGRESS`
2. Agent container is launched
3. ACP notifications begin streaming (if subscribed)

### `orchestrator.stop`

**Purpose:** Stop a running task execution.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "orchestrator.stop",
  "payload": {
    "task_id": "task-uuid",
    "reason": "User cancelled",
    "force": false
  }
}
```

| Payload Field | Type | Required | Description |
|--------------|------|----------|-------------|
| `task_id` | string | ✅ | Task ID to stop |
| `reason` | string | ❌ | Reason for stopping |
| `force` | bool | ❌ | Force kill (default: false) |

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "orchestrator.stop",
  "payload": {
    "success": true
  }
}
```

### `orchestrator.prompt`

**Purpose:** Send a follow-up prompt to a running agent (multi-turn conversation).

**Flow Position:** Use during agent execution for clarifications or additional instructions.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "orchestrator.prompt",
  "payload": {
    "task_id": "task-uuid",
    "prompt": "Also add unit tests for the new function"
  }
}
```

| Payload Field | Type | Required | Description |
|--------------|------|----------|-------------|
| `task_id` | string | ✅ | Task ID (must have running agent) |
| `prompt` | string | ✅ | Follow-up prompt text |

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "orchestrator.prompt",
  "payload": {
    "success": true
  }
}
```

**Effect:** The prompt is sent to the agent via `agentctl` HTTP sidecar.

### `orchestrator.complete`

**Purpose:** Mark a task as completed.

**Flow Position:** Called when agent finishes successfully or user approves the work.

**Request:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "orchestrator.complete",
  "payload": {
    "task_id": "task-uuid"
  }
}
```

| Payload Field | Type | Required | Description |
|--------------|------|----------|-------------|
| `task_id` | string | ✅ | Task ID to complete |

**Response:**
```json
{
  "id": "uuid",
  "type": "response",
  "action": "orchestrator.complete",
  "payload": {
    "success": true,
    "message": "task completed"
  }
}
```

---

## Server Notifications (ACP Streaming)

These are push notifications from the server to subscribed clients. They have `type: "notification"` and no `id` field.

### `acp.progress`

**Purpose:** Agent progress update.

**When:** Periodically during agent execution.

```json
{
  "type": "notification",
  "action": "acp.progress",
  "payload": {
    "task_id": "task-uuid",
    "agent_id": "agent-uuid",
    "progress": 45,
    "message": "Analyzing code structure...",
    "current_file": "src/main.go",
    "files_processed": 12,
    "total_files": 27
  },
  "timestamp": "2026-01-10T12:05:00Z"
}
```

### `acp.log`

**Purpose:** Agent log message (debug, info, warn, error).

**When:** As agent produces output.

```json
{
  "type": "notification",
  "action": "acp.log",
  "payload": {
    "task_id": "task-uuid",
    "agent_id": "agent-uuid",
    "level": "info",
    "message": "Starting code generation for feature X",
    "metadata": {
      "component": "code-generator"
    }
  },
  "timestamp": "2026-01-10T12:05:30Z"
}
```

| Payload Field | Type | Description |
|--------------|------|-------------|
| `level` | string | `debug`, `info`, `warn`, `error` |

### `acp.result`

**Purpose:** Agent produced a result artifact.

**When:** Agent completes a unit of work.

```json
{
  "type": "notification",
  "action": "acp.result",
  "payload": {
    "task_id": "task-uuid",
    "agent_id": "agent-uuid",
    "status": "success",
    "summary": "Created 3 new files",
    "artifacts": [
      { "type": "file", "path": "src/feature.go" },
      { "type": "file", "path": "src/feature_test.go" }
    ]
  },
  "timestamp": "2026-01-10T12:10:00Z"
}
```

### `acp.error`

**Purpose:** Agent encountered an error.

**When:** Error during agent execution.

```json
{
  "type": "notification",
  "action": "acp.error",
  "payload": {
    "task_id": "task-uuid",
    "agent_id": "agent-uuid",
    "code": "COMPILE_ERROR",
    "message": "Failed to compile: syntax error",
    "details": {
      "file": "src/main.go",
      "line": 42
    }
  },
  "timestamp": "2026-01-10T12:06:00Z"
}
```

### `acp.status`

**Purpose:** Agent status changed.

**When:** Agent starts, completes, or fails.

```json
{
  "type": "notification",
  "action": "acp.status",
  "payload": {
    "task_id": "task-uuid",
    "agent_id": "agent-uuid",
    "status": "completed",
    "previous_status": "running",
    "exit_code": 0
  },
  "timestamp": "2026-01-10T12:15:00Z"
}
```

### `acp.heartbeat`

**Purpose:** Keep-alive signal from agent.

**When:** Periodically while agent is running (prevents timeout).

```json
{
  "type": "notification",
  "action": "acp.heartbeat",
  "payload": {
    "task_id": "task-uuid",
    "agent_id": "agent-uuid"
  },
  "timestamp": "2026-01-10T12:05:00Z"
}
```

### `task.updated`

**Purpose:** Task properties changed.

**When:** Task state, title, or other properties change.

```json
{
  "type": "notification",
  "action": "task.updated",
  "payload": {
    "id": "task-uuid",
    "state": "IN_PROGRESS",
    "previous_state": "TODO"
  },
  "timestamp": "2026-01-10T12:00:05Z"
}
```

### `agent.updated`

**Purpose:** Agent status changed.

**When:** Agent starts, stops, or changes state.

```json
{
  "type": "notification",
  "action": "agent.updated",
  "payload": {
    "id": "agent-uuid",
    "task_id": "task-uuid",
    "status": "running",
    "progress": 0
  },
  "timestamp": "2026-01-10T12:00:03Z"
}
```

---

## Complete Usage Example

```javascript
const ws = new WebSocket('ws://localhost:8080/ws');
const pending = new Map();

// Handle incoming messages
ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);

  if (msg.type === 'response' || msg.type === 'error') {
    // Resolve pending request
    const resolve = pending.get(msg.id);
    if (resolve) {
      pending.delete(msg.id);
      resolve(msg);
    }
  } else if (msg.type === 'notification') {
    // Handle real-time updates
    console.log(`[${msg.action}]`, msg.payload);
  }
};

// Helper to send requests
function request(action, payload) {
  return new Promise((resolve) => {
    const id = crypto.randomUUID();
    pending.set(id, resolve);
    ws.send(JSON.stringify({ id, type: 'request', action, payload }));
  });
}

// Full workflow example
async function runTask() {
  // 1. Create board
  const board = await request('board.create', { name: 'My Project' });

  // 2. Create column
  const column = await request('column.create', {
    board_id: board.payload.id,
    name: 'To Do',
    state: 'TODO'
  });

  // 3. Create task
  const task = await request('task.create', {
    board_id: board.payload.id,
    column_id: column.payload.id,
    title: 'Implement user authentication',
    agent_type: 'auggie'
  });

  // 4. Subscribe to updates
  await request('task.subscribe', { task_id: task.payload.id });

  // 5. Start execution
  await request('orchestrator.start', { task_id: task.payload.id });

  // 6. ACP notifications will now stream automatically...

  // 7. Send follow-up prompt (optional)
  await request('orchestrator.prompt', {
    task_id: task.payload.id,
    prompt: 'Also add JWT token validation'
  });

  // 8. When done, complete the task
  await request('orchestrator.complete', { task_id: task.payload.id });

  // 9. Unsubscribe
  await request('task.unsubscribe', { task_id: task.payload.id });
}
```

---

## Quick Reference

| Action | Purpose |
|--------|---------|
| `health.check` | Server health status |
| `board.create` | Create board |
| `board.list` | List all boards |
| `board.get` | Get board by ID |
| `board.update` | Update board |
| `board.delete` | Delete board |
| `column.create` | Create column |
| `column.list` | List columns in board |
| `column.get` | Get column by ID |
| `task.create` | Create task |
| `task.list` | List tasks in board |
| `task.get` | Get task by ID |
| `task.update` | Update task properties |
| `task.delete` | Delete task |
| `task.move` | Move task to column/position |
| `task.state` | Change task state |
| `task.subscribe` | Subscribe to task notifications |
| `task.unsubscribe` | Unsubscribe from task |
| `agent.list` | List agent instances |
| `agent.launch` | Launch agent (low-level) |
| `agent.status` | Get agent status |
| `agent.logs` | Get agent logs |
| `agent.stop` | Stop agent |
| `agent.types` | List available agent types |
| `orchestrator.status` | Get orchestrator status |
| `orchestrator.queue` | Get task queue |
| `orchestrator.trigger` | Queue task for execution |
| `orchestrator.start` | Start task execution |
| `orchestrator.stop` | Stop task execution |
| `orchestrator.prompt` | Send follow-up prompt |
| `orchestrator.complete` | Complete task |

