---
id: "01-nonblocking-stop-owner-registration"
title: "Make stop-owner registration non-blocking"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
acceptance_criteria:
  - AC-TASKS-RUNTIME-CLEANUP-001.1
system_design:
  - ../../specs/tasks/system-design/runtime-cleanup.md
---

# Task 01: Make Stop-Owner Registration Non-blocking

## Summary

Prevent advisory teardown registration from waiting on a contended session
guard. Preserve existing registration behavior when the guard is available.

## In scope

- Add a failing concurrency regression test for contended registration.
- Replace the blocking registration lock with a non-blocking lock attempt.
- Preserve reference release, teardown claims, and force escalation.
- Make overlapping Docker removal requests converge successfully.

## Out of scope

- Change guard ownership in queue or cancellation operations.
- Change runtime stop scheduling or durable cleanup retry policy.
- Add browser or end-to-end coverage.

## Acceptance

- Contended stop-owner registration returns before the guard owner releases the
  lock.
- Uncontended registration still records the exact teardown claim.
- A repeated force registration still upgrades a graceful claim.

## Verification

```bash
go test -tags fts5 -run 'TestRegisterExecutionStopOwner_' ./internal/orchestrator -count=1 -v
go test -race -tags fts5 -run 'TestRegisterExecutionStopOwner_' ./internal/orchestrator -count=1
```

Run these commands from `apps/backend`.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/execution_teardown_ownership_test.go`
- `apps/backend/internal/agent/docker/client.go`
- `apps/backend/internal/agent/docker/storage_test.go`

## Dependencies

None.

## Risks

- A skipped claim can permit an idempotent duplicate teardown attempt.

## Parallelism

`sequential`

## Inputs

- `AC-TASKS-RUNTIME-CLEANUP-001.1`
- Task Runtime Cleanup System Design sections about stop-owner registration.
- Existing `RegisterExecutionStopOwner` claim and force-escalation tests.

## Results

- Added `TestRegisterExecutionStopOwner_ContendedGuardDoesNotBlock` as the
  regression test for `AC-TASKS-RUNTIME-CLEANUP-001.1`.
- Replaced the blocking registration lock with `TryLock`. A contended call now
  releases its guard reference and returns without a teardown claim.
- The focused test failed before the production change and passed after it.
- Both work-order verification commands passed. The race command reported no
  data races.
- Applied the CodeRabbit follow-up by registering guard cleanup with `t.Cleanup`.
- Reproduced the unrelated CI failure in `TestPortProxyCapabilityRoundTrip`;
  the local `-race` run passed, so the gateway code remains unchanged.
- Reproduced the containers E2E archive cleanup failure with overlapping Docker
  removal owners. `Client.RemoveContainer` now treats Docker's conflict for an
  already-in-progress removal as successful convergence, with focused coverage.
