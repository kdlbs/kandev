# Git Operations for Worktrees

This document describes the architecture for Git operations (Pull, Push, Rebase, Merge) on worktrees in Kandev.

## Overview

Git operations are executed **in agentctl**, not in the backend. This ensures consistent behavior whether running locally or in Docker/remote environments. The backend acts as a proxy, forwarding requests from the frontend WebSocket to agentctl's HTTP API.

## Supported Operations

| Operation | Direction | Description |
|-----------|-----------|-------------|
| **Pull** | Remote → Worktree | Fetch and merge changes from remote for the worktree's branch |
| **Push** | Worktree → Remote | Push local commits to remote |
| **Rebase** | Base → Worktree | Rebase worktree branch onto base branch (rewrites history) |
| **Merge** | Base → Worktree | Merge base branch into worktree branch (creates merge commit) |
| **Abort** | — | Abort an in-progress merge or rebase |

## Architecture

### Request Flow

```
Frontend UI (Git Actions Dropdown)
    │
    ▼
WebSocket Client
    │ worktree.pull / worktree.push / worktree.rebase / worktree.merge
    ▼
Backend WebSocket Gateway
    │
    ▼
Git Handlers (internal/agent/handlers/git_handlers.go)
    │ GetExecutionBySessionID(sessionID)
    ▼
Lifecycle Manager → AgentExecution → agentctl.Client
    │
    │ HTTP POST /api/v1/git/<operation>
    ▼
agentctl HTTP API (server/api/git.go)
    │
    ▼
Git Command Execution (server/process/git.go)
    │ exec.Command("git", ...) in worktree directory
    ▼
HTTP Response → Backend Handler → WebSocket Response → Frontend
```

### Notification Flow (Git Status Updates)

After any git operation, the workspace state changes. Updates flow through the event bus:

```
agentctl WorkspaceTracker (detects git status change)
    │ workspace stream: git_status message
    ▼
Backend StreamManager (OnGitStatus callback)
    │
    ▼
EventPublisher.PublishGitStatus()
    │
    ▼
Event Bus
    │
    ▼
TaskNotificationBridge
    │
    ▼
Hub.BroadcastToSession(sessionID)
    │
    ▼
Frontend WebSocket → Zustand Store update
```

## agentctl API

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/git/pull` | Pull from remote |
| POST | `/api/v1/git/push` | Push to remote |
| POST | `/api/v1/git/rebase` | Rebase onto base branch |
| POST | `/api/v1/git/merge` | Merge base branch in |
| POST | `/api/v1/git/abort` | Abort merge/rebase |

### Request/Response Format

**Pull Request:**
```json
{ "rebase": false }
```

**Push Request:**
```json
{ "force": false, "set_upstream": true }
```

**Rebase/Merge Request:**
```json
{ "base_branch": "main" }
```

**Abort Request:**
```json
{ "operation": "merge" }
```

**Response (all operations):**
```json
{
  "success": true,
  "operation": "pull",
  "output": "Already up to date.",
  "error": null,
  "conflict_files": []
}
```

## Locking

A per-instance mutex in agentctl prevents concurrent git operations on the same worktree. If an operation is already running, new requests return HTTP 409 Conflict.

## Conflict Handling

- **Rebase conflicts**: Automatically aborted (`git rebase --abort`), conflict files returned
- **Merge conflicts**: Left in place for user/agent resolution, conflict files returned
- **Abort endpoint**: Allows manual abort of conflicted merge/rebase

## Frontend Integration

### WebSocket Actions

- `worktree.pull` - Pull from remote
- `worktree.push` - Push to remote  
- `worktree.rebase` - Rebase onto base
- `worktree.merge` - Merge base branch
- `worktree.abort` - Abort operation

### Hook Usage

```typescript
const { pull, push, rebase, merge, isLoading, error } = useGitOperations(sessionId);

// In component
<Button onClick={() => pull()} disabled={isLoading}>Pull</Button>
```

## Files

### agentctl
- `server/api/git.go` - HTTP handlers
- `server/process/git.go` - Git command execution
- `client/git.go` - Client methods

### Backend
- `internal/agent/handlers/git_handlers.go` - WebSocket handlers
- `pkg/websocket/actions.go` - Action constants

### Frontend
- `lib/types/backend.ts` - TypeScript types
- `lib/ws/actions/git.ts` - WebSocket action functions
- `lib/hooks/useGitOperations.ts` - React hook

