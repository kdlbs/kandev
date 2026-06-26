---
id: "06-office-domain"
title: "Office domain"
status: in_progress
wave: 3
depends_on: ["03-query-options-taxonomy", "04-query-bridge-audit"]
plan: "plan.md"
spec: "../../specs/ui/tanstack-query-server-state.md"
---

# Task 06: Office Domain

## Acceptance

- Office dashboard, tasks, projects, agents, inbox, activity, routines, routing,
  costs, skills, and run-detail data read from TanStack Query.
- Office tasks list uses `useInfiniteQuery` for keyset pagination and preserves
  filters/sort/load-more behavior.
- Office WS events patch/invalidate query keys and no longer rely on
  `useOfficeRefetch` once readers are migrated.

## Verification

- PASS `cd apps && pnpm --filter @kandev/web test -- app/office hooks/domains/office lib/query`
  - 25 files, 124 tests.
- PASS `cd apps/web && pnpm typecheck`
- PASS `cd apps/web && pnpm e2e:docker -- tests/office/realtime-dashboard.spec.ts tests/office/realtime-tasks.spec.ts tests/office/dashboard.spec.ts tests/office/tasks.spec.ts`
  - 21 tests.
- PASS `cd apps/web && pnpm e2e:docker -- --project=mobile-chrome tests/office/mobile-onboarding.spec.ts`
  - 6 tests.
- PASS `cd apps/web && e2e/scripts/run-e2e.sh --docker --no-build --project routing`
  - 7 tests.
  - Note: `pnpm e2e:docker -- --project=routing` runs routing first but also leaves
    the runner's default chromium project selected. Use the runner-native
    `--project routing` flag before `--`.

## Files Likely Touched

- `apps/web/src/office-routes.tsx`
- `apps/web/app/office/**/*.tsx`
- `apps/web/hooks/use-office-refetch.ts`
- `apps/web/hooks/domains/office/*`
- `apps/web/lib/query/query-options/office.ts`
- `apps/web/lib/query/bridge/office.ts`
- `apps/web/lib/ws/handlers/office.ts`
- `apps/web/lib/state/slices/office/*`

## Dependencies

- Tasks 03 and 04.

## Inputs

- Old PR office query/bridge files.
- Existing spec: `docs/specs/office/live-updates.md`.
- Existing current hook: `apps/web/app/office/tasks/use-paginated-tasks.ts`.

## Output Contract

Update this task to `done`, list office store fields removed/retained, and call
out any mobile E2E coverage added or reused.

## Implementation Notes

- Reopened on 2026-06-26 after a current-state audit found remaining Office
  store readers/writers and `useOfficeRefetch` compatibility paths. The routing
  sub-wave now reads Query directly, but dashboard, agents, projects, inbox,
  meta, routines, costs, skills, and old Office WS fanout still need cleanup
  before this task can return to `done`.
- The Office shell/sidebar sub-wave moved the top-level dashboard page, agents
  page, app sidebar inbox badge, Office navigation counters, and sidebar
  agent/project lists to Query-owned Office caches. Deeper meta/routines/costs/
  skills/project/task helpers and old Office refetch fanout remain.
- The Office inbox/activity bridge sub-wave moved the Inbox page off the
  `office.inboxItems` store mirror, removed the Activity page's
  `useOfficeRefetch("activity")` subscription, and made
  `office.run.queued`/`office.run.processed` invalidate task comments so
  comment run-status badges no longer depend on the legacy comments refetch
  trigger.
- The Office meta sub-wave moved status/priority/role/executor/project/inbox/
  routine/skill metadata readers to `qk.office.meta()` through
  `useOfficeMetaData`, seeds the query cache during Office route bootstrap, and
  removed the unused `setMeta` Zustand action.
- The Office bridge parity sub-wave filled remaining invalidation gaps that
  blocked removing old refetch triggers: task-linked project counts/details and
  agent summaries, task activity for comments/reviews/decisions/runs, agent
  summaries/runs/run details, run-driven dashboard data, approval-driven agent
  data, and agent route queries for routing events.
