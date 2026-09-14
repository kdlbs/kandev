---
id: "01-implement-kanban-task-reordering"
title: "Implement Kanban task reordering"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-KANBAN-TASK-REORDERING-001
acceptance_criteria:
  - AC-TASKS-KANBAN-TASK-REORDERING-001.1
  - AC-TASKS-KANBAN-TASK-REORDERING-001.5
  - AC-TASKS-KANBAN-TASK-REORDERING-001.8
  - AC-TASKS-KANBAN-TASK-REORDERING-001.15
  - AC-TASKS-KANBAN-TASK-REORDERING-001.16
  - AC-TASKS-KANBAN-TASK-REORDERING-001.17
  - AC-TASKS-KANBAN-TASK-REORDERING-001.19
  - AC-TASKS-KANBAN-TASK-REORDERING-001.23
  - AC-TASKS-KANBAN-TASK-REORDERING-001.25
  - AC-TASKS-KANBAN-TASK-REORDERING-001.27
  - AC-TASKS-KANBAN-TASK-REORDERING-001.34
  - AC-TASKS-KANBAN-TASK-REORDERING-001.38
system_design:
  - ../../specs/tasks/system-design/kanban-task-reordering.md
---

# Task 01: Implement Kanban task reordering

## Scope

Add the persisted reorder contract across the task repository and service,
HTTP and WebSocket transport, Kanban state, and desktop and mobile board
surfaces. Keep band membership derived from task state and keep workflow-step
changes on their existing path.

## Implementation

- Validate task-write authorization and the complete band membership before
  writing positions.
- Lock the workspace, source, and target step rows in a stable order, then
  allocate dense positions for the two bands.
- Publish step revisions with the reordered task positions.
- Apply HTTP and WebSocket updates to both workflow snapshots and the main
  Kanban task projection, while retaining the newest buffered revision.
- Support pointer and keyboard reorder interactions in the Kanban surfaces;
  keep Pipeline display-only.

## Verification

Focused backend service, handler, and repository tests cover authorization,
HTTP errors, source-step lock drift, and persisted ordering. Focused frontend
tests cover HTTP/WebSocket revision reconciliation and both Kanban projections.
Web type checking, scoped lint and formatting, and the affected post-merge
Kanban tests pass.

## References

The requirement and system design define the complete ordering, validation,
concurrency, and presentation contract. This work order records the delivery
slice for that contract and its regression evidence.
