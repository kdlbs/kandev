---
id: "03-responsive-task-menu-context"
title: "Responsive task-menu context"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/plugins/requirements/plugins.md"
---

# Task 03: Responsive task-menu context

## Acceptance

- Plugin task-menu `visible` and `run` callbacks receive `presentation: "mobile"` from
  the phone kanban and `"desktop"` from desktop composition.
- Presentation is passed from layout composition through `KanbanColumn` and
  `KanbanCard`; cards do not instantiate their own breakpoint hook.
- Dropdown and context-menu construction continue to share the same plugin context.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- components/kanban-card-edit-submenu.test.tsx components/kanban-card-menu-items.test.tsx components/kanban-card-plugin-context.test.tsx
```

Add the mobile-context regression first and confirm it observes the current hardcoded
`"desktop"` value.

## Files likely touched

- `apps/web/components/kanban/swipeable-columns.tsx`
- `apps/web/components/kanban-column.tsx`
- `apps/web/components/kanban-card.tsx`
- `apps/web/components/kanban-card-plugin-context.test.tsx` (new)
- Existing menu tests only if shared test helpers need the new prop

## Dependencies

None.

## Parallelism

`parallel-safe` with Tasks 01, 02, and 07; it owns only kanban presentation wiring.

## Inputs

- Spec: Kanban contribution contract and responsive presentation scenario.
- Plan: **Responsive task-menu context**.
- Mobile kanban entry through `SwimlaneKanbanContent` → `SwipeableColumns`.

## Risks

Do not derive mobile from pointer precision inside cards; the mobile kanban layout is
the authoritative composition even on emulated or unusual input hardware.

## Output contract

Report prop flow, files changed, red-test evidence, exact Vitest results, and
synchronize task/plan status/results.

## Results

- Red phase: the new mobile-context regression observed the existing hardcoded
  `presentation: "desktop"` value.
- Added an explicit presentation prop from `SwimlaneKanbanContent` through
  `SwipeableColumns`/`KanbanColumn` to `KanbanCard`; the shared menu context now
  forwards it to both `visible` and `run`.
- `rtk pnpm --filter @kandev/web test -- --run components/kanban-card-plugin-context.test.tsx components/kanban-card-edit-submenu.test.tsx components/kanban-card-menu-items.test.tsx`
  — 3 files, 21 tests passed.
