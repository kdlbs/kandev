---
spec: docs/specs/tasks/requirements/interrupted-task-indicator.md
created: 2026-08-02
status: complete
---

# Implementation Plan: Interrupted Task Indicator

## Overview

Detect tasks whose sessions were mid-turn (`STARTING`/`RUNNING`) when the
backend died, persist a durable `interrupted_at` marker in task metadata at
startup reconciliation, expose `interrupted` on the task DTO, and render a red
alert icon on every task-list surface until the task's session starts again.
No schema migration; no new task/session state; no new events or routes.

## Backend

### Marker key and detection

- Add `MetaKeyInterruptedAt = "interrupted_at"` beside the other task metadata
  keys in `apps/backend/internal/task/models/models.go` (the `MetaKey*` const
  block).
- In `apps/backend/internal/orchestrator/service.go` `reconcileOneSessionOnStartup`,
  in the active-states branch (`STARTING`/`RUNNING`/`WAITING_FOR_INPUT`), when
  `previousState` is `TaskSessionStateStarting` or `TaskSessionStateRunning`,
  write the marker with the archive-atomic conditional
  `SetTaskMetadataKeyIfNotArchived(ctx, running.TaskID, models.MetaKeyInterruptedAt, time.Now().UTC().Format(time.RFC3339))`
  — the SQL guards `archived_at IS NULL` in the same statement, so an archive
  that commits concurrently can never leave a marker on an archived task.
  Log a warning on failure without aborting reconciliation.
- The `sessionExecutorStore` interface in the same file must gain
  `SetTaskMetadataKey(ctx, taskID, key string, value interface{}) error`,
  `SetTaskMetadataKeyIfNotArchived(ctx, taskID, key string, value interface{}) (bool, error)`,
  and `RemoveTaskMetadataKey(ctx, taskID, key string) (bool, error)`. The
  concrete sqlite/Postgres repository implements all three
  (`apps/backend/internal/task/repository/sqlite/task.go`); test wrappers that
  embed the interface inherit them.

### DTO exposure

- Add `Interrupted bool \`json:"interrupted,omitempty"\`` to `v1.Task` in
  `apps/backend/pkg/api/v1/task.go` AND to `TaskDTO` in
  `apps/backend/internal/task/dto/dto.go` (TaskDTO is the rich wire payload
  for boot/task.updated; `v1.Task` is the list-endpoint payload).
- Derive it at both task serializers where `Metadata` is copied:
  `FromTaskWithSessionInfo` in `apps/backend/internal/task/dto/dto.go`
  (~line 714) and `(*Task).ToAPI` in `apps/backend/internal/task/models/models.go`
  (~line 1675): `Interrupted: task.Metadata[models.MetaKeyInterruptedAt] != nil`.

### Clearing

- In `apps/backend/internal/orchestrator/event_handlers_streaming.go`, clear
  the marker inside the session-start funnel. Verify the funnel: the
  `updateTaskSessionStateWithHook` helper (and `setSessionStarting`) must cover
  every path that moves a session into `STARTING`/`RUNNING` — launch, resume,
  prompt dispatch, agent-ready wake. On a transition to `STARTING` or
  `RUNNING`, call `RemoveTaskMetadataKey(taskID, MetaKeyInterruptedAt)`; when
  it reports the key was removed, load the task and
  `publishTaskUpdated(ctx, task)` so open clients drop the icon live.
- Do not clear in `reconcileSessionsOnStartup` itself (it writes
  `WAITING_FOR_INPUT` directly, not through the funnel).

## Frontend

### Data plumbing

- `apps/web/lib/types/http.ts` — `Task` gains `interrupted?: boolean`.
- `apps/web/lib/kanban/map-task.ts` — `TaskLike` gains `interrupted?: boolean`;
  `toKanbanTask` maps `interrupted: source.interrupted ?? undefined`.
- `apps/web/lib/state/slices/kanban/types.ts` — the kanban task row gains
  `interrupted?: boolean`.
- `apps/web/lib/ws/handlers/tasks.ts` — `mergeTaskUpdate` preserves an existing
  `interrupted` value when the payload omits the field (mirror the
  `foreground_activity` guard) so a lightweight `task.updated` cannot wipe a
  set marker.
- `apps/web/components/task/task-switcher.tsx` — `TaskSwitcherItem` gains
  `interrupted?: boolean`; `TaskRow` passes it to `TaskItem`.
- `apps/web/components/task/task-session-sidebar-item.ts` — `buildSidebarItem`
  maps `interrupted: task.interrupted`.
- `apps/web/components/task/mobile/session-task-switcher-sheet-hooks.ts` —
  map the same field into mobile switcher items (the mobile drawer shares
  `TaskItem`, so the icon renders there automatically once the field flows).

### Icon rendering

