---
created: 2026-08-28
status: done
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
system_design:
  - ../../specs/tasks/system-design/runtime-cleanup.md
legacy_specs: []
---

# Implementation Plan: Non-blocking Stop-Owner Registration

## Overview

Make advisory stop-owner registration non-blocking when another operation owns
the session guard. Add a focused regression test before the production change.

## Scope

### In scope

- Prevent `RegisterExecutionStopOwner` from waiting on a contended session guard.
- Preserve teardown claims and force escalation when the guard is available.
- Treat an overlapping Docker container removal as an idempotent cleanup result.
- Add deterministic concurrency coverage for the contended path.

### Out of scope

- Finding or changing every possible long-running owner of `cancelInFlightGuard`.
- Changing queue dispatch, session state, or durable cleanup scheduling.
- Changing HTTP, WebSocket, frontend, or public documentation contracts.

## Technical approach

Update `RegisterExecutionStopOwner` in
`apps/backend/internal/orchestrator/event_handlers_agent.go`. Use `TryLock` and
release the guard reference when the lock is not available. Keep the existing
claim and force-escalation logic unchanged after successful lock acquisition.

Add a channel-based regression test in
`apps/backend/internal/orchestrator/execution_teardown_ownership_test.go`. Hold
the guard, call registration in another goroutine, and require registration to
return before the guard is released.

Make `Client.RemoveContainer` converge when Docker reports that removal of the
same container is already in progress. The non-blocking registration can allow
the explicit stop and durable cleanup owners to overlap, and Docker's conflict
response represents the desired removal state rather than a failed cleanup.
Cover this response with a focused Docker client test.

## Tests

- `AC-TASKS-RUNTIME-CLEANUP-001.1`: The contended registration test proves that
  an advisory claim cannot stall the explicit runtime stop path.
- Existing stop-owner tests prove that uncontended registration still suppresses
  duplicate orphan cleanup and preserves force escalation.

## Work orders

- [x] [Task 01: Make stop-owner registration non-blocking](task-01-nonblocking-stop-owner-registration.md)

## Verification results

- `go test -tags fts5 -run 'TestRegisterExecutionStopOwner_' ./internal/orchestrator -count=1 -v`: passed, 2 tests.
- `go test -race -tags fts5 -run 'TestRegisterExecutionStopOwner_' ./internal/orchestrator -count=1`: passed, 2 tests.
- CodeRabbit follow-up: registered the test guard cleanup with `t.Cleanup`; the
  focused normal and race tests still pass.
- CI-only `TestPortProxyCapabilityRoundTrip` failure was reproduced locally with
  `-race` and passed, so no unrelated gateway change was made.
- The containers E2E exposed an overlapping Docker teardown race. The exact
  archive-and-remove test reproduced the failure locally; treating Docker's
  "removal already in progress" conflict as convergence fixed the race.

## Risks

- A skipped advisory claim can permit duplicate teardown attempts. Runtime stop
  and durable cleanup already require idempotent behavior for this condition.
- The regression test needs a bounded deadline only to fail a blocking build.
  Channels control the ownership boundary and cleanup order.
