---
status: current
system: tasks
requirements:
  - REQ-TASKS-TRANSIENT-LAUNCH-RETRY-001
---

# Transient launch retry

## Purpose and boundaries

The orchestrator owns interactive launch failures for task sessions. This
design adds one automatic retry on that failure path and reuses the existing
runtime retry recovery. It does not change how a failure is classified for
display, the recovery actions, or the dynamic routing fallback.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| AC-TASKS-TRANSIENT-LAUNCH-RETRY-001.1, 001.2 | Eligibility, Retry worker |
| AC-TASKS-TRANSIENT-LAUNCH-RETRY-001.3 | Once per session |
| AC-TASKS-TRANSIENT-LAUNCH-RETRY-001.4 | Retry worker |

## Eligibility

`Service.handleSessionLaunchFailure` records the failure through
`recordSessionLaunchFailure`, which reports whether the session and the task
were both moved to `FAILED`. Only then does it call
`scheduleTransientLaunchRetry` (`internal/orchestrator/launch_retry.go`).

`isTransientLaunchFailure` accepts `context.DeadlineExceeded` and a closed list
of sanitized messages: `agentctl not ready`, `context deadline exceeded`,
`failed to launch workspace`, and `workspace execution is not ready`. Office
tasks, automation-origin tasks, and sessions whose metadata holds a
`workflow_parking` record are skipped.

## Once per session

`claimTransientLaunchRetry` stores the attempt time per session in
`Service.transientLaunchRetries` and refuses a second claim for one hour
(`transientLaunchRetryTTL`). Each claim sweeps expired entries, so the map stays
bounded and a failure long after the first one can still recover.

## Retry worker

The worker runs under a service-owned context (`launchRetryCtx`).
`beginTransientLaunchRetryWorker` registers it in `launchRetries` unless the
service is stopping; `Service.Stop` calls `stopTransientLaunchRetries`, which
cancels the context and waits for the workers with the shared shutdown
timeout. After a two-second delay the worker re-reads the task and the session
and gives up unless both are still `FAILED`. It then calls `RecoverSession`
with `recoveryActionRuntimeRetry` under a five-minute timeout. A second failure
is logged and leaves the task `FAILED`.

## Failure and recovery

A failed or skipped retry leaves the previous state and recovery actions. No
new API, event, database field, or setting is added.

## Observability

The worker logs scheduling, a refused second attempt, success, and failure,
with task and session IDs.
