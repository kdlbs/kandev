---
id: "05-prove-sidebar-activity-sort"
title: "Prove sidebar activity sorting"
status: completed
wave: 4
depends_on: ["01-stabilize-github-pr-refresh", "04-add-sidebar-sort"]
plan: "plan.md"
spec: "../../specs/ui/requirements/sidebar-last-activity-sort.md"
---

# Task 05: Prove sidebar activity sorting

Add production-build browser regressions for the saved option, stable activity
order, persistence, and phone geometry. Finish the change-aware verification
gates.

## Acceptance

- Desktop proves **Last activity** order and row time survive save and reload.
  A provider-only refresh does not reorder activity.
- Mobile proves the same saved option and order from the task-switcher drawer.
- Mobile assertions cover touch interaction, viewport containment, the task-list
  scroll owner, and zero document horizontal overflow.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm e2e:run tests/task/sidebar-filter.spec.ts -- --grep "last activity"
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-sidebar-views.spec.ts -- --grep "last activity"
make -C apps/backend test lint build
cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet
cd apps && pnpm --filter @kandev/web lint
```

## Files likely touched

- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/pages/sidebar-filter-popover.ts`
- `apps/web/e2e/tests/task/sidebar-filter.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-views.spec.ts`
- `apps/web/components/task/task-item-stats-row.tsx`

## Dependencies

Tasks 01 and 04.

## Risks

- Seed distinct activity milestones instead of relying on fixed sleeps.
- Scope portaled selectors to the active desktop or mobile surface.
- Confirm Playwright discovers one desktop test and one mobile test before
  treating the runs as evidence.

## Result

- Desktop discovery found one targeted test, then the managed production E2E
  run passed. It verified Last activity ordering, row timestamp mapping, saved
  view persistence after reload, and stable order after two provider-only PR
  refreshes.
- Mobile discovery found one targeted test, then the managed mobile-chrome E2E
  run passed. It verified touch selection, saved-view persistence, drawer
  containment, the task-list scroll owner, and zero document horizontal
  overflow.
- The desktop fixture explicitly uses the existing None group so provider
  association does not intentionally change repository grouping while the test
  measures activity ordering.
