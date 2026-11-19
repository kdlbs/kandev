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
callback `Service.persistFullTaskSessionIfCurrent`. A shared model predicate
checks that a valid `workflow_parking` record names the launching session as
its source. When a switch changes the session to `WAITING_FOR_INPUT`, that
record proves the handover. When the state is already `WAITING_FOR_INPUT`, the
record supersedes a launch only if its stamp differs from the stamp in the
launch snapshot. This also makes a later explicit resume valid when it observed
the current stamp.

The SQLite full-row update compares the observed parking stamp with the stored
stamp in the same conditional write. This prevents a switch from recording a
new stamp between launch classification and persistence, including when the
session state stays `WAITING_FOR_INPUT`. The late `LaunchAgent` error path
checks the same ownership before it marks workspace materialization failed.

Consumers treat the error as a handover:

- `Executor.handleEarlyLaunchFailure` and `Executor.persistLaunchState` log it
  at info level and return it without failure bookkeeping.
- `Executor.launchPreparedSession` does not release the task environment
  materialization, which the destination session reuses.
- MCP auto-start does not record an auto-start failure for a superseded launch.
- The WebSocket launch API returns a suppressed activation outcome instead of
  reporting an internal error.
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
