---
status: current
system: tasks
requirements:
  - REQ-TASKS-STALE-CREATED-AUTOSTART-001
created: 2026-10-20
owners:
  - kandev
---

# Stale CREATED auto-start system design

## Purpose and boundaries

This design classifies a lost CREATED race at the STARTING persistence boundary
and lets the workflow auto-start dispatch continue from the state that won. It
does not change how other paths move sessions, how prompts are queued, or how
genuine launch failures are recorded.

## Components

| Surface | Responsibility |
| --- | --- |
| `executor.ErrSessionStateChangedBeforeStart` | Marks a refused STARTING write whose stored session is non-terminal and owns no agent |
| `Service.persistFullTaskSessionIfCurrent` | Orchestrator STARTING persistence; wraps the marker for CREATED and WAITING_FOR_INPUT |
| `Executor.persistSessionFullRowIfCurrentState` | Executor fallback persistence; wraps the same marker |
| `Service.startCreatedSession` | Returns the marked error without `handleSessionLaunchFailure`, completing the turn it opened |
| `Service.launchCreatedAutoStartStepPrompt` | Routes a stale snapshot or a lost race to `continueAutoStartAfterCreatedStateLost` |
| `Service.continueAutoStartAfterCreatedStateLost` | Delivers through `retryAutoStartStepPrompt` or returns the hand-off to the queue |

## Persistence classification

The guarded STARTING write already classifies its refusals. A terminal session
yields `SessionStateSupersededError`; STARTING and RUNNING yield
`ErrExecutionAlreadyRunning`. The remaining non-terminal states, CREATED and
WAITING_FOR_INPUT, own no agent, so the refusal means another path advanced the
session and this launch has nothing to clean up beyond the execution it already
stops. Those refusals wrap `ErrSessionStateChangedBeforeStart`. The message text
is unchanged apart from the appended marker.

`startCreatedSession` treats the marker like `ErrExecutionAlreadyRunning`: it
completes the turn it opened and returns the error. It does not call
`handleSessionLaunchFailure`, which is the call that wrote FAILED to the session
and, through `updateTaskStateForEarlyLaunchFailure`, to the task.

## Auto-start continuation

`launchCreatedAutoStartStepPrompt` reloads the session under the
cancel-in-flight guard. A terminal session keeps its terminalized error. A
session that is no longer CREATED releases the guard and continues without
launching. A launch that returns the marker retires its start attempt, releases
the guard, reloads the session, and continues the same way.

The continuation preserves the original workflow-entry and session-incarnation
snapshot. It reuses an existing ceiling entry binding and derives one from the
captured transition where the existing repository supports that path. It never
rebinds old input to a successor workflow entry.

A WAITING_FOR_INPUT session takes retryAutoStartStepPrompt. A fresh STARTING
or RUNNING session retains the entire raw step prompt through the existing
busy-queue policy, preserving the recorded-message flag, attachments, references,
plan mode and handoff metadata. Successful full queueing does not separately
restore the taken handoff. Callers that disable queueing keep their existing
waiting-session retry behavior.

Queue insertion uses the existing session/workflow-entry atomic admission when
available. The existing compatibility path remains available to focused or
legacy queue stores without that capability. Prompt retries carry validation
at the normal dispatch and final provider-admission callbacks. The same scoped
callbacks travel through composed fresh fallback and initial prompt admission;
ordinary prompt/start callers receive no new admission requirement.

Fresh fallback validates under the existing task-entry and cancellation guards
before resetting the original session. An intentional stale-entry refusal returns
without dispatching or entering generic launch-failure handling. The winning
session and task state remain owned by the winning path. A taken handoff is
restored once across the continuation's refusal callbacks.

## Verification

`event_handlers_stale_created_autostart_test.go` runs the production shape with
a prepared workspace and in-process fakes: the session advances to
WAITING_FOR_INPUT between the launch's reload and its STARTING write, with and
without a step move, and a replay whose snapshot is already stale.
