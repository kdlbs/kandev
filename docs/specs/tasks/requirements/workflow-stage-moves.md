---
status: draft
system: tasks
created: 2026-10-12
owners:
  - kandev
---

# Workflow Stage Move Integrity Requirements

## Overview

A task moves between workflow steps through several paths: an immediate board
or agent move, a `move_task_kandev` call deferred until the calling turn ends,
admission from a full step's WIP queue, and an automatic advance after a
signal-gated step's completion signal. Each path must leave the board, the
destination step's entry, and the agent's view of the request consistent with
what actually happened.

These requirements close four gaps on those paths:

- A deferred move can relocate a task after a later move already placed it
  somewhere else.
- A same-step WIP queue admission can skip the destination step's entry, or
  run it again for a newer entry.
- A signal-gated step can advance while its own entry prompt is still waiting
  for session capacity, so the step's turn never runs.
- An agent can attach a hand-off to a step that never starts an agent, and
  move failures do not tell the agent why the call failed.

## Terms

- **Deferred move:** A `move_task_kandev` step change requested while the
  calling session is `RUNNING` or `STARTING`. It is recorded as a pending move
  and applied when the turn ends.
- **Later move:** Any step change of the same task that commits after the
  deferred move was recorded and before it is applied, including a change
  that later returns the task to the original step.
- **Same-step WIP admission:** Admission of a task that was queued on a full
  step. The task entered the step when it was queued, so the admission itself
  records no new step transition.
- **Step entry prompt:** The destination step's own prompt that a workflow
  entry delivers to the destination session. It can wait for session capacity
  as a ceiling-deferred launch bound to that entry.
- **Manual step:** A step whose entry starts no work by itself: it declares no
  `auto_start_agent`, `queue_run`, `queue_run_for_each_participant`, or
  `run_code_review` `on_enter` action and no pull source.

## Requirements

### REQ-TASKS-WORKFLOW-STAGE-MOVES-001: Superseded deferred moves

**Intent:** A deferred move applies only against the board position it was
requested from, so it never overwrites a later move.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-STAGE-MOVES-001.1:** When a later move has placed the
  task on a step other than the step the deferred move was requested from,
  the system shall discard the deferred move at turn end and leave the task
  on its current step.
- **AC-TASKS-WORKFLOW-STAGE-MOVES-001.2:** When a later move has left the
  original step and returned to it before the deferred move is applied, the
  system shall discard the deferred move at turn end and leave the task on
  its current step.
- **AC-TASKS-WORKFLOW-STAGE-MOVES-001.3:** When a later move commits between
  the turn-end check and the atomic application of the deferred move, the
  system shall reject the application without changing the task and discard
  the deferred move.
- **AC-TASKS-WORKFLOW-STAGE-MOVES-001.4:** When the system discards a
  superseded deferred move, it shall also remove that move's queued hand-off
  prompt and record the drop in a structured log entry.
- **AC-TASKS-WORKFLOW-STAGE-MOVES-001.5:** When no later move has committed,
  the system shall apply the deferred move, its entry options, and its
  destination-step entry exactly as before.
- **AC-TASKS-WORKFLOW-STAGE-MOVES-001.6:** When a pending move was recorded
  before the system stored its originating step, the system shall apply it
  with the existing current-step behavior. When the transition history cannot
  be read at turn end, the system shall keep the step-based check and shall
  not discard the move on that uncertainty alone.

### REQ-TASKS-WORKFLOW-STAGE-MOVES-002: Same-step WIP admission entry

**Intent:** Admitting a queued task runs the entry of the step it was queued
on exactly once.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-STAGE-MOVES-002.1:** When a task queued on a full step
  is admitted to that same step, the system shall run that step's entry,
  including `auto_start_agent`, for the entry recorded when the task was
  queued.
- **AC-TASKS-WORKFLOW-STAGE-MOVES-002.2:** When the admitted task leaves the
  step and enters it again before the admission's entry is handled, the
  system shall not run the newer entry's actions for the stale admission.
- **AC-TASKS-WORKFLOW-STAGE-MOVES-002.3:** When an admission marker was
  written before the system recorded the admitted entry on it, the system
  shall resolve the entry from the task's latest step transition.

