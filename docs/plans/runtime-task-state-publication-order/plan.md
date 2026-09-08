---
spec: docs/specs/tasks/requirements/runtime-state-publication-order.md
created: 2026-07-30
status: complete
---

# Implementation Plan: Runtime Task-State Publication Order

## Overview

Close the confirmed event-ordering window without changing task-state meaning
or frontend grouping. The orchestrator will reconcile a successfully
transitioned session's owning task inside the existing pre-publication hook,
then publish the running session event. Focused Go tests will prove the event
order, persistence result, race guards, and no-redundant-write behavior.

The confirmed root cause is that `setSessionRunningForExecution` currently
publishes `session.state_changed(RUNNING)` from `updateTaskSessionState` and only
then calls `reconcileTaskStateForRuntimeLocked`. The sidebar immediately paints
the running session indicator while Group by State still reads persisted
`tasks.state = REVIEW`.

The remote-delivery path also requires the gateway to preserve this ordering:
task and session lifecycle notifications share one NATS-style subscription so
separate callback scheduling cannot reintroduce the race before WebSocket
delivery.

## Backend

### Runtime transition ordering

- Update `setSessionRunningForExecution` in
  `apps/backend/internal/orchestrator/event_handlers_streaming.go` to use
  `updateTaskSessionStateWithHook`.
- Run `reconcileTaskStateForRuntimeLocked` from the hook after the session
  compare-and-set succeeds but before `publishTaskSessionStateChanged`.
- Capture and log reconciliation errors without rolling back or suppressing the
  truthful session transition.
- Remove the post-publication reconciliation call for the changed-state path.
- Preserve the `wasAlreadyRunning` fast path so repeated tool/stream events do
  not perform task-state reads or writes.
- Keep `writeTaskInProgressForRuntime` and the executor-success callback as the
  eventual healing path.

### Gateway delivery ordering

- Subscribe the WebSocket task broadcaster to one NATS-style wildcard for task
  and session state events, resolving the WebSocket action from `event.Type`.
- Keep all other event subscriptions and routing behavior unchanged.
- Ensure the in-memory event bus implements the same `>` wildcard semantics so
  local development and regression tests exercise the production contract.

### Event and persistence contract

- Continue using `UpdateTaskStateIfSessionState` so the session state, archive
  status, terminal transitions, clarification/cancellation races, and Office
  exclusion remain guarded by existing code.
- Do not add a schema, API field, WebSocket event, frontend-derived task state,
  or new in-memory lifecycle cache.

## Frontend

No frontend production change is planned. Group by State must continue to use
persisted `task.state`, while the shared desktop/mobile task row continues to
render live session activity. Correct backend event order makes those existing
contracts converge.

Mobile parity is state-only: the desktop sidebar and phone task drawer use the
same sidebar task aggregation and `TaskItem` rendering. There is no composition,
navigation, scroll, safe-area, pointer, or touch-target change.

## Tests

- **What:** a real `WAITING_FOR_INPUT -> RUNNING` transition persists
  `tasks.state = IN_PROGRESS` before observers receive the running session
  event.
  **File:**
  `apps/backend/internal/orchestrator/event_handlers_streaming_test.go`.
  **How:** use the SQLite test repository, the real task service, and one
  recording/memory event bus; subscribe to task/session state events and assert
  both durable state and event order.
- **What:** the regression test fails on the current implementation because
  `session.state_changed(RUNNING)` is observed while the task is still
  `REVIEW`.
  **File:**
  `apps/backend/internal/orchestrator/event_handlers_streaming_test.go`.
  **How:** record task and session state events through the real task service
  rather than relying on sleeps or browser rendering.
- **What:** same-state stream churn remains deduplicated.
  **File:**
  `apps/backend/internal/orchestrator/event_handlers_streaming_test.go`.
  **How:** retain and run `TestSetSessionRunning_NoRedundantTaskWrites`; extend
  it only if needed to assert no state event.
- **What:** clarification/cancellation and archive races cannot be overwritten.
  **Files:**
  `apps/backend/internal/orchestrator/event_handlers_runtime_state_race_test.go`
  and `apps/backend/internal/task/repository/sqlite/task_state_cas_test.go`.
  **How:** run the existing guarded-CAS regression tests alongside the new
  ordering test, including `-race`.
- **What:** gateway delivery keeps task-state notification ahead of the
  running-session notification for a shared wildcard subscription.
  **Files:** `apps/backend/internal/gateway/websocket/task_notifications.go`,
  `apps/backend/internal/gateway/websocket/task_notifications_test.go`, and
  `apps/backend/internal/events/bus/memory_test.go`.
  **How:** assert lifecycle subjects use one wildcard, publish both events
  through the in-memory NATS-compatible bus, and assert WebSocket action order.

## E2E Tests

No permanent browser test is planned. The repaired behavior is the absence of
an intermediate render between two WebSocket events; a Playwright
`MutationObserver` or polling assertion would be scheduler-dependent and could
pass on the broken implementation when React batches both events. The
producer-level integration test deterministically observes the same contract
at the event boundary.

Existing frontend tests already pin the unchanged display semantics:

- `apps/web/lib/sidebar/apply-view.test.ts` proves State grouping uses persisted
  task state.
- `apps/web/components/task/task-item.test.tsx` proves running session activity
  selects the spinner.
- `apps/web/e2e/tests/task/mobile-sidebar-workflow-completion-icon.spec.ts`
  proves the phone drawer uses the shared task-row status rendering.

## Implementation Waves And Parallel Candidates

Wave 1:

- [ ] [Task 01: Order runtime state publication](task-01-order-runtime-state-publication.md)

The task is sequential because production ordering and its regression test
touch the same orchestrator lifecycle seam. No subagent authorization is
implied.

## Risks

- Publishing task state first changes event order for clients that observe both
  subjects. Payloads and final states are unchanged.
- Reconciliation errors must not suppress a truthful running-session event.
- The pre-publication hook runs while the task runtime-state mutex is held;
  implementation must call the existing locked reconciliation helper and must
  not reacquire that mutex.
