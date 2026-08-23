---
spec: docs/specs/tasks/requirements/task-create-workflow-memory.md
created: 2026-08-08
status: done
---

# Implementation Plan: Task Create Workflow Memory

## Overview

Extend the existing backend-owned task-create preference with a targeted
per-workspace workflow map, then make the shared dialog resolver prefer that
map over unlocked board/list context. Backend contract and merge semantics land
first, frontend mapping and resolution follow, and a production-build E2E test
proves persistence across workspace switches and conflicting filters.

## Confirmed root cause

`TaskCreateLastUsed` and every wire/store mapper omit workflow, while
`resetTaskForm` initializes `selectedWorkflowId` only from the caller's
`workflowId`. The selector's `lastUsedWorkflowId` is component-local and only
sorts menu choices. Consequently, a dialog opened with `workflowId=null` has no
durable workflow default when multiple workflows exist, and a dialog opened
under a filter inherits that filter rather than the workflow that created the
previous task.

## Backend

### Per-workspace preference contract

- Add `WorkflowIDsByWorkspace map[string]string` with JSON key
  `workflow_ids_by_workspace` to
  `apps/backend/internal/user/models/models.go:TaskCreateLastUsed`.
- Keep the field inside the existing `users.settings.task_create_last_used`
  JSON object; no table or column migration is required.
- Treat a non-empty workflow map as a meaningful task-create patch in
  `apps/backend/internal/user/service/service.go` and preserve it through DTO,
  HTTP response, WebSocket event, and boot-state mapping.
- Extend `apps/backend/internal/backendapp/boot_state_routes.go` so the boot
  payload exposes the complete map and marks the task-create state synced when
  the map is the only populated preference.

### Targeted SQLite and PostgreSQL merge

- Extend `makeTaskCreateLastUsedJSONSetArgs` in
  `apps/backend/internal/user/store/sqlite.go` to update one nested
  `workflow_ids_by_workspace.<workspace_id>` entry at a time.
- Extend `applyPostgresTaskCreateLastUsedPatch` with equivalent parameterized
  `jsonb_set` paths. Do not serialize and replace the whole map: alternating or
  concurrent task creation in two workspaces must preserve both entries.
- Preserve the existing targeted-update behavior for repository, branch, agent
  profile, and executor profile, including broad user-settings writes racing
  with task creation.

### Record the effective workflow

- Extend `buildTaskCreateLastUsedPatch` in
  `apps/backend/internal/task/handlers/task_http_handlers.go` to add
  `body.WorkflowID` under `body.WorkspaceID` after successful creation.
- Update the WebSocket task-create bridge in
  `apps/backend/internal/task/handlers/task_ws_handlers.go` to pass
  `req.WorkspaceID` and `req.WorkflowID` into the same recorder path.
- Preserve the current warning-only behavior when preference recording fails;
  successful task creation is not rolled back.

## Frontend

### Wire mapping and queued overlay

- Add `workflow_ids_by_workspace?: Record<string, string>` to
  `TaskCreateLastUsedApi` and `workflowIdsByWorkspace: Record<string, string>`
  to the settings store state.
- Update `apps/web/lib/ssr/user-settings.ts`, boot/HTTP/WS mapping tests, and
  default settings so absent maps normalize to `{}` and existing installations
  remain compatible.
- Extend the task-create queued overlay in
  `apps/web/components/task-create-dialog-handlers.ts` to derive the entry from
  submitted `workspace_id` and `workflow_id`. Merge queued workflow entries
  into, rather than replace, the authoritative map and clear the overlay only
  after every queued entry matches backend settings.
- Update `apps/web/components/state-provider.tsx` equality logic for map
  contents so settings publication clears the overlay deterministically.

### Workflow default resolution

- Add a pure resolver in
  `apps/web/components/task-create-dialog-defaults.ts` for the confirmed
  precedence: locked workflow; current-dialog manual choice; valid visible
  per-workspace last-used workflow; valid visible unlocked context; sole
  visible workflow; otherwise no selection.
