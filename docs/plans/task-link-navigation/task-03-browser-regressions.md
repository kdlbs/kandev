---
id: "03-browser-regressions"
title: "Desktop and touch navigation proof"
status: done
wave: 3
depends_on: ['02-enforce-task-links']
plan: "plan.md"
requirements:
  - REQ-UI-TASK-NAVIGATION-001
acceptance_criteria:
  - AC-UI-TASK-NAVIGATION-001.1
  - AC-UI-TASK-NAVIGATION-001.2
  - AC-UI-TASK-NAVIGATION-001.3
  - AC-UI-TASK-NAVIGATION-001.4
  - AC-UI-TASK-NAVIGATION-001.5
  - AC-UI-TASK-NAVIGATION-001.6
system_design:
  - ../../specs/ui/system-design/task-navigation.md
---

# Task 03: Desktop and touch navigation proof

## Summary

Prove real task navigation retains the browser document on both responsive surfaces.

## In scope

- Seed related tasks through existing API fixtures and exercise both dependency directions.
- Assert canonical destination plus target task identity, document sentinel, document request count, disclosure dismissal, and browser Back.
- Phone: use mobile-chrome defaults and Drawer selectors; assert 44px rows, containment, single list scroll owner, no horizontal overflow.
- Keep native modified-click and guard cancellation coverage in component tests from Task 01.

## Out of scope

Backend behavior and unrelated navigation redesign.

## Acceptance

- Record RED evidence against the pre-fix implementation for navigation tests, using temporary local reversion of only the fix if needed; restore it before GREEN.
- Desktop and mobile scenarios pass on fresh builds, not stale assets.
- Record all command results and mark work orders/manifest complete only after their checks pass.

## ASCII UI preview

UI-01: Dependency navigation, expanded state (AC .1, .4, .5, .6).

```text
Desktop: [blocks 2] -> Popover: [Task B | REVIEW] -> Task B workbench
Phone:   [blocks 2] -> Drawer:
                      Dependencies        [Close]
                      [Task B | REVIEW]   (44px+ tap row)
                      [Task C | CREATED]  (list scrolls)
                      -> Task B workbench; drawer closes
Before: selecting a row loads a new document through /tasks/:id.
After:  selecting a row commits SPA navigation to /t/:id.
```

Control order, touch targets, and successful-navigation dismissal are required.
Spacing is illustrative. Retain existing localized labels and status displays.
Cancelled navigation keeps the disclosure; modified clicks keep the current page.

See [full preview](plan.md#ascii-ui-preview).

## Verification

Run from repository root. Install once with `(cd apps && pnpm install --frozen-lockfile)` if this worktree lacks dependencies.

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/task/task-dependencies.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-dependency-navigation.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/e2e/tests/task/task-dependencies.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-dependency-navigation.spec.ts (new)`
- `docs/plans/task-link-navigation/plan.md`
- `docs/plans/task-link-navigation/task-*.md`

## Dependencies

02-enforce-task-links.

## Risks

Preserve existing route guards, query context, and browser-native alternate-tab behavior.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/task-navigation.md)
- [System design](../../specs/ui/system-design/task-navigation.md)
- Existing AppLink, linkToTask, local ESLint rules, and task dependency tests.

## Results

Completed on 2026-09-21.

- The initial component RED run provided pre-fix navigation failure evidence;
  no temporary production reversion was needed for the browser scenarios.
- Chromium dependency coverage passed all 6 tests, including both dependency
  directions, no document reload, disclosure closure, and browser Back.
- Mobile dependency coverage passed 1 test, including the real drawer, both
  directions, 44px rows, containment, no document reload, and Back.
- Specification validation, specification lint, and `git diff --check` passed.
