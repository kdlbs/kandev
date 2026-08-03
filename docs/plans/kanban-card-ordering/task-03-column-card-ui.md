---
id: "03-column-card-ui"
title: "Column and card sortable UI"
status: done
wave: 3
depends_on: ["02-reorder-helper-dnd"]
plan: "plan.md"
spec: "../../specs/ui/kanban-card-ordering.md"
---

# Task 03: Column and card sortable UI

## Acceptance

- Each Kanban column wraps admitted card ids in `SortableContext`.
- Admitted cards use `useSortable`; queued overflow cards are not sortable and render after
  admitted cards.
- Desktop, tablet, and phone Kanban columns use the board comparator.
- Multi-select display order matches visible board order.

## Files likely touched

- `apps/web/components/kanban-column.tsx`
- `apps/web/components/kanban-card.tsx`
- `apps/web/components/kanban-card-content.tsx` (transition only if needed)
- `apps/web/components/kanban/swipeable-columns.tsx`
- `apps/web/components/kanban/swimlane-kanban-content.tsx`
- `apps/web/hooks/use-task-multi-select.ts`

## Dependencies

Task 02.

## Parallelism

sequential

## Verification

```bash
cd apps/web && pnpm run typecheck
cd apps/web && pnpm exec vitest run lib/kanban/task-order.test.ts lib/kanban/reorder-admitted.test.ts
```

## Risks

- File size / complexity limits in swimlane and card modules — extract helpers rather than grow
  handlers past eslint thresholds.

## Results

```bash
cd apps/web && pnpm exec tsc --noEmit
cd apps/web && pnpm exec vitest run lib/kanban/task-order.test.ts lib/kanban/reorder-admitted.test.ts
```

Typecheck clean; unit tests passed.
