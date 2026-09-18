---
status: current
system: tasks
requirements:
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
| REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003 | Diagnostic response, Recovery instructions, Verification |

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

## Related decisions and contracts

- [ADR 0015](../../../decisions/0015-explicit-completion-signal-for-auto-advance.md)
- [Turn stamp design](workflow-task-step-transition-ledger.md)
- [Requirements](../requirements/workflow-explicit-completion-signal.md)

## Implementation plans

- [Issue 3772 recovery package](../../../plans/step-completion-stale-turn-recovery/plan.md)
