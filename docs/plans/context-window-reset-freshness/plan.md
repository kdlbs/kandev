---
spec: docs/specs/ui/requirements/context-window-reset-freshness.md
created: 2026-07-31
status: implemented
---

# Implementation Plan: Context Window Reset Freshness

## Overview

Invalidate context-window data at the successful reset boundary, then teach the frontend to treat an explicit empty metadata value as cache invalidation. The initiating client also clears its local cache after a successful reset response so it does not wait for asynchronous session notifications. A focused browser regression proves that old usage disappears and a later fresh agent report can repopulate the ring.

## Confirmed root cause

`ResetAgentContext` replaces the provider conversation but `resetAgentContext` only clears `acp_session_id`; it leaves `task_sessions.metadata.context_window` intact. The frontend also stores the last reading independently in `contextWindow.bySessionId`, and `extractContextWindow` only handles populated readings. Consequently, neither the reset response nor the subsequent session-state events invalidate the old ring; it remains visible until another agent usage frame overwrites it.

## Backend

### Successful reset metadata invalidation

- Update `apps/backend/internal/orchestrator/event_handlers_workflow.go` in `resetAgentContext` to clear `context_window` after `agentManager.ResetAgentContext` succeeds, alongside `acp_session_id` cleanup.
- Keep the clear after the irreversible runtime reset so failed resets preserve the previous reading.
- Log a metadata persistence failure without reporting the already-completed provider reset as failed; always clear the preloaded in-memory snapshot so the initiating client does not receive stale usage back through the final state event.
- Extend `apps/backend/internal/orchestrator/event_handlers_workflow_triggers_test.go` to prove successful resets clear both metadata keys and failed resets retain context-window metadata.

The public user-request path and workflow reset path share this helper, so the invariant applies to both without adding a new API or event type. After a successful metadata clear, the existing transition back to `WAITING_FOR_INPUT` reloads session metadata and publishes it through `session.state_changed`.

## Frontend

### Explicit context-window invalidation

- Update `apps/web/lib/ws/handlers/agent-session.ts` so context-window extraction recognizes both live `metadata` patches and full `session_metadata` snapshots. When either explicitly contains `context_window: null`, call `clearContextWindow`; when it contains a valid reading, keep the existing `setContextWindow` behavior; when the key is absent, do nothing.
- Extend `apps/web/lib/ws/handlers/agent-session.test.ts` with populated, cleared, and unrelated-metadata cases.

### Initiating-client reset response

- Update `apps/web/components/task/chat/reset-context-button.tsx` to clear the session context-window cache only after `session.reset_context` resolves successfully.
- Add `apps/web/components/task/chat/reset-context-button.test.tsx` to prove success clears the correct session and request failure retains the prior cache.

No layout, breakpoint, touch, or scroll behavior changes. Desktop and mobile already render the same `TokenUsageDisplay` from the shared session-runtime state; `apps/web/e2e/tests/chat/mobile-context-window-source.spec.ts` remains the nearest mobile exemplar and covers the mobile trigger/tooltip path. The repair therefore uses shared-state tests rather than a new mobile composition.

## Tests

- **What:** successful resets clear persisted context-window metadata while failed resets retain it.
  - **File:** `apps/backend/internal/orchestrator/event_handlers_workflow_triggers_test.go`
  - **How:** extend the existing real-repository reset helper tests, including successful provider reset plus metadata-clear failure, and run only `TestProcessOnEnterResetAgentContext`.
- **What:** explicit cleared metadata invalidates the frontend cache, while metadata without the key leaves it untouched.
  - **File:** `apps/web/lib/ws/handlers/agent-session.test.ts`
  - **How:** dispatch `session.state_changed` payloads against the focused store fake.
- **What:** the initiating client clears usage after a successful reset response but not after rejection.
  - **File:** `apps/web/components/task/chat/reset-context-button.test.tsx`
  - **How:** render the control with mocked WebSocket and Zustand boundaries and exercise the confirmation action.

## E2E Tests

- **Scenario:** **GIVEN** an idle session displays old context usage, **WHEN** the user resets context, **THEN** the old ring disappears; **WHEN** the fresh mock agent later reports usage, **THEN** the ring reappears with the fresh reading.
  - **File:** `apps/web/e2e/tests/session/session-recovery.spec.ts`
  - **What to verify:** the existing reset dialog/divider/idle flow, absence of the old context trigger after reset, and a fresh context trigger after a mock-agent command that emits `usage_update`.

## Implementation Waves And Parallel Candidates

Execution is sequential in the primary conversation.

- [x] [Task 01: Invalidate context-window state on successful reset](task-01-invalidate-reset-context-usage.md) — done
- [x] [Task 02: Cover reset freshness end to end](task-02-context-reset-e2e.md) — done

## Validation commands

- `cd apps/backend && go test -run TestProcessOnEnterResetAgentContext ./internal/orchestrator`
- `cd apps && pnpm --filter @kandev/web test -- --run lib/ws/handlers/agent-session.test.ts components/task/chat/reset-context-button.test.tsx`
- `cd apps/web && pnpm e2e:run --host tests/session/session-recovery.spec.ts -- --grep "reset context hides stale usage"`

## Risks and boundaries

- Session-state payloads use both `metadata` for narrow context updates and `session_metadata` for full state transitions; invalidation must support both without clearing on unrelated partial metadata.
- Reset failure occurs before metadata invalidation and must preserve the old reading.
- The existing reset control remains intentionally hidden while busy; changing prompt-admission or cancellation semantics is outside this fix.
