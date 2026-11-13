---
status: active
system: tasks
created: 2026-06-23
owners:
  - cfl
---
# Subtask Completion Trigger Requirements

## Overview

This document is the migrated task-system source for the capability. The source detail below remains authoritative while the system is migrated into separate requirement and design records.

## Requirements

### REQ-TASKS-SUBTASK-COMPLETION-TRIGGER-001: Subtask Completion Trigger

**Intent:** Preserve the observable task or workflow behavior recorded by the legacy specification.

#### Acceptance criteria

- **AC-TASKS-SUBTASK-COMPLETION-TRIGGER-001.1:** When a consumer uses this capability, the system shall provide the observable behavior and exclusions documented below.

### REQ-TASKS-SUBTASK-COMPLETION-TRIGGER-002: Parent Transition Commit

**Intent:** A parent move driven by `on_children_completed` is either persisted
or reported as not applied, whichever path completed the last child.

#### Acceptance criteria

- **AC-TASKS-SUBTASK-COMPLETION-TRIGGER-002.1:** When the last active child
  enters a completion-enabled step through its own agent turn and the parent
  step's `on_children_completed` actions move the parent, the system shall
  persist the parent's new workflow step, or its queued admission to that step
  when the step's WIP limit is reached.
- **AC-TASKS-SUBTASK-COMPLETION-TRIGGER-002.2:** If an engine transition commit
  persists neither the task's step change nor a queued admission to the target
  step, the system shall report the transition as not applied and shall not
  record step history, run the target step's `on_enter`, or mark the
  children-completed operation applied.

### REQ-TASKS-SUBTASK-COMPLETION-TRIGGER-003: Successful child completion

**Intent:** The parent reacts to successful child completion. A failed or
cancelled child keeps the parent waiting.

