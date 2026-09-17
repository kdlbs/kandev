---
id: "01-runtime-health"
title: "Coordinate runtime health with maintenance"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007
acceptance_criteria:
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.1
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.3
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.4
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.6
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.7
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.8
system_design:
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
---

# Task 01: Coordinate runtime health with maintenance

## Summary

Prevent managed SQLite maintenance from causing a false unhealthy transition.
Keep startup strict and preserve actual runtime failure detection.

## In scope

- Add a periodic-probe boundary that obtains the existing maintenance lease.
- Defer busy SQLite probes without changing tracker state or timestamps.
- Preserve the two-second check budget, 15-second interval, and PostgreSQL path.
- Emit a debug deferral event and release acquired leases on every exit.

## Out of scope

- HTTP allowlist changes, writer-pool sizing, schema changes, and UI changes.
- Suppression of ordinary database failures outside managed maintenance.

## Acceptance

1. A held maintenance lease and writer do not change health or check timestamps.
2. A mixed healthy/unhealthy catalog retains every state during deferral. Startup never skips its check.
3. After release, full checks resume. Missing tables still cause HTTP 503 and recover after repair.

## TDD and test design

First add `TestRuntimeHealthDefersDuringMaintenance` in `health_test.go`.
Use a real SQLite pool, initialized tracker, maintenance lease, and held writer.
Drive the periodic-check boundary with a short test context. Before the fix,
the probe times out and records unhealthy state. Assert that failure first.

Add `TestRuntimeHealthDeferralPreservesMixedStoreStates` with one healthy and
one unhealthy store. Compare complete snapshots, including check timestamps.
Add `TestStartupHealthDoesNotDefer` against strict `Health.Check`.
Add `TestRuntimeHealthResumesAfterMaintenance` for release, fault detection,
and repair. Assert lease release after successful and failed probes, and stop
cancellation. Keep `TestProbeTablesHonorsContextDeadline` unchanged in meaning.

Extend `persistence_middleware_test.go` with
`TestPersistenceMiddlewareRemainsAvailableDuringManagedMaintenance`.
Exercise the periodic boundary, middleware, and real health tracker together.
Use the existing middleware harness, not a synthetic always-healthy stub.
Do not export an API solely for tests. An external-package integration test
can drive `Start` with `SetInterval` and synchronize on observable probe events.
Retain the existing unhealthy response and recovery tests.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race ./internal/persistence/requiredstores ./internal/system/maintenance ./internal/system/database)
(cd apps/backend && go test -race ./internal/backendapp -run 'Test.*(Persistence|RequiredStore|Health|Ready)')
git diff --check
```

The backend Makefile's generic test target is broader than this repair.
Use these focused package commands and verify that the new test names run.

## Files likely touched

- `apps/backend/internal/persistence/requiredstores/health.go`
- `apps/backend/internal/persistence/requiredstores/health_test.go`
- `apps/backend/internal/backendapp/persistence_middleware_test.go`
- `apps/backend/internal/system/maintenance/guard_test.go` if lease assertions need extension

## Dependencies

None. Existing maintenance owners already use the shared pool guard.

## Risks

Do not reverse lock order. The probe takes maintenance admission before database
connections and releases it after all database work. Never hold a writer while
waiting for maintenance admission. Preserve restore/reset shutdown behavior.
The lease defers fault detection only while managed maintenance owns the pool.

## Parallelism

`sequential`

## Inputs

- [Platform requirements](../../specs/platform/requirements/postgres-domain-store-parity.md), required-store health.
- [Platform design](../../specs/platform/system-design/postgres-domain-store-parity.md), runtime health.
- [Decision](../../decisions/2026-09-17-maintenance-health-probe-coordination.md).
- Existing `health_test.go`, `guard_test.go`, and `persistence_middleware_test.go` patterns.

## Results

Implemented periodic SQLite health-probe admission through the shared
maintenance guard. A busy guard now defers without changing tracker state or
timestamps, while startup `Health.Check` remains strict and PostgreSQL keeps
the existing path. Added real SQLite contention coverage for mixed state
preservation, recovery, lease release, and middleware availability.

Verification passed:

- `go test -race ./internal/persistence/requiredstores ./internal/system/maintenance ./internal/system/database`
- `go test -race ./internal/backendapp -run 'Test.*(Persistence|RequiredStore|Health|Ready)'`
- `git diff --check`
