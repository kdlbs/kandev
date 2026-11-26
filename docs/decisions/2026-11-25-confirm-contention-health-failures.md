# ADR-2026-11-25-confirm-contention-health-failures: Confirm contention-only health failures before failing closed

**Status:** accepted
**Date:** 2026-11-25
**Area:** backend

## Context

SQLite has one writer connection. Each periodic required-store probe pings that
writer and reads the schema catalog through it within a two-second deadline.
One failed probe makes every required store unhealthy. Stateful HTTP requests
then return `503 persistence_unavailable` until the next probe, which runs
fifteen seconds later.

On November 25, 2026, persistence became unhealthy three times for thirteen
seconds each. In every incident, the only probe failure was
`deadline_exceeded` with one writer connection in use. Nearby durability-barrier
commits spent 0.6 to 2.1 seconds in `COMMIT`, with negligible write time, while
the host disk reported sustained I/O stall. A short writer stall therefore
became a thirteen-second application-wide outage. Every unrelated read also
failed, including sidebar pages that use separate reader connections.

The [maintenance coordination decision](2026-09-17-maintenance-health-probe-coordination.md)
deliberately retained fail-fast behavior for ordinary writer contention without
production evidence to justify a policy change. That evidence now exists.

## Decision

A periodic SQLite probe failure confirms itself before it changes a healthy
store to unhealthy, when every failure in that probe is a deadline expiry.
The health loop does not record that first result. It runs one confirmation
probe three seconds later with the same two-second deadline and maintenance
lease rules. It records the confirmation result, whether that result succeeds
or fails.

Every other failure class records immediately, including missing tables,
driver errors, missing pools, and cancellation. A store that is already
unhealthy records each probe immediately. Startup checks remain strict.
PostgreSQL probes are unchanged. An unconfirmed deadline emits one bounded
warning with the existing sanitized pool diagnostics.

## Consequences

A writer stall shorter than about five seconds no longer turns into an
application-wide 503 window. A real persistent outage that presents only as
deadline expiry is detected up to five seconds later than before. Actual
errors are detected as quickly as before. Readiness, caller errors, and
recovery semantics are unchanged once a failure is recorded.

The probe still exercises the writer. It never records a timeout as success.
This decision does not reduce writer stall duration. Disk pressure and long
writer holders remain separate performance work.

## Alternatives Considered

- A longer probe deadline still turns any stall that exceeds it into a full
  fifteen-second outage, and it delays every real failure.
- Reader-only probes lose the existing writer-health guarantee.
- Keeping the current policy preserves repeated global 503 windows during
  ordinary disk pressure.
- Requiring several consecutive failed intervals delays real outage detection
  by thirty seconds or more.
