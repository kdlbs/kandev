---
id: "02-mobile-kanban-navigation"
title: "Mobile Kanban navigation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/mobile-task-navigation.md"
---

# Task 02: Mobile Kanban navigation

## Acceptance

- Mobile mounts one valid focused workflow and one active step while All workflows remains the persisted filter.
- Workflow and step drawers expose labels, counts, current state, and 44px controls; previous/next and swipe remain available.
- No mobile workflow stack, hard per-workflow viewport height, or document-level horizontal overflow remains.

## Files likely touched

- `apps/web/components/kanban/swimlane-container.tsx`
- `apps/web/components/kanban/mobile-workflow-picker.tsx`
- `apps/web/components/kanban/mobile-column-tabs.tsx`
- `apps/web/components/kanban/swipeable-columns.tsx`
- `apps/web/components/kanban/mobile-drop-targets.tsx`
- `apps/web/components/kanban/mobile-menu-sheet.tsx`
- mobile Kanban Playwright spec/page object

## Verification

```bash
cd apps/web && pnpm e2e:run tests/kanban/mobile-kanban.spec.ts
```

## Inputs

- Spec Kanban scenarios.
- Current `SwimlaneContainer` → `MobileKanbanLayout` → `SwipeableColumns` path.
- Existing `MobilePickerSheet` and Drawer patterns.

## Output contract

Report behavior, files changed, tests run, blockers/risks, and set this task plus linked plan item to done.
