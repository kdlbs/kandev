---
id: "02-sidebar-new-view-ui"
title: "Sidebar new-view UI"
status: done
wave: 2
depends_on: ["01-instant-create-state"]
plan: "plan.md"
spec: "../../specs/ui/sidebar-view-creation.md"
---

# Task 02: Sidebar New-View UI

Connect one shared create/rename flow to the desktop picker and mobile task-switcher sheet.

## Acceptance

- Desktop exposes a separated `New view` menu item; mobile exposes a fixed, labeled, non-hover action with a non-overlapping hit area of at least 40×40 px.
- Creation opens the viewport-safe filter popover with the automatic name focused for rename; save keeps the popover open, while Escape/Cancel/close keeps the created view.
- Dirty-draft and 50-view states disable creation with clear reasons; stale rename requests clear after close or optimistic rollback, and existing manual Rename/`Save as…` behavior still works.

## Verification

```bash
cd apps
pnpm --filter @kandev/web test -- \
  components/app-sidebar/sections/tasks-section.test.tsx \
  components/task/sidebar-filter/use-sidebar-view-popover.test.tsx \
  components/task/sidebar-filter/view-manager.test.tsx
cd web
pnpm run typecheck
```

## Files likely touched

- `apps/web/components/task/sidebar-filter/use-sidebar-view-popover.ts`
- `apps/web/components/task/sidebar-filter/use-sidebar-view-popover.test.tsx`
- `apps/web/components/task/sidebar-filter/sidebar-filter-popover.tsx`
- `apps/web/components/task/sidebar-filter/view-manager.tsx`
- `apps/web/components/task/sidebar-filter/view-manager.test.tsx`
- `apps/web/components/app-sidebar/sections/tasks-view-picker.tsx`
- `apps/web/components/app-sidebar/sections/tasks-section.test.tsx`
- `apps/web/components/task/sidebar-filter/sidebar-filter-bar.tsx`
- `apps/web/components/task/sidebar-filter/sidebar-view-chips.tsx`

## Dependencies

Task 01 (`createSidebarView()` and shared limit/default helpers).

## Inputs

- Spec: desktop/mobile entry points, optional rename, blocked states, and narrow-viewport scenarios.
- Plan: Shared creation and rename flow; Desktop and mobile entry points; Tests.
- Existing patterns: `WorkspacePickerContent` in `app-sidebar-workspace-picker.tsx`, Radix mocks in its test, `ViewHeaderRow` in `view-manager.tsx`, and the current mobile `SidebarFilterBar` mount in `session-task-switcher-sheet.tsx`.
- Mobile parity: keep actions reachable without hover or horizontal page scrolling; preserve chip swipe and touch-drag separation.
- Interface polish: use existing hierarchy/transitions, explicit `New view` wording, and minimum non-overlapping hit areas; add no decorative motion.

## Output contract

Report desktop/mobile behavior, accessibility/focus handling, files changed, focused tests/typecheck run, blockers, and residual risks. Set this task to `done` and update its checkbox in `plan.md` only after acceptance and verification pass.