### REQ-TASKS-WORKFLOW-STAGE-MOVES-003: Signal-gated advance after the entry prompt

**Intent:** A signal-gated step advances only after its own entry prompt has
been delivered or is no longer valid.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-STAGE-MOVES-003.1:** When a ceiling-deferred step entry
  prompt that is bound to the task's current entry finds its destination
  session `STARTING` or `RUNNING`, the system shall keep the prompt for a
  later delivery attempt instead of discarding it as superseded.
- **AC-TASKS-WORKFLOW-STAGE-MOVES-003.2:** When a ceiling-deferred prompt has
  no workflow-entry binding and finds its destination session `STARTING` or
  `RUNNING`, the system shall keep the existing superseded disposition.
- **AC-TASKS-WORKFLOW-STAGE-MOVES-003.3:** While the current signal-gated
  step's own entry prompt is still queued and valid, the system shall not
  advance the task on a completion signal, and shall leave the session
  `WAITING_FOR_INPUT`.
- **AC-TASKS-WORKFLOW-STAGE-MOVES-003.4:** When the step's entry prompt has
  been delivered or is no longer valid, or the turn ended by user
  cancellation, the system shall evaluate the completion signal as before.
- **AC-TASKS-WORKFLOW-STAGE-MOVES-003.5:** When the queued entry prompt cannot
  be validated, the system shall treat it as still pending.

### REQ-TASKS-WORKFLOW-STAGE-MOVES-004: Agent moves to manual steps

**Intent:** An agent cannot attach a hand-off that no step will receive, and a
rejected move tells the agent how to correct the call.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-STAGE-MOVES-004.1:** When `move_task_kandev` requests a
  step change to a manual step with a non-empty `prompt` or `entry_options`,
  the system shall reject the call with a validation error that names the
  missing agent recipient, even when the task has a session that could receive
  the hand-off. It shall not record a pending move. This agent-surface rule is
  deliberately stricter than the `task.move` rule in
  AC-TASKS-WORKFLOW-STAGE-MOVES-004.5.
- **AC-TASKS-WORKFLOW-STAGE-MOVES-004.2:** When the destination step starts
  work on entry through `auto_start_agent`, `queue_run`,
  `queue_run_for_each_participant`, `run_code_review`, or a pull source, the
  system shall accept the entry options under the existing rules.
- **AC-TASKS-WORKFLOW-STAGE-MOVES-004.3:** When `move_task_kandev` fails with
  a known validation or conflict reason, the system shall include that reason
  in the agent-facing error text without internal identifiers. An
  unrecognized error shall keep the existing generic text.
- **AC-TASKS-WORKFLOW-STAGE-MOVES-004.4:** The `move_task_kandev` tool
  description shall state that a move to a step that does not start an agent
  must omit `prompt` and `entry_options`.
- **AC-TASKS-WORKFLOW-STAGE-MOVES-004.5:** The HTTP and WebSocket `task.move`
  surfaces shall keep their existing entry-option acceptance rules.

## Out of scope

- Explicit, authorized cancellation of one exact pending move by a caller.
  These requirements cover only automatic supersession at turn end.
- Age-based expiry of pending moves, which the existing pending-move reaper
  owns.
- Changes to the `on_enter` dispatch ownership and idempotency rules in
  [`workflow-on-enter-action-dispatch`](../../workflow-on-enter-action-dispatch/spec.md).
- Session-ceiling admission order and ceiling replay retry policy.
- The completion-signal contract and the `advances` response field in
  [`workflow-explicit-completion-signal`](workflow-explicit-completion-signal.md),
  which these requirements do not change.
- Entry-option semantics defined by
  [`workflow-step-move-overrides`](../../workflow-step-move-overrides/spec.md),
  except the agent-surface manual-step rule in REQ-TASKS-WORKFLOW-STAGE-MOVES-004.

## Related documents

- [Workflow task step transition ledger](workflow-task-step-transition-ledger.md)
- [MCP task move results](mcp-task-move-results.md)
- [Deferred move on_enter dispatch identity](deferred-move-onenter-dispatch-identity.md)
- [Workflow step move overrides](../../workflow-step-move-overrides/spec.md)
