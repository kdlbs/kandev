---
id: "01-project-and-render-priority"
title: "Project and render task-switcher priority"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PRIORITY-VISIBILITY-001
  - REQ-TASKS-PRIORITY-VISIBILITY-004
  - REQ-TASKS-PRIORITY-VISIBILITY-005
acceptance_criteria:
  - AC-TASKS-PRIORITY-VISIBILITY-005.1
  - AC-TASKS-PRIORITY-VISIBILITY-005.2
  - AC-TASKS-PRIORITY-VISIBILITY-005.3
  - AC-TASKS-PRIORITY-VISIBILITY-005.7
system_design:
  - ../../specs/tasks/system-design/task-priority-visibility.md
---

# Task 01: Project and Render Task-Switcher Priority

## Summary

Make priority part of the shared task-switcher item and render one generic task
priority indicator on board cards and task rows. Both desktop and mobile
projection adapters must retain the same stored value.

## In scope

- Move priority tokens and display metadata to a task-owned shared module.
- Replace the kanban-only indicator with a reusable task priority indicator.
- Project priority into desktop sidebar and mobile task-switcher items.
- Render the indicator after the title in its inline badge area at every breakpoint.
- Add focused projection, rendering, fallback and accessibility tests.

## Out of scope

- Priority mutation menus or E2E interaction flows.
- New copy, priority tokens, state ownership or backend contracts.

## Acceptance

- Critical, high and low tasks show distinct indicators after the task title,
  while every task title keeps the same starting position.
- Medium, absent and invalid values show no indicator or raw token.
- Desktop and mobile task-switcher projections retain the stored priority and localized accessible name.

## Verification

```bash
cd apps && pnpm --filter @kandev/web exec vitest run components/task/task-priority-indicator.test.tsx components/kanban-card-content.test.tsx components/task/task-item.test.tsx components/task/task-session-sidebar-item.test.ts components/task/mobile/session-task-switcher-sheet-item.test.ts --reporter=dot
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
git diff --check
```

## Files likely touched

- `apps/web/lib/kanban/task-priority.ts`
- `apps/web/lib/tasks/task-priority.ts`
- `apps/web/components/kanban-card-priority-indicator.tsx`
- `apps/web/components/task/task-priority-indicator.tsx`
- `apps/web/components/kanban-card-content.tsx`
- `apps/web/components/task/task-switcher-types.ts`
- `apps/web/components/task/task-session-sidebar-item.ts`
- `apps/web/components/task/mobile/session-task-switcher-sheet-item.ts`
- `apps/web/components/task/task-switcher-row.tsx`
- `apps/web/components/task/task-item.tsx`
- Focused test files beside these modules.

## Dependencies

None.

## Risks

- A stale import can leave one existing priority surface on a second metadata map.
- Inline priority and existing badges can reduce title width on dense nested rows.

## Parallelism

`sequential`

## Inputs

- Task priority visibility requirement and system-design indicator contracts.
- Existing `KanbanCardPriorityIndicator` behavior and tests.
- Existing `buildSidebarItem` and `toSheetItem` projection patterns.

## Results

- Added task-owned priority tokens, labels and validation under `lib/tasks`.
- Added a generic priority indicator and kept the kanban adapter compatible.
- Projected priority into desktop and mobile task-switcher items.
- Rendered non-medium priority after the title and before change-request badges.
- Added focused projection, fallback, accessibility and ordering coverage.