- `apps/web/components/task/task-item.tsx` — `TaskItem` gains an
  `interrupted?: boolean` prop; `TaskStateIcon` gains a branch that renders a
  red alert icon (`IconAlertCircle`, `text-red-500`, `data-testid="task-state-interrupted"`)
  with a focusable tooltip/aria-label ("Interrupted by restart", externalized).
  Precedence: after pending permission/clarification, generating/background
  activity, preparing, and the running spinner; before the review/done branch.
  It must never override a `COMPLETED`/`FAILED`/`CANCELLED` affordance.
- `apps/web/lib/ui/state-icons.tsx` — `getTaskStateIconConfig` /
  `getTaskStateIcon` gain an `interrupted` parameter; the interrupted config
  renders a red `IconAlertCircle` and wins over the coarse-state fallback for
  non-terminal states. Update every caller:
  `apps/web/components/kanban-card-content.tsx`,
  `apps/web/components/kanban/graph2-step-node.tsx`,
  `apps/web/components/kanban/swimlane-graph-content.tsx`,
  `apps/web/app/tasks/rich-task-list-row.tsx` (path TBD),
  `apps/web/components/task/task-state-actions.tsx`.
- i18n: add the tooltip/aria copy to the appropriate locale namespace
  (`apps/web/src/locales/en/*.json`) and reference it through `t()`; the string
  is new code and must pass `pnpm run i18n:ratchet`.

## Tests

- **Reconciliation marks interrupted tasks** — extend
  `apps/backend/internal/orchestrator/task_operations_test.go` /
  `reconcile_restart_test.go`: a `RUNNING` and a `STARTING` session's task gets
  `interrupted_at` metadata; a `WAITING_FOR_INPUT` session's task does not; an
  archived task is not marked; a `CREATED`/terminal session is not marked.
- **DTO derivation** — a task DTO built from a model with
  `metadata["interrupted_at"]` reports `interrupted: true`; without it,
  `false`/omitted.
- **Clearing** — after a session transitions to `STARTING` (via the funnel),
  the marker is removed and `task.updated` is published; a transition that does
  not remove the key publishes nothing extra.
- **Frontend unit tests** —
  `apps/web/components/task/task-item.test.tsx` (icon precedence cases),
  `apps/web/lib/ui/state-icons.test.tsx` (interrupted param cases),
  `apps/web/lib/kanban/map-task.test.ts` (mapping),
  `apps/web/lib/ws/handlers/tasks.test.ts` (merge preservation).

## E2E Tests

- **File:** `apps/web/e2e/tests/tasks/task-interrupted-icon.spec.ts` (create the
  directory if the convention differs — follow sibling spec layout).
- **Scenario 1:** seed two tasks via the API client
  (`createTask(workspaceId, title, { metadata: { interrupted_at: "<rfc3339>" } })`
  and one without), open the task list, assert the marked task shows
  `task-state-interrupted` and the plain task does not.
- **Scenario 2:** a task with `interrupted_at` and `state: "COMPLETED"` shows
  the done icon, not the red icon (precedence).
- The e2e harness runs with `KANDEV_MOCK_AGENT`/e2e profile; no real executor
  is needed.

## Implementation Waves And Parallel Candidates

Execution remains sequential in the primary conversation.

Wave 1:

- [x] [task-01-backend-startup-marker](task-01-backend-startup-marker.md)

Wave 2:

- [x] [task-02-backend-dto-and-clearing](task-02-backend-dto-and-clearing.md)

Wave 3:

- [x] [task-03-frontend-data-plumbing](task-03-frontend-data-plumbing.md)

Wave 4:

- [x] [task-04-frontend-icon-rendering](task-04-frontend-icon-rendering.md)

Wave 5:

- [x] [task-05-e2e-verification](task-05-e2e-verification.md)

Task 02 depends on the marker key from Task 01; Task 03 depends on the DTO
field from Task 02; Task 04 depends on the plumbed field from Task 03; Task 05
depends on the rendered icon from Task 04. No tasks are parallel-safe.

## Risks

- **Funnel coverage:** if the clear hook misses a start path (e.g. a direct
  repo write on resume), the icon persists after resume. Mitigate by clearing
  inside `updateTaskSessionStateWithHook` for both `STARTING` and `RUNNING`
  next-states and asserting the funnel in tests.
- **Client merge wipe:** a `task.updated` without `interrupted` must preserve
  the marker; the `foreground_activity` guard is the model.
- **Remote rows:** a live remote agent may be marked interrupted until its
  status poll re-reports it; acceptable per spec, do not special-case without
  approval.
- **Precedence regressions:** the red icon must never cover permission,
  clarification, activity, or running affordances; the task-item and
  state-icons test suites pin this.
- **i18n ratchet:** the new tooltip copy must go through `t()` with a real
  locale entry, or `pnpm run i18n:ratchet` fails.
