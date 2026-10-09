# ADR-2026-09-17-maintenance-health-probe-coordination: Coordinate runtime health probes with maintenance

**Status:** accepted
**Date:** 2026-09-17
**Area:** backend

## Context

SQLite has one writer connection. Vacuum holds that connection while runtime
health probes need it for pings and schema checks. Probe deadlines therefore
mark every required store unhealthy during successful maintenance.

The shared pool already has a maintenance admission guard. Required persistence
must still fail startup and retain known runtime failures.

## Decision

Periodic SQLite probes participate in the existing maintenance guard. They try
admission without waiting and retain the lease throughout the bounded probe.
A busy guard defers the probe without changing store states or check timestamps.
Startup checks remain strict. PostgreSQL probes remain unchanged.

The next scheduled tick resumes full probing after maintenance releases its lease.
Deferred probes emit a bounded debug event. They never manufacture healthy state.
Destructive SQLite restore and factory-reset operations use the existing
unhealthy state to fail closed before quiescing or replacing data. Their
restart-required result keeps database-backed work disabled until a fresh
process initializes the database again.

## Consequences

Managed maintenance no longer causes false global persistence failures.
The guard prevents a race between checking maintenance state and taking the writer.
Actual faults that arise during maintenance can remain undetected until lease release.
Diagnostics retain the last completed check time, and previous failures remain visible.
This does not guarantee that writes complete promptly during vacuum.

The [required persistence decision](2026-09-05-required-internal-persistence.md)
still governs startup and actual runtime failures.

## Alternatives Considered

- Longer timeouts cannot bound vacuum duration and retain the false-failure race.
- Reader-only checks lose the existing writer-health guarantee.
- Skipping every probe while any job runs confuses job tracking with database ownership.
- Bypassing HTTP health middleware hides actual failures and does not repair readiness.
- A separate maintenance flag duplicates the existing admission state.
