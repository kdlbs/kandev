---
spec: docs/specs/tasks/requirements/subtask-reparenting-drag-drop.md
created: 2026-08-04
status: complete
---

# Implementation Plan: Subtask re-parenting by drag and drop

## Overview

The backend reparent contract (`PATCH /api/v1/tasks/:id` with `parent_id`, validated by `resolveParentID` + `validateReparentDepth`) already exists and is tested. Two deltas make drag-drop "strictly equivalent" to the menu composite (un-nest + nest): (1) normalize `inherit_parent` workspace mode to `shared_group` on any effective parent change in the canonical update path, and (2) add the same normalization to the Office dashboard PATCH for parity. The real work is frontend: hoist the sidebar tree's per-level `DndContext` into one per-group context, add nest drop zones with a pure drop-decision helper, and wire `onReparentTask` through `TaskSwitcher` to the existing `useNestTask` hook. E2E (desktop + mobile touch) comes last.

---

## Backend

### Canonical update normalization (`apps/backend/internal/task/service/service_tasks.go`)

In `Service.UpdateTask`, the parent block (~line 1225) already resolves and assigns `task.ParentID` only when the parent effectively changed. After that block, add a small helper `normalizeWorkspaceModeAfterReparent(task)` that copies `task.Metadata["workspace"]` and changes only `mode: "inherit_parent"` → `"shared_group"` — the exact detach semantics — applied whenever the effective parent changed (set or cleared). The existing full-row persist writes the metadata; the existing `parent_id` event emission (explicit nil on clear) is unchanged. This covers core PATCH, WS update, the sidebar menu, the Office parent picker (`updateTask`), and the drag-drop, all through one path.

### Office dashboard parity (`apps/backend/internal/office/dashboard/service_tasks.go`)

