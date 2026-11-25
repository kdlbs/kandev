---
id: "session-admission-lifecycle-lock-03"
title: "Retryable deferred-move timeout"
status: done
wave: 3
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-BOUNDED-QUEUE-ADMISSION-001
acceptance_criteria:
  - AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.7
system_design:
  - ../../specs/tasks/system-design/bounded-session-queue-admission.md
---

# Work Order 03: Retryable deferred-move timeout

## Outcome

When an agent's `move_task` cannot record its deferred move because session
admission timed out, the agent receives a retryable `UNAVAILABLE` error instead
of `INTERNAL_ERROR: failed to persist deferred move`.

## Scope

- `config_task_handlers.go`, the deferred-move branch: when `SetPendingMove`
  returns an error matching `errors.Is(err, messagequeue.ErrSessionAdmissionTimeout)`,
  return `ws.ErrorCodeUnavailable` with a message saying the session is busy
  and the move can be retried shortly. All other errors keep the current
  `INTERNAL_ERROR` response.

## Exclusions

No retry loop inside the handler and no change to `SetPendingMove`.

## Acceptance

1. An admission timeout from `SetPendingMove` yields an `UNAVAILABLE` error
   whose message says a retry can succeed.
2. Any other `SetPendingMove` error still yields `INTERNAL_ERROR`.

## Regression test (fails before the fix)

In `apps/backend/internal/mcp/handlers/move_task_admission_timeout_test.go`,
`TestMoveTaskDeferredAdmissionTimeoutIsRetryable` checks that an admission
timeout returns `UNAVAILABLE` with a retry hint, while another persistence
error remains `INTERNAL_ERROR`.

## Files

`apps/backend/internal/mcp/handlers/config_task_handlers.go` and
`apps/backend/internal/mcp/handlers/move_task_admission_timeout_test.go`.

## Verification

```text
(cd apps/backend && go test -trimpath -count=1 ./internal/mcp/handlers/ -run 'TestMoveTask')
bash scripts/lint-go-changed apps/backend/internal/mcp/handlers/config_task_handlers.go apps/backend/internal/mcp/handlers/move_task_admission_timeout_test.go
```

## Results

Implemented. An admission timeout from `SetPendingMove` returns
`UNAVAILABLE` with a retry hint; other errors keep `INTERNAL_ERROR`. The
regression test covers both outcomes and lives in
`move_task_admission_timeout_test.go`.
