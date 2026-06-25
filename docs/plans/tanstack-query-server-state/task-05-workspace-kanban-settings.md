---
id: "05-workspace-kanban-settings"
title: "Workspace kanban settings"
status: done
wave: 3
depends_on: ["03-query-options-taxonomy", "04-query-bridge-audit"]
plan: "plan.md"
spec: "../../specs/ui/tanstack-query-server-state.md"
---

# Task 05: Workspace Kanban Settings

## Acceptance

- Workspace, repository, workflow, kanban, features, and settings server-state
  readers use TanStack Query.
- Kanban task/workflow WS handlers write the same query keys mounted UI reads.
- First paint for `/`, `/tasks`, `/github`, `/jira`, `/linear`, and `/gitlab`
  still seeds from Go boot/app-state data.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- apps/web/hooks apps/web/src apps/web/lib/query`
- `cd apps/web && pnpm e2e:docker tests/task/task-list.spec.ts tests/task/task-list-filters.spec.ts tests/kanban/kanban-board.spec.ts tests/kanban/workflow-filter.spec.ts tests/settings/config-management.spec.ts`
- `cd apps/web && pnpm e2e:docker --project mobile-chrome tests/kanban/mobile-kanban.spec.ts tests/task/mobile-task-list-search.spec.ts`

## Files Likely Touched

- `apps/web/hooks/use-workflow-snapshot.ts`
- `apps/web/hooks/use-tasks.ts`
- `apps/web/hooks/domains/kanban/*`
- `apps/web/src/spa-routes.tsx`
- `apps/web/lib/query/query-options/kanban.ts`
- `apps/web/lib/query/query-options/workspace.ts`
- `apps/web/lib/query/query-options/settings.ts`
- `apps/web/lib/query/bridge/kanban.ts`
- `apps/web/lib/query/bridge/workspace.ts`
- `apps/web/lib/query/bridge/settings.ts`
- old handlers under `apps/web/lib/ws/handlers/{kanban,tasks,workflows,workspaces,users}.ts`

## Dependencies

- Tasks 03 and 04.

## Inputs

- Old PR kanban query/bridge files.
- Current boot routing files: `apps/web/src/spa-routes.tsx` and
  `apps/web/lib/routing/kanban-route-hydration.ts`.

## Output Contract

Status: done.

Summary:

- Migrated workspace, repository, workflow, kanban task, task-list, and settings
  readers to TanStack Query while keeping Zustand mirrors for UI-only state and
  compatibility during the one-shot PR.
- Added the canonical workflow snapshot mapper in
  `apps/web/lib/kanban/snapshot.ts` so query data and legacy store mirrors share
  one shape.
- Updated task-list query filters to include `page` in the key and request, and
  removed the over-specific boot seed that reused workflow-filtered data for
  the All Workflows view.
- Extended the query bridge so task/workflow/kanban/workspace events invalidate
  or remove the same query keys mounted UI now reads.
- Added a bounded WS teardown drain in the E2E fixture so strict accounting
  compares after trailing parsed frames settle instead of flagging browser-side
  timing skew as a drop.

Retained Zustand paths:

- UI selection and client-only display state remains in Zustand, including
  active workspace/workflow ids and kanban display preferences.
- Query-backed hooks still mirror server snapshots into the legacy store where
  older components have not yet been removed by Task 10.

Removed or reduced Zustand ownership:

- Workspace repository lists, branches, scripts, workflow snapshots, task list
  rows, task lookup helpers, user display settings, settings discovery, editor,
  prompt, agent/model, and notification-provider readers now source server data
  from TanStack Query.

Temporary bridge dual-write:

- The WS bridge now invalidates/patches query keys for migrated domains while
  existing Zustand handlers remain installed until Task 10 removes legacy
  server-state handlers after all readers have moved.

Verification:

- `cd apps/web && pnpm test -- hooks/use-workflow-snapshot.test.ts hooks/use-tasks.test.ts hooks/domains/kanban/use-all-workflow-snapshots.test.ts hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts lib/query/keys.test.ts lib/query/query-options/query-options.test.ts lib/query/bridge/index.test.ts`
  passed: 7 files, 36 tests.
- `cd apps/web && pnpm test -- hooks/domains/settings hooks/domains/workspace hooks/use-user-display-settings.ts hooks/use-kanban-display-settings.ts`
  passed: 2 files, 14 tests.
- `cd apps/web && pnpm typecheck` passed.
- `cd apps && pnpm --filter @kandev/web test -- hooks src lib/query` passed:
  89 files, 672 tests.
- `cd apps/web && pnpm e2e:docker --no-build -- tests/task/task-list.spec.ts tests/task/task-list-filters.spec.ts tests/kanban/kanban-board.spec.ts tests/kanban/workflow-filter.spec.ts tests/settings/config-management.spec.ts`
  passed: 35 tests.
- `cd apps/web && pnpm e2e:docker --no-build --project mobile-chrome -- tests/kanban/mobile-kanban.spec.ts tests/task/mobile-task-list-search.spec.ts`
  passed: 12 tests.
