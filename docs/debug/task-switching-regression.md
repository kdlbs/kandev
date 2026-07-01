# Task Switching Regression Debug Log

Date: 2026-07-01
Branch: `feature/tanstack-migration-801`
Initial head: `5c4b244d2c0361f1c4575619b4ccd8f8dd5f12b9`
Comparison target: `origin/main`

## User Report

Switching between multiple tasks from the sidebar feels slower on this branch than on `main`.
This branch briefly shows the full-page `Loading task...` spinner when switching task details;
`main` does not visibly show that spinner in the same workflow.

## Working Hypothesis

The visible spinner is rendered by `TaskLoadingState` in
`apps/web/components/task/task-page-content.tsx`.

Early candidate: `useTaskDetails()` fetches `taskQueryOptions(effectiveTaskId)` when
`activeTaskId !== initialTaskId`. During client-side sidebar navigation, the route shell may still
have the previous route's `initialTask`, while `activeTaskId` is already updated to the clicked
task. If Query has no task-detail row yet, `resolveTaskContentState()` can choose the full-page
loading state until the task detail query resolves.

## Timeline

- Captured repo state: clean branch, 1 commit over `origin/main`.
- Read frontend guidance in `apps/web/AGENTS.md`.
- Found likely spinner and task-detail query path in `TaskPageContent`.
- Created detached comparison worktree at `/tmp/kandev-main-compare` from `origin/main`.
- Compared `TaskPageContent` between branch and `origin/main`.
- Confirmed `main` used `state.kanban.tasks` as an immediate fallback when the sidebar changed
  `activeTaskId` before route props caught up. This branch removed that fallback as part of
  Zustand server-state cleanup, so an uncached task-detail query produced the full-page loading
  state.
- Implemented Query-owned replacement fallback:
  - scan cached workflow snapshots via `workflowSnapshotQueryData(queryClient)`;
  - use the matching snapshot task as a temporary task row;
  - keep the detail query enabled so full task details still replace the snapshot row when loaded.
- Verified the real sidebar click path with a Playwright regression:
  - create two real tasks in the same workflow;
  - open task A and wait until task B is visible in the sidebar, proving the sidebar snapshot is
    cached;
  - block `/api/v1/tasks/:taskB`;
  - click task B in the sidebar;
  - assert the URL and breadcrumb switch to task B while `task-loading-state` never mounts.
- Did not add temporary frontend/backend debug logs. The branch/main code comparison and the
  blocked-detail browser repro isolated the issue to the frontend fallback path.

## Local Verification Commands

- Passed: `rtk pnpm --dir apps/web test components/task/task-page-content.test.tsx`
  - 1 file, 7 tests.
- Passed: `rtk pnpm --dir apps/web test components/task/task-select-routing-hydration.test.ts components/task/task-select-helpers.test.ts components/task/task-session-sidebar.test.tsx components/task/mobile/session-task-switcher-sheet-hooks.test.tsx hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts`
  - 5 files, 32 tests.
- Passed: `rtk pnpm --dir apps/web exec eslint --max-warnings 0 components/task/task-page-content.tsx components/task/task-page-content-helpers.ts components/task/task-page-content.test.tsx e2e/tests/task/task-loading-state.spec.ts e2e/tests/task/task-loading-state-helpers.ts`
- Passed: `rtk pnpm --dir apps/web typecheck`
- Passed: `rtk git diff --check`
- Passed: `rtk pnpm --dir apps/web e2e:run --host --project chromium tests/task/task-loading-state.spec.ts tests/office/tasks.spec.ts tests/office/topbar-breadcrumb.spec.ts tests/office/projects.spec.ts tests/office/project-repository-picker.spec.ts`
  - 17 passed, 1 skipped.
- Passed: `rtk pnpm --dir apps/web e2e:run --host --project chromium tests/task/task-loading-state.spec.ts`
  - 2 Chromium browser tests.

## Findings

- Root cause is frontend-side. No backend delay was needed to explain the extra spinner.
- The regression is not TanStack Query itself; it is the missing immediate fallback after deleting
  the old `state.kanban.tasks` mirror.
- Query workflow snapshots already contain full `Task` rows, so they can replace the old fallback
  without reintroducing Zustand server state.
- The sidebar task source is already Query workflow snapshots (`useWorkspaceSidebarTasks` ->
  `useAllWorkflowSnapshots`), so a visible sidebar task has the same snapshot data needed for this
  fallback.

## Fix Notes

- Changed `useTaskDetails()` to resolve `taskDetails > initialTask > cachedSnapshotTask`.
- Extended `hasResolvedTaskDetails()` so snapshot fallback suppresses the load-error/loading path.
- Added a focused test for rendering a changed active task from cached workflow snapshot data while
  the task detail query is still in flight.
- Added a browser regression that blocks the target task-detail endpoint while switching via the
  sidebar and asserts the full-page loading spinner does not appear.

## Follow-up Audit: Other Fast-Path Detail Pages

User follow-up: check whether other pages have the same "render from warm cache while detail loads"
mechanism.

### Already covered

- Kanban/task detail: now uses `taskDetails > initialTask > cached workflow snapshot task`.
- Office agent detail: reads agent identity from the already-seeded `qk.office.agents(workspaceId)`
  list via `useOfficeAgentProfile`; there is no separate agent-detail fetch spinner in the layout.
- Office run detail: the server route loads both the run aggregate and recent-runs sidebar before
  render, then seeds Query in `RunDetailView`; the client uses `detailQuery.data ?? initial`.
- Office list/dashboard pages: list pages use `query.data ?? initial* ?? []` where server initial
  payloads exist, so refreshes do not blank the page.

### Gaps found and fixed

- Office project detail previously read only `qk.office.project(projectId)` and showed
  `Loading project...` if the detail query was not warm. It now subscribes to the cached workspace
  project list and renders the matching project row while the canonical detail query loads.
- Office task detail previously initialized local task state only from
  `officeTaskQueryOptions(workspaceId, taskId)`. It now subscribes to cached office task infinite
  pages and maps the matching row through the existing `mapOfficeTaskToTask` fallback while the
  canonical detail query, comments, activity, and sessions continue loading.

### Follow-up Verification Commands

- Passed: `rtk pnpm --dir apps/web test app/office/projects/[id]/project-query-cache.test.ts app/office/tasks/[id]/task-detail-query-cache.test.ts components/task/task-page-content.test.tsx`
  - 3 files, 14 tests.
- Passed: `rtk pnpm --dir apps/web exec eslint --max-warnings 0 app/office/projects/[id]/page.tsx app/office/projects/[id]/project-query-cache.ts app/office/projects/[id]/project-query-cache.test.ts app/office/tasks/[id]/page.tsx app/office/tasks/[id]/task-detail-query-cache.ts app/office/tasks/[id]/task-detail-query-cache.test.ts components/task/task-page-content.tsx components/task/task-page-content-helpers.ts components/task/task-page-content.test.tsx e2e/tests/task/task-loading-state.spec.ts e2e/tests/task/task-loading-state-helpers.ts`
- Passed: `rtk pnpm --dir apps/web typecheck`
- Passed: `rtk git diff --check`
