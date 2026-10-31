---
created: 2026-10-20
status: implemented
requirements:
  - REQ-TASKS-WIP-LIMIT-PULL-SYSTEM-001
system_design:
  - ../../specs/tasks/system-design/wip-limit-pull-system.md
legacy_specs: []
---

# Implementation plan: clear the queue marker on engine feeder pulls

## Overview

A WIP-limited step that a workflow engine transition vacates pulls the next
feeder-queued task through the orchestrator workflow store. That pull admits the
task into the destination but writes back its in-memory `queued_for_step_id`
and `queued_at`. The task then occupies a WIP slot (`wip_admitted = true`) while
still carrying a queue marker, and `autoStartTaskForLoadedStep` skips any task
with a queue marker, so destination entry never runs. The design already states
that feeder promotion "moves the task into the destination step and clears the
queued state", and that `queued_for_step_id` is empty when admitted; the task
service feeder promotion already clears both fields. Only the orchestrator copy
does not.

## Investigation of the reported admission bypass

The reported defect claimed controller moves (`mcp_move`) and `on_turn_complete`
transitions skip destination WIP admission and that leaving a step does not
release its slot. Every reachable path was checked and admits:

| Path | Admission | Release on leave |
| --- | --- | --- |
| MCP immediate move (`applyMoveTaskImmediate` -> `MoveTaskWithOptions`) | `updateMovedTaskCrossStep` | `pullNextTaskOnVacate` after commit |
| MCP deferred move (`ApplyDeferredMoveTransition`) | `UpdateTaskWithWorkflowStepAdmissionForDeferredMove` | store `pullNextTaskOnVacate` |
| Engine transition (`workflowStore.applyTransition`) | `updateTransitionTask` | store `pullNextTaskOnVacate` |
| Guarded engine transition (`applyTransitionIfAtStep`) | `UpdateTaskWithWorkflowStepAdmissionIfAtStep` | store `pullNextTaskOnVacate` |
| Legacy `executeStepTransition` | `updateTransitionTaskWithCapacityAndEffect` | store `pullNextTaskOnVacate` |

One-off probes against the real SQLite repository confirmed queued placement on a
full step and same-step promotion when the occupant leaves for an unlimited step,
for both the service and the store paths. The production ledger matched: the
second and third tasks entering the limited step were queued and ran only after
promotion. The defect that did reproduce is the feeder-pull marker above.

## Scope

- In scope: clear `queued_for_step_id` and `queued_at` (and set admitted) on the
  candidate in `workflowStore.pullOneFeederTask` before the atomic promotion,
  matching `Service.promoteFeederQueuedTask`; one regression test.
- Out of scope: session launch paths that take a workflow step
  (`moveTaskToWorkflowStep`, `advanceTaskWorkflowStep`), which are not moves and
  are a separate decision; data repair (affected rows recover on the next move).

## Work orders

- [x] [Task 01: Clear the queue marker on engine feeder pulls](task-01-clear-feeder-pull-queue-marker.md)

## Verification results

See the work order results.
