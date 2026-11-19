---
status: current
system: tasks
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
---

# Workflow switch during an in-flight launch

## Purpose and boundaries

The task system owns the session state machine and the profile switch that
parks a source session. This design extends
[session lifecycle](workflow-profile-session-lifecycle.md) for one race: a
launch that is still preparing its workspace or agent when a step move parks
its session. It does not change switch policy, parking, or recovery actions.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.17 | Superseded launch |

## Superseded launch

A task created with an immediate start launches its session in the background
(`Service.PrepareTaskSession`). If the task moves to a step with another
profile before that launch persists its runtime, `parkSessionForProfileSwitch`
writes `workflow_parking` and moves the session from `CREATED` to
`WAITING_FOR_INPUT`. The launch's compare-and-set persistence then sees an
unexpected state. Before this design it returned a generic error, and
`handleSessionLaunchFailure` marked the session and task `FAILED`, which also
stopped the switch.

`executor.SessionStateSwitchedError` (sentinel `ErrSessionStateSwitched`,
helper `IsSessionStateSwitched`) classifies that state. Both persistence paths
return it: `Executor.persistSessionFullRowIfCurrentState` and the orchestrator
callback `Service.persistFullTaskSessionIfCurrent`. The durable proof is a
valid `workflow_parking` record whose `SourceSessionID` is the launching
session while the session is `WAITING_FOR_INPUT`; a state change alone, or a
record naming another session, stays a launch failure.

Consumers treat the error as a handover:

- `Executor.handleEarlyLaunchFailure` and `Executor.persistLaunchState` log it
  at info level and return it without failure bookkeeping.
- `Executor.launchPreparedSession` does not release the task environment
  materialization, which the destination session reuses.
- `Service.handleSessionLaunchFailure` returns before recording a recovery
  block or a failed state, so no failure toast is published.
- The background launch in `PrepareTaskSession` keeps route ownership instead
  of marking the dynamic route `action_required`.
- `IsBenignLaunchTeardownErr` reports it as benign.

The losing agent process is still stopped by the existing cleanup of an
unstarted execution after a persistence error.

## Failure and recovery

If classification misses a switch, behavior is the previous failure path, and
the task keeps its existing manual recovery actions. No new API, event,
database field, or setting is added.

## Observability

Superseded launches log `launch superseded by a concurrent workflow session
switch` at info level, with task and session IDs.