- The Office page refetch sub-wave removed `useOfficeRefetch` subscriptions from
  Query-backed projects, routines, agent dashboard, agent layout, and agent runs
  surfaces. Task list/detail remain for the task-store cleanup wave.
- The Office task list/detail sub-wave moved the task list's fetched rows and
  loading state out of `office.tasks.items`/`office.tasks.isLoading`, removed the
  task page's SSR-to-store hydration, removed task detail's store fallback, and
  dropped the last production `useOfficeRefetch` callers. `usePaginatedTasks`
  now returns flattened infinite-query data directly, while task filters/sort/
  grouping/nesting remain in Zustand as client-only UI state.
- The Office task helper/scaffold cleanup moved project task sections, agent run
  linked task labels, and simple-pane parent/blocker pickers to Query-backed
  task reads. It removed the unused `useOfficeRefetch` hook, legacy Office WS
  handler registration/test, `office.refetchTrigger`, and the unused
  `office.tasks.items`/loading server-state fields/actions.
- Migrated office dashboard, tasks, task search, task detail comments/activity,
  agents, agent run/detail routes, projects/project tasks, inbox, activity,
  routines, routing, costs, budgets, and skills to TanStack Query readers.
- Added office query keys/options for task comments, task activity, task search,
  project detail, provider health, routing preview, agent routes, run attempts,
  agent summaries/runs/run detail, routines/routine runs/triggers, cost
  breakdown, budgets, and skills.
- Added an office WS query bridge that patches task pages/details and provider
  health/run attempts where possible, then invalidates the affected query
  families for sparse events.
- Retained `office.tasks.filters`, `viewMode`, `sortField`, `sortDir`, `groupBy`,
  `nestingEnabled`, dialogs, and local edit state as client-only UI state.
- Removed the legacy `useOfficeRefetch` hook definition, old Office WS fanout,
  Office refetch trigger state, and unused Office task server-state mirror.
  Remaining Office cleanup is now concentrated in agent/project/routine/skill
  store mirrors and readers.
- Reused existing mobile coverage through `tests/office/mobile-onboarding.spec.ts`
  in Docker.

## Reopened Wave Evidence

- Task list/detail cleanup:
  - `rtk pnpm --dir apps/web test app/office/tasks/use-paginated-tasks.test.tsx hooks/use-optimistic-task-mutation.test.tsx lib/query/bridge/index.test.ts`
    passed 3 files / 26 tests.
  - `rtk pnpm --dir apps/web typecheck` passed.
  - `rtk pnpm --dir apps/web lint` passed.
  - `rtk pnpm --dir apps/web e2e:docker tests/office/tasks.spec.ts tests/office/realtime-tasks.spec.ts tests/office/task-filters.spec.ts tests/office/task-sorting.spec.ts tests/office/topbar-breadcrumb.spec.ts tests/office/comment-input.spec.ts tests/office/simple-advanced-toggle.spec.ts tests/office/regression-fixes.spec.ts tests/office/property-pickers.spec.ts`
    passed 36 Docker tests with strict WS accounting.
- Task helper/scaffold cleanup:
  - `rtk pnpm --dir apps/web test components/task/simple/components/blockers-picker.test.tsx hooks/use-optimistic-task-mutation.test.tsx app/office/tasks/use-paginated-tasks.test.tsx lib/query/bridge/index.test.ts lib/ws/router.test.ts lib/ws/handlers/agent-session.test.ts components/state-hydrator.test.tsx`
    passed 7 files / 52 tests.
  - `rtk pnpm --dir apps/web typecheck` passed.
  - `rtk pnpm --dir apps/web lint` passed.
  - `rtk pnpm --dir apps/web e2e:docker tests/office/tasks.spec.ts tests/office/realtime-tasks.spec.ts tests/office/task-filters.spec.ts tests/office/task-sorting.spec.ts tests/office/topbar-breadcrumb.spec.ts tests/office/comment-input.spec.ts tests/office/simple-advanced-toggle.spec.ts tests/office/regression-fixes.spec.ts tests/office/property-pickers.spec.ts tests/office/projects.spec.ts tests/office/agent-run-detail.spec.ts tests/system/ws-event-accounting.spec.ts`
    passed 43 Docker tests / 1 skipped with strict WS accounting.
