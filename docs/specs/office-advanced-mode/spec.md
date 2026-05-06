---
status: shipped
created: 2026-05-02
owner: cfl
---

# Office Advanced Mode — Execution Lifecycle

## Problem

When a user enters office advanced mode (`/office/tasks/[id]?mode=advanced`), the dockview panels (files, terminal, changes) need a running execution (agentctl process) to function. Office tasks are typically one-off: the agent runs, completes, and the execution is torn down. When the user later enters advanced mode to inspect results or continue work, there's no running execution — files show "Preparing workspace...", terminal shows "Connecting terminal...", and changes can't fetch git status.

The task details page avoids this because it calls `useEnsureTaskSession` on page load, which creates a session if none exists. But for office, sessions already exist (the agent already ran) — `EnsureSession` short-circuits at `findExistingSession` and returns without ensuring the execution is actually running.

## Current Architecture

### Execution ensure flow (task details page)

```
task-page-content.tsx
  → useEnsureTaskSession(task)           # ensures session exists
    → session.ensure (WS)               # backend: idempotent, returns existing or creates
      → EnsureSession()                  # returns existing session (short-circuits)

Panel readiness:
  File browser  → gates on agentctlStatus.isReady (via WS events)
  Terminal      → connects when session/env available
  Changes       → fetches git status via sessionId
```

### Execution resume flow (on prompt)

```
PromptTask / StartTask
  → ensureSessionRunning(session)        # checks if agent actually running
    → executor.GetExecutionBySession()   # in-memory check
    → if not running:
      - CREATED + has AgentExecutionID   → startAgentOnPreparedWorkspace()
      - has ExecutorRunning record       → executor.ResumeSession()
    → waitForSessionReady()              # polls until WAITING_FOR_INPUT
```

`ensureSessionRunning` is only called on the prompt/start path — never when entering advanced mode.

### What panels depend on

All panels gate on `agentctlStatus.isReady`:
- Backend emits `session.agentctl_starting` → `session.agentctl_ready` via WS
- File browser explicitly checks `agentctlStatus.isReady` before loading tree
- Terminal connects via WS to the execution's shell endpoint
- No panel independently creates an execution — they all assume it exists

## Solution

### Backend: Add `ensure_execution` flag to `session.ensure`

Extend the `session.ensure` WS endpoint to accept an optional `ensure_execution: true` parameter. When set, and the session already exists but has no running execution:

```go
// In EnsureSession:
if existing := s.findExistingSession(ctx, taskID); existing != nil {
    if req.EnsureExecution {
        session, _ := s.repo.GetTaskSession(ctx, existing.SessionID)
        if session != nil {
            _ = s.ensureSessionRunning(ctx, existing.SessionID, session)
            // Non-fatal: if resume fails, still return the session
            // (panels will show appropriate "not available" states)
        }
    }
    return existing, nil
}
```

This reuses the existing `ensureSessionRunning` logic which handles all session states (CREATED with prepared workspace, WAITING_FOR_INPUT with executor record, already running).

### Frontend: Call `session.ensure` with `ensure_execution` in office advanced mode

In `OfficeDockviewLayout`, on mount (or in `useAdvancedSession`), call `session.ensure` with the new flag:

```typescript
// office-dockview-layout.tsx
useEffect(() => {
  if (taskId && sessionId) {
    setActiveSession(taskId, sessionId);
    // Ensure execution is running for file/terminal/changes panels
    ensureTaskSession(taskId, { ensureExecution: true });
  }
}, [taskId, sessionId]);
```

Update `ensureTaskSession` in `session-launch-service.ts` to accept the new option:

```typescript
export async function ensureTaskSession(
  taskId: string,
  opts?: { ensureExecution?: boolean; timeout?: number },
): Promise<EnsureSessionResponse> {
  const client = getWebSocketClient();
  if (!client) throw new Error("WebSocket client not available");
  return client.request<EnsureSessionResponse>(
    "session.ensure",
    { task_id: taskId, ensure_execution: opts?.ensureExecution },
    opts?.timeout ?? 15_000,
  );
}
```

### Panel behavior during execution startup

When `ensure_execution` triggers a resume:
1. Backend emits `session.agentctl_starting` → panels enter "Preparing workspace..." state
2. Backend emits `session.agentctl_ready` → panels load (file tree, terminal connects, git status fetches)
3. If resume fails → panels show appropriate error states (existing behavior)

No panel changes needed — they already gate on `agentctlStatus.isReady`.

## Files to Modify

| File | Change |
|------|--------|
| `backend/internal/orchestrator/session_ensure.go` | Add `EnsureExecution` field to request, call `ensureSessionRunning` when set |
| `backend/internal/orchestrator/handlers/handlers.go` | Pass `ensure_execution` from WS request to `EnsureSession` |
| `backend/internal/orchestrator/session_ensure_test.go` | Test: existing session + ensure_execution triggers resume |
| `web/lib/services/session-launch-service.ts` | Add `ensureExecution` option to `ensureTaskSession` |
| `web/app/office/tasks/[id]/office-dockview-layout.tsx` | Call `ensureTaskSession` with `ensureExecution: true` on mount |

## Non-Goals

- Auto-starting the agent process (sending a prompt) on advanced mode entry — execution just means agentctl is running, the agent is idle
- Changing how the task details page works — `ensure_execution` is opt-in
- Creating new sessions — only resumes executions for existing sessions

## Testing

1. Create an office task, let agent run and complete (execution torn down)
2. Enter advanced mode → execution resumes, files/terminal/changes load
3. Leave and re-enter advanced mode → idempotent, no duplicate execution
4. Backend restart while in advanced mode → `GetOrEnsureExecution` handles recovery (existing behavior)
5. Task with no prior session → `EnsureSession` creates one with execution (existing behavior)
