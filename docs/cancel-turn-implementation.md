# Cancel/Stop Agent Turn Implementation

This document describes how to implement end-to-end functionality for cancelling the current agent turn for both Codex and Auggie agents.

## Overview

Users need the ability to stop an agent mid-execution without killing the entire agent process. This allows for quick iteration—cancelling a wrong path and sending a new prompt—without waiting for the agent to finish or restarting the session.

## Protocol Summary

### Auggie (ACP Protocol)
- **Method**: `session/cancel` (notification)
- **Transport**: JSON-RPC 2.0 over stdin/stdout
- **Message format**:
  ```json
  {"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":"<session-id>"}}
  ```
- **Key characteristics**:
  - It's a notification (no `id` field, no response expected)
  - Only requires `sessionId`
  - The pending `session/prompt` request returns with `{"stopReason":"cancelled"}`
  - Works immediately—agent stops current operation

### Codex (App-Server Protocol)
- **Method**: `turn/interrupt` (request)
- **Transport**: JSON-RPC variant over stdin/stdout (omits `jsonrpc` field)
- **Message format**:
  ```json
  {"id":N,"method":"turn/interrupt","params":{"threadId":"<thread-id>","turnId":"<turn-id>"}}
  ```
- **Key characteristics**:
  - It's a request (has `id` field, expects response)
  - Requires both `threadId` AND `turnId`
  - `threadId` = session ID (from `thread/start` response)
  - `turnId` = operation ID (from `turn/start` response)

## Current State

### Already Implemented ✅

1. **AgentAdapter interface** has `Cancel(ctx context.Context) error` method
2. **ACPAdapter.Cancel()** is fully implemented using `acpConn.Cancel()`
3. **CodexAdapter.Cancel()** is fully implemented using `turn/interrupt`
4. **AgentAdapter.GetOperationID()** returns `turnID` for Codex (needed for cancel)
5. **WebSocket action constant** `ActionAgentCancel = "agent.cancel"` exists

### Not Implemented ❌

1. **agentctl HTTP endpoint** for cancel (`POST /api/v1/acp/cancel`)
2. **agentctl Client method** to call the cancel endpoint
3. **Backend orchestrator/lifecycle integration** to route cancel requests
4. **WebSocket handler** for `agent.cancel` action
5. **Frontend UI** cancel button and hook

## High-Level Flow

```
Frontend (Cancel Button)
    |
    |  agent.cancel (WS request)
    v
Backend (WS Gateway → Dispatcher)
    |
    |  orchestrator handler
    v
Backend (Executor → AgentManager)
    |
    |  CancelAgent(ctx, sessionID)
    v
Lifecycle Manager
    |
    |  HTTP POST /api/v1/acp/cancel
    v
agentctl (api server)
    |
    |  adapter.Cancel(ctx)
    v
Agent subprocess (ACP or Codex protocol)
    |
    v
Agent stops current turn
    |
    |  Returns stopReason=cancelled (ACP) or turn/completed with cancelled (Codex)
    v
Prompt() returns → lifecycle emits session.cancelled event → UI updates
```

## Implementation Plan

### Layer 1: agentctl Server (HTTP API)

**File**: `apps/backend/internal/agentctl/server/api/acp.go`

Add a new endpoint:
- `POST /api/v1/acp/cancel` → `handleACPCancel`

Handler implementation:
1. Get the adapter from process manager
2. Call `adapter.Cancel(ctx)`
3. Return success/error response

**File**: `apps/backend/internal/agentctl/server/api/server.go`

Register the new route in the router group.

### Layer 2: agentctl Client

**File**: `apps/backend/internal/agentctl/client/acp.go`

Add `Cancel(ctx context.Context) error` method:
1. Make HTTP POST to `/api/v1/acp/cancel`
2. Parse response and return error if failed

### Layer 3: Lifecycle Manager

**File**: `apps/backend/internal/agent/lifecycle/manager.go`