- Keep unlocked board/list `workflowId` as fallback context rather than seeding
  it as an authoritative form selection. Preserve explicit locked flows by
  passing the existing `lockedFields.workflow` signal through form reset and
  late-value synchronization.
- Thread `taskCreateLastUsed.workflowIdsByWorkspace[workspaceId]` and user
  settings readiness through `useTaskCreateDialogData` / `useDialogComputed`.
  Validate every candidate against the visible workflows in the current
  workspace before returning `effectiveWorkflowId`.
- Make workflow-step loading and workflow-agent override effects consume the
  resolved effective workflow so a restored default receives the same steps
  and agent lock as a manual selection.
- Remove the misleading component-local `lastUsedWorkflowId` sorting state;
  durable task-create recency now comes from backend settings and option order
  remains workflow `sort_order`.

### Mobile design contract

- **Desktop and mobile outcome:** standard Create Task restores the same
  workspace-specific workflow regardless of viewport or current filter.
- **Nearest shipped mobile exemplar:** the existing Kanban FAB → Create Task
  dialog remains the entry point and presentation baseline.
- **Presentation:** no overlay, navigation, hierarchy, scroll owner, safe-area,
  focus, or touch-target changes. Exactly-one-workflow suppression remains
  shared across viewports.
- **Shared logic:** wire mapping, queued overlay, and default resolution stay in
  shared state/hooks; no mobile-specific preference is introduced.
- **Mobile proof:** focused unit tests cover the shared resolver and the
  existing mobile Kanban workflow-context E2E continues to cover dialog
  reachability. No new mobile-only Playwright scenario is required because the
  change is state normalization inside unchanged composition.

## Tests

- **What:** HTTP and WebSocket task creation record the effective workflow under
  the correct workspace key.
  - **File:** `apps/backend/internal/task/handlers/task_http_handlers_test.go`
    (the existing file covers both HTTP and WebSocket task-create handlers).
  - **How:** extend recorder-capture tests for both transports and assert the
    workspace-to-workflow entry alongside existing profile/repository fields.
- **What:** targeted task-create preference writes preserve workflow entries for
  other workspaces and all existing last-used fields on SQLite and PostgreSQL
  query paths.
  - **File:** `apps/backend/internal/user/store/sqlite_test.go`.
  - **How:** real SQLite repository test plus PostgreSQL query-builder
    assertions using two workspace keys and partial patches.
- **What:** service, DTO, event, and boot payload treat workflow-only history as
  loaded task-create state.
  - **File:** `apps/backend/internal/user/service/service_test.go` and
    `apps/backend/internal/backendapp/boot_state_routes_test.go`.
  - **How:** record a workflow-only patch and assert the published/boot map and
    `synced` state.
- **What:** the frontend mapper and queued overlay preserve multiple workspace
  entries and clear only after backend convergence.
  - **File:** `apps/web/lib/ssr/user-settings.test.ts`,
    `apps/web/hooks/use-ensure-user-settings.test.ts`,
    `apps/web/components/state-provider.test.tsx`, and
    `apps/web/components/task-create-dialog-handlers.test.ts`.
  - **How:** map missing and populated wire values, merge two workspace entries,
    and exercise stale/fresh settings publication.
- **What:** workflow resolution prefers per-workspace last-used state over an
  unlocked conflicting filter, while manual and locked selections win and
  invalid history falls through.
  - **File:** `apps/web/components/task-create-dialog-defaults.test.ts` and
    focused dialog state/effect tests.
  - **How:** table-driven pure resolver cases plus hook coverage proving steps
    and workflow-agent overrides follow the effective workflow.
- **What:** one visible workflow remains implicit with no selector.
  - **File:** existing
    `apps/web/components/task-create-dialog-form-body.test.tsx` and
    `apps/web/e2e/tests/task/create-task.spec.ts` regressions.
  - **How:** retain the current unit and E2E assertions unchanged unless wiring
    changes require fixture updates.

