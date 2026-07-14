---
status: building
---

# WIP Limit Pull System

## What

Workflow steps can define a work-in-progress limit and an optional feeder step.
`wip_limit` is a non-negative integer on each workflow step. `0` means unlimited
and preserves existing behavior. `pull_from_step_id` is empty by default; when it
is set, it must reference another step in the same workflow.

When a limited step is full, manual moves, drag/drop moves, API moves, MCP moves,
bulk moves, and workflow-engine transitions into that step are rejected instead
of overfilling the step. Same-step reordering is allowed.

When a task leaves a limited step that has a feeder configured, Kandev attempts
to pull queued tasks from the feeder into the vacated step until the step reaches
its limit or no feeder task remains. Pull order is deterministic: position ASC,
priority rank (`critical`, `high`, `medium`, `low`, none/unknown), created time
ASC, then task id ASC.

The Kanban board shows the current task count for unlimited steps and
`occupied/limit` for limited steps. If legacy or concurrent data leaves a step
over limit, the board shows the over-limit count as a warning state.

## Why

Kanban teams often work by pulling the next highest-priority task when capacity
opens instead of pushing arbitrary tasks forward. Without a step-level limit,
Kandev can start too many tasks in the same workflow stage and cannot model a
simple queue-to-work pull system.

## Data Model

`workflow_steps` stores:

- `wip_limit INTEGER NOT NULL DEFAULT 0`
- `pull_from_step_id TEXT NOT NULL DEFAULT ''`

Workflow step API responses, workflow template definitions, workflow export and
import data, task DTOs, WebSocket payloads, and MCP workflow-step config tools
all preserve these fields.

Workflow export stores the pull source portably as a step position instead of an
instance-specific UUID. Import maps that position back to the newly-created step
ID.

## Failure Modes

Moving into a full limited step returns a conflict with a user-visible message
that includes the target step and limit. Optimistic UI moves must roll back.

If a pull attempt races with another actor that fills the slot, the pull attempt
stops without overfilling the target step.

Deleting a step clears any `pull_from_step_id` that points at it.

## Scope Note

`agent_profiles.max_concurrent_sessions` is related but separate. This feature
adds per-step WIP limits and pull behavior. Kanban enforcement of profile-level
session caps remains tracked separately unless implemented in the same change.
