---
id: "01-batch-schema-probes"
title: "Batch SQLite schema probes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007
  - REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001
acceptance_criteria:
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003.1
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005.5
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.1
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.2
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.3
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.4
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.5
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.6
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.7
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.8
  - AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.1
system_design:
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
  - ../../specs/platform/system-design/runtime-failure-attribution.md
---

# Task 01: Batch SQLite schema probes

## Summary

Implement the measured schema-query optimization through the shared database
schema layer. Retain required-store failure, recovery, and maintenance semantics.
Use TDD after the user requests implementation of this package.

## In scope

- Add `db.ExistingSQLiteTablesContext` for existing names of requested SQLite tables.
- Use it once per SQLite health sweep after the existing writer and reader pings.
- Preserve independent descriptor results and bounded error attribution.
- Add correctness tests for actual query count and real pool behavior.
- Add the exact schema-helper exemption to `internal/db/sqlguard/exemptions.json`.
- Repeat the isolated HTTP workload and record final implementation evidence.

## Out of scope

- Schema migrations, production deployment, new pools, retries, timeouts,
  maintenance scheduling, or changes to the PostgreSQL health path.
- Task/message write optimizations, frontend changes, and permanent experimental logs.

## Acceptance

1. `TestHealthSQLiteCatalogUsesOneSchemaQuery` fails against the original path
   because it observes multiple schema queries. It passes with one catalog query
   after implementation. Count actual executed queries, not source text.
2. Missing, duplicate, and multi-table descriptors retain correct independent
   results. Failed queries, held writer/reader connections, and cancellation fail
   closed. A later complete check recovers without a restart. Maintenance deferral
   preserves both healthy and unhealthy states and their last-check timestamps.
3. Three alternating 35-second comparisons against the original probe use fresh
   copies of the same synthetic fixture and eight 16 KiB HTTP writers. Record
   successful-write latency separately from rejected requests. Preserve probe
   outcomes, read failures, workload counts, and measurement limits. If the final
   build still produces probe deadlines under this workload, investigate before
   marking the work order done.

## Verification

Run from the repository root. The named new tests are implementation deliverables.
The existing control tests are retained.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/persistence/requiredstores -run '^TestHealthSQLiteCatalogUsesOneSchemaQuery$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/db/dialect -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/db ./internal/persistence/requiredstores ./internal/backendapp -run 'Test(ExistingSQLiteTablesContext|HealthSQLiteCatalog|PersistenceContentionFixture|RuntimeHealth|StartupHealth|HealthCheck|ProbeTables|RequiredPersistence|PersistenceMiddleware)' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/persistence/storeconformance -count=1)
bash scripts/lint-go-changed apps/backend/internal/db/schema_tables.go apps/backend/internal/db/schema_tables_test.go apps/backend/internal/persistence/requiredstores/health.go apps/backend/internal/persistence/requiredstores/health_batch_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the query-count regression before production edits and record its expected
failure. The other commands must pass after implementation. If file placement
changes, update the lint command to name the actual changed files before running it.

The experiment is described in [evidence.md](evidence.md). Its replay script
and raw observations are preserved under `.kandev/diagnostics/stress-503/` in
this task workspace. For the final comparison, use the original source from
commit `595df36ed5c1cf02fd90cb972b623ad6edc53df8` as the control and the implemented
source as the candidate. Do not compare two copies of the optimized code.
Record the exact replay command and the two source hashes in the results.
Do not run compilation, race tests, or another owned load test during measurement.

The new helper is explicitly SQLite-only and lives in `internal/db`. The
existing PostgreSQL health branch continues to use the portable point-query
helper. No new PostgreSQL SQL or behavior is introduced by this work order.
Existing database conformance remains a required check.

## Files likely touched

- `apps/backend/internal/db/schema_tables.go` (new helper)
- `apps/backend/internal/db/schema_tables_test.go` (new helper tests)
- `apps/backend/internal/db/sqlguard/exemptions.json` (exact helper symbol and SQLite catalog rule)
- `apps/backend/internal/persistence/requiredstores/health.go`
- `apps/backend/internal/persistence/requiredstores/health_batch_test.go` (new)
- `apps/backend/internal/persistence/requiredstores/health_test.go` only if the
  existing fixtures require focused extensions
- `docs/plans/persistence-probe-batching/plan.md`
- This work order and a separate final implementation evidence file

## Dependencies

None. The earlier message reservation optimization is already in the checkout.

## Risks

- Partial rows followed by an iteration error must fail the batch.
- A successful empty result means all requested tables are missing, not that
  the query failed or that persistence is healthy.
- A table shared by multiple descriptors must not lose any dependent outcome.
- A read-only shortcut can conceal writer checkout starvation and is excluded.
- Throughput can vary on this shared host. Do not turn measurements into strict
  timing assertions or claim historical workload attribution.

## Parallelism

`sequential`

## Inputs

- [Required persistence requirements](../../specs/platform/requirements/postgres-domain-store-parity.md)
- [SQLite schema probe batching design](../../specs/platform/system-design/postgres-domain-store-parity.md#sqlite-schema-probe-batching)
- [Runtime failure attribution design](../../specs/platform/system-design/runtime-failure-attribution.md)
- [Experiment evidence](evidence.md)
- [Maintenance health decision](../../decisions/2026-09-17-maintenance-health-probe-coordination.md)
- [Writer admission decision](../../decisions/2026-10-05-sqlite-writer-transaction-admission.md)
- Backend guidance and the TDD backend testing reference

## Results

Completed on 2026-10-20. The query-count regression failed with 97 queries before
implementation and passed with one afterward. All task-defined code checks passed.
Three final paired HTTP runs produced 3,073 write 503s with the original probe
and zero with the implementation. PostgreSQL integration cases were skipped
because no test DSN was configured.

CI also exposed a test-package import cycle through `internal/db/dialect`.
The helper now compares the SQLite driver name directly, as other shared schema
helpers do. The complete dialect package is included in required verification.

Subsequent CI exposed a timing failure in SSH orphan termination. Its separate
CI remediation and exact validation commands are recorded in the implementation
evidence. Final remote validation remains pending.

See [implementation-evidence.md](implementation-evidence.md) and
[implementation-measurements.json](implementation-measurements.json) for exact
commands, source hashes, results, and measurement limits.
