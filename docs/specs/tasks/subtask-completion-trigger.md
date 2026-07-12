---
status: draft
created: 2026-06-23
owner: cfl
---

# Subtask Completion Trigger

## Why

Parent agents can delegate work by creating subtasks with `create_task_kandev`
and `parent_id`, but today they must poll `list_related_tasks_kandev` to learn
when all delegated workstreams are done. Polling burns agent turns and pushes
orchestration logic into the parent prompt instead of the workflow system.

## What

- A workflow step can define an `on_children_completed` event.
- The event fires on a parent task's current workflow step when every direct,
  active child task has reached terminal completion.
- Terminal task states are `COMPLETED`, `FAILED`, and `CANCELLED`.
- A child also counts as terminal when it is in a final workflow step named
  `Done`, `Complete`, `Completed`, or `Approved`; entering that step persists
  `tasks.state = COMPLETED`.
- The event is driven by task state changes and workflow-step moves into a
  terminal step, including subtasks created through `create_task_kandev` with
  `parent_id`.
- The event runs the step actions configured on `on_children_completed`, such as
  queueing a parent-agent run or moving the parent to a verification step.
- Workflow authors can configure the event from the workflow step editor with
  the "When Child Tasks Complete" transition selector. The editor explains that
  the trigger belongs on the parent step, waits for direct active children only,
  treats terminal task states and terminal workflow-step membership as terminal,
  and ignores archived or ephemeral child tasks.
- The event fires at most once for the same completed child set. If a child is
  reopened and later returns to a terminal state, the parent can receive a new
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
feature, matching existing active-child listing semantics.

The existing `tasks.state` field defines terminal child completion. The terminal
state set for this feature is:

```text
COMPLETED
FAILED
CANCELLED
```

`tasks.workflow_step_id` is also considered. A child task is complete when its
current step is the final step in that workflow and the step name is `Done`,
`Complete`, `Completed`, or `Approved`. This keeps visual board completion and
API state consistent without adding a new persistent terminal-step flag.

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

- A child enters terminal completion when its task state changes from a
  non-terminal state to `COMPLETED`, `FAILED`, or `CANCELLED`, or when it moves
  into a terminal workflow step.
- On that transition or move, the system checks the child's parent.
- If the child has no parent, no parent event is considered.
- If any active direct child of the parent is still non-terminal, no parent event
  fires.
- If every active direct child is terminal, the parent task receives
  `on_children_completed` at its current workflow step.
- If configured actions transition the parent step, the normal workflow
  transition lifecycle applies: `on_exit`, persisted step change, data patch
  persistence, then `on_enter`.
- If a child later moves from terminal to non-terminal, the parent is considered
  waiting again. A later transition of that child back to terminal can produce a
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
- If terminal step lookup fails, the child is evaluated by task state only.
- If the parent has no active or primary session, no new parent session is
  created. The skip is logged at debug/info level.
- If the workflow engine rejects or fails a configured action, the error is
  logged and the child task's terminal state remains unchanged.
- If multiple children finish concurrently, engine operation idempotency prevents
  duplicate parent actions for the same completed child set.

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
  final workflow step named `Done`, **THEN** the child persists
  `state = COMPLETED` and the parent completion check treats that child as
  terminal.

- **GIVEN** a parent step configures `on_children_completed` with `move_to_next`,
  **WHEN** all direct children are terminal, **THEN** the parent moves to the
  next workflow step and that step's `on_enter` actions run normally.

- **GIVEN** a parent step configures `on_children_completed` with `queue_run` for
  the primary agent, **WHEN** all direct children are terminal, **THEN** a parent
  run is queued with the children-completed reason and child summary context.

- **GIVEN** one child finishes as `FAILED` and all other direct children are
  terminal, **WHEN** the failed child transitions from non-terminal to `FAILED`,
  **THEN** the parent receives `on_children_completed`.

- **GIVEN** a child is already `COMPLETED`, **WHEN** a duplicate update attempts
  to set it to `COMPLETED` again, **THEN** no duplicate parent trigger fires.

- **GIVEN** all direct children are terminal and the parent trigger has fired,
  **WHEN** one child is reopened to `IN_PROGRESS` and later changes back to
  `COMPLETED`, **THEN** the parent can receive a new `on_children_completed`
  trigger for the new completion cycle.

- **GIVEN** a child has its own child, **WHEN** the grandchild completes, **THEN**
  only the direct parent of that grandchild is evaluated for
  `on_children_completed`.

- **GIVEN** a parent has no active or primary session, **WHEN** all direct
  children become terminal, **THEN** no session is created automatically and the
  skip is logged.

## Out of scope

- Adding a second event name such as `on_all_children_complete` or
  `on_subtasks_done`.
- Recursive descendant aggregation.
- Creating parent sessions automatically when no parent session exists.
- Adding a new MCP wait primitive or long-poll API.
- Requiring every child agent to know whether it is the last sibling.
