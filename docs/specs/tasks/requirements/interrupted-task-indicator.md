---
status: draft
system: tasks
created: 2026-08-02
updated: 2026-09-21
owners:
  - kandev
---

# Interrupted task warning

## Overview

A backend restart can interrupt work while preserving its conversation for recovery.
Users must be able to identify affected tasks until their agents resume.
The task system owns the durable interruption marker and its visible lifecycle.

This revision changes the existing red error-style circle to a warning triangle.
It also prevents a failed resume attempt from erasing the interruption marker.
Implementation belongs to the [recovery package](../../../plans/orphaned-session-open-recovery/plan.md).

## Requirements

### REQ-TASKS-INTERRUPTED-TASK-INDICATOR-001: Interrupted task warning

**Intent:** Show which tasks lost running work to a restart until recovery succeeds.

- **AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.1:** Superseded by criteria
  001.2 through 001.7. The previous migrated criterion required a red alert
  circle and cleared the marker when startup began.
- **AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.2:** When restart recovery finds
  interrupted starting or running work, the system shall retain a durable
  interruption marker on its unarchived task. Idle conversations and executions
  that survived the restart shall not gain that marker.
- **AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.3:** Unresumed interrupted tasks
  shall show a warning triangle in the sidebar and task card. The shared
  interruption indicator shall use the existing warning color instead of error red.
  Its accessible label shall remain "Interrupted by restart".
- **AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.4:** Opening Kandev, focusing a
  task, or accepting a resume request shall not clear the marker. Failed,
  cancelled, or blocked recovery attempts shall preserve it. Successful agent
  recovery shall clear it and update visible task surfaces without a reload.
- **AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.5:** The marker shall survive
  reload, reconnect, repeated restart, and updates that omit interruption state.
  A stale callback shall not clear a newer interruption marker.
- **AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.6:** Phone task navigation and
  board cards shall show the same warning and recovery lifecycle. The meaning
  shall remain accessible without hover or reliance on color alone.
- **AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.7:** Existing permission,
  clarification, current activity, and explicit terminal-state precedence shall
  remain. Temporary startup progress shall not erase the durable warning.
  Failed recovery shall leave the interruption state available alongside the
  actual recovery error. Explicit failure and cancellation icons remain unchanged.

## Boundaries

Recovery follows [the interruption contract](restart-orphaned-session-terminalization.md).
The warning does not cancel work, start an agent, replay a prompt, or advance
its workflow. Successful recovery can clear the warning before a new message
arrives; restoring the agent and continuing work are separate actions.

No new task state, manual dismiss action, or Office dashboard behavior is added.
Reuse the existing marker and translated label. This is a task-owned indicator,
not a separate UI-owned requirement.

## Related

- [System design](../system-design/interrupted-task-indicator.md)
- [Task recovery design](../system-design/restart-orphaned-session-terminalization.md)
- [Sidebar task completion icons](../../ui/requirements/sidebar-task-completion-icons.md)
