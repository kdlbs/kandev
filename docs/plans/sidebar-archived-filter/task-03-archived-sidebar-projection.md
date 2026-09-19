---
id: "03-archived-sidebar-projection"
title: "Build archived sidebar projection"
status: done
wave: 2
depends_on: ["01-archived-only-query", "02-restore-archived-filter"]
plan: "plan.md"
spec: "../../specs/ui/requirements/sidebar-archived-filter.md"
---

# Task 03: Build archived sidebar projection

Add the runtime-only archived task cache, paginated loader, and WebSocket
lifecycle synchronization without changing active Kanban snapshots.

## Acceptance

- A positive archived view lazily loads every archived-only page once per
  workspace, shares an in-flight gate across consumers, rejects stale writes,
  retains a successful cache on refresh failure, and supports foreground retry.
- Archived candidates merge only for the requesting view and current workspace;
  active Kanban tasks/snapshots never receive archived rows and task IDs are
  deduplicated.
- Archive, unarchive, and delete WebSocket events keep active and archived
  projections mutually exclusive while preserving all existing lifecycle side
  effects.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- --run lib/api/domains/kanban-api.test.ts lib/kanban/map-task.test.ts lib/state/slices/kanban/kanban-slice.test.ts hooks/domains/kanban/use-sidebar-archived-tasks.test.ts hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts lib/ws/handlers/tasks-archive.test.ts lib/ws/handlers/tasks-unarchive.test.ts lib/ws/handlers/tasks.deleted.test.ts
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/api/domains/kanban-api.ts`
- `apps/web/lib/api/domains/kanban-api.test.ts`
- `apps/web/lib/kanban/map-task.ts`
- `apps/web/lib/kanban/map-task.test.ts`
- `apps/web/lib/state/slices/kanban/types.ts`
- `apps/web/lib/state/slices/kanban/kanban-slice.ts`
- `apps/web/lib/state/slices/kanban/kanban-slice.test.ts`
- `apps/web/hooks/domains/kanban/use-sidebar-archived-tasks.ts`
- `apps/web/hooks/domains/kanban/use-sidebar-archived-tasks.test.ts`
- `apps/web/hooks/domains/kanban/use-workspace-sidebar-tasks.ts`
- `apps/web/hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts`
- `apps/web/lib/ws/handlers/tasks.ts`
- `apps/web/lib/ws/handlers/tasks-archive.test.ts`
- `apps/web/lib/ws/handlers/tasks-unarchive.test.ts`
- `apps/web/lib/ws/handlers/tasks.deleted.test.ts`

## Dependencies

Tasks 01 and 02.

## Parallelism

`sequential` — consumes both the backend query and restored view predicate and
owns the shared task/store/WS state used by later UI work.

## Inputs

- Spec archive query, live event, workspace isolation, failure, and persistence
  scenarios.
- Plan **Archived task runtime projection**.
- `use-all-workflow-snapshots.ts` request-generation and foreground-refresh
  patterns; `tasks.ts` active cache eviction/upsert behavior.

## Risks

- Desktop and mobile may mount together; read-then-fetch logic outside a store
  gate can duplicate requests.
- Unarchive events omit `archived_at`, so every non-archived update must remove
  a stale archived entry before the active upsert.
- The loader must not mark a partial page sequence as complete.

## Output contract

Report the cache shape, request/dedupe/generation behavior, event transitions,
exact files changed, commands/results, blockers, and update this task plus
`plan.md` status/results.

## Results

- Added a separate `sidebarArchivedTasks` cache keyed by workspace, with
  loaded/loading/error state plus replace, upsert, and remove actions. Active
  Kanban tasks and workflow snapshots remain unchanged.
- Added a paginated archived-only loader with a store-level loading gate,
  request-generation and workspace guards, foreground refresh, stable empty
  selectors, and successful-cache retention when refresh fails. Archived rows
  merge only for a positive `archived is true` view clause and are deduplicated
  by task ID.
- Updated task WebSocket handling so archive, unarchive, and delete transitions
  keep active and archived projections mutually exclusive while preserving the
  existing lifecycle side effects.
- Verification:
  - `cd apps && pnpm --filter @kandev/web test -- --run lib/api/domains/kanban-api.test.ts lib/kanban/map-task.test.ts lib/state/slices/kanban/kanban-slice.test.ts hooks/domains/kanban/use-sidebar-archived-tasks.test.ts hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts lib/ws/handlers/tasks-archive.test.ts lib/ws/handlers/tasks-unarchive.test.ts lib/ws/handlers/tasks.deleted.test.ts components/task/task-session-sidebar-item.test.ts components/task/mobile/session-task-switcher-sheet-hooks.test.ts components/task/task-select-helpers.test.ts components/task/task-switcher-click.test.ts components/task/task-switcher.test.tsx` — 13 files, 114 tests passed.
  - `cd apps/web && pnpm run typecheck` — passed.
  - Changed-file ESLint, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` — passed.
