---
spec: docs/specs/ui/requirements/sidebar-archived-filter.md
created: 2026-08-04
status: implemented
---

# Implementation Plan: Sidebar Archived Task Views

## Overview

Keep active workflow snapshots unchanged and add a separate, workspace-scoped
archived-task projection for the sidebar. Restore the Archived filter contract,
load the projection only for views that explicitly request archived tasks, and
keep it synchronized through the existing task lifecycle WebSocket events.
Finish by proving the same browse-and-open flow in the desktop sidebar and the
mobile task-switcher drawer.

The current branch contains a superseded implementation that removed the
Archived dimension. The implementation tasks below restore that contract and
replace the retirement tests; they do not revert unrelated work on the branch.

## Backend

### Archived-only workspace task query

- `apps/backend/internal/task/handlers/task_http_handlers.go`: parse
  `only_archived=true` on `GET /api/v1/workspaces/:id/tasks`. Pass both archive
  flags through the service; `only_archived` takes precedence when both are
  present.
- `apps/backend/internal/task/service/service_tasks.go` and
  `apps/backend/internal/task/repository/interface.go`: extend
  `ListTasksByWorkspace` and `prSearchOptions` with `onlyArchived bool` without
  changing authorization, pagination, enrichment, ephemeral, config-task, or
  PR-number search behavior.
- `apps/backend/internal/task/repository/sqlite/task.go`: apply exactly one
  archive predicate before count/query execution:
  `archived_at IS NOT NULL` for archived-only, no predicate for
  include-archived, and `archived_at IS NULL` otherwise. Apply the same mode to
  search and PR-match augmentation so `total` and page contents agree.
- Update affected repository mocks mechanically; no schema or migration is
  required.

## Frontend

### Restore the saved-view filter contract

- Restore `archived` in
  `apps/web/lib/state/slices/ui/sidebar-view-types.ts`, `KNOWN_DIMENSIONS` in
  `ui-slice.ts`, the dimension registry, and the `apply-view.ts` extractor.
- Keep the generic draft migration helper and its boot/hydration/WebSocket
  calls, but update regressions so valid archived clauses are preserved rather
  than removed.
- Add a pure predicate that returns true when an effective view contains the
  positive boolean clause `archived is true` or its equivalent
  `archived is_not false`. The default view and `Archived: Hide` must not
  request or merge archived candidates.

### Archived task runtime projection

- `apps/web/lib/api/domains/kanban-api.ts`: add `onlyArchived` to
  `listTasksByWorkspace` and emit `only_archived=true`.
- Extend the Kanban slice with a separate runtime cache:
  `sidebarArchivedTasks.itemsByWorkspaceId`, `loadedByWorkspaceId`,
  `loadingByWorkspaceId`, and `errorByWorkspaceId`, plus replace/upsert/remove
  actions. Do not place archived rows in `kanban.tasks` or
  `kanbanMulti.snapshots`.
- Extend `TaskLike`/`toKanbanTask` with `workspace_id` and `archived_at` so API
  and WebSocket rows carry workspace identity and archive state through one
  mapper.
- Add `use-sidebar-archived-tasks.ts`. When enabled, page through the
  archived-only API with `page_size=100`, commit only if workspace and request
  generation are still current, retain a successful cache on refresh failure,
  and register a foreground refresh. Multiple desktop/mobile consumers must
  share the store loading gate rather than issue duplicate requests.
- `use-workspace-sidebar-tasks.ts` merges the current workspace's archived
  projection into the active aggregate only when the effective view requires
  it, deduplicating by task ID. Loading and error state cover the archived
  request as well as the initial workflow snapshot request.
- `apps/web/lib/ws/handlers/tasks.ts`: on archived `task.updated`, remove the
  task from active Kanban caches and upsert it into its workspace's archived
  cache; on non-archived `task.updated`, remove any archived copy before the
  existing active upsert; on `task.deleted`, remove it from both projections.
  These updates must preserve the current redirect, recent-task, sidebar-pref,
  and Office-refetch behavior.

### Desktop and mobile consumption

- `task-session-sidebar-item.ts` and
  `mobile/session-task-switcher-sheet-hooks.ts` map cached archived tasks to
  `TaskSwitcherItem` with `isArchived: true`, full workflow/step/repository
  labels, and no duplicate synthetic current-task row.
- Desktop and mobile selection paths look in the archived projection. An
  archived row navigates directly to task detail and never attempts to prepare
  or launch a session; the existing detail top bar owns Unarchive.
- Archived rows remain read-only for active-task operations: modifier clicks
  navigate instead of entering multi-selection, and active-only rename/edit,
  pin, link, subtask, archive, detach, and move actions are absent. Existing
  delete behavior may remain available.
- Extend the shared task switcher loading/empty treatment with a localized
  archived-load failure and Retry action. Add keys to
  `apps/web/src/locales/en/sidebar.json`; do not introduce hardcoded UI copy.

## Mobile design contract

- **Outcome and entry point:** desktop uses the AppSidebar Tasks section;
  phones use the existing `SessionTaskSwitcherSheet` opened from task chrome.
  Both edit the same saved view and show the same archived result set.
- **Nearest exemplar:**
  `apps/web/components/task/mobile/session-task-switcher-sheet.tsx` remains the
  shipped inset bottom-drawer composition. Its fixed header, filter bar,
  `min-h-0` scrolling task body, dismiss behavior, and safe-area handling stay
  intact.