This amendment addresses [issue #5258](https://github.com/kdlbs/kandev/issues/5258).
The user selected one automatic rule, with no additional setting. Existing
session, admission, and source-specific Office wake rules remain prerequisites.
Independent Office wakes are separate from this workflow trigger.

#### Acceptance criteria

- **AC-TASKS-SUBTASK-COMPLETION-TRIGGER-003.1:** `on_children_completed` shall
  require every counted child to have persisted state `COMPLETED`.
- **AC-TASKS-SUBTASK-COMPLETION-TRIGGER-003.2:** Any child state other than
  `COMPLETED` shall block the trigger. This includes failed batches, cancelled
  children, and mixed sets of successful and failed children.
- **AC-TASKS-SUBTASK-COMPLETION-TRIGGER-003.3:** An otherwise eligible trigger
  shall run its configured actions once when every counted child is
  `COMPLETED`. Parent transitions shall retain normal admission, exit, and
  entry behavior.
- **AC-TASKS-SUBTASK-COMPLETION-TRIGGER-003.4:** The success check shall count
  direct, active children only. Archived, ephemeral, and automation-run
  children shall remain excluded. An empty counted set shall not trigger
  parent actions.
- **AC-TASKS-SUBTASK-COMPLETION-TRIGGER-003.5:** A failed or cancelled child can
  later complete successfully. When every counted child then becomes
  `COMPLETED`, the parent shall remain eligible for one completion reaction.
  Duplicate delivery shall not repeat that reaction.
- **AC-TASKS-SUBTASK-COMPLETION-TRIGGER-003.6:** A blocked child-completion
  reaction shall not move the parent, create transition history, start or
  queue an agent, clear decisions, or consume the completion cycle. Child
  tasks, sessions, and workspaces shall retain existing recovery behavior.
- **AC-TASKS-SUBTASK-COMPLETION-TRIGGER-003.7:** The rule shall apply to existing,
  new, and imported workflows automatically. It shall introduce no additional
  setting, action, or trigger name.
- **AC-TASKS-SUBTASK-COMPLETION-TRIGGER-003.8:** Existing desktop and phone
  workflow help shall explain the successful-child requirement, the nonempty
  counted-child requirement, excluded child types, and the effect of failed or
  cancelled children. Grandchildren shall be identified as out of scope. The
  existing controls shall remain sufficient.
- **AC-TASKS-SUBTASK-COMPLETION-TRIGGER-003.9:** Missing, unknown, or unreadable
  child outcomes shall block the reaction without consuming its completion
  cycle. A later eligible child event can retry the reaction.
- **AC-TASKS-SUBTASK-COMPLETION-TRIGGER-003.10:** The change shall preserve global
  terminal-state definitions and previously applied workflow operations. It
  shall not rewind parents or create missing parent sessions.

The [completion design](../system-design/subtask-completion-success-policy.md)
defines this amendment. The [fix package](../../../plans/children-completed-success-policy/plan.md)
records implementation and validation work.

## Migrated source detail

The following source detail includes the successful-child amendment. Existing
workflows that previously advanced after failure or cancellation now wait for
successful completion.

## Why

Parent agents can delegate work by creating subtasks with `create_task_kandev`
and `parent_id`, but today they must poll `list_related_tasks_kandev` to learn
when all delegated workstreams are done. Polling burns agent turns and pushes
orchestration logic into the parent prompt instead of the workflow system.

## What

- A workflow step can define an `on_children_completed` event.
- The event fires on a parent task's current workflow step when every counted
  direct child task has persisted state `COMPLETED`.
- Global terminal task states remain `COMPLETED`, `FAILED`, and `CANCELLED`.
  This trigger requires success, so failure and cancellation block it.
- Entering the final step with **Complete task when entering this step** enabled
  persists `tasks.state = COMPLETED`. Step name or final position alone does not establish
  completion. [Task completion](task-completion.md) owns this setting and its
  legacy compatibility rules.
- The event is driven by task state changes and workflow-step moves into a
  terminal step, including subtasks created through `create_task_kandev` with
  `parent_id`.
- The event runs the step actions configured on `on_children_completed`, such as
  queueing a parent-agent run or moving the parent to a verification step.
- Workflow authors can configure the event from the workflow step editor with
  the "When Child Tasks Complete" transition selector. The editor explains that
  the trigger belongs on the parent step, waits for direct active children only,
  requires persisted `COMPLETED` outcomes, and ignores archived, ephemeral,
  and automation-run child tasks.
- The event fires at most once for the same completed child set. If a child is
  reopened and later returns to `COMPLETED`, the parent can receive a new
  event for the new completion cycle.
- The trigger considers direct children only. Grandchildren complete their own
  parent before any higher-level parent can react.
- Parent tasks with no active or primary session do not auto-create a session
  for this event; the event is skipped and logged.
- Existing workflows that do not configure `on_children_completed` behave
  unchanged.

## Data model

No new persistent table is required.

### `tasks`

The existing `tasks.parent_id` relationship defines the direct child set.
Children with `archived_at` set or `is_ephemeral = 1` are ignored by this
feature, as are children with origin `automation_run`.

The existing `tasks.state` field defines child success. The accepted state for
this trigger is:

```text
COMPLETED
```

Workflow entry can persist a terminal task state through the explicit
completion setting. Membership in a step alone is not evidence of completion.
The parent observes the committed task outcome.

### Workflow operation idempotency

The existing workflow engine operation-id store records each synthesized
`on_children_completed` trigger. The operation id is deterministic for the
parent and completed direct-child set, including enough child completion version
data to let a reopen-and-complete cycle fire again.

## API surface

No new HTTP route, WebSocket event, MCP tool, or CLI command is introduced.

Workflow step event JSON accepts and returns `on_children_completed` in the
existing `events` object:

```json
{
  "events": {
    "on_children_completed": [
      {
        "type": "queue_run",
        "config": {
          "target": "primary",
          "task_id": "this",
          "reason": "children_completed"
        }
      }
    ]
  }
}
```

The workflow settings UI exposes the transition-oriented subset of this event:

```json
{
  "events": {
    "on_children_completed": [
      {
        "type": "move_to_step",
        "config": {
          "step_id": "verification-step-id"
        }
      }
    ]
  }
}
```

The event uses the existing workflow-engine trigger payload:

```json
{
  "child_summaries": [
    {
      "task_id": "child-task-id",
      "status": "COMPLETED",
      "summary": "optional child completion summary",
      "pr_links": ["https://github.com/org/repo/pull/123"]
    }
  ]
}
```

`summary` and `pr_links` are best-effort enrichment. A missing summary or PR link
does not block the trigger.

## State machine

- A child completes successfully when its task state changes to `COMPLETED`.
  A move to a completion-enabled step first persists that outcome.
- On that transition or move, the system checks the child's parent.
- If the child has no parent, no parent event is considered.
- If any counted direct child of the parent is not `COMPLETED`, no parent event
  fires.
- If every counted direct child is `COMPLETED`, the parent task receives
  `on_children_completed` at its current workflow step.
- If configured actions transition the parent step, the normal workflow
  transition lifecycle applies: `on_exit`, persisted step change, data patch
  persistence, then `on_enter`.
- If a child leaves `COMPLETED`, the parent is considered waiting again.
  A later transition of that child back to `COMPLETED` can produce a
  new event.

## Permissions

This feature adds no new permission boundary.

- Agents and users can only create child tasks through existing task creation
  permissions and MCP tool exposure.
- Agents cannot fire `on_children_completed` directly; they can only cause it by
  changing task state or moving a child task through existing task completion
  paths.
- Workflow authors control the parent reaction by configuring
  `on_children_completed` actions on the parent step.

## Failure modes

- If the child-state event cannot be decoded, the event is dropped and a warning
  is logged.
- If sibling lookup fails, no parent trigger fires for that event and a warning
  is logged. A later child state transition can retry the check.
- If step completion persistence fails, that move does not supply a successful
  child outcome. The parent continues to evaluate committed task state.
- If the parent has no active or primary session, no new parent session is
  created. The skip is logged at debug/info level.
- If an `auto_start_agent` action fires while the parent session is still in a
  turn, the step prompt is queued instead of failing the launch. The message
  queue delivers it once after the current turn ends, and the completed child
  set is recorded as handled so a second delivery does not queue it again.
- If the workflow engine rejects or fails a configured action, the error is
  logged and the child task's terminal state remains unchanged.
- If multiple children finish concurrently, engine operation idempotency prevents
  duplicate parent actions for the same completed child set.
- If the parent's commit persists neither the step change nor a queued
  admission to the target step, the transition is reported as not applied: no step history row is recorded, the target step's `on_enter`
  does not run, and the operation is not marked applied, so a later child
  state transition can retry it.

## Persistence guarantees

- Child task state, parent-child relationships, workflow step configuration, and
  workflow operation ids survive backend restarts.
- A child completion event that was already applied before restart is not
  re-applied for the same completed child set.
- In-memory event delivery itself is not replayed after restart. A future child
  state transition or reopen-and-complete cycle can synthesize the trigger again.

## Scenarios

- **GIVEN** a parent task has two direct children and one child is still
  `IN_PROGRESS`, **WHEN** the first child changes to `COMPLETED`, **THEN** the
  parent's `on_children_completed` event does not fire.

- **GIVEN** a parent task has two direct children and the first child is
  `COMPLETED`, **WHEN** the second child changes from `IN_PROGRESS` to
  `COMPLETED`, **THEN** the parent receives one `on_children_completed` trigger.

- **GIVEN** a child task still has `state = REVIEW`, **WHEN** it moves into a
  final workflow step with completion enabled, **THEN** the child persists
  `state = COMPLETED` and the parent completion check treats that child as
  successful.

- **GIVEN** a parent step configures `on_children_completed` with `move_to_next`,
  **WHEN** all counted direct children are `COMPLETED`, **THEN** the parent moves to the
  next workflow step and that step's `on_enter` actions run normally.

- **GIVEN** a parent step configures `on_children_completed` with `move_to_step`
  and the last child moves into a completion-enabled step through its own
  agent turn (`on_turn_complete`), **WHEN** that move completes the child,
  **THEN** the parent's step change is persisted. The parent transition does
  not reuse the child turn's workflow delivery effect.

- **GIVEN** a parent step configures `on_children_completed` with `queue_run` for
  the primary agent, **WHEN** all counted direct children are `COMPLETED`, **THEN** a parent
  run is queued with the children-completed reason and child summary context.

- **GIVEN** one child finishes as `FAILED` and all other counted direct children
  are `COMPLETED`, **WHEN** the failed outcome is persisted, **THEN** the parent
  continues waiting. A later successful completion can release the trigger.

- **GIVEN** a child is already `COMPLETED`, **WHEN** a duplicate update attempts
  to set it to `COMPLETED` again, **THEN** no duplicate parent trigger fires.

- **GIVEN** all counted direct children are `COMPLETED` and the parent trigger has fired,
  **WHEN** one child is reopened to `IN_PROGRESS` and later changes back to
  `COMPLETED`, **THEN** the parent can receive a new `on_children_completed`
  trigger for the new completion cycle.

- **GIVEN** a child has its own child, **WHEN** the grandchild completes, **THEN**
  only the direct parent of that grandchild is evaluated for
  `on_children_completed`.

- **GIVEN** a parent step configures `on_children_completed` with
  `auto_start_agent` and the parent session is running a turn, **WHEN** the last
  counted direct child becomes `COMPLETED`, **THEN** the parent is not prompted mid-turn and
  receives the step prompt exactly once after that turn ends.

- **GIVEN** a parent has no active or primary session, **WHEN** all direct
  children become `COMPLETED`, **THEN** no session is created automatically and the
  skip is logged.

## Out of scope

- Adding a second event name such as `on_all_children_complete` or
  `on_subtasks_done`.
- Recursive descendant aggregation.
- Creating parent sessions automatically when no parent session exists.
- Adding a new MCP wait primitive or long-poll API.
- Requiring every child agent to know whether it is the last sibling.

## Implementation plans

- [Original trigger](../../../plans/subtask-completion-trigger/plan.md)
- [Parent transition commit](../../../plans/children-completed-parent-commit/plan.md)
- [Busy parent wake](../../../plans/children-completed-busy-parent-wake/plan.md)
- [Successful children policy](../../../plans/children-completed-success-policy/plan.md)
