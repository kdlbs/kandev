---
id: "01-preview-and-hierarchy"
title: "Preview and hierarchy"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/rich-task-title-previews.md"
---

# Task 01: Preview and hierarchy

- **Acceptance:** Fine-pointer task titles show the reusable preview. Keyboard activation opens it. Subtask activation does not open the parent.
- **Verification:** `cd apps && pnpm --filter @kandev/web test -- --run components/task/task-title-hover-card.test.tsx hooks/domains/kanban/use-task-subtasks.test.tsx`.
- **Files likely touched:** task title, subtask, task-row, Kanban-card, and rich-list components and tests.
- **Dependencies:** None.
- **Parallelism:** sequential.

## Results

`cd apps && pnpm --filter @kandev/web test -- --run components/task/task-title-hover-card.test.tsx hooks/domains/kanban/use-task-subtasks.test.tsx` passed as part of the final focused run. The full focused run passed 130 tests across seven files. A new focus regression failed before the explicit focus fix and passed after it. External side effects: None.
