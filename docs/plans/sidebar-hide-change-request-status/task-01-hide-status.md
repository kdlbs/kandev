---
id: "01-hide-status"
title: "Suppress hidden change-request status"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-TASK-ROW-PRESENTATION-001
acceptance_criteria:
  - AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.17
system_design:
  - ../../specs/ui/system-design/sidebar-task-row-presentation.md
---

# Task 01: Suppress hidden change-request status

## Summary and scope

Correct the shared sidebar row so Nothing hides title and trailing provider
status without changing the independent PR number detail or the task menu.
Leave persisted settings and other trailing selections unchanged.

## Acceptance

- None removes PR, MR and registered-provider status while preserving details,
  other badges and actions, and switching back restores the status.
- Desktop and mobile selection previews the same shared result.

## ASCII UI preview

UI-01: Right side set to Nothing, desktop and phone. See the [plan](plan.md)
for the current and intended row. The phone action remains touch accessible.

```text
[state] Task title  [actions]
[optional PR number and other details]
```

## Verification

```bash
(cd apps && pnpm --filter @kandev/web test -- --run components/task/task-item-hidden-status.test.tsx)
(cd apps/web && CAPTURE_PR_ASSETS=1 ../../scripts/run-quiet e2e --summary -- pnpm e2e:run e2e/tests/task/sidebar-filter.spec.ts -- --grep "hides change-request status with Nothing")
(cd apps/web && CAPTURE_PR_ASSETS=1 ../../scripts/run-quiet e2e --summary -- pnpm e2e:run --no-build --project mobile-chrome e2e/tests/task/mobile-sidebar-views.spec.ts -- --grep "hides change-request status with Nothing")
(cd apps/web && ../../scripts/run-quiet e2e --summary -- pnpm e2e:run --no-build e2e/tests/task/sidebar-filter.spec.ts -- --grep "previews, saves, discards, and reloads task row presentation with compact trailing content" --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `apps/web/components/task/task-item.tsx` and `task-item-hidden-status.test.tsx`
- `apps/web/e2e/tests/task/sidebar-filter.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-views.spec.ts`
- UI requirements, system design, and public tasks documentation

## Dependencies

None. Sequential.

## Risks

Existing title status must remain for other choices; PR-number details and
separate title badges must not be hidden.

## Results

The new regression failed before the fix because the PR icon remained mounted.
The final component test passed (1); desktop and mobile managed Playwright
scenarios passed (1 each). The mobile run reused the fresh desktop build.
Specification validation and lint passed. TypeScript checking passed with an
8 GiB Node heap after the default heap was exhausted. The full pre-existing task-item
suite also passed (49 tests). CI exposed an existing optimistic-save timing
race in the desktop presentation test; its persistence assertion now polls
the settings API. The focused scenario passed with retries disabled (1).
