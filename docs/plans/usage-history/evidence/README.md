# Usage history benchmark evidence

The benchmark files require the `usage_history_scanstatus`, `sqlite_trace`, and `cgo` build tags.
Default builds do not include these files.
The benchmark requires the pinned `github.com/mattn/go-sqlite3 v1.14.33` amalgamation.
The `libsqlite3` build tag is not part of this procedure.
Linux `/proc` supplies process peak RSS and hardware information.

The first code phase permits compilation and small SQLite fixtures only.
The large fixtures, PostgreSQL execution, and performance evidence require the coordinator's resource window.
This document does not claim a completed large benchmark or PostgreSQL parity run.

## Small checks

From the repository root, configure the instrumented build:

```bash
export CGO_ENABLED=1
export CGO_CFLAGS='-O2 -g -DSQLITE_ENABLE_STMT_SCANSTATUS'
export GOMAXPROCS=2
export GOMEMLIMIT=3GiB
```

Run the small checks:

```bash
cd apps/backend
go test -p 1 -trimpath -tags 'sqlite_trace usage_history_scanstatus' \
  ./internal/analytics/repository/sqlite \
  -run '^TestUsageHistoryBenchmark(Fixture|ScanVisits|SourceIdentities)$' -count=1 -v
```

Check benchmark discovery separately:

```bash
go test -p 1 -trimpath -tags 'sqlite_trace usage_history_scanstatus' \
  ./internal/analytics/repository/sqlite -list '^BenchmarkUsageHistory'
```

Discovery must list both benchmark names. Discovery does not execute a benchmark.

## Large runs after the resource window opens

From the repository root, select the durable evidence directory:

```bash
export KANDEV_USAGE_BENCH_EVIDENCE_DIR="$PWD/docs/plans/usage-history/evidence"
export CGO_ENABLED=1
export CGO_CFLAGS='-O2 -g -DSQLITE_ENABLE_STMT_SCANSTATUS'
export GOMAXPROCS=2
export GOMEMLIMIT=3GiB
```

Run SQLite first:

```bash
cd apps/backend
go test -p 1 -trimpath -tags 'sqlite_trace usage_history_scanstatus' \
  ./internal/analytics/repository/sqlite -run '^$' \
  -bench '^BenchmarkUsageHistoryFiveYears$' -benchtime=10x -benchmem \
  -count=1 -timeout=60m \
  > "$KANDEV_USAGE_BENCH_EVIDENCE_DIR/usage-history-worker4-bench-sqlite-benchmark.log" 2>&1
```

After SQLite completes, load the private PostgreSQL configuration without printing its values:

```bash
set -a
. /tmp/usage-history-pr3660-postgres.env
set +a
: "${KANDEV_TEST_POSTGRES_DSN:?PostgreSQL test DSN must be configured}"
```

Run PostgreSQL separately:

```bash
go test -p 1 -trimpath -tags 'sqlite_trace usage_history_scanstatus' \
  ./internal/analytics/repository/sqlite -run '^$' \
  -bench '^BenchmarkUsageHistoryPostgresFiveYears$' -benchtime=10x -benchmem \
  -count=1 -timeout=60m \
  > "$KANDEV_USAGE_BENCH_EVIDENCE_DIR/usage-history-worker4-bench-postgres-benchmark.log" 2>&1
```

A missing PostgreSQL DSN or server version below 18 fails the benchmark.
The harness does not skip PostgreSQL or substitute SQLite results.
The production analytics admission gate and query deadline remain active.
`GOMEMLIMIT` is a soft limit for the Go runtime in each process.
This limit does not bound native SQLite memory, process RSS, or PostgreSQL server memory.
The harness does not change PostgreSQL server memory configuration.

## Fixture construction

The main captured scope contains at least 1,000,000 retained native observations and 100,000 retained external observations.
These rows are normalized retained history, not synthetic counts of eligible records.
The fixture includes another organization, multiple actors, unknown amounts, several currencies, deleted tasks, and archived tasks.
Special records include corrections, equivalent overlap, obsolete unapplied builds, mixed metric actors, and lifetime alternatives.
The fixture includes both transitions for New York daylight saving time and Lisbon source-calendar days.

The seed uses batches of at most 128 rows.
Each batch stores retained observations, complete encoded canonical facts, and independent token and cost contributions.
The seed uses the production selection, contribution, summary, and publication-metadata builders.
Applied build pointers exclude obsolete canonical rows.
The seed records construction duration and actual storage counts separately from query measurements.
This bulk construction does not measure production ingestion throughput or admission performance.

The initial fixture contains 2025 data and fixed special records.
The second phase adds published data and retained observations for 2021 through 2024.
Both phases analyze the same indexed tables.
Recent reports use the same requested boundaries in both phases.
The comparison file requires identical report rows, totals, and provider options after the publication changes.

The scenarios include UTC, America/New_York, Asia/Kathmandu, and Europe/Lisbon.
They cover recent trends, five-year trends, daylight saving time, source calendars, foreign coarse boundaries, lifetime alternatives, and empty ranges.
Every scenario calls the actual `ListSessionUsage` API, including totals, provider options, metadata, sorting, and pagination SQL.
An independent scalar oracle checks nullable totals and native token amounts.
Pagination pins the opaque publication token and fails on a changed token.

## Measurement meanings

SQLite instrumentation reads `SQLITE_SCANSTAT_NVISIT` from the actual report connection during each statement's profile callback.
The evidence keeps every statement and each loop's name, plan, `NLOOP`, and `NVISIT`.
`NVISIT` already includes all loop iterations. The harness does not multiply it by `NLOOP`.
These counters count examined entries, including repeated visits. They do not count unique records or physical page reads.
Missing instrumentation fails the measurement.
Result-row counts and VM steps do not substitute for visits.

PostgreSQL instrumentation captures the actual report statements and their bindings.
It replays that exact sequence against the frozen fixture, including the temporary calendar, in a separate transaction.
Each read uses `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)`.
This separate transaction keeps EXPLAIN overhead outside the production report deadline.
The harness checks the publication again after the replay and rolls back the replay transaction.
No benchmark-specific optimized query replaces a report query.

PostgreSQL scan totals use each scan node's `Actual Rows` multiplied by `Actual Loops`.
The evidence separately records filtered and rechecked rows multiplied by loops.
The raw JSON preserves buffer counts and plan-node memory details.
These execution counters are not a claim about unique physical heap tuples.

Latency samples cover the complete report, including every page.
Fixture construction and oracle checks remain outside the benchmark timer and allocation counters.
Latency samples use no trace callbacks. SQLite scanstatus collection is disabled on their connection.
The binary still contains scanstatus support, so this measurement is not a comparison of two different compiler builds.
The evidence reports raw samples, sample count, and nearest-rank percentiles.
Ten samples do not support a capacity promise or a latency SLA.

Each scenario also starts separate plain and instrumented query processes.
Their process peak RSS includes initialization and excludes bulk fixture construction.
SQLite native allocator high-water measurement resets immediately before the query.
Native allocator memory excludes the Go heap and system allocator overhead.
Go allocation counters and process RSS have separate fields.
PostgreSQL client RSS and the largest reported plan-node memory value do not represent whole-server peak memory.
The whole-server peak remains unmeasured unless separate server instrumentation supplies it.

All receipt filenames start with `usage-history-worker4-bench-`.
Per-scenario JSON files contain plans, scans, latency samples, memory, publication identity, hardware, and fixture construction details.
The `*-recent-before-after.json` files compare recent report contents and measured scan work across both fixture shapes.
The standard benchmark logs contain `ns/op`, `B/op`, and `allocs/op`.