`DashboardService.UpdateTaskParentID` non-empty path currently writes `parent_id` directly and publishes `["parent_id"]`. Change it so that after the repo write it loads the task, and if workspace mode is `inherit_parent`, persists `shared_group` and publishes fields `["parent_id", "metadata"]`. Add `UpdateTaskWorkspaceMode(ctx, taskID, mode)` to the office sqlite repository (dialect-aware JSON set, mirroring `detachTaskQuery`'s mode branch) and to the `DashboardService` repo interface. The empty-parent path keeps routing to the canonical detacher.

### Events

No new event types. `task.updated` already carries `parent_id` (nil when cleared) and `metadata`; the normalization rides along. Office `OfficeTaskUpdated` gains `metadata` in its `fields` list only when the mode changed.

## Frontend

### Drop-decision logic (`apps/web/components/task/task-switcher-subtask-dnd.tsx`)

- Extract a pure, unit-testable `resolveSidebarDrop({ activeId, overId, groupRootIds, childrenByParent })` returning `{kind:"nest", parentTaskId}` | `{kind:"reorder", level:"group"|"subtasks", parentTaskId?, orderedTaskIds}` | `null`. `overId` prefixed `nest:` → nest; otherwise both tasks must share a level (group root list or one parent's children) → `arrayMove` within that level; anything else → `null`.
- Add `NestDropZone` (small component): `useDroppable({ id: "nest:"+taskId })`, absolute left-edge strip, `data-testid="nest-drop-zone"`, `data-task-id`, hover affordance label "Nest under <title>". Rendered only for rows that are valid targets of the active drag.
- Add a `computeNestTargets(activeTask, groupTasks)` helper: flatten the group's subtree, filter to the active task's `workflowId`, run the existing `computeNestCandidates` from `apps/web/lib/sidebar/nest-candidates.ts`. Returns the candidate id set (empty for tasks with children).

### DndContext hoisting (`task-switcher-subtask-dnd.tsx` + `apps/web/components/task/task-switcher.tsx`)

- `SortableTaskLevel` currently creates one `DndContext` per sibling level, which makes cross-level drops impossible. Change `SortableTaskLevel` so it renders only `SortableContext` + nodes when a group-level context is present (new prop, e.g. `externalDragContext`), preserving current behavior for non-reorderable callers.
- `GroupSection` in `task-switcher.tsx` renders ONE `DndContext` (sensors from `taskSwitcherDragActivationConstraints`, custom `collisionDetection`: `pointerWithin` filtered to `nest:` ids wins, else `closestCenter`, `onDragEnd` dispatching through `resolveSidebarDrop`).
- Track the active drag id (`onDragStart`) in the group; compute `computeNestTargets` and thread candidate ids through `TaskTreeContext` so `TaskTreeNode` renders `NestDropZone` on candidate rows.
- New `TaskSwitcher` prop `onReparentTask?: (taskId: string, parentTaskId: string) => void`, threaded through `GroupSection` → `TaskTreeContext` → drop handling.

### Sidebar + mobile wiring

- `apps/web/components/task/task-session-sidebar.tsx`: `handleReparentTask(taskId, parentTaskId)` resolves the task's `workflowId` from `displayTasks` and calls the existing `useNestTask` (non-null path → `updateTask(taskId, { parent_id })` → composite semantics after the backend change; same optimistic snapshot patch, rollback, and toast).
- `apps/web/components/task/mobile/session-task-switcher-sheet.tsx`: same wiring; touch drag already works via the existing `TouchSensor` (250 ms delay / 5 px tolerance).

### i18n

New user-facing copy (nest-zone label, aria text) goes through `t()` / `<Trans>` with keys added to the sidebar's locale namespace; do not hardcode.

## Tests

### Backend

- **What:** reparenting an `inherit_parent` child normalizes mode to `shared_group` (persisted + event metadata).
  - **File:** `apps/backend/internal/task/service/service_reparent_test.go`
  - **How:** extend the existing reparent fixture; assert `Metadata["workspace"]["mode"]` in the returned task, the persisted row, and the `task.updated` payload.
- **What:** non-`inherit_parent` modes (`shared_group`, `new_workspace`) and root-task reparents are untouched.
  - **File:** same, table-driven.
- **What:** un-nest via `UpdateTask` (empty `parent_id`) normalizes like the detach endpoint.
  - **File:** same.
- **What:** Office dashboard non-empty reparent normalizes `inherit_parent` and publishes `metadata` in fields.
  - **File:** `apps/backend/internal/office/dashboard/service_detachment_test.go` (mirror `TestUpdateTaskParentIDUsesCanonicalDetacherWhenCleared`).
- Existing reparent/detach/handler suites must stay green (no changes to `resolveParentID` / `validateReparentDepth`).

### Frontend

- **What:** drop decision maps `nest:` over-ids and same-level reorders correctly, rejects cross-level drops.
  - **File:** `apps/web/components/task/task-switcher-subtask-dnd.test.ts`
  - **How:** pure-function unit tests for `resolveSidebarDrop`.
- **What:** nest targets = same-workflow roots per `computeNestCandidates`, empty for tasks with children.
  - **File:** `apps/web/lib/sidebar/nest-candidates.test.ts` (extend) or a new sibling test.
- **What:** existing `TaskSwitcher` / sortable rendering behavior is preserved (regression).
  - **File:** `apps/web/components/task/task-switcher.test.tsx`.

### E2E

See task 04. Desktop spec `apps/web/e2e/tests/task/subtask-reparent-drag-drop.spec.ts` + mobile touch variant, modeled on `subtask-detachment.spec.ts` / `mobile-subtask-detachment.spec.ts` and the `Input.dispatchTouchEvent` pattern in `mobile-automations-scroll.spec.ts`.

## E2E Tests

- **Scenario:** subtask dragged onto another root's nest zone re-parents (API `parent_id` + sidebar nesting, no reload).
- **Scenario:** `inherit_parent` subtask ends with `shared_group` (assert via API metadata).
- **Scenario:** root dragged onto root nests; task with children offers no nest zone; drop on an invalid row is a no-op.
- **Scenario:** sibling reorder still works (regression).
- **Scenario:** mobile touch drag re-parents (mobile-chrome project).
- **File:** `apps/web/e2e/tests/task/subtask-reparent-drag-drop.spec.ts`

## Verification Results

All task checks passed; see each task's `## Results` for exact commands and counts.

- Backend: `go test ./internal/task/service ./internal/task/handlers ./internal/office/dashboard` — service + office/dashboard `ok`; handlers `ok` under `umask 022` (3 pre-existing env failures under umask 002, verified on base).
- Web: full unit suite passes (8441+ of 8450; the 5 failures are pre-existing env issues: 3 Docker-bridge `http-git-server`, 2 load-sensitive file-browser timeouts that pass in isolation); typecheck, lint, i18n ratchet clean.
- E2E: `subtask-reparent-drag-drop.spec.ts` 5/5 (chromium), `mobile-subtask-reparent-drag-drop.spec.ts` 1/1 (mobile-chrome touch).
- Root: `make fmt`, `make typecheck`, `make lint` pass. `make test` residual failures are pre-existing environment-dependent tests (launchd/systemd/cli-shim) unrelated to this feature.

## Implementation Waves And Parallel Candidates

```
Wave 1:
- [x] [task-01-backend-composite-semantics](task-01-backend-composite-semantics.md)

Wave 2:
- [x] [task-02-frontend-nest-drop-infrastructure](task-02-frontend-nest-drop-infrastructure.md)

Wave 3:
- [x] [task-03-frontend-wiring-and-affordances](task-03-frontend-wiring-and-affordances.md)

Wave 4:
- [x] [task-04-e2e-and-verification](task-04-e2e-and-verification.md)
```

All tasks are sequential (each depends on the previous). No parallel candidates.

## Open Questions

None.
