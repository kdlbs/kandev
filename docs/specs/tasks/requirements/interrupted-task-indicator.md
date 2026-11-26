---
status: draft
system: tasks
created: 2026-08-02
updated: 2026-11-25
owners:
  - kandev
---

# Interrupted task warning

## Overview

A backend restart can interrupt work while preserving its conversation. An
agent can restore the conversation and wait for another message. Users must be
able to identify tasks with interrupted work that still needs attention.

## Requirements

### REQ-TASKS-INTERRUPTED-TASK-INDICATOR-001: Interrupted task warning

**Intent:** Show which tasks still need to continue work interrupted by a restart.

- **AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.1:** Superseded by criteria
  001.2 through 001.7. The previous migrated criterion required a red alert
  circle and cleared the marker when startup began.
- **AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.2:** When restart recovery finds
  interrupted starting or running work, the system shall retain a durable
  interruption marker on its unarchived task. Idle conversations and executions
  that survived the restart shall not gain that marker.
- **AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.3:** Interrupted tasks that still
  need continuation shall show an amber warning triangle in the sidebar and
  task card. Its accessible label shall be "Interrupted by restart".
- **AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.4:** Opening Kandev, focusing a
  task, or restoring a conversation without a prompt shall preserve the
  warning. A failed, cancelled, or blocked recovery shall preserve it.
  Criterion 001.8 replaces the previous provider-ready clearing rule.
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
- **AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.8:** When the agent accepts a new
  prompt for an interrupted session, Kandev shall resolve that interruption.
  A prompt that is only queued, refused, or has an unknown delivery outcome
  shall preserve the warning. The task status shall update without a reload.
- **AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.9:** If at least one interrupted
  session still needs continuation, the task shall retain its warning.
  Continuing or deleting one interrupted session shall resolve only that
  session's interruption. A prompt in a healthy sibling session shall preserve
  the warning.
- **AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.10:** When an existing warning lacks
  a session owner, Kandev shall retain it until the task next accepts a prompt.

## Boundaries

The warning follows [the interruption contract](restart-orphaned-session-terminalization.md).
Scope covers the warning lifecycle and its existing task surfaces. Agent startup,
prompt replay policy, workflow transitions, and Office dashboard behavior belong
to their respective contracts.

## Related

- [System design](../system-design/interrupted-task-indicator.md)
- [Implementation plan](../../../plans/interrupted-task-warning-continuation/plan.md)
- [Task recovery design](../system-design/restart-orphaned-session-terminalization.md)
- [Sidebar task completion icons](../../ui/requirements/sidebar-task-completion-icons.md)