Add `CancelAgent(ctx context.Context, executionID string) error`:
1. Look up execution by ID
2. Validate execution is in running state
3. Call `execution.agentctl.Cancel(ctx)`
4. The running Prompt() call will return with cancelled stop reason

**File**: `apps/backend/internal/agent/lifecycle/session.go`

Handle cancelled stop reason in `SendPrompt()`:
- When prompt returns with `stopReason=cancelled`, don't treat as error
- Emit appropriate event for UI notification

### Layer 4: Executor & Orchestrator

**File**: `apps/backend/internal/orchestrator/executor/executor.go`

Add interface method to `AgentManagerClient`:
```go
CancelAgent(ctx context.Context, sessionID string) error
```

Add `Cancel(ctx, taskID, sessionID string) error`:
1. Look up session by ID
2. Get agent execution ID
3. Call `agentManager.CancelAgent(ctx, executionID)`

**File**: `apps/backend/cmd/kandev/adapters.go`

Implement `CancelAgent` on `lifecycleAdapter`:
- Delegate to `mgr.CancelAgent()`

### Layer 5: WebSocket Handler

**File**: `apps/backend/internal/orchestrator/handlers/handlers.go`

Add handler for `agent.cancel`:
```go
func (h *Handlers) wsCancelAgent(ctx context.Context, msg *ws.Message) (*ws.Message, error)
```

Request payload:
```go
type wsCancelAgentRequest struct {
    TaskID    string `json:"task_id"`
    SessionID string `json:"session_id"`
}
```

Register in `RegisterHandlers()`:
```go
d.RegisterFunc(ws.ActionAgentCancel, h.wsCancelAgent)
```

**File**: `apps/backend/internal/orchestrator/controller/controller.go`

Add controller method:
```go
func (c *Controller) CancelAgent(ctx context.Context, req dto.CancelAgentRequest) (dto.CancelAgentResponse, error)
```

**File**: `apps/backend/internal/orchestrator/controller/dto/dto.go`

Add request/response types:
```go
type CancelAgentRequest struct {
    TaskID    string `json:"task_id"`
    SessionID string `json:"session_id"`
}

type CancelAgentResponse struct {
    Success bool   `json:"success"`
    Error   string `json:"error,omitempty"`
}
```

### Layer 6: Frontend

**File**: `apps/web/hooks/use-session-agent.ts`

Add `handleCancelTurn` callback:
```typescript
const handleCancelTurn = useCallback(async () => {
    if (!task?.id || !agentSessionId) return;
    const client = getWebSocketClient();
    if (!client) return;
    await client.request('agent.cancel', {
        task_id: task.id,
        session_id: agentSessionId
    });
}, [task?.id, agentSessionId]);
```

Return from hook: `{ ..., handleCancelTurn }`

**UI Integration** (location TBD—likely task chat panel or top bar):
- Add "Stop" button visible when agent is running
- Wire to `handleCancelTurn()`
- Show loading state during cancel
- Button disabled when no active turn

## Event Flow After Cancel

1. `adapter.Cancel()` sends protocol message
2. Agent stops and responds with cancelled status
3. `Prompt()` returns with `stopReason = "cancelled"`
4. Lifecycle manager:
   - Updates execution status to READY (not COMPLETED)
   - Emits `session.agent.cancelled` or similar event
5. Frontend receives event and updates UI:
   - Stops spinner
   - Shows "Cancelled" indicator
   - Re-enables input for new prompt

## Error Handling

| Scenario | Behavior |
|----------|----------|
| No active turn | Return success (no-op) |
| Agent not running | Return error "agent not running" |
| Session not found | Return error "session not found" |
| Cancel timeout | Return error, but cancel still in flight |
| Cancel during permission request | Cancel the permission, return cancelled |

## Testing

1. **Unit tests** for each layer (adapter, client, handler)
2. **Integration test**: Start agent, send prompt, cancel mid-execution
3. **E2E test**: UI cancel button flow
4. **Protocol tests**: Verify correct JSON-RPC messages for ACP and Codex

