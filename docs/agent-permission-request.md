# Agent Permission Requests

This document describes how permission requests flow between the agent, backend, and frontend.

## Overview
Some agents (e.g., Auggie) require user approval for sensitive tool actions (run command, write file, etc.). The agentctl sidecar streams permission requests to the backend, which then broadcasts them to the UI. The user responds via the UI, and the backend forwards the response back to agentctl.

## High-Level Flow
```
Agent (ACP/CLI)
  |
  |  permission_request (tool call needs approval)
  v
agentctl (permission stream)
  |
  |  WS /api/v1/acp/permissions/stream
  v
Backend (lifecycle manager)
  |
  |  events.ACPMessage (type=permission_request)
  v
Backend (WS gateway)
  |
  |  session.permission.requested (WS notification)
  v
Frontend (CommandApprovalDialog / tool-call UI)
  |
  |  permission.respond (WS request)
  v
Backend (orchestrator)
  |
  |  RespondToPermissionBySessionID
  v
agentctl (POST /api/v1/acp/permissions/respond)
  |
  v
Agent continues (approved/rejected)
```

## Backend Components
- **agentctl permission stream**
  - agentctl exposes `GET /api/v1/acp/permissions/stream`.
  - Streams `PermissionNotification` from the agent process.
- **lifecycle StreamManager**
  - Connects to the permission stream.
  - Publishes `events.ACPMessage` with `type=permission_request`.
- **orchestrator watcher**
  - Parses ACP messages and forwards permission requests to WS clients.
- **gateway websocket**
  - Broadcasts `session.permission.requested` to session subscribers.
- **orchestrator handlers**
  - Handles `permission.respond` and forwards to agentctl.

## Frontend Components
- **CommandApprovalDialog**
  - Shows permission requests not tied to a visible tool call.
- **ToolCallMessage inline actions**
  - Shows approve/reject inline for tool call messages.
- **WebSocket client**
  - Subscribes to `session.permission.requested`.
  - Sends `permission.respond` with `{ session_id, pending_id, option_id, cancelled }`.

## Message Shapes

### Permission Requested (WS notification)
```json
{
  "type": "notification",
  "action": "session.permission.requested",
  "payload": {
    "pending_id": "perm-123",
    "session_id": "session-abc",
    "task_id": "task-xyz",
    "tool_call_id": "tool-456",
    "title": "Command Execution Approval",
    "options": [
      { "option_id": "allow_once", "name": "Allow once", "kind": "allow_once" },
      { "option_id": "reject_once", "name": "Reject", "kind": "reject_once" }
    ],
    "action_type": "command",
    "action_details": { "command": "rm -rf .", "cwd": "/workspace" },
    "created_at": "2026-01-18T00:00:00Z"
  }
}
```

### Permission Respond (WS request)
```json
{
  "type": "request",
  "action": "permission.respond",
  "payload": {
    "session_id": "session-abc",
    "pending_id": "perm-123",
    "option_id": "allow_once",
    "cancelled": false
  }
}
```

## Notes
- Permissions are scoped to a **session**, not a task.
- If the session has no active UI, the pending permission is still stored on the backend and sent when the client subscribes.
- The agent can define different tools that require permission via its registry config.
