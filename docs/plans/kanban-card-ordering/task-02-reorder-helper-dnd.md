---
id: "02-reorder-helper-dnd"
title: "Reorder helper and DnD end handler"
status: done
wave: 2
depends_on: ["01-board-comparator"]
plan: "plan.md"
spec: "../../specs/ui/kanban-card-ordering.md"
---

# Task 02: Reorder helper and DnD end handler

## Acceptance

- Pure helper computes same-step admitted densify patches (`0..n-1`) and excludes queued cards.
- Swimlane Kanban drag-end reorders on same-step card drops; cross-step drops still append.
- Optimistic snapshot update rolls back if any move persist fails.

## Files likely touched

- `apps/web/lib/kanban/reorder-admitted.ts`
- `apps/web/lib/kanban/reorder-admitted.test.ts`
- `apps/web/components/kanban/swimlane-kanban-content.tsx`
- Possibly `apps/web/hooks/domains/kanban/use-swimlane-kanban-dnd.ts` if extracted

## Dependencies

Task 01.

## Parallelism

sequential

## Verification

```bash
cd apps/web && pnpm exec vitest run lib/kanban/reorder-admitted.test.ts lib/kanban/task-order.test.ts
```

## Risks

- Resolving `over.id` when it may be a task id or a step id; do not treat orphan sentinel as a
  move target.

## Results

```bash
cd apps/web && pnpm exec vitest run lib/kanban/reorder-admitted.test.ts lib/kanban/task-order.test.ts
```

Passed. DnD end handler extracted to `hooks/domains/kanban/use-swimlane-kanban-dnd.ts`.
