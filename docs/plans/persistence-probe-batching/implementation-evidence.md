# Implementation evidence

Implementation authorized after review of the design package. Tests and
measurements run on October 20, 2026, against the working tree based on
`595df36ed5c1cf02fd90cb972b623ad6edc53df8`.

## Change

`db.ExistingSQLiteTablesContext` deduplicates requested names, binds them into
one SQLite catalog query, and returns a complete table inventory. Query, scan,
and iteration errors discard all results. Rows close before the helper returns.
The helper rejects other drivers.

After both existing pool pings, SQLite health uses this inventory for independent
store results. Missing shared tables fail all dependent stores. An incomplete
inventory fails all stores and produces one batch diagnostic. PostgreSQL retains
its existing point-query path. The timeout, interval, maintenance admission,
tracker publication, middleware, and writer/reader pool sizes are unchanged.

## Regression proof

Before production edits, this command failed with **expected 1, actual 97**:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/persistence/requiredstores -run '^TestHealthSQLiteCatalogUsesOneSchemaQuery$' -count=1 -v)
```

After implementation, it passed with one executed schema query. A test-local
SQLite driver wrapper counts actual queries across the complete production
catalog. It adds no production observation hook.

Further tests cover bound unusual names, duplicate requests, views and indexes,
empty inventories, shared missing tables, multi-table stores, scan failures,
partial iteration failures, query failures, cancellation, connection release,
and recovery. Factory-created writer and reader pools verify that holding all
connections still fails the probe at the correct ping stage and that releasing
them restores health.

## Checks

Passed:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/db ./internal/persistence/requiredstores ./internal/backendapp -run 'Test(ExistingSQLiteTablesContext|HealthSQLiteCatalog|PersistenceContentionFixture|RuntimeHealth|StartupHealth|HealthCheck|ProbeTables|RequiredPersistence|PersistenceMiddleware)' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
```

Also passed:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/persistence/storeconformance -count=1)
PATH="/tmp/kandev-merge-tools:$PATH" bash scripts/lint-go-changed apps/backend/internal/db/schema_tables.go apps/backend/internal/db/schema_tables_test.go apps/backend/internal/persistence/requiredstores/health.go apps/backend/internal/persistence/requiredstores/health_batch_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

Store conformance passed in 181.415 seconds. Lint reported zero issues. The
linter was absent from the initial PATH; the existing local golangci-lint
2.9.0 binary, built with Go 1.26.6, supplied the required command. Documentation
coverage passed for all three changed production paths. Catalog validation
covered 384 decisions and 1,616 specifications.

The final isolated comparison passed, as recorded below.
PostgreSQL integration coverage requires `KANDEV_TEST_POSTGRES_DSN`, which is
not configured in this environment. No PostgreSQL behavior changes in this patch.


## Final isolated HTTP comparison

```bash
python3 .kandev/diagnostics/stress-503/replay.py --candidate current --rounds 3 --seconds 35
```

Each run restored the same synthetic seed. Eight clients updated separate task
descriptions with 16 KiB payloads for 35 seconds. The order alternated between
original and implemented probes. No task-owned compilation, race test, or other
load ran during measurement. Both binaries were built before measurement.

| Run | Successful writes | Write 503s | Workspace-read 503s | Successful-write p95 (ms) | Probe failures |
| --- | ---: | ---: | ---: | ---: | ---: |
| pair1-baseline | 866 | 1031 | 64 | 475.416 | 1 |
| pair1-batched | 1268 | 0 | 0 | 533.703 | 0 |
| pair2-batched | 1606 | 0 | 0 | 377.536 | 0 |
| pair2-baseline | 1011 | 1021 | 64 | 380.070 | 1 |
| pair3-baseline | 1023 | 1021 | 64 | 374.429 | 1 |
| pair3-batched | 1510 | 0 | 0 | 411.598 | 0 |

Across the three runs, the original probe produced 3,073 write 503s and 192
workspace-read 503s. It completed 2,900 writes. The implemented probe completed
4,384 writes with zero write, workspace-read, or readiness errors. All three
original runs reported a table-probe deadline failure; all implemented runs
remained healthy. Diagnostics recorded startup plus two periodic probe timestamps
in each implemented run, so zero failures does not mean the probe was skipped.

Successful-write latency remained in a similar range. This change improves
availability for the tested workload; it does not establish faster individual
writes or unlimited write capacity. Other host workloads can affect timing.
The workload does not identify the writer operation behind the historical incident.

The baseline uses the preserved original algorithm with its temporary total-time
logging. The candidate uses the final production source without success logging.
Do not compare successful probe durations between these two builds. Both retain
the normal failure diagnostic and the same timeout and interval.

Exact SHA-256 hashes:

- Baseline health source: `100cb85696302ab72e6816982846e8f0279d17b9398ae5bc4806848231f95299`.
- Implemented health source: `310a63968440fc7aa776f8bdf0f44e0b602ea76cd9d51c1a719ceca3f1ede7af`.
- New schema helper: `2e83a7cd844038be049161a4f435c0eafca44f5eb84a1c0ca185e3d0e842f992`.

[implementation-measurements.json](implementation-measurements.json) contains
counts, latencies, sampled health states, timestamps, diagnostics, and hashes.
Raw evidence remains in the ignored task-local directory
`.kandev/diagnostics/stress-503/replay-20261020T091747/`.

## Cleanup and delivery

All six backends stopped. A detached Hermes discovery helper survived process-group
shutdown and recreated its disposable home. Its environment identified this
replay root; it was stopped separately, and its files were removed. No process
with the replay root in its command or environment remained. The live instance
was not changed. The implementation is ready for code review; it has not been deployed.
