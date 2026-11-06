---
id: "01-attribute-database-phases"
title: "Attribute database phases"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001
acceptance_criteria:
  - AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.1
  - AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.8
system_design:
  - ../../specs/platform/system-design/runtime-failure-attribution.md
---

# Task 01: Attribute database phases

## Summary

Separate connection acquisition from actual probe execution.
Record phase timings for the actual shared durability run without changing its lifetime or delivery guarantees.

## In scope

- Acquire each probe connection explicitly and release it before the next stage.
- Preserve the existing probe context, deadline, health decisions, and maintenance admission.
- Add the existing repository logger to the durability-barrier constructor and update its two callers.
- Record acquisition, ownership, begin, generation-write, and commit durations in one local run record.
- After connection release, emit one warning for a failed run or a run lasting at least one second.
  Emit it outside the barrier mutex.
- Keep unreached phases absent and keep per-caller metrics unchanged.
- Add the operational explanation through `/docs-maintainer` and `/simple-english`.

## Out of scope

- A generic writer observer, driver wrapper, new metrics, settings, or new goroutines.
- Health hysteresis, larger timeouts, retry changes, extra writer connections, or new durability policy.
- Changes to coalescing, SQL, journal release, payload retention, or caller cancellation semantics.

## Acceptance

1. Held writer and reader connections produce acquisition stages. An acquired connection with a failed ping produces its ping stage.
   Existing readiness, recovery, maintenance, and missing-schema controls retain their results.
2. One actual run supplies accurate reached-stage timings and omits unreached phases.
   Coalesced callers and a canceled initiator cannot duplicate or prematurely settle that run diagnostic.
3. Writer release and covered-ticket publication occur independently of log emission.
   Existing FULL-before-release, rollback, and bad-connection discard controls pass. Fast successful runs remain quiet.

## Regression approach

Extend `TestRuntimeHealthProbeFailureLogsBoundedStageAndRecovers` with acquisition expectations.
That expectation fails before the correction because a held connection reports `writer_ping` or `reader_ping`.
Add a controlled driver failure for a ping after acquisition.
Keep file-backed SQLite coverage for the production pool configuration.

Add `TestDurabilityBarrierRunDiagnosticPhases`, `TestDurabilityBarrierRunDiagnosticCancellation`,
and `TestDurabilityBarrierRunDiagnosticReleaseBeforeLog` beside the existing barrier tests.
Use controlled boundaries rather than host-dependent latency assertions.
Cover acquisition timeout, a failed generation or commit, quiet success, and exactly one record for coalesced callers.
Keep `TestDurabilityBarrierCanceledCallerKeepsRun` and `TestDurabilityBarrierFailureDiscardsConnection`.
The new observer assertions fail because the current barrier emits no run diagnostic.

## Verification

Execute from the repository root:

```bash
(cd apps/backend && go test -race ./internal/db ./internal/persistence/requiredstores -count=1)
(cd apps/backend && go test -race ./internal/task/repository/sqlite -run 'Durability|Barrier' -count=1)
bash scripts/lint-go-changed apps/backend/internal/db/durability_barrier.go apps/backend/internal/db/durability_barrier_diagnostics.go apps/backend/internal/db/durability_barrier_test.go apps/backend/internal/db/sqlite_test.go apps/backend/internal/persistence/requiredstores/health.go apps/backend/internal/persistence/requiredstores/health_test.go apps/backend/internal/task/repository/sqlite/agent_delivery_durability.go
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `apps/backend/internal/db/durability_barrier.go`
- `apps/backend/internal/db/durability_barrier_diagnostics.go` (new, local record and logging only)
- `apps/backend/internal/db/durability_barrier_test.go`
- `apps/backend/internal/db/durability_barrier_diagnostics_test.go` (new)
- `apps/backend/internal/db/sqlite_test.go`
- `apps/backend/internal/persistence/requiredstores/health.go`
- `apps/backend/internal/persistence/requiredstores/health_test.go`
- `apps/backend/internal/persistence/requiredstores/health_batch_test.go`
- `apps/backend/internal/persistence/requiredstores/health_contention_test.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_durability.go`
- `docs/public/operations.md`

## Dependencies

None.

## Risks

- `sql.DB.Conn` includes connection creation, not only queue wait.
- The background run remains live after a caller deadline.
- Slow logging cannot hold the writer or the barrier mutex.
- Other transaction owners remain outside this diagnostic coverage.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/platform/requirements/runtime-failure-attribution.md), criteria .1 and .8.
- [Database phase design](../../specs/platform/system-design/runtime-failure-attribution.md#database-phase-diagnostics).
- Existing barrier cancellation, coalescing, discard, and required-store probe tests.
- Existing delivery durability metrics and repository integration tests.

## Results

Completed on 4 November 2026.

- `Health.pingObserved` now acquires and releases each pool connection
  explicitly and reports `writer_acquire`, `writer_ping`, `reader_acquire`, or
  `reader_ping`. `elapsed_ms` covers only the failing stage.
- `TestRuntimeHealthProbeReportsAcquiredPingStage` wraps the real SQLite driver
  so acquisition succeeds and the ping fails. The held-connection tests now
  expect the acquisition stages.
- `DurabilityBarrier` owns one local run record. It emits one warning for a
  failed or one-second run, after connection release and outside the barrier
  mutex. The run result reaches the initiating caller before that warning is
  emitted, so a blocked log sink cannot replace the outcome with a caller
  deadline. A nil logger keeps the barrier silent, and fast successful runs stay
  quiet.
- Barrier tests cover reached and unreached phases, acquisition timeout,
  cancellation of the initiating caller, coalescing, quiet fast success,
  release before log emission, and run-result delivery before a blocked log
  sink. Callers block in the registration hook until every coalesced caller
  has registered, tests wait explicitly for the asynchronous run record, the
  slow-run threshold is a package-private test seam instead of a sleep, and
  blocking gates release through `sync.Once` cleanup.
- `docs/public/operations.md` describes the stage split, phase timings, and the
  remaining limits of writer attribution.
- `go test -race ./internal/db ./internal/persistence/requiredstores` passed.
  `go test -race ./internal/task/repository/sqlite -run 'Durability|Barrier'`
  passed. `bash scripts/lint-go-changed` reported 0 issues. Public-document
  validation passed.
