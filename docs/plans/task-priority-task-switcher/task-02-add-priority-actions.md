---
id: "02-add-priority-actions"
title: "Add responsive task-switcher priority actions"
status: completed
wave: 2
depends_on:
  - "01-project-and-render-priority"
plan: "plan.md"
requirements:
  - REQ-TASKS-PRIORITY-VISIBILITY-003
  - REQ-TASKS-PRIORITY-VISIBILITY-004
  - REQ-TASKS-PRIORITY-VISIBILITY-005
acceptance_criteria:
  - AC-TASKS-PRIORITY-VISIBILITY-005.4
  - AC-TASKS-PRIORITY-VISIBILITY-005.5
  - AC-TASKS-PRIORITY-VISIBILITY-005.6
  - AC-TASKS-PRIORITY-VISIBILITY-005.7
  - AC-TASKS-PRIORITY-VISIBILITY-005.8
system_design:
  - ../../specs/tasks/system-design/task-priority-visibility.md
---

# Task 02: Add Responsive Task-Switcher Priority Actions

## Summary

Add the four-value Priority submenu to the shared single-task menu and connect
it to the existing field-scoped priority update. Prove desktop right-click and
visible mobile task-actions flows, including live convergence and menu geometry.

## In scope

- Add a flag-labelled priority submenu to the live single-task menu.
- Show all four localized options and the exact current marker.
- Reuse `useUpdateTaskPriority`; do not add optimistic row state.
- Keep priority absent from archived and multi-selection menus.
- Add focused unit tests and desktop plus mobile Playwright coverage.

## Out of scope

- Bulk updates, archived-task updates or another menu primitive.
- Backend, API, event, persistence or localization catalog changes.

## Acceptance

- Desktop right-click and the visible phone or tablet Task actions control expose the same Priority submenu.
- Selecting or reselecting a token persists only priority; success converges through task events and failure preserves the last stored display.
- Phone menu triggers and rows remain touch-sized, viewport-contained, safe-area clear and free of document horizontal overflow.

## Verification

```bash
cd apps && pnpm --filter @kandev/web exec vitest run components/task/task-switcher.test.tsx components/task/task-switcher-context-menu.test.tsx hooks/use-update-task-priority.test.ts --reporter=dot
make build-web
cd apps/web && pnpm e2e:run --host --no-build --project chromium -- tests/task/sidebar-layout.spec.ts --grep "priority" --workers=1
cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome -- tests/task/mobile-sidebar-task-actions.spec.ts --grep "priority" --workers=1
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
git diff --check
```

## Files likely touched

- `apps/web/components/task/task-switcher-context-menu.tsx`
- `apps/web/components/task/task-switcher.test.tsx`
- `apps/web/components/task/task-switcher-context-menu.test.tsx`
- `apps/web/hooks/use-update-task-priority.ts`
- `apps/web/e2e/tests/task/sidebar-layout.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts`
- `apps/web/e2e/pages/sidebar-tasks-page.ts`

## Dependencies

- Task 01 supplies priority on `TaskSwitcherItem` and the row indicator used to prove convergence.

## Risks

- The portaled nested menu can exceed a short phone viewport if scroll ownership regresses.
- Menu clicks must not bubble into task navigation or drag sensors.

## Parallelism

`sequential`

## Inputs

- Task priority visibility requirement and system-design menu contracts.
- Existing kanban card priority submenu and `useUpdateTaskPriority` behavior.
- Existing sidebar Edit and Move desktop/mobile menu tests.

## Results

- Added a flag-labelled four-option Priority submenu to live single-task menus.
- Reused `useUpdateTaskPriority` and live `task.updated` convergence.
- Kept the action out of archived and multi-selection menus.
- Added desktop right-click and Pixel 5 visible Task actions E2E coverage.
- Verified the mobile nested options settle at the existing 44-pixel touch target.
