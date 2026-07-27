---
status: shipped
---

# WIP Limit Pull System

## What

Workflow steps can define a work-in-progress limit and an optional feeder step.
`wip_limit` is a non-negative integer on each workflow step. `0` means unlimited
and preserves existing behavior. `pull_from_step_id` is empty by default; when it
is set, it must reference another step in the same workflow.

When a limited step is full, new task creation, manual moves, drag/drop moves,
API moves, MCP moves, bulk moves, and workflow-engine transitions into that
step are rejected instead of overfilling the step. The creation rule applies
whether the caller names the step explicitly or Kandev resolves it as the
workflow's start step. Same-step reordering is allowed.

Capacity admission is atomic per workflow step. Concurrent task creation and
move attempts cannot collectively admit more active, non-archived,
non-ephemeral tasks than the configured limit.

Integration watchers use the same admission rule as interactive and API task
creation. If a GitHub review watch observes more pull requests than its target
step can accept, Kandev creates only the tasks that obtain capacity and
auto-starts those tasks only when the configured step enables
`auto_start_agent`. Pull requests rejected for capacity remain eligible for a
later poll; Kandev does not leave a task, session, repository association, or
permanent review-watch reservation for a rejected attempt.

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

Creating a task in a full limited step returns the same conflict classification
and does not persist any part of the rejected task. Internal automation treats
the conflict as deferred work rather than an integration failure.

If a pull attempt races with another actor that fills the slot, the pull attempt
stops without overfilling the target step.

Deleting a step clears any `pull_from_step_id` that points at it.

## Scenarios

- **GIVEN** a workflow step with `wip_limit: 2` and two active tasks, **WHEN** a
  user, API client, MCP client, or integration watcher creates another
  non-ephemeral task directly in that step, **THEN** creation is rejected as a
  WIP-capacity conflict and no task or session is persisted.
- **GIVEN** a workflow whose limited start step has `wip_limit: 2`, **WHEN** a
  caller creates a task without specifying `workflow_step_id` and the start
  step is full, **THEN** creation is rejected by the same capacity rule.
- **GIVEN** an empty auto-start step with `wip_limit: 2`, **WHEN** a GitHub
  review watch concurrently dispatches eight newly observed pull requests,
  **THEN** exactly two review tasks obtain capacity and auto-start, while the
  other six pull requests retain no task or reservation and remain eligible for
  a later poll.
- **GIVEN** a review watch targets a `Review` step with
  `on_enter: auto_start_agent` and `on_turn_complete: move_to_next`, **WHEN** an
  admitted review task starts, **THEN** it remains in `Review` throughout agent
  startup and the active turn, ignores boot-ready events for workflow
  advancement, and moves to the next step exactly once only after the real
  agent turn completes.
- **GIVEN** the review-watch scenario above and one accepted review task later
  leaves the limited step, **WHEN** the watch polls again, **THEN** one
  previously deferred pull request can obtain the newly available capacity.
- **GIVEN** a step with `wip_limit: 0`, **WHEN** callers create tasks
  concurrently in that step, **THEN** creation remains unlimited.

## Out of scope

- WIP limits do not replace `agent_profiles.max_concurrent_sessions` or impose
  a profile-wide execution budget.
- This change does not add a separate GitHub review-watch
  `max_inflight_tasks` setting.
