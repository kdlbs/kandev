---
status: draft
system: tasks
created: 2026-11-07
owners:
  - kandev
---

# Recorded task wait reasons requirements

## Overview

A task that is not working may be held by a workflow slot, a session-capacity
slot, or a provider limit. The workflow state alone cannot tell these apart, so
task lists and board views show the recorded wait instead. Tasks owns this
contract because the waits come from task admission and the task's current
dynamic route.

## Requirements

### REQ-TASKS-RECORDED-WAIT-REASONS-001: Distinguish recorded waits

**Intent:** Show why a task is waiting from recorded facts, never from a guess.

#### Acceptance criteria

- **AC-TASKS-RECORDED-WAIT-REASONS-001.1:** Task lists and board views shall
  distinguish a recorded WIP queue, a session-capacity queue, and a
  deadline-backed provider quota wait. Explicit permission or clarification and
  observed live activity shall keep their existing priority.
- **AC-TASKS-RECORDED-WAIT-REASONS-001.2:** `WAITING_FOR_INPUT` alone shall not
  be shown as a question that awaits an answer.
- **AC-TASKS-RECORDED-WAIT-REASONS-001.3:** The quota wait shall come from the
  current session's logical profile and route generation. Historical routes and
  terminal sessions shall not supply a current wait.
- **AC-TASKS-RECORDED-WAIT-REASONS-001.4:** A failed quota read shall not
  establish an empty observation or block updates from other task-status
  sources.
- **AC-TASKS-RECORDED-WAIT-REASONS-001.5:** Missing summaries shall be rebuilt
  with the current wait, and changes to existing summaries shall use their
  revisioned update path. A repair shall confirm the wait against the route
  before it replaces a stored wait, and if another writer wins it shall read
  both the latest summary and the route before retrying.
- **AC-TASKS-RECORDED-WAIT-REASONS-001.6:** The quota wait's help shall name the
  reset time, including the date when the reset is not today. A malformed
  deadline shall fall back to the generic help.
