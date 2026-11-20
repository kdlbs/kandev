---
status: current
system: tasks
requirements:
  - REQ-TASKS-SUBTASK-COMPLETION-TRIGGER-002
---

# Subtask Completion Parent Commit System Design

## Purpose and boundaries

This design covers how the orchestrator commits the parent's
`on_children_completed` move. Trigger detection, child-set idempotency, and
step actions follow the [subtask completion trigger
requirements](../requirements/subtask-completion-trigger.md) and are unchanged.

## Requirement mapping

| Acceptance criteria | Design sections |
| --- | --- |
| `AC-TASKS-SUBTASK-COMPLETION-TRIGGER-002.1` | Workflow delivery effect scope |
| `AC-TASKS-SUBTASK-COMPLETION-TRIGGER-002.2` | Commit result |

## Workflow delivery effect scope

An agent turn's `on_turn_complete` handling carries a workflow delivery effect
(`models.AgentDeliveryEffect`, key `workflow.on_turn_complete:<turn>`) in its
context. `workflowStore.updateTransitionTask` commits a step move together with
that effect through `UpdateTaskWithWorkflowStepAdmissionAndEffect`, and the
repository refuses the move when the effect key already exists. This makes a
replayed turn unable to move its task twice.

The effect belongs to the task whose turn produced it. When that move puts the
last child into a completion-enabled step, `markTaskCompletedForTerminalStep`
reaches `processOnChildrenCompleted` for the parent with the same context.
`processOnChildrenCompleted` drops the effect (`withoutWorkflowEffect`) before
it evaluates and commits the parent, so the parent's move is committed through
the plain admission path. Child-set idempotency for the parent stays with the
workflow operation ID (`childCompletionOperationID`) and its lock.

## Commit result

`applyEngineTransitionWithMode` commits through
`workflowStore.applyTransitionReportingApplied`, which returns whether the task
row was written. `applyEngineTransitionWithCommitMode` already treats
`applied=false` as a failed commit: it records a workflow transition error and
returns `false` before step history, data patch, terminal completion, and
`on_enter`. `processOnChildrenCompleted` marks the operation applied only after
a successful transition, so a later child state change can retry the parent.

A commit that admits the task to a queue for the target step (WIP limit
reached) writes the task row and returns `applied=true`. The lifecycle records
history and leaves destination state and `on_enter` to queue promotion, as for
any queued move.

`workflowStore.ApplyTransition`, which implements the engine's `Store`
interface, keeps its error-only signature.

## Verification boundary

`event_handlers_children_completed_turn_test.go` drives the last child through
`processOnTurnCompleteViaEngineWithCause` with a workflow effect and checks the
parent's persisted step, and checks that a commit refused for a consumed effect
returns not applied without engine ledger rows or `on_enter`.
