---
id: "04-add-mobile-thread-view-drawer"
title: "Add the mobile Threads view drawer"
status: done
wave: 4
depends_on:
  - "03-add-desktop-thread-view-controls"
plan: "plan.md"
requirements:
  - REQ-UI-THREADS-SAVED-VIEWS-001
  - REQ-UI-THREADS-SAVED-VIEWS-002
  - REQ-UI-THREADS-SAVED-VIEWS-003
  - REQ-UI-THREADS-SAVED-VIEWS-004
acceptance_criteria:
  - AC-UI-THREADS-SAVED-VIEWS-004.2
  - AC-UI-THREADS-SAVED-VIEWS-004.3
  - AC-UI-THREADS-SAVED-VIEWS-004.4
  - AC-UI-THREADS-SAVED-VIEWS-004.5
  - AC-UI-THREADS-SAVED-VIEWS-004.6
  - AC-UI-THREADS-SAVED-VIEWS-004.7
  - AC-UI-THREADS-SAVED-VIEWS-004.8
  - AC-UI-THREADS-SAVED-VIEWS-004.9
system_design:
  - ../../specs/ui/system-design/threads-saved-views.md
---

# Task 04: Add the Mobile Threads View Drawer

## Summary

Give tablet and phone users the full Threads saved-view workflow through one
inset bottom drawer with internal pages.

## In scope

- Add a compact active-view button at the start of the mobile header strip.
- Open one inset bottom drawer for saved views and editing.
- Replace the drawer body for filter-value and task-picker pages.
- Reuse the desktop state adapter, validation, view actions, and query output.
- Add 44-pixel rows and standalone actions.
- Bound long names, own vertical scroll, clear the bottom safe area, and return
  focus to the trigger.
- Keep horizontal task paging independent from vertical drawer interaction.

## Out of scope

- A compressed desktop popover, nested drawer, or horizontal task tabs inside
  the editor.

## Acceptance

- Phone and tablet provide every desktop view outcome.
- The task picker stays inside the same drawer.
- All interactive rows meet the 44 by 44 CSS-pixel target.
- The drawer and long labels cause no document-level horizontal overflow.
- Closing the drawer restores focus to the active-view trigger.

## Verification

Write failing drawer navigation, parity, geometry, and focus tests first. Then
run:

```bash
(cd apps && pnpm --filter @kandev/web test -- --run components/threads/thread-view-mobile-drawer.test.tsx components/threads/thread-view-controls.test.tsx components/kanban/kanban-header.test.tsx)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run typecheck)
```

## Files likely touched

- `apps/web/components/threads/thread-view-mobile-drawer.tsx`
- `apps/web/components/threads/thread-view-controls.tsx`
- `apps/web/components/threads/thread-task-picker.tsx`
- `apps/web/components/kanban/kanban-header.tsx`
- `apps/web/src/locales/*/threads.json`

## Dependencies

- Task 03 supplies shared controls, editor primitives, and localized text.

## Risks

- Do not stack drawers for task or filter selection.
- Do not let drawer drag gestures move the task-column pager.

## Parallelism

`sequential`

## Results

Implemented and verified. Tablet and phone Threads headers now open one inset
bottom drawer with saved-view, editor, and task-picker pages. The drawer reuses
the shared view state and actions, has one vertical scroll owner, 44-pixel
controls, safe-area clearance, and focus restoration to the trigger. The
component suite passes 5 tests, and the mobile saved-view browser journey
passes.
