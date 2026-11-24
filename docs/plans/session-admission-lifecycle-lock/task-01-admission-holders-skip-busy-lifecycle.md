---
id: "session-admission-lifecycle-lock-01"
title: "Admission holders skip a busy lifecycle"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-BOUNDED-QUEUE-ADMISSION-001
acceptance_criteria:
  - AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.5
system_design:
  - ../../specs/tasks/system-design/bounded-session-queue-admission.md
---

# Work Order 01: Admission holders skip a busy lifecycle

## Outcome

Code that holds session admission never blocks on the session lifecycle lock.
Focus refresh of a live session and idle-suspension revalidation try the lock
without waiting. If it is busy, they skip this attempt and release admission.

## Scope

- `refreshFocusedLiveSession` (`idle_session_focus.go`): replace the blocking
  `acquireSessionLifecycleLock` with a non-blocking acquire. When the lock is
  busy, return without persisting the focus marker and restart the in-memory
  idle interval.
- `suspendWorkspaceIdleCandidate` (`idle_session_reaper.go`): use the same
  non-blocking acquire inside the admission callback. When the lock is busy,
  return with `revalidated == false`, so no suspension happens and the candidate
  is retried on a later scan.
- Reuse the existing `tryAcquireSessionLifecycleLock` in `service.go`
  (`sync.Mutex.TryLock`). Its comment now states the invariant: callers that
  hold queue admission must not block on the lifecycle lock.

## Exclusions

The idle-suspended focus resume (`resumeFocusedIdleSuspension`) is unchanged
here; work order 04 moves it out of admission. No change to admission, the reaper schedule, or focus
eligibility.

## Acceptance

1. With the session lifecycle lock held by the test, a live `WAITING_FOR_INPUT`
   session's `focusTaskSession` returns promptly, without persisting
   `idle_parking_focus`. A following `messageQueue.WithSessionAdmission` with a
   1 s budget succeeds.
2. With the lifecycle lock held, reaper revalidation of an eligible candidate
   returns without suspending, and admission is free afterwards.
3. With the lock free, existing focus refresh and suspension behavior is
   unchanged (existing `idle_session_focus*` and `idle_session_reaper` tests
   pass).

## Regression tests (fail before the fix)

`apps/backend/internal/orchestrator/idle_session_admission_lock_order_test.go`:

- `TestFocusDuringBusyLifecycleKeepsAdmissionFree` and
  `TestFocusDuringBusyLifecycleDefersIdleSuspension`: hold the lifecycle lock,
  run `focusTaskSession`, and confirm the call returns while session admission
  stays free.
- `TestIdleSuspensionRevalidationSkipsBusyLifecycle`: same pattern through
  `suspendWorkspaceIdleCandidate`, reusing the fixtures in
  `idle_session_reaper_test.go`.

## Files

`apps/backend/internal/orchestrator/service.go`,
`idle_session_focus.go`, `idle_session_reaper.go`, and the new test file.

## Verification

```text
(cd apps/backend && go test -trimpath -race -count=1 ./internal/orchestrator/ -run 'TestFocusDuringBusyLifecycleKeepsAdmissionFree|TestFocusDuringBusyLifecycleDefersIdleSuspension|TestIdleSuspensionRevalidationSkipsBusyLifecycle|TestFocusTaskSession|TestIdleSession')
bash scripts/lint-go-changed apps/backend/internal/orchestrator/service.go apps/backend/internal/orchestrator/idle_session_focus.go apps/backend/internal/orchestrator/idle_session_reaper.go apps/backend/internal/orchestrator/idle_session_admission_lock_order_test.go
```

## Results

Implemented. `refreshFocusedLiveSession` and `suspendWorkspaceIdleCandidate`
use `tryAcquireSessionLifecycleLock` and skip when it is busy. Both regression
tests failed first (focus and revalidation each blocked for the 2 s budget) and
now pass, including their positive controls. The focus, idle-session,
idle-parking, and idle-focus suites pass under `-race`. Work order 04 later
made a focus during a busy lifecycle lock restart the in-memory idle interval,
and renamed the focus test `TestFocusDuringBusyLifecycleKeepsAdmissionFree`.
