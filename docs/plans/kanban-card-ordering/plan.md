---
spec: docs/specs/ui/kanban-card-ordering.md
created: 2026-08-03
status: complete
---

# Implementation Plan: Kanban card ordering

## Overview

Add within-column reorder for admitted Kanban cards by switching column display sort to
`position` (with `createdAt` tie-break), wiring `@dnd-kit/sortable` for admitted cards, and
persisting dense same-step positions through the existing `MoveTask` API. Pipeline and queued
overflow reorder stay out of scope. No backend schema changes.

---

## Backend

None required. Rely on existing `MoveTask` same-step position support
(`TestService_MoveTaskAllowsSameStepReorderWhenStepAlreadyOverLimit`).

---

## Frontend

### Sort helper

- `apps/web/lib/kanban/task-order.ts`: add board comparator
  (`position ASC`, `createdAt DESC`, `id`) and helpers to split admitted vs queued overflow;
  keep `compareTasksByCreatedDesc` for bulk-move stability where still needed or migrate
  multi-select display sort to the board comparator.
- `apps/web/lib/kanban/reorder-admitted.ts`: pure densify / arrayMove helpers + position patch list.

### DnD

- Extract/extend swimlane Kanban drag-end in
  `apps/web/components/kanban/swimlane-kanban-content.tsx` (or a sibling hook) to handle
  card-over-card same-step reorder vs cross-step append.
- `apps/web/components/kanban-column.tsx`: `SortableContext` over admitted IDs; render admitted
  then queued.
- `apps/web/components/kanban-card.tsx` / shell: admitted cards use `useSortable`; queued keep
  `useDraggable` for cross-step moves only.
- `apps/web/components/kanban/swipeable-columns.tsx`: use board comparator.

### API / state

- Reuse `moveTask` / `moveTaskById`. Optimistic `setWorkflowSnapshot` + rollback on failure for
  densify batches.

---

## Tests

- **What:** board comparator ordering and ties — **File:** `task-order.test.ts` — **How:** vitest
- **What:** densify / reorder patches exclude queued — **File:** `reorder-admitted.test.ts` — **How:** vitest
- **What:** same-step reorder persists; queued not sortable; cross-step still works —
  **File:** `e2e/tests/kanban/card-reorder.spec.ts` — **How:** Playwright

---

## E2E Tests

- **Scenario:** two admitted cards reorder within a step and survive reload
- **Scenario:** queued overflow card is not reorderable via sortable path
- **Scenario:** cross-step drag still appends
- **File:** `apps/web/e2e/tests/kanban/card-reorder.spec.ts`

---

## Verification Results

- `pnpm exec vitest run lib/kanban/task-order.test.ts lib/kanban/reorder-admitted.test.ts` — pass
- `pnpm exec tsc --noEmit` (apps/web) — pass
- `e2e/scripts/run-e2e.sh --no-build --host -- tests/kanban/card-reorder.spec.ts` after FE build — 2 passed

---

## Implementation Waves And Parallel Candidates

```
Wave 1:
- [x] [task-01-board-comparator](task-01-board-comparator.md)

Wave 2:
- [x] [task-02-reorder-helper-dnd](task-02-reorder-helper-dnd.md)

Wave 3:
- [x] [task-03-column-card-ui](task-03-column-card-ui.md)

Wave 4:
- [x] [task-04-e2e-mobile](task-04-e2e-mobile.md)
```

Default execution is sequential in the primary conversation.

---

## Open Questions

(none)
