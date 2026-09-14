---
status: active
system: tasks
created: 2026-07-30
updated: 2026-08-29
owners:
  - kandev
---

# Runtime Task-State Publication Order Requirements

## Overview

Task surfaces combine persisted task state with live session state. They also
receive task data from WebSocket events and workflow snapshots.

The task system owns the authoritative task state. All task surfaces must keep
that state consistent when live events and delayed snapshots arrive in either
order.

## Terminology

- **Task-state projection:** The task state and task update time that a server
  response or event supplies to a client.
- **Workflow snapshot:** A server response that contains the steps and tasks for
  one workflow.
- **Active task projection:** The task copy that supports the open task surface.

## Requirements

### REQ-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001: Consistent runtime task state

**Intent:** Users must see the current persisted task state on every task
surface while an agent starts or resumes work.

#### Acceptance criteria

- **AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.1:** When an owning non-Office
  session changes to `RUNNING`, the eligible task shall become `IN_PROGRESS`
  before observers receive the running-session state.
- **AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.2:** When reconciliation changes
  the task state, observers shall receive the task-state event before the
  running-session event.
- **AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.3:** When a delayed workflow
  snapshot contains an older task-state projection, it shall not replace a
  newer task-state projection that the client already received.
- **AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.4:** When the active task
  projection is newer than the workflow snapshot, task lists shall use its task
  state even if both copies have the same status-summary revision.
- **AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.5:** When a task list groups by
  state, a running task shall remain in the `IN_PROGRESS` group without a page
  reload or a later foreground refresh.
- **AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.6:** Desktop and mobile task
  lists shall derive the same task state from their shared task projection.
- **AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.7:** When a workflow snapshot
  refresh fails, the current task projection shall remain usable. A later
  refresh can retry the request.
- **AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.8:** A stale runtime writer shall
  not promote an archived task, an Office task, or a task whose authoritative
  session no longer owns active work.
- **AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.9:** Repeated activity for an
  already-running session shall not produce duplicate task-state writes or
  state-change events.

## Out of scope

- Changing task-state or session-state values.
- Inferring task state from session activity in the frontend.
- Changing workflow-step transitions or State-group labels.
- Changing desktop or mobile layout, navigation, scrolling, or touch behavior.
- Changing Office task-state ownership.