- **Hierarchy and action:** the filter remains a temporary popover choice; the
  task list remains the single scroll owner and selecting an archived row is
  the primary action. No desktop panel is added or squeezed into the drawer.
- **Shared logic:** API loading, workspace cache, view predicate, filtering,
  sorting, grouping, archive state, and navigation guards are shared. Mobile
  code only adapts the existing drawer presentation and closes it after
  navigation.
- **Geometry:** the drawer and portaled filter selector remain within the Pixel
  5 viewport, long archived results scroll inside the task body, and document
  horizontal overflow remains zero.

## Tests

- **Archived-only repository/API contract:** default, all, and only-archived
  modes return the correct rows and totals with search/pagination; conflicting
  flags choose only-archived. **Files:**
  `apps/backend/internal/task/repository/archive_repository_test.go`,
  `apps/backend/internal/task/service/service_pr_search_test.go`, and
  `apps/backend/internal/task/handlers/task_http_handlers_test.go`.
- **Filter restoration and persistence:** the registry exposes Archived,
  `applyView` evaluates Show/Hide, and saved views/drafts preserve the clause at
  every settings boundary. **Files:**
  `apps/web/components/task/sidebar-filter/filter-dimension-registry.test.ts`,
  `apps/web/lib/sidebar/apply-view.test.ts`,
  `apps/web/lib/state/slices/ui/ui-slice-migration.test.ts`,
  `apps/web/lib/state/hydration/hydrator.test.ts`, and
  `apps/web/lib/ws/handlers/users.test.ts`.
- **Archived projection loader:** verifies positive-clause gating, pagination,
  dedupe, loading/error/cache retention, foreground retry, and stale workspace
  rejection. **Files:** `apps/web/lib/api/domains/kanban-api.test.ts`,
  `apps/web/lib/state/slices/kanban/kanban-slice.test.ts`,
  `apps/web/hooks/domains/kanban/use-sidebar-archived-tasks.test.ts`, and
  `apps/web/hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts`.
- **Live lifecycle projection:** archive upserts the archived cache while
  evicting active caches; unarchive and delete remove the archived copy.
  **Files:** `apps/web/lib/ws/handlers/tasks-archive.test.ts`,
  `tasks-unarchive.test.ts`, and `tasks.deleted.test.ts`.
- **Rendered row behavior:** desktop/mobile mapping sets the archived badge,
  selection never launches a session, active-only actions are unavailable,
  and retry state is reachable. **Files:**
  `apps/web/components/task/task-session-sidebar-item.test.ts`,
  `apps/web/components/task/mobile/session-task-switcher-sheet-hooks.test.ts`,
  and `apps/web/components/task/task-switcher.test.tsx`.

## E2E Tests

- **Desktop:** seed one active and one archived task, apply Archived: Show,
  verify only the archived row remains, select it, and verify archived task
  detail plus the existing Unarchive action. Then archive another active task
  and verify the live row appears once without reloading.
  **File:** `apps/web/e2e/tests/task/sidebar-filter.spec.ts`.
- **Mobile:** from the task-switcher drawer, apply the same filter, verify the
  archived row and badge, open its detail, and assert the drawer/filter geometry
  is viewport-contained with no document horizontal overflow.
  **File:** `apps/web/e2e/tests/task/mobile-sidebar-views.spec.ts`.

## Verification Results

- Backend repository/service/handler tests and the broader task/backend target
  suite pass, including archived-only pagination, search, totals, and
  conflicting-flag behavior.
- The frontend focused archived projection and responsive row suite passes: 13
  files, 114 tests. Web typecheck, changed-file ESLint, `i18n:check`, and
  `i18n:ratchet` pass.
- Desktop and mobile host E2E archived-flow regressions each pass (1 test per
  project) with the managed web/backend build.
- Task-level details and exact commands are recorded in Tasks 01–05.

## Implementation Waves And Parallel Candidates

Wave 1 (parallel candidates; user authorization required):

- [x] [Task 01 — Add archived-only task query](task-01-archived-only-query.md)
- [x] [Task 02 — Restore archived filter contract](task-02-restore-archived-filter.md)

Wave 2:

- [x] [Task 03 — Build archived sidebar projection](task-03-archived-sidebar-projection.md)

Wave 3:

- [x] [Task 04 — Integrate archived rows](task-04-integrate-archived-rows.md)

Wave 4:

- [x] [Task 05 — Prove desktop and mobile flows](task-05-archived-sidebar-e2e.md)

Tasks 01 and 02 are parallel-safe because their backend and frontend contract
files are disjoint. Task 03 consumes both contracts; Task 04 consumes the
projection; Task 05 verifies the completed vertical slice.

## Risks

- The workspace list API is paginated. The loader must finish all archived
  pages before declaring a successful empty/complete result and must guard
  every page against workspace/request changes.
- Desktop and mobile surfaces can mount concurrently. A store-level loading
  gate is required to avoid duplicate archived fetches.
- `archived_at` is omitted after unarchive. The non-archived task update must
  remove a stale archived cache entry before performing the existing active
  upsert.
- Archived task rows can lack a runnable workspace/session. Navigation must
  bypass prepare/launch logic and let the archived detail route render its
  existing recovery UI.
- The current branch's removal tests assert the opposite contract. They must be
  replaced, not retained alongside the new behavior.

## Out of scope

- Changing active Kanban snapshot/query semantics.
- Adding new archive/unarchive mutations or sidebar-specific persistence.
- Redesigning the desktop sidebar or mobile task-switcher drawer.
