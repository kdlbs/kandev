# Worker 4 benchmark first phase

Status: READY-FOR-BENCH. Production files and Task 05 Results remain unchanged.
All new Go files require `cgo && sqlite_trace && usage_history_scanstatus`.
The 1M/100k fixtures and PostgreSQL execution did not run in this phase.

The current code passed these checks with `CGO_ENABLED=1`,
`CGO_CFLAGS='-O2 -g -DSQLITE_ENABLE_STMT_SCANSTATUS'`,
`GOMAXPROCS=2`, and `GOMEMLIMIT=3GiB`.
Commands ran from `apps/backend`.

```bash
go test -p 1 -trimpath -tags 'sqlite_trace usage_history_scanstatus' \
  ./internal/analytics/repository/sqlite \
  -run '^TestUsageHistoryBenchmark(Fixture|ScanVisits)$' -count=1 -v

go test -p 1 -trimpath -tags 'sqlite_trace usage_history_scanstatus' \
  ./internal/analytics/repository/sqlite -list '^BenchmarkUsageHistory'

go vet -p 1 -trimpath -tags 'sqlite_trace usage_history_scanstatus' \
  ./internal/analytics/repository/sqlite

GOFLAGS='-p=1 -trimpath' golangci-lint run \
  ./internal/analytics/repository/sqlite \
  --build-tags=sqlite_trace,usage_history_scanstatus \
  --new-from-patch=/tmp/usage-history-benchmark-instrumentation/owned-benchmark.patch \
  --whole-files --timeout=5m --concurrency=2
```

The small check selected and passed two tests. It used configured counts of
100 native and 50 external observations, plus fixed controls and another organization.
It exercised the actual report API, all configured calendars, nullable totals,
independent actors, deleted tasks, lifetime alternatives, and publication changes.
It also verified isolated query processes and actual SQLite scan counters.
The scan proof returned the same sum with 10 indexed visits and 1,000 full-scan visits.
Both repeated prepared executions preserved those counts.

Discovery listed `BenchmarkUsageHistoryFiveYears` and
`BenchmarkUsageHistoryPostgresFiveYears`. Discovery did not execute either benchmark.
Scoped lint returned `0 issues.`. Vet exited successfully without diagnostics.
The lint patch contains every line of the 15 owned new Go files.
Formatting, build-tag isolation, and the 800-line file limit also passed.

The benchmark needs no production query hook. PostgreSQL captures the actual
statements and bindings through a benchmark-only driver wrapper, then explains
the exact sequence in a separate transaction against the frozen fixture.
No production index or query tuning occurred. Large-fixture performance remains unmeasured.

`README.md` contains the exact serial large-run commands and private PostgreSQL setup.
Every generated receipt filename starts with `usage-history-worker4-bench-`.
The requested Go memory target is a soft 3 GiB per process.
Native SQLite memory and process RSS have separate measurements.
PostgreSQL client RSS and plan-node memory do not claim whole-server peak memory.

Remaining work requires the coordinator's resource window: both 10-sample large
benchmarks, real PostgreSQL EXPLAIN execution, and review of their actual plans,
scan counts, latency, memory, fixture costs, and recent-range comparisons.
No test or benchmark process remains running at this handoff.
