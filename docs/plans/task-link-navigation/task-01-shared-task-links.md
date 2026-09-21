---
id: "01-shared-task-links"
title: "Shared task links and dependency fix"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-NAVIGATION-001
acceptance_criteria:
  - AC-UI-TASK-NAVIGATION-001.1
  - AC-UI-TASK-NAVIGATION-001.2
  - AC-UI-TASK-NAVIGATION-001.3
  - AC-UI-TASK-NAVIGATION-001.4
  - AC-UI-TASK-NAVIGATION-001.5
system_design:
  - ../../specs/ui/system-design/task-navigation.md
---

# Task 01: Shared task links and dependency fix

## Summary

Centralize task anchors and repair both dependency directions with the existing router.

## In scope

- Add TaskLink and compatible linkToTask options; preserve raw IDs and query context.
- Add AppLink onNavigated within the successful guard callback.
- Switch DependencyRow and pass a close callback; scope 44px minimum rows to touch.

## Out of scope

Backend behavior and unrelated navigation redesign.

## Acceptance

- Failing regression: clicking a dependency row is intercepted and opens `/t/b`; today its native click is not prevented.
- Cover both directions, canonical URL/encoding, ordinary and keyboard activation, modified/middle clicks, explicit targets, prevented clicks, guard rejection/acceptance, and callback timing.
- Desktop stays compact; phone drawer closes only on committed same-tab navigation.

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
(cd apps/web && pnpm exec vitest run lib/links.test.ts components/routing/app-link.test.tsx components/routing/task-link.test.tsx components/task/task-dependency-chip.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/links.ts components/routing/app-link.tsx components/routing/task-link.tsx components/task/task-dependency-chip.tsx --max-warnings 0)
```

## Files likely touched

- `apps/web/lib/links.ts`
- `apps/web/lib/links.test.ts`
- `apps/web/components/routing/app-link.tsx`
- `apps/web/components/routing/app-link.test.tsx`
- `apps/web/components/routing/task-link.tsx (new)`
- `apps/web/components/routing/task-link.test.tsx (new)`
- `apps/web/components/task/task-dependency-chip.tsx`
- `apps/web/components/task/task-dependency-chip.test.tsx`

## Dependencies

None.

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

- The initial focused RED run failed on the six expected pre-fix behaviors:
  canonical URL construction, AppLink callback timing, dependency interception,
  disclosure dismissal, and touch-row sizing.
- The focused Vitest suite passed 4 files and 32 tests after implementation.
- `pnpm run typecheck` passed.
- Focused ESLint passed with zero warnings.
