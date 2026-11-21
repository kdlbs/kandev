---
id: "02-start-capture-budget"
title: "Persist captured start endpoints within the admission budget"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TURN-CHANGES-003
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-003.1
  - AC-TASKS-TURN-CHANGES-003.6
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
---

# Task 02: Persist captured start endpoints within the admission budget

## Summary

Start capture runs checkouts with a concurrency of 2 per execution, including
overlapping admissions, in a capture deadline that ends 6 seconds before the
admission deadline. The reserve exceeds SQLite's 5-second writer busy timeout.
Endpoint acceptance uses the remaining reserve, so checkouts captured in time
are persisted and only the missed checkouts are unavailable. A persistence
failure reports the capture failures that caused it.

## In scope

- `task/changes/coordinator.go` `captureStart`:
  - Add `turnChangeStartPersistenceReserve = 6 * time.Second` and
    `turnChangeStartCaptureConcurrency = 2`.
  - Derive the capture context from the admission context. When it has a
    deadline, end capture at `deadline - reserve`. Otherwise use it unchanged.
  - Share a keyed capture limiter by `ExecutionID` across overlapping admissions
    so the executor never has more than two start captures in flight. Release
    unused limiter entries after the final waiter or capture completes.
  - Extract the per-checkout body into a helper that returns
    `(row, captureErr)`. Run it with a bounded worker group (for example,
    `errgroup` with `SetLimit`, or a semaphore channel; the helper never returns
    an error to the group). Store results by checkout index so row and error
    order match `admission.Checkouts`.
  - Call `AcceptTurnChangeSetStart` with the admission context.
  - On acceptance error, return
    `errors.Join(fmt.Errorf("persist turn change start endpoints: %w", err), errors.Join(captureFailures...))`.
- Move the capture loop to a new file (for example,
  `coordinator_start_capture.go`) if `coordinator.go` would exceed the
  function or file limits.

## Out of scope

Absent-scope registration for repositories added to a running task,
leftover-ref sweeping, changing the 10-second admission budget, moving capture
off the prompt path, agentctl capture internals, and UI.

## Acceptance

1. With a fake client where checkout 1 captures promptly and checkout 2 blocks
   past the capture deadline, admission returns within the admission budget.
   The stored set has `start_accepted=1`, checkout 1 `pending` with its start
   OIDs, and checkout 2 `unavailable`. The returned error names checkout 2.
2. Four checkouts that each take time T complete in about 2T, not 4T: the fake
   observes a maximum of 2 concurrent captures and never more. Row order matches
   the checkout order.
3. Overlapping admissions for one execution never exceed 2 concurrent captures.
4. When `AcceptTurnChangeSetStart` fails, the returned error contains both the
   persistence error and each capture failure. Existing start-failure settlement
   (`coordinator_start_failure_test.go`) and lifecycle admission-before-dispatch
   tests still pass.

## Verification

Write the regressions first. Acceptance 1 must fail before the fix with
`persist turn change start endpoints: context deadline exceeded` and no
repository rows. Use a short admission timeout seam in tests. Do not wait
10 real seconds: make the admission timeout and reserve package variables or
pass them through an unexported field set by the test. The default reserve
exceeds SQLite's five-second writer busy timeout; tests may use a shorter
explicit reserve. From the repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/task/changes -run 'CoordinatorStart|TurnChange' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/changes ./internal/agent/runtime/lifecycle -run 'Coordinator|TurnChange|PromptAdmissionCapture|TerminalCapture' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/changes ./internal/agent/runtime/lifecycle)
bash scripts/lint-go-changed \
  apps/backend/internal/task/changes/coordinator.go \
  apps/backend/internal/task/changes/coordinator_fakes_test.go \
  apps/backend/internal/task/changes/coordinator_start_budget_test.go \
  apps/backend/internal/task/changes/coordinator_start_capture.go \
  apps/backend/internal/task/changes/start_capture_limiter.go
```

## Files likely touched

- `apps/backend/internal/task/changes/coordinator.go`
- `apps/backend/internal/task/changes/coordinator_fakes_test.go`
- `apps/backend/internal/task/changes/coordinator_start_capture.go`
- `apps/backend/internal/task/changes/start_capture_limiter.go` (new)
- `apps/backend/internal/task/changes/coordinator_start_budget_test.go`

## Dependencies

None.

## Risks

The fake client must be safe for concurrent use. Run the package under `-race`.
Keep capture-failure rows identical to the sequential path (reason via
`checkpointReason`). Do not start a capture after the capture context is done.
The shared limiter must include queue time in the capture deadline and discard
per-execution entries after all waiters and captures release them.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md), #5399 root cause and reporter's ref evidence.
- [Turn changed files design](../../specs/tasks/system-design/turn-changed-files.md), Start steps and Bounds.
- [Prior start-failure repair](../turn-change-start-failure/plan.md).

## Results

Implemented and verified:

```bash
(cd apps/backend && go test -trimpath ./internal/task/changes -run 'CoordinatorStart|TurnChange' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/changes ./internal/agent/runtime/lifecycle -run 'Coordinator|TurnChange|PromptAdmissionCapture|TerminalCapture' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/changes ./internal/agent/runtime/lifecycle)
```

PR-fixup regression checks also passed after sharing the capture limit across
overlapping admissions and increasing the persistence reserve:

```bash
(cd apps/backend && go test -trimpath ./internal/task/changes -run 'TestCoordinatorStartCapture(ConcurrencyIsSharedPerExecution|PersistenceReserveExceedsSQLiteBusyTimeout|CapturesCheckoutsConcurrently)$' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/changes -run 'CoordinatorStart|TestResumeUnfinishedMixedRepositorySettlesOnlyMissingEnd' -count=1)
```

Changed-Go lint passed across all PR Go files, as listed in [plan.md](plan.md).
