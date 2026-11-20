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
| AC-TASKS-TRANSIENT-LAUNCH-RETRY-001.5 | Eligibility, Retry worker |

## Eligibility

An early interactive launch failure is eligible after
`recordSessionLaunchFailure` moves both the session and task to `FAILED`. An
executor-owned launch failure is eligible after the executor records the
session failure and task failure. Workspace-only preparation uses the same
failure bookkeeping but does not request an agent start and cannot schedule a
retry.

`isTransientLaunchFailure` accepts `context.DeadlineExceeded` and a closed list
of sanitized messages: `agentctl not ready`, `context deadline exceeded`,
`failed to launch workspace`, and `workspace execution is not ready`. A
cancelled launch is never transient, even when the cancellation surfaced as
`agentctl not ready: context canceled`. Office
tasks, automation-origin tasks, and sessions whose metadata holds a
`workflow_parking` record are skipped.

## Once per session

The worker is registered before the claim, so a service that is stopping
refuses the retry without consuming the session's attempt.
`claimTransientLaunchRetry` stores the attempt time per session in
`Service.transientLaunchRetries` and refuses a second claim for one hour
(`transientLaunchRetryTTL`). Each claim sweeps expired entries, so the map stays
bounded and a failure long after the first one can still recover.

## Retry worker

The worker captures the failed session's start-attempt ID, error stamp, and
updated time. It checks that identity after its two-second delay and again at
resume admission. It stops if a later attempt replaced the failure. The worker
passes automatic origin to the existing session ceiling. If capacity defers the
resume, the replay record keeps the failure identity and checks it again.

The worker runs under a service-owned context (`launchRetryCtx`).
`beginTransientLaunchRetryWorker` registers it unless the service is stopping.
`Service.Stop` closes the retries (`closeTransientLaunchRetries`) while it
holds the service mutex, then cancels and waits for that closed generation
(`awaitTransientLaunchRetries`). `Service.Start` opens a new generation under
the same mutex, so a Start that races a finishing Stop keeps its retries. The
retry deadline remains attached to the resume attempt, so shutdown can cancel a
launch already in progress. A second failure
is logged and leaves the task `FAILED`. The automatic retry starts the agent
synchronously. The worker therefore keeps its deadline until startup settles,
while the resume attempt still accepts shutdown cancellation.

## Failure and recovery

A failed or skipped retry leaves the previous state and recovery actions. No
new API, event, database field, or setting is added.

## Observability

The worker logs scheduling, a refused second attempt, success, and failure,
with task and session IDs.
