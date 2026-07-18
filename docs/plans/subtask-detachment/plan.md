---
spec: docs/specs/tasks/subtask-detachment.md
created: 2026-07-18
status: complete
---

# Implementation Plan: Subtask detachment

## Overview

Add one canonical backend detach operation that atomically clears hierarchy and normalizes inherited workspace policy, then expose it through the shared task-menu surfaces. Backend and frontend implementation can proceed independently after the HTTP contract is fixed; integrated desktop/mobile E2E follows both.

## Backend

### Canonical service operation

- Add `Service.DetachTask(ctx, taskID)` in `apps/backend/internal/task/service/service_detachment.go`.
- Load the task, return roots unchanged, and otherwise clear `ParentID` while preserving all unrelated fields.
- Copy and normalize `Metadata["workspace"]`: change only `mode: "inherit_parent"` to `mode: "shared_group"`.
- Persist through the task repository, reload repositories for the response, and publish `task.updated`.
- Update `apps/backend/internal/task/service/service_events.go` so `parent_id` is always present in task lifecycle payloads, including its cleared value.

### HTTP contract and Office parity

- Register `POST /api/v1/tasks/:id/detach` in `apps/backend/internal/task/handlers/task_handlers.go` and implement the handler beside existing task HTTP handlers.
- Return the updated task DTO, map missing tasks to `404`, and surface persistence errors consistently.
- Route the Office dashboard's empty-parent mutation through the canonical detach operation while leaving non-empty reparenting behavior unchanged.

## Frontend

### API and action state

- Add `detachTask(taskId)` to `apps/web/lib/api/domains/kanban-api.ts` and its barrel export.
- Add a focused detach action hook that calls the endpoint, reports failure through the existing toast pattern, and prevents duplicate confirmation submissions.
- Ensure the task mapper/store retains the workspace mode needed to render the shared-workspace warning.

### Menus and confirmation

- Add a reusable `TaskDetachConfirmDialog` under `apps/web/components/task/` with concise hierarchy-only copy and a conditional shared-workspace warning.
- Add `Detach from parent` with the existing unlink icon to `apps/web/components/task/task-switcher-context-menu.tsx` for single subtasks only.
- Thread the action through `TaskSwitcher`, desktop sidebar actions, and mobile action presentation.
- Add the same entry to `apps/web/components/kanban-card-menu-items.tsx`; the shared entry builder keeps card right-click and three-dot menus aligned.
- Route the Office parent picker's `No parent` selection through `detachTask`; non-empty parent changes continue using the Office update API.

## Tests

- **Canonical detach:** table-driven service tests cover root no-op, ordinary child, `inherit_parent`, `shared_group`, and `new_workspace` modes in `apps/backend/internal/task/service/service_detachment_test.go`.
- **Relationship preservation:** service tests prove descendants, blockers, repositories, sessions, and workspace-group rows are not mutated by the task-row operation.
- **Event clearing:** service/event tests assert `task.updated.parent_id` is present and empty after detach.
- **HTTP integration:** handler tests exercise success, root idempotency, missing task, and persistence failure.
- **Menu visibility:** component tests assert the action is present only for a subtask and hidden for root and multi-selection menus.
- **Card parity:** `apps/web/components/kanban-card-menu-items.test.tsx` asserts both menu renderers receive the detach entry and invoke the action.
- **Mapper/state:** frontend tests assert a cleared `parent_id` removes cached nesting and workspace mode is retained for confirmation copy.
- **Office picker:** component tests assert `No parent` calls the detach endpoint while selecting a parent uses the existing update endpoint.

## E2E Tests

- Add `apps/web/e2e/tests/task/subtask-detachment.spec.ts` for sidebar right-click, confirmation, live root promotion, card three-dot parity, root-menu absence, and descendant preservation.
- Add `apps/web/e2e/tests/task/mobile-subtask-detachment.spec.ts` for the touch-accessible task action and confirmation flow on `mobile-chrome`.
- Seed an inherited-workspace subtask and assert the confirmation warning and post-detach workspace context remain shared.

## Implementation Waves

Wave 1 (parallel):

- [x] [Task 01: Backend detach contract](task-01-backend-detachment.md) (`done`)
- [x] [Task 02: Frontend detach actions](task-02-frontend-actions.md) (`done`)

Wave 2:

- [x] [Task 03: E2E and verification](task-03-e2e-and-verification.md) (`done`)

## Verification

From `apps/backend`:

```bash
rtk go test ./internal/task/service ./internal/task/handlers ./internal/office/dashboard
```

From `apps/web`:

```bash
rtk pnpm test -- components/task/task-switcher.test.tsx components/kanban-card-menu-items.test.tsx components/task/simple/components/parent-picker.test.tsx lib/kanban/map-task.test.ts
rtk pnpm run typecheck
rtk pnpm run lint
rtk pnpm e2e:run e2e/tests/task/subtask-detachment.spec.ts e2e/tests/task/mobile-subtask-detachment.spec.ts
```

From the repository root after `make fmt`:

```bash
rtk make typecheck
rtk make test
rtk make lint
```

## Risks

- Omitting a cleared `parent_id` from WebSocket payloads would leave existing clients visually nested until reload.
- Leaving `inherit_parent` after clearing the parent would silently provision a separate workspace on a future launch.
- Mobile context menus are not assumed to be touch-accessible; the action must be wired through the existing mobile action presentation.
