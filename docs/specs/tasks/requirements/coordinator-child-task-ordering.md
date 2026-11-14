---
status: active
system: tasks
created: 2026-10-03
owners:
  - maintainers
---

# Coordinator Child Task Ordering Requirements

## Overview

A coordinating task needs to choose the processing order of its children
without inventing input dependencies. The tasks system owns the persisted
board order and the existing queue admission order.

## Terminology

- **Direct child:** A task whose parent is the calling task, in its workspace.
- **Band:** The admitted or queued group within a workflow step.

## Requirements

### REQ-TASKS-COORDINATOR-CHILD-TASK-ORDERING-001: Child processing order

**Intent:** Let a coordinator prioritize work while preserving real input
dependency semantics.

#### Acceptance criteria

- **AC-TASKS-COORDINATOR-CHILD-TASK-ORDERING-001.1:** Task-mode MCP shall expose
  `reorder_child_tasks_kandev` with required `ordered_task_ids` and optional
  `placement`, defaulting to `in_place`. Other MCP modes shall omit the tool.
- **AC-TASKS-COORDINATOR-CHILD-TASK-ORDERING-001.2:** Every named task shall be a
  direct child in the caller's workspace, in the same workflow step and band.
  Empty, duplicate, missing, or unauthorized IDs and mixed steps or bands shall
  fail before writing an order.
- **AC-TASKS-COORDINATOR-CHILD-TASK-ORDERING-001.3:** With `in_place`, the named
  tasks shall occupy their current slots in the requested order, preserving
  every other task's slot in the band's order.
- **AC-TASKS-COORDINATOR-CHILD-TASK-ORDERING-001.4:** With `front`, the named tasks
  shall precede the rest of their band in the requested order, preserving the
  relative order of the remaining tasks.
- **AC-TASKS-COORDINATOR-CHILD-TASK-ORDERING-001.5:** A successful reorder shall
  persist the existing board positions and publish `task.reordered`. The board
  and normal queued-task admission shall observe that same order.
- **AC-TASKS-COORDINATOR-CHILD-TASK-ORDERING-001.6:** A membership conflict shall
  trigger at most one fresh read and retry. A second conflict or a named task
  leaving the authorized set shall return an error rather than a false success.
- **AC-TASKS-COORDINATOR-CHILD-TASK-ORDERING-001.7:** The response shall identify
  the workflow step, band, revision, and resulting task IDs and positions.
  Tool guidance shall recommend ordering for scheduling and dependencies only
  when one task consumes another task's output.

## Out of scope

- Starting, stopping, moving, or reparenting tasks; changing priorities or edges.
- Reordering across workflow steps or bands, or exposing an admin reorder tool.
- Changing the position-first ordering of normal queued-task admission.
- Global ceiling replay ordering, owned by the agents session ceiling contract.
