---
id: P01-01
title: Bound lifecycle waits under session admission
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-TASKS-BOUNDED-QUEUE-ADMISSION-001
acceptance_criteria:
  - AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.5
  - AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.6
  - AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.7
system_design:
  - ../../specs/tasks/system-design/bounded-session-queue-admission.md
---

# Work Order: Bound lifecycle waits under session admission

## Outcome

A long lifecycle-lock holder can no longer keep a session's queue admission
gate held. Admission timeouts name the current holder. A deferred MCP
`move_task` that times out on admission returns a retryable error.

## Scope

1. `refreshFocusedLiveSession`: replace `acquireSessionLifecycleLock` with
   the existing `tryAcquireSessionLifecycleLock`. If the lock is busy, return
   without writing the focus marker.
2. `suspendWorkspaceIdleCandidate`: same swap inside the admission callback.
   If the lock is busy, return nil with `revalidated == false` so the
   candidate is retried on a later tick.
3. `messagequeue`: add a `heldSince time.Time` field to `sessionAdmission`.
   Set it after the gate is acquired and clear it on release, both under
   `admissionMu`. Log one `Warn` with `session_id` and `held_for` when the
   internal admission budget or caller deadline expires while waiting for the
   gate. Preserve the caller deadline error.
4. `mcp/handlers/config_task_handlers.go`: when `SetPendingMove` returns
   `errors.Is(err, messagequeue.ErrSessionAdmissionTimeout)`, return
   `ws.ErrorCodeUnavailable` with "session is busy; retry move_task shortly".
   Other errors keep `INTERNAL_ERROR`.

## Exclusions

The idle-suspended resume branch of `focusTaskSession`, resume-path lifecycle
locking, the agentctl client RWMutex, and finding the long lifecycle-lock
holder are out of scope. See the plan's risks.

## Acceptance

1. With the lifecycle lock held, a focus refresh and an idle revalidation
   each return immediately. A following `WithSessionAdmission` for the same
   session is admitted.
2. An admission timeout logs how long the holder has held the gate. Other callback
   errors and timings are unchanged.
3. A deferred `move_task` under held admission returns `UNAVAILABLE`, and no
   pending move is stored.

## Regression tests (write first; each must fail before the fix)

New test files; do not append to existing large test files.

- `apps/backend/internal/orchestrator/admission_lifecycle_wait_test.go`:
  `TestFocusRefreshReleasesAdmissionWhenLifecycleLockHeld`,
  `TestIdleSuspensionRevalidationReleasesAdmissionWhenLifecycleLockHeld`.
- `apps/backend/internal/orchestrator/messagequeue/session_admission_holder_test.go`:
  `TestSessionAdmissionTimeoutLogsHolder` and
  `TestSessionAdmissionCallerDeadlineLogsHolder` (zap observer); both release
  and boundedly join the holder during cleanup.
- `apps/backend/internal/mcp/handlers/move_task_admission_timeout_test.go`:
  `TestMoveTaskDeferredAdmissionTimeoutIsRetryable`.

## Files

`orchestrator/idle_session_focus.go`, `orchestrator/idle_session_reaper.go`,
`orchestrator/messagequeue/service.go`,
`orchestrator/messagequeue/session_admission_gate.go`,
`mcp/handlers/config_task_handlers.go`, and the new test files above.

## Verification

From the repository root:

```sh
(
cd apps/backend
go test -trimpath -race -count=1 ./internal/orchestrator/messagequeue -run 'TestSessionAdmission'
go test -trimpath -race -count=1 ./internal/orchestrator -run 'TestFocusRefresh|TestIdleSuspension|TestFocusTaskSession|TestIdleSession'
go test -trimpath -race -count=1 ./internal/mcp/handlers -run 'TestMoveTask'
)
make -C apps/backend build
bash scripts/lint-go-changed apps/backend/internal/orchestrator/idle_session_focus.go apps/backend/internal/orchestrator/idle_session_reaper.go apps/backend/internal/orchestrator/messagequeue/service.go apps/backend/internal/mcp/handlers/config_task_handlers.go apps/backend/internal/orchestrator/admission_lifecycle_wait_test.go apps/backend/internal/orchestrator/messagequeue/session_admission_holder_test.go apps/backend/internal/mcp/handlers/move_task_admission_timeout_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Results

Implemented and verified:

- `go test -trimpath -race -count=1 ./internal/orchestrator/messagequeue -run 'TestSessionAdmission'`
- `go test -trimpath -race -count=1 ./internal/orchestrator -run 'TestFocusRefresh|TestIdleSuspension|TestFocusTaskSession|TestIdleSession'`
- `go test -trimpath -race -count=1 ./internal/mcp/handlers -run 'TestMoveTask'`
- `make -C apps/backend build`
- `bash scripts/lint-go-changed apps/backend/internal/orchestrator/idle_session_focus.go apps/backend/internal/orchestrator/idle_session_reaper.go apps/backend/internal/orchestrator/messagequeue/service.go apps/backend/internal/mcp/handlers/config_task_handlers.go apps/backend/internal/orchestrator/admission_lifecycle_wait_test.go apps/backend/internal/orchestrator/messagequeue/session_admission_holder_test.go apps/backend/internal/mcp/handlers/move_task_admission_timeout_test.go`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
