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
  `nestingEnabled`, dialogs, local edit state, and the store server-state mirrors
  as compatibility fields for sidebar/simple-pane readers.
- Did not remove `useOfficeRefetch` in this task. It now points at query
  refetches in migrated readers and remains as a compatibility bridge until the
  cleanup task removes old store fanout.
- Reused existing mobile coverage through `tests/office/mobile-onboarding.spec.ts`
  in Docker.
