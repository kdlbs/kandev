---
created: 2026-09-04
status: complete
requirements:
  - REQ-TASKS-KANBAN-TASK-REORDERING-001
system_design:
  - ../../specs/tasks/system-design/kanban-task-reordering.md
---

# Implementation Plan: Kanban Task Reordering

## Outcome

Persist the user-defined order of tasks within each workflow-step band. The
Kanban and Pipeline views use the same ordering contract, pointer and keyboard
reordering share one request path, and every connected view converges on the
server result.

## Delivery

- [x] [Task 01: implement task reordering](task-01-implement-kanban-task-reordering.md)

The implementation uses the existing task service, repository, WebSocket, and
Kanban state paths. It adds the step revision needed to reject stale client
orders without changing workflow-step ownership or task-move behavior.

## Mobile follow-up

[Remove mobile Kanban dragging](../remove-mobile-kanban-drag/plan.md) replaces
the phone drag interaction and positive mobile reorder test. This completed
package retains its historical results; desktop/tablet and backend scope remain.
