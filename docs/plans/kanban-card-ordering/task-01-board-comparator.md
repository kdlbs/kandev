---
id: "01-board-comparator"
title: "Board comparator"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/kanban-card-ordering.md"
---

# Task 01: Board comparator

## Acceptance

- Kanban board sort helper orders by `position ASC`, then `createdAt DESC`, then `id`.
- All-zero positions preserve newest-first display.
- Unit tests cover position, createdAt ties, missing fields, and id stability.
- Helper can identify admitted vs queued overflow cards for column splitting.

## Files likely touched

- `apps/web/lib/kanban/task-order.ts`
- `apps/web/lib/kanban/task-order.test.ts`
- `apps/web/lib/kanban/wip-limit.ts` (reuse admitted predicate if already sufficient)

## Dependencies

None.

## Parallelism

sequential

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm exec vitest run lib/kanban/task-order.test.ts
```

## Risks

- Multi-select `sortByDisplayOrder` should follow the visible board order once columns switch
  comparators (update in a later task if still created-desc-only).

## Results

```bash
cd apps/web && pnpm exec vitest run lib/kanban/task-order.test.ts
```

26 kanban unit tests in the suite batch passed (including task-order). No security/external side effects.
