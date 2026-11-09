---
status: draft
system: tasks
created: 2026-11-07
owners:
  - kandev
---

# Recorded Task Wait Reasons Requirements

## Overview

Task lists and board views distinguish a recorded WIP queue, session-capacity
queue, and deadline-backed provider quota wait from ordinary work in progress.
The task system owns this contract because the waits come from recorded task,
launch-queue, and session routing state that it projects into the task status
summary.

## Terminology

- **Recorded wait:** A wait backed by stored state: a WIP overflow queue entry,
  a session-capacity launch-queue entry, or a quota-limited route with a known
  retry deadline.

## Requirements

### REQ-TASKS-TASK-WAIT-REASONS-001: Recorded task wait reasons

**Intent:** A task that is waiting for a board slot, session capacity, or a
provider quota deadline shall not look like ordinary work in progress.

**User story:** As a user, I want to see why a task is waiting, so that I know
whether a slot, capacity, or a provider reset will release it.

#### Acceptance criteria

- **AC-TASKS-TASK-WAIT-REASONS-001.1:** When a task has a recorded quota wait,
  session-capacity queue entry, or unadmitted WIP queue entry, task lists, the
  board, the pipeline graph, the desktop sidebar, and the mobile switcher shall
  show that wait, with a quota wait ranked first and a WIP queue ranked last.
- **AC-TASKS-TASK-WAIT-REASONS-001.2:** Explicit permission or clarification
  requests and observed live activity shall keep their existing priority over a
  recorded wait. `WAITING_FOR_INPUT` alone shall not be shown as a question that
  awaits an answer, and a terminal or interrupted task shall keep its own icon.
- **AC-TASKS-TASK-WAIT-REASONS-001.3:** The quota wait shall come only from the
  current session's logical profile and route generation with a classified quota
  failure and a retry deadline. Historical routes, removed sessions, and
  terminal sessions shall not supply a current wait.
- **AC-TASKS-TASK-WAIT-REASONS-001.4:** A failed quota read shall not establish
  an empty observation or block updates from other task-status sources. Missing
  summaries shall be rebuilt with the current wait, existing summaries shall
  change through their revisioned update path, and a repair that loses to
  another writer shall read the latest summary and route before retrying.

## Out of scope

- Provider block policy, candidate model order, and quota classification belong
  to agent routing.
- The control-only waiting-reasons projection is a separate view.
