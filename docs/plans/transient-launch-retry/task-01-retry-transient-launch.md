---
id: "01-retry-transient-launch"
title: "Retry a transient launch failure once"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TRANSIENT-LAUNCH-RETRY-001
acceptance_criteria:
  - AC-TASKS-TRANSIENT-LAUNCH-RETRY-001.1
  - AC-TASKS-TRANSIENT-LAUNCH-RETRY-001.2
  - AC-TASKS-TRANSIENT-LAUNCH-RETRY-001.3
  - AC-TASKS-TRANSIENT-LAUNCH-RETRY-001.4
system_design:
  - ../../specs/tasks/system-design/transient-launch-retry.md
---

# Task 01: Retry a transient launch failure once

## Summary

A transient launch failure that leaves the session and the task `FAILED` gets
one delayed retry through runtime retry recovery; everything else keeps the
current failure path.

## In scope

- `internal/orchestrator/launch_retry.go`: `isTransientLaunchFailure`,
  `scheduleTransientLaunchRetry`, `claimTransientLaunchRetry`, the worker, and
  `stopTransientLaunchRetries`.
- `Service` fields for the claim map and the worker lifecycle, and the call in
  `Service.Stop`.
- `recordSessionLaunchFailure` reports whether the session and the task were
  marked failed; `handleSessionLaunchFailure` schedules the retry only then.

## Out of scope

- Failure classification for display, recovery actions, and the frontend.

## Acceptance

1. A transient failure is retried once through `RecoverSession` with the
   resume token intact, and a second failure does not retry again.
2. A non-transient failure, an Office or automation task, and a parked
   session do not retry.
3. A retry pending when `Stop` begins does not relaunch.

## Verification

```bash
# from the repository root
(cd apps/backend && go test -trimpath -tags fts5 -count=1 \
  -run 'TransientLaunch|ScheduleTransientLaunchRetry|ClaimTransientLaunchRetry|StopTransientLaunchRetries' \
  ./internal/orchestrator/)
(cd apps/backend && go test -trimpath -tags fts5 -count=1 ./internal/orchestrator/)
bash scripts/lint-go-changed $(git diff --name-only origin/main -- 'apps/backend/*.go')
```

## Likely files

- `apps/backend/internal/orchestrator/launch_retry.go` (new)
- `apps/backend/internal/orchestrator/launch_retry_test.go` (new)
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/task_operations.go`

## Results

- The new tests in `launch_retry_test.go` pass: transient classification,
  once-only retry, a real `RecoverSession` retry that keeps the resume token,
  the claim TTL and sweep, no retry for a non-transient failure, and no
  relaunch after `Stop`. They need the new functions, so they do not compile
  against the previous package.
- The full `internal/orchestrator` run on the Windows development host fails
  the same tests as before the change (two passthrough-attachment tests, and
  `TestCompleteTurnsExceptReportsIterationExhaustion` intermittently).
