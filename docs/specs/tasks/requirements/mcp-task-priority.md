---
status: active
system: tasks
created: 2026-10-26
owners:
  - kandev
---

# MCP task priority requirements

## Overview

MCP clients need to record task urgency during creation and change it during
later triage. The tasks system owns this contract because priority belongs to
the durable task record. This extends the automation write surface excluded
from [task priority visibility](task-priority-visibility.md).

The vocabulary remains `critical`, `high`, `medium`, and `low`. Creation uses
`medium` when the caller omits priority. Update omission preserves the stored
value.

## Requirements

### REQ-TASKS-MCP-PRIORITY-001: Priority throughout the MCP task lifecycle

**Intent:** Automation clients can set and revise native task urgency without
manual intervention or a task-state transition.

#### Acceptance criteria

- **AC-TASKS-MCP-PRIORITY-001.1:** Wherever `create_task_kandev` is available,
  it shall accept an optional `priority` argument with exactly the four tokens.
  A new task shall persist and return the requested token.
- **AC-TASKS-MCP-PRIORITY-001.2:** When creation omits priority, a new task
  shall receive `medium`, including a subtask whose parent has another priority.
  Existing creation callers shall retain their current behavior.
- **AC-TASKS-MCP-PRIORITY-001.3:** Wherever `update_task_kandev` is available,
  it shall accept optional priority. An authorized call containing only
  `task_id` and priority shall persist and return that priority. External MCP
  shall expose this update tool alongside its existing creation tool.
- **AC-TASKS-MCP-PRIORITY-001.4:** When update omits priority, the stored
  priority shall remain unchanged. A priority-only update shall preserve every
  other business field, board position, workflow placement, and session state.
  Normal update timestamps can advance. Repeating a valid priority shall succeed.
- **AC-TASKS-MCP-PRIORITY-001.5:** MCP shall advertise the exact enum in both
  tools. Explicit empty, padded, uppercase, unknown, null, and non-string values
  shall fail before backend dispatch. Invalid backend string values shall fail
  validation before creation, deferred-prompt replacement, or another field write.
- **AC-TASKS-MCP-PRIORITY-001.6:** A deduplicated creation shall return the
  existing task and its stored priority. Neither a different requested priority
  nor an omitted priority shall change that task. Clients shall use update for
  later reprioritization.
- **AC-TASKS-MCP-PRIORITY-001.7:** Task listing and existing task events shall
  carry the stored priority after each successful write. Connected board views
  shall reflect an MCP priority update through the existing task-update event.
- **AC-TASKS-MCP-PRIORITY-001.8:** Priority writes shall retain current task
  ownership, workspace, executor, and automation target checks. A denied call
  shall leave the target unchanged. Office sessions shall gain no MCP task-write
  tool. Existing external access to authorized Office tasks shall remain valid.
- **AC-TASKS-MCP-PRIORITY-001.9:** Existing `update_task_state_kandev` calls
  shall keep their current input and behavior. Configuration catalogs shall
  retain their current tools. Priority shall remain independent of scheduling
  policy, task assignment, and workspace materialization.

## Out of scope

- Office CLI priority support or Office assignment and vocabulary changes.
- New task-update tool names, new endpoints, or generic update-field expansion.
- UI controls, sorting or filtering changes, or new translations.
- Database changes, bulk priority updates, or configurable priority defaults.
- New permissions, runtime flags, scheduler behavior, or idempotency rules.

## Design and delivery

- [System design](../system-design/mcp-task-priority.md)
- [Implementation plan](../../../plans/mcp-task-priority/plan.md)
- [External MCP contract](../../integrations/requirements/external-mcp.md)
