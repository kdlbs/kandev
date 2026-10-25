---
id: "01-board-priority-sort-filter"
title: "Implement board priority sort and filter"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-001
  - REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-002
  - REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-003
  - REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-004
  - REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-005
acceptance_criteria:
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-001.1
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-001.4
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-001.6
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-001.10
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.1
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.2
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.3
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.10
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.11
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-003.1
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-003.2
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-003.3
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.1
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.2
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.3
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.4
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.5
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.6
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.7
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.8
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.9
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-005.1
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-005.2
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-005.3
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-005.4
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-005.5
system_design:
  - ../../specs/tasks/system-design/board-priority-sort-filter.md
---

# Task 01: Implement board priority sort and filter

## Acceptance

- The display dropdown and mobile menu expose the same persisted board sort and
  priority filter choices.
- Backend boot, HTTP, and WebSocket settings paths preserve and normalize the
  two view values without changing unrelated user settings.
- Kanban and pipeline projections apply the filter and retain their native
  default ordering when the sort is `created_desc`.
- Priority ordering uses deterministic native tie-breakers. Bulk moves use the
  same ordering as the rendered selection, including explicitly selected hidden
  workflows.

## Verification

```shell
cd apps/web && pnpm exec vitest run components/kanban-card-move-menu-actions.test.ts hooks/use-task-multi-select.test.ts lib/kanban/task-order.test.ts
cd apps/web && pnpm run typecheck && pnpm run lint
```

## Files likely touched

- `apps/backend/internal/user/`
- `apps/web/components/kanban/`
- `apps/web/hooks/use-task-multi-select.ts`
- `apps/web/lib/kanban/`
- `apps/web/lib/ws/handlers/kanban.ts`
