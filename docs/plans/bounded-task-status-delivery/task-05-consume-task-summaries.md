---
id: "05-consume-task-summaries"
title: "Consume summaries in task switchers"
status: completed
wave: 5
depends_on: ["04-stabilize-session-transport"]
plan: "plan.md"
spec: "../../specs/platform/bounded-task-status-delivery.md"
---

# Task 05: Consume Summaries in Task Switchers

## Acceptance

- Task mapping/store handlers keep the newest summary revision, preserve it
  when partial task updates omit the field, and reject stale deltas.
- Desktop and mobile rows derive pending, activity, error, Git, and PR status
  from the same summary contract while preserving existing icon precedence and
  local error acknowledgment.
- Mounting/switching task rows creates no inactive `session.subscribe`, removes
  `useBulkGitStatusSubscription`, and does not workspace-fetch detailed PRs for
  switcher decoration.

## TDD sequence

1. RED: add mapper/reducer tests for hydration, omitted fields, revision
   ordering, and reconnect replacement.
2. RED: update desktop/mobile hook and row tests to supply summaries and prove
   all existing precedence/error/PR/Git outcomes without message/session-store
   fixtures.
3. RED: add a subscription-spy component test proving inactive rows and task
   switches do not subscribe.
4. GREEN: add types/handlers, migrate both switchers, remove bulk Git and row PR
   detail loads, and keep active detail consumers unchanged.
5. REFACTOR: share summary-to-row derivation and delete obsolete message/Git
   aggregation paths that have no remaining consumers.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run \
  lib/kanban/map-task.test.ts \
  components/task/task-session-sidebar.test.tsx \
  components/task/task-session-sidebar-item.test.ts \
  components/task/mobile/session-task-switcher-sheet-hooks.test.ts
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/types/http.ts`
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/kanban/map-task.ts`
- `apps/web/lib/kanban/map-task.test.ts`
- Kanban task/store slice types and task WebSocket handlers
- `apps/web/components/task/task-session-sidebar.tsx`
- `apps/web/components/task/task-session-sidebar-item.ts`
- `apps/web/components/task/task-session-sidebar-aggregate.ts`
- `apps/web/components/task/mobile/session-task-switcher-sheet-hooks.ts`
- `apps/web/components/task/task-item.tsx`
- task summary/PR icon helpers and focused tests

## Dependencies

Tasks 01–04 supply complete snapshots/deltas, independent Git freshness, and
idempotent selected-session transport.

## Parallelism

Sequential. Desktop/mobile share derivation and must migrate in one task.

## Inputs

- Spec **Derivation rules**, rollout fallback, and mobile scenario.
- Existing `TaskSwitcher`/`TaskItem` shared rendering.
- Existing local error acknowledgment stamps and PR attention-color rules.

## Risks

- Do not remove active-detail Git or PR data needed for diffs/tooltips outside
  task switchers.
- Missing summaries omit only unknown decorations; they must not appear clean
  or trigger a hidden subscription fallback.
- Keep desktop/mobile behavior shared rather than duplicating summary rules.

## Verification results

- `cd apps/web && pnpm exec vitest run` — passed (1,011 files; 7,743 tests, 4 skipped).
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps/web && pnpm run lint` — passed.
- Desktop and mobile task rows now derive Git, PR, error, pending, activity,
  and primary-session state from `statusSummary` with coarse-field fallback.
- The sidebar no longer mounts `useBulkGitStatusSubscription`, raw session
  message decoration, or workspace-wide PR decoration fetches. Active detail
  surfaces retain their session Git/message/PR subscriptions.
- A dedicated Playwright subscription-spy scenario is still part of Task 07;
  the component-level behavior is covered by the summary mapper/reducer tests
  and source inspection.

## Output contract

Report obsolete subscriptions/fetches removed, shared derivation, precedence
coverage, desktop/mobile test results, typecheck, exact files changed, and any
detail-surface exceptions retained.
