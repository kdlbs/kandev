---
id: "01-owned-reservations"
title: "Own reservations through cleanup"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-AGENTCTL-INSTANCE-STOP-001
acceptance_criteria:
  - AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.1
  - AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.2
  - AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.3
  - AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.4
  - AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.5
  - AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.6
  - AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.7
  - AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.8
system_design:
  - ../../specs/platform/system-design/agentctl-instance-stop.md
---

# Task 01: Own reservations through cleanup

## Summary

Carry immutable leases through allocator, provisional creation, and registered
teardown. Preserve ownership after failed cleanup and retry retained bundles
during shutdown. This work order owns the complete allocator signature migration.

## In scope

- Add `PortLease`, owner indexing, nonzero generations, and matching mutations.
- Migrate all allocation, release, unavailable, and direct test-fixture callers.
- Arm a single cleanup guard after successful bind and balance `abandonWG`.
- Reject create IDs held by provisional bundles, including failed cleanup.
- Retain failed bundles and serialize each retry outside the manager mutex.
- Retry retained bundles once during shutdown after asynchronous cleanup drains.
- Preserve registered stop idempotency, pointer checks, and occupied-port retry.
- Update scoped agentctl guidance with the lease and cleanup invariant.

## Out of scope

- Diagnostic payloads, lifecycle reclamation, and pool metrics.
- New background retry loops, public APIs, migrations, and range changes.
- Issue #3962 worktree or runtime-exit behavior.

## Acceptance

1. One owner has at most one reservation. Stale mutations cannot release or block
   a successor, including a successor with the same owner and a new generation.
2. Every pre-registration exit owns one cleanup operation. Failed listener or
   process cleanup retains the exact bundle and lease for a serialized retry.
3. Shutdown drains cleanup and reports unresolved errors. Successful registered
   stops release once, while concurrent duplicate stops remain benign.

## TDD sequence

First add `TestPortAllocatorReusesOwnerReservation` against the current API.
It must fail because two requests return different ports. Adapt that regression
to value leases during implementation.

Add `TestPortAllocatorStaleReleaseAfterReuse` and
`TestPortAllocatorStaleMarkUnavailable`. Cover absent leases, forged owner,
wrong generation, same-owner reuse, and matching release. Verify both indexes
and future capacity after each operation. Add concurrent unique-owner and
same-owner allocation, deterministic exhaustion, and generation-overflow cases
in `TestPortAllocatorGenerationAndConcurrentExhaustion`.

Use the existing `afterTrackerStart` seam for cancellation. Keep the real
process-manager regression and a one-slot pool. Assert no registered instance,
listener closure, shutdown drain, and lease reuse only after cleanup succeeds.

Add fake-based failure tests through the existing `processManager` interface:
`TestAbandonRetainsLeaseOnCleanupFailure`,
`TestAbandonRetrySameOwnerDoesNotRebind`,
`TestShutdownRetriesFailedAbandon`, and
`TestAbandonListenerCloseFailureRetainsLease`.
Block cleanup deterministically, then assert that another create does not wait
on the manager mutex. Assert that the pending owner cannot bind another listener.
Force one retry failure, then success, and verify exact-once capacity return.

Retain existing registered-stop and bind-conflict regressions. The fork's
release-before-teardown order must not survive this work order.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race -tags fts5 ./internal/agentctl/server/instance ./internal/agentctl/server/api -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/server/instance/port_allocator.go`
- `apps/backend/internal/agentctl/server/instance/port_allocator_test.go` (new)
- `apps/backend/internal/agentctl/server/instance/instance.go`
- `apps/backend/internal/agentctl/server/instance/manager.go`
- `apps/backend/internal/agentctl/server/instance/manager_abandon_test.go`
- `apps/backend/internal/agentctl/server/instance/manager_ports_test.go`
- `apps/backend/internal/agentctl/server/instance/manager_shutdown_test.go`
- `apps/backend/internal/agentctl/AGENTS.md`

The implementing session owns these files and this work order's result record.
Synchronize the parent plan after completion.

## Dependencies

None. Complete this work order before Task 02.

## Risks

- Same-owner allocator idempotency is unsafe without the manager bundle guard.
- Wait-group registration must precede shutdown's drain barrier.
- A stored cleanup error must not outlive successful teardown or hide a later error.
- A failed cleanup must not lose its listener or process-manager reference.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/agentctl-instance-stop.md), criteria .1 through .8.
- [System design](../../specs/platform/system-design/agentctl-instance-stop.md), owned reservations and provisional cleanup.
- [Plan](plan.md), current-head evidence and preserved fork differences.
- Existing manager abandon, bind, and shutdown test fixtures.

## Results

Red/green evidence:

- `TestPortAllocatorReusesOwnerReservation`, `TestPortAllocatorStaleReleaseAfterReuse`, and `TestPortAllocatorStaleMarkUnavailable` failed against the original allocator for the expected ownership defects.
- The new allocator and provisional-cleanup regressions pass, including stale same-owner generation, generation exhaustion, concurrent exhaustion, failed cleanup retention, same-owner create rejection, and shutdown retry.

Verification passed:

- `(cd apps/backend && go test -race -tags fts5 ./internal/agentctl/server/instance ./internal/agentctl/server/api -count=1)`
- `gofmt` on changed Go files.
