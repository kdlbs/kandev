---
created: 2026-10-19
status: done
requirements:
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007
  - REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001
system_design:
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
  - ../../specs/platform/system-design/runtime-failure-attribution.md
legacy_specs: []
---

# Reduce SQLite health probe contention

## Overview

Batch SQLite required-table checks so one health sweep does not repeatedly
queue behind application writes. Preserve the existing readiness policy.
One sequential work order implements the measured optimization and its
regression coverage.

Platform owns the shared database and readiness contract. Existing required-store
requirements define the behavior. This package changes the technical query
strategy, not the product contract, so it does not add a duplicate requirement.

## Evidence and confirmed cause

The October 19 incident produced 442 API 503 responses during a 74-second
persistence outage. Two table probes exceeded the shared two-second deadline.
The writer connection was occupied while four reader connections were idle.
The exact historical writer operation remains unknown.

On a fresh isolated backend, eight concurrent HTTP clients updating separate
16 KiB task descriptions reproduced `table_probe` deadline failures and global
API rejection. One client and the idle control did not fail. The checkout's
catalog contains 97 table declarations for 96 distinct table names.
The current probe returns to the writer pool for each table query.

A disposable build overlay batches the table lookup while preserving both
pool pings, the deadline, interval, maintenance lease, and stateful middleware.
See [evidence.md](evidence.md) and [measurements.json](measurements.json) for
completed prototype comparisons and their limits. The design experiment used
no permanent production edits. The approved implementation and its verification
are recorded in [implementation-evidence.md](implementation-evidence.md).

## Scope

### In scope

- A SQLite-only, context-aware schema helper in `internal/db` for the catalog lookup.
- A SQLite-specific batch path in `requiredstores.Health.check`.
- Independent descriptor outcomes from the returned table set.
- Deterministic correctness and query-count regression tests.
- A repeated isolated HTTP comparison with the same synthetic data and workload.

### Out of scope

- Live load, production data copies, deployment, or a live restart.
- More database connections, reader-only health, longer deadlines, retries,
  new health states, or middleware exceptions.
- PostgreSQL probe redesign, schema changes, retention scheduling, and
  optimizations of task or message writes.
- UI, public API, configuration, localization, or agent behavior changes.

## Technical approach

Follow [SQLite schema probe batching](../../specs/platform/system-design/postgres-domain-store-parity.md#sqlite-schema-probe-batching).
Keep `pingObserved`, `checkRuntimeDetailed`, tracker publication, and the
periodic loop as the existing owners of their boundaries.

Deduplicate the descriptor table names and query their presence once through
the writer. Keep SQL in `internal/db/schema.go` or a focused sibling.
Bind every table name. The helper must distinguish an empty result from a query,
scan, iteration, or cancellation error. Close rows on every exit.

For a successful query, compare each descriptor against the same result set.
Report the first missing table for that descriptor, while continuing to check
all other descriptors. Query failure marks each affected descriptor unhealthy.
Capture the existing bounded diagnostic at the batch boundary, once per sweep.
Preserve the direct `probeTables` behavior where existing tests or the unchanged
PostgreSQL path still need it.

| Path | Intended result | Verification |
| --- | --- | --- |
| SQLite, short queued writes | One schema lookup avoids repeated checkout | Query-count control and isolated HTTP comparison |
| SQLite, missing required table | Only dependent descriptors fail after a successful lookup | Mixed healthy/missing fixture |
| SQLite, shared table name | Every dependent descriptor receives the same presence result | Duplicate-name fixture |
| Writer or reader connection held past deadline | Existing failure and recovery behavior | `TestPersistenceContentionFixture` |
| Managed maintenance | Preserve all previous states and timestamps while deferred | Existing runtime-health tests |
| PostgreSQL | Existing probe, current schema, and errors | Existing health and store-conformance checks |

The prototype directly embeds SQLite SQL in the disposable health source for
measurement. The implementation must place that SQL in the shared schema layer.
The prototype is not a ready production patch.

## Tests

- AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003.1: a catalog-driven fixture proves every descriptor participates,
  including a duplicate name and a descriptor with several tables.
- AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005.5: the shared schema helper owns the dialect-specific query.
- AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.1 through .5: missing-table, failed-query, HTTP rejection, liveness,
  and subsequent recovery checks.
- AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.6 through .8: retain the maintenance deferral and resumption tests.
- AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.1: retain one bounded failure event with
  the existing stage, error class, elapsed duration, and pool statistics.

Use a test-local SQL driver wrapper or query observer to count actual catalog
queries. Do not assert source strings or machine-dependent timing limits.
Exercise separate factory-created writer and reader pools for contention.
The HTTP stress run supplies end-to-end evidence without a browser-only test.

## Work orders

- [x] [Task 01: Batch SQLite schema probes](task-01-batch-schema-probes.md)

## Verification results

Design validation passed: catalog, specification lint, 36 linter tests,
documentation coverage preflight, and whitespace checks. Experimental measurements
and fault controls are recorded in [evidence.md](evidence.md).
Implementation, regression tests, race checks, SQL guard, store conformance,
changed-file lint, and final stress comparisons passed. The original probe
produced 3,073 write 503s and 192 workspace-read 503s; the implementation produced
zero across three paired runs. PostgreSQL integration cases were skipped without
a test DSN. See [implementation-evidence.md](implementation-evidence.md) for
commands, results, limits, and cleanup.

CI remediation also covers a delayed SIGKILL delivery failure in the existing
SSH orphan-stop contract. The implementation evidence records its deterministic
regression and validation separately. Later CI remediation limits held-diff
instrumentation to its controlled Git command and preserves logical terminal
lines in the browser-test reader. The evidence records the reproduced timeout,
16 focused unit tests, 11 browser checks, and separate pressure observations.
Final remote checks remain pending.

## Risks

- One batched-query failure affects all descriptors because no complete inventory
  is available. A missing table after a successful query must remain scoped.
- Query scan failures must never be mistaken for absent tables or a healthy database.
- Heavy writes can still exhaust the deadline at either pool ping or the single
  inventory query. This change does not promise unlimited write capacity.
- Shared-host timing varies. A synthetic task-update workload does not prove
  that task updates caused the historical incident.
- The live instance runs an older build. Its relevant health mechanism matches
  this checkout, but this experiment is not a full replay of the live workload.

## Documentation and delivery

Only the internal design and this plan change. The public-doc audit found no
new command, API, setting, or user workflow requiring public documentation.
The existing completed contention and message-write packages retain their
historical results. This follow-up does not reopen or rewrite them.
The user approved implementation after the design checkpoint. Keep the local
patch available for review. Production deployment is outside this work order.
