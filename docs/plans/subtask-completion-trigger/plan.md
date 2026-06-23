---
spec: docs/specs/tasks/subtask-completion-trigger.md
created: 2026-06-23
status: implemented
---

# Implementation Plan: Subtask Completion Trigger

## Overview

The workflow model already contains `on_children_completed`, but generic
MCP/kanban subtasks do not synthesize that trigger when their task state reaches
terminal. Implement this by adding active-child completion helpers, preserving
the event shape through API/frontend types, and wiring `task.state_changed` into
an orchestrator trigger path that applies workflow transitions with normal
`on_exit`/`on_enter` behavior. Finish with focused backend tests and one E2E
that proves child task completion advances the parent without polling.

---

## Backend

### Task Repository

Files:
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/repository/task_repository_test.go`

Changes:
- Add a repository helper that returns active direct children with fields needed
  for completion checks and idempotency, for example:
  `ListChildCompletionRows(ctx, parentID string) ([]ChildCompletionRow, error)`.
- `ChildCompletionRow` should include `ID`, `State`, `Title`, and `UpdatedAt`.
- Match `ListChildren` active semantics: `parent_id = ?`, `archived_at IS NULL`,
  and `is_ephemeral = 0`.
- Treat `COMPLETED`, `FAILED`, and `CANCELLED` as terminal.

Reason:
- The generic task layer must not import Office repository helpers, and Office's
  current helper omits `FAILED` from terminal states.

### Workflow Trigger Dispatch

Files:
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/event_handlers_children_completed.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/workflow_store.go`

Changes:
- Wire `watcher.EventHandlers.OnTaskStateChanged` to a new orchestrator handler.
- In the handler, ignore events where `parent_id` is empty or `new_state` is
  not terminal.
- Load active direct children of the parent and return early until all are
  terminal.
- Build `engine.OnChildrenCompletedPayload` from child rows.
- Build a deterministic operation id such as
  `children_completed:<parentID>:<sorted child id/state/updated_at digest>`.
- Resolve the parent active session with the existing repository session lookup.
- Evaluate `engine.TriggerOnChildrenCompleted` in `EvaluateOnly` mode and apply
  transition results through the same transition helper used by
  `on_turn_complete`, so `on_exit`, data persistence, DB step change, and
  `on_enter` fire correctly.
- If the trigger has only side-effect actions and no transition, mark the
  derived operation as applied after the engine evaluates callbacks.

Reason:
- `office/engine_dispatcher.Dispatcher` calls `engine.HandleTrigger` directly
  and does not apply returned transitions. Parent step movement must use the
  orchestrator transition lifecycle.

### Workflow Event DTOs

Files:
- `apps/backend/internal/task/dto/dto.go`
- `apps/backend/internal/task/dto/converters.go`
- `apps/backend/internal/task/dto/converters_test.go`

Changes:
- Extend `StepEventsDTO` to include `on_turn_start`, `on_exit`, and all generic
  trigger slices, especially `on_children_completed`.
- Update `FromWorkflowStep` so API responses preserve every non-empty event
  slice in `workflow_step.events`.

Reason:
- Existing converters only return `on_enter` and `on_turn_complete`, which would
  make configured generic triggers disappear from API consumers.

---

## Frontend

### Type Parity

Files:
- `apps/web/lib/types/workflow-actions.ts`
- `apps/web/lib/state/slices/kanban/types.ts`
- `apps/web/e2e/helpers/api-client.ts`

Changes:
- Add generic workflow action types for `move_to_next`, `move_to_previous`,
  `move_to_step`, `auto_start_agent`, `queue_run`,
  `queue_run_for_each_participant`, and `clear_decisions`.
- Extend `StepEvents` and `KanbanStepEvents` with
  `on_children_completed?: GenericAction[]` plus the other already-supported
  generic trigger names where useful for parity.
- Extend E2E API helper workflow-step update typings so tests can configure
  `on_children_completed`.

Reason:
- This feature does not require new visible UI, but TypeScript should accept and
  preserve workflow event JSON that the backend already supports.

---

## Tests

- **What:** active children terminal detection includes `COMPLETED`, `FAILED`,
  and `CANCELLED`; non-terminal states block; archived/ephemeral rows are
  ignored.
  **File:** `apps/backend/internal/task/repository/task_repository_test.go`
  **How:** SQLite-backed table-driven repository test.

- **What:** workflow-step DTO conversion preserves `on_children_completed`.
  **File:** `apps/backend/internal/task/dto/converters_test.go`
  **How:** unit test on `dto.FromWorkflowStep`.

- **What:** last direct child terminal state fires one parent
  `on_children_completed` trigger.
  **File:** `apps/backend/internal/orchestrator/event_handlers_children_completed_test.go`
  **How:** orchestrator unit/integration test with fake engine/repository.

- **What:** first child terminal state does not fire while siblings remain
  non-terminal.
  **File:** `apps/backend/internal/orchestrator/event_handlers_children_completed_test.go`
  **How:** table-driven orchestrator test.

- **What:** duplicate terminal updates do not duplicate parent actions.
  **File:** `apps/backend/internal/orchestrator/event_handlers_children_completed_test.go`
  **How:** invoke handler twice with same operation inputs and assert one engine
  action/transition.

- **What:** parent `move_to_next` action uses normal workflow transition
  lifecycle.
  **File:** `apps/backend/internal/orchestrator/event_handlers_children_completed_test.go`
  **How:** integration-style test asserts persisted parent step and `on_enter`
  side effect.

- **What:** frontend workflow event typings accept `on_children_completed`.
  **File:** no dedicated unit test unless a nearby type helper exists.
  **How:** `cd apps && pnpm --filter @kandev/web typecheck`.

---

## E2E Tests

- **Scenario:** GIVEN a parent task has two active children and its current
  workflow step defines `on_children_completed`, WHEN the first child completes
  and the second remains non-terminal, THEN the parent stays put. WHEN the
  second child completes, THEN the parent moves to the configured done step.
  **File:** `apps/web/e2e/tests/workflow/workflow-children-completed.spec.ts`
  **What to verify:** parent task workflow step changes only after every active
  direct child is terminal.

---

## Implementation Waves

Wave 1 (parallel):
- [x] [task-01-backend-child-completion-repository](task-01-backend-child-completion-repository.md)
- [x] [task-02-workflow-event-contracts](task-02-workflow-event-contracts.md)

Wave 2:
- [x] [task-03-orchestrator-trigger-dispatch](task-03-orchestrator-trigger-dispatch.md)

Wave 3:
- [x] [task-04-e2e-verification](task-04-e2e-verification.md)

---

## Verification Commands

Format first:

```bash
make fmt
```

Targeted backend tests:

```bash
go test ./internal/task/repository ./internal/task/dto ./internal/orchestrator ./internal/workflow/engine
```

Frontend typecheck:

```bash
cd apps && pnpm --filter @kandev/web typecheck
```

Focused E2E:

```bash
cd apps/web && pnpm e2e workflow-children-completed.spec.ts
```

Full verification:

```bash
make typecheck test lint
```

---

## Open Questions

None for the first implementation pass. The spec intentionally fixes direct
children only, best-effort child summaries, and no automatic parent session
creation.