## E2E Tests

- **Scenario:** **GIVEN** workspace A remembers Dev while its board is filtered
  to PR Review, **WHEN** standard Create Task opens, **THEN** Dev is selected.
  - **File:** `apps/web/e2e/tests/task/create-task.spec.ts`.
  - **What to verify:** create the remembered task through the real HTTP API,
    persist the conflicting filter, open the dialog through the regular UI,
    and assert the visible workflow trigger text.
- **Scenario:** **GIVEN** workspace A remembers Dev and workspace B remembers
  Support, **WHEN** the user opens standard Create Task in each workspace,
  **THEN** each workspace restores its own workflow.
  - **File:** `apps/web/e2e/tests/task/create-task.spec.ts`.
  - **What to verify:** seed both histories through successful task creation,
    navigate between workspaces, and assert each dialog independently after a
    page reload so the Go boot payload and frontend mapper are exercised.
- **Scenario:** **GIVEN** a single visible workflow, **WHEN** Create Task opens,
  **THEN** no workflow selector appears.
  - **File:** existing `apps/web/e2e/tests/task/create-task.spec.ts` scenario.
  - **What to verify:** keep the existing hidden-workflow-plus-one-visible
    regression green.

## Verification Results

All planned checks pass:

- Backend: `cd apps/backend && go test ./internal/user/store ./internal/user/service ./internal/task/handlers ./internal/backendapp` — all four packages passed.
- Frontend: `cd apps/web && pnpm test -- --run lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts hooks/use-ensure-user-settings.test.ts components/state-provider.test.tsx components/task-create-dialog-handlers.test.ts components/task-create-dialog-defaults.test.ts components/task-create-dialog-state.test.ts components/task-create-dialog-effects.test.ts components/task-create-dialog-form-body.test.tsx` — 9 files, 158 tests passed.
- Frontend typecheck: `cd apps/web && pnpm run typecheck` — passed.
- E2E focused production-build run: `cd apps/web && pnpm e2e:run tests/task/create-task.spec.ts -- --grep "remembered workflow"` — 2 tests passed.
- E2E selector/regression run: `cd apps/web && pnpm e2e:run --no-build tests/task/create-task.spec.ts -- --grep "single visible workflow|remembered workflow"` — 3 tests passed.
- E2E full task-create spec: `cd apps/web && pnpm e2e:run --no-build tests/task/create-task.spec.ts` — 15 tests passed in 39 seconds.
- Managed E2E runners exited cleanly and removed their isolated `/tmp/kandev-e2e-*` worker directories; no failure artifacts were produced.

## Implementation Waves And Parallel Candidates

Execution is sequential in the primary conversation because the frontend wire
contract depends on the backend field and the E2E scenario depends on both.
No task is marked parallel-safe and no subagent delegation is authorized.

Wave 1:

- [x] [Task 01: Persist per-workspace workflow memory](task-01-backend-workflow-memory.md)

Wave 2:

- [x] [Task 02: Restore the remembered workflow](task-02-frontend-workflow-resolution.md)

Wave 3:

- [x] [Task 03: Prove workflow memory through Create Task](task-03-create-task-e2e.md)

## Risks And Out Of Scope

- Nested JSON path construction must remain parameterized and safe for both
  SQLite and PostgreSQL; replacing the complete map would reintroduce lost
  updates across workspaces.
- Unlocked caller context currently doubles as form state. Separating fallback
  context from manual/locked selection must preserve edit and Improve Kandev
  locked flows.
- User settings can arrive after the dialog opens. The resolver must not
  overwrite a manual selection made in the current open cycle when settings
  settle.
- Workflow deletion does not eagerly prune history; validation ignores stale
  IDs and the next successful creation overwrites that workspace entry.
- Changing the board/list filter, dialog composition, workflow ordering, and
  cancelled-dialog persistence remain out of scope.
