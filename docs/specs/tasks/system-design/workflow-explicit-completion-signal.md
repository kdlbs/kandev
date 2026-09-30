---
status: current
system: tasks
requirements:
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003
---

# Completion signal recovery system design

## Purpose and boundaries

The tasks system owns completion eligibility and its recovery instructions.
This design adds diagnostics to the existing completion contract.
It preserves the current turn stamp, signal claim, and workflow transition rules.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001 | Applicability response (`advances`/`note`) |
| REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003 | Diagnostic response, Recovery instructions, Verification |

## Applicability response (`advances`/`note`)

`accepted:true` means the signal was durably recorded — it does not mean the
step will actually advance. A step whose `auto_advance_requires_signal` is
`false` never reads the recorded signal at turn end, so a caller could accept a
signal that changes nothing and have no way to tell from the response alone
(this is exactly what happened in ISSUE-5: an agent woken on a non-auto-start
step called `step_complete_kandev`, got `accepted:true`, and nothing moved).

`internal/mcp/handlers/handlers.go:handleStepComplete` resolves the calling
turn's step through the existing `h.workflowCtrl.GetStep` call already used
for the stale-turn diagnostic above, and additively sets two response fields
before returning:

- `advances` (bool) — `true` only when the step's `AutoAdvanceRequiresSignal`
  is set and `WorkflowStep.AdvancesOnTurnComplete()` reports a move the
  engine runs on turn completion.
- `note` (string) — present only when `advances` is `false`, explaining that
  the step does not advance on a completion signal, or, for a signal-gated
  step, that it has no `on_turn_complete` move that runs automatically.

`internal/workflow/models.WorkflowStep.AdvancesOnTurnComplete` mirrors
`internal/workflow/engine.compileOnTurnComplete` and `evaluateActions`:
`move_to_next` and `move_to_previous` count, `move_to_step` counts only with a
non-empty `step_id`, and a move marked `requires_approval` does not count
because neither engine nor legacy turn completion runs it. `disable_plan_mode`
changes session settings and never moves the task. A move behind a
`wait_for_quorum` guard counts, because quorum re-evaluation can still apply
it. The field describes configuration, not a promise: a clarification barrier,
an unsatisfied guard, or a last-step `move_to_next` can still leave the task
in place.

A signal-gated step without such a move still accepts and records the signal.
The turn-end gate finds the signal, the engine selects no transition, and the
session waits for input. The bag entry stays until a new user turn or a
clarification pause clears it, or until the task leaves the step and the entry
becomes stale. A signal on a non-gated step is handled the same way today: it
is recorded and does not drive a transition. Rejecting the call instead was
considered and not chosen. It would change the `accepted` contract that
signal-gated prompts rely on, and it would answer the same "this signal will
not move the task" situation with an error for gated steps and a success for
non-gated steps.

`accepted` is never changed by this — ADR 0015 semantics and existing agent
prompts depend on it staying `true` once the signal is recorded. When the step
lookup itself fails (no workflow controller wired, or the step ID does not
resolve), both `advances` and `note` are omitted entirely rather than defaulted
to a guessed value — a caller must not infer "does not advance" from a lookup
failure that says nothing about the step's actual configuration.

## Diagnostic response

`internal/mcp/handlers/handlers.go:handleStepComplete` compares the launch step
from `stepCompletionLaunchStep` with the task's current workflow step.
The mismatch continues to return `VALIDATION_ERROR` before the signal claim.

The error retains its existing prefix for compatibility. Its text includes both
step IDs and the recovery instruction. This information belongs in the message,
because `internal/mcp/server/server.go:stepCompleteHandler` forwards `err.Error()`.
A details-only change would not reliably reach the agent.

Suggested message:

> workflow step changed before signal was recorded. This turn started in step
> <launch_step_id>. The current step is <current_step_id>. No signal was recorded.
> Retrying in this turn cannot recover. End this turn and ask the user to resume
> this session for the current step. After satisfying that step, signal completion
> from the new turn. Do not move the task solely to bypass this error.

A missing stamp or failed turn read remains a separate internal error.
The message does not promise that a concurrent future move cannot occur.

## Recovery instructions

The registered tool description explains the distinction between a stale-turn
error and `already_signaled`. A duplicate preserves the existing accepted signal.
A stale-turn rejection records nothing and requires a fresh turn.

`internal/sysprompt/sysprompt.go` supplies concise recovery guidance in
`stepCompleteSection` and `officeStepCompleteInstruction`. Gating and mode
visibility stay unchanged. The long explanation resides in the returned error.
This keeps prompt and tool-description size budgets intact.

The agent ends its stale turn with a clear recovery request. A user resumes the
same session on the current step. Normal turn creation stamps that step.
The agent evaluates current-step work before its final completion call.
No tool call silently creates a turn, restamps an old turn, or moves the task.

The existing operator move remains documented as a separate manual decision.
An error does not grant an agent new authority to use `move_task_kandev`.

## Persistence and compatibility

There is no schema change, new response envelope, or new MCP argument.
`workflow_step_id_at_start` remains an immutable historical fact.
The task-at-step conditional signal claim and duplicate handling remain intact.
RUNNING and WAITING_FOR_INPUT sessions both retain the mismatch guard.
Idleness does not prove that work belongs to the new step.

## Verification

Handler tests use the SQLite-backed existing fixtures. They reproduce Review to
Work movement after turn creation, reject repeated stale calls, and accept a
call after a new Work turn starts. Both session states retain the guard.

Server tests exercise error forwarding through the MCP handler. Prompt tests
cover gated task and Office contexts, omission from ungated instructions, and
existing size budgets. These are protocol tests, not browser layout changes.

Handler tests also cover the `advances`/`note` response fields directly: a
signal-gated step with a move action returns `advances:true` with no `note`; a
non-signal-gated step returns `advances:false` with a `note`; a signal-gated
step without a runnable move returns `advances:false` with a `note` while the
signal is still recorded and published; a step that fails to resolve omits
both fields. A model test pins `AdvancesOnTurnComplete`, and an engine test
compares it with the compiled `on_turn_complete` actions.

## Related decisions and contracts

- [ADR 0015](../../../decisions/0015-explicit-completion-signal-for-auto-advance.md)
- [Turn stamp design](workflow-task-step-transition-ledger.md)
- [Requirements](../requirements/workflow-explicit-completion-signal.md)

## Implementation plans

- [Issue 3772 recovery package](../../../plans/step-completion-stale-turn-recovery/plan.md)
- [Applicability response package](../../../plans/office-assignment-step-eligibility/plan.md)
