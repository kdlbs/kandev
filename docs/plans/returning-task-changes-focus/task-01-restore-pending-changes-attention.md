---
id: "01-restore-pending-changes-attention"
title: "Restore pending Changes attention"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/task-layout-profiles.md"
---

# Task 01: Restore Pending Changes Attention

## Intent

Make a meaningful Git or commit update detected while a desktop task is inactive activate its existing Changes panel after the user returns, even when that task has a saved Dockview layout. Preserve reload baselining and all current restore/panel safety rules.

## Acceptance

- A pending inactive-task change activates Changes after Dockview restoration even when an environment-specific layout exists.
- Reloading a task with already-known changes does not activate Changes solely because the count is non-zero.
- Missing Changes panels and Changes panels grouped with Agent sessions retain their current safe behavior.

## Files likely touched

- `apps/web/components/task/changes-panel-focus.test.ts`
- `apps/web/components/task/changes-panel-focus.ts`
- `apps/web/e2e/tests/layout/changes-panel-focus.spec.ts`

## Dependencies

None.

## Parallelism

`sequential`. The source and both regression levels describe one focus-state transition and must be changed in a single TDD cycle.

## Inputs

- `docs/specs/ui/requirements/task-layout-profiles.md`: returning-task attention and reload scenarios.
- `docs/plans/returning-task-changes-focus/plan.md`: root cause, responsive boundary, and risks.
- Regression commit `ba8d99f33`, which added `hasSavedLayout` suppression and the conflicting E2E expectation.
- Existing `applyChangesPanelAutoFocusState`, `activateChangesPanel`, and `markInactiveChangesIncreases` behavior.

## TDD Sequence

1. Set this task and its plan item to `in_progress`.
2. RED: reverse the saved-layout unit expectation and the returning-task E2E expectation without changing production code.
3. Run both focused regressions and confirm they fail because Changes is not activated.
4. GREEN: remove only the saved-layout suppression and its now-unused lookup/argument.
5. Run the focused unit and E2E checks until green.
6. REFACTOR: remove obsolete naming/comments and keep the state transition explicit.
7. Run typecheck, mark this task `done`, and update the plan checkbox/status.

## Verification

Run `pnpm install --frozen-lockfile` from `apps/` first when `apps/node_modules` is absent.

```bash
# From apps/
pnpm --filter @kandev/web test -- --run components/task/changes-panel-focus.test.ts
pnpm --filter @kandev/web typecheck

# From apps/web/
pnpm e2e:run tests/layout/changes-panel-focus.spec.ts -- --grep "new git updates focus changes when returning to a task with a saved layout"
```

The unit and E2E regression commands must be observed failing during RED and passing after GREEN. The final reported verification results are the post-change runs.

## Output contract

Report the state-transition change, files changed, RED and GREEN command results, any E2E artifacts or blockers, remaining risks, and the synchronized task/plan statuses.

## Results

- Removed the environment-layout lookup and blanket `hasSavedLayout` suppression from Changes attention.
- Preserved inactive-change detection, first-observation baselining, restore deferral, missing-panel handling, and Agent-group blocking.
- Updated the unit scenario and browser regression to require Changes activation on return.
- RED unit failed with zero activation calls; RED E2E observed `dv-inactive-tab`.
- GREEN unit passed all 16 tests, including the maximized-layout review regression.
- Direct web typecheck passed.
- GREEN managed E2E passed the focused Chromium production-build scenario in 1.9s.
- Desktop screenshot capture passed with
  `rtk pnpm e2e:run tests/layout/pr-capture.spec.ts -- --project=chromium` and produced the
  synthetic 1280×720 PR asset.
- Mobile screenshot capture passed with
  `rtk pnpm e2e:run tests/layout/mobile-pr-capture.spec.ts -- --project=mobile-chrome` and
  produced the synthetic Pixel 5 PR asset; both temporary capture specs were then removed.
- PNG compression passed with
  `rtk pnpm dlx pngquant-bin@9.0.0 --quality 65-90 --ext .png --force web/.pr-assets/*.png`,
  and both assets were embedded in PR #2091.
- `git diff --check` passed.
- No mobile product regression test was required because the affected hook is
  desktop-Dockview-only and no mobile/tablet behavior changed.
