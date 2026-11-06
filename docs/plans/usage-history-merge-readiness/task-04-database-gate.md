---
id: "04-database-gate"
title: "Run the full database gate"
status: blocked
wave: 4
depends_on:
  - "01-native-backfill-order"
  - "02-deletion-of-unprovable-usage"
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-HISTORY-005
acceptance_criteria:
  - AC-COSTS-USAGE-HISTORY-005.2
  - AC-COSTS-USAGE-HISTORY-005.5
system_design:
  - ../../specs/costs/system-design/usage-history.md
---

# Task 04: Run the full database gate

## Summary

Run the five-year benchmark on SQLite and PostgreSQL 18.6 on the final code, with unchanged inputs. This isn't a CI job; CI's PostgreSQL tests are already green. On a real run, a query that exceeds the analytics deadline fails the gate. The toggle doesn't affect it, because the benchmark calls the repository directly.

The last full PostgreSQL result, on `ee5b9aa`, passed 28 of 32 scenarios. New York, Kathmandu, and Lisbon five-year views and deleted-task reporting failed with `analytics busy: timeout: context deadline exceeded`. The rerun on `a0d13b9` ended with exit 143 during fixture construction, so later query changes have never been measured at five years.

## In scope

1. Check out the final #3660 head in a clean detached worktree at `/tmp/usage-gate`, and record its SHA. Keep the machine otherwise idle: no CI, browser, or build jobs.
2. Start PostgreSQL 18.6:
   `docker run -d --name usage-gate-pg -e POSTGRES_PASSWORD=gate -e POSTGRES_DB=usage_history -p 127.0.0.1:55366:5432 postgres:18.6`
3. Create `/tmp/usage-gate-run.sh` with the commands below, then start it detached so the calling tool can't kill it. Exit 143 means something sent SIGTERM, most likely a tool timeout.

```bash
#!/usr/bin/env bash
set -uo pipefail
cd /tmp/usage-gate/apps/backend
export CGO_ENABLED=1 CGO_CFLAGS='-O2 -g -DSQLITE_ENABLE_STMT_SCANSTATUS' GOMAXPROCS=2 GOMEMLIMIT=3GiB GOFLAGS=''
unset KANDEV_USAGE_BENCH_TRACE_SQLITE KANDEV_USAGE_BENCH_TRACE_POSTGRES KANDEV_USAGE_HISTORY_BENCH_CHILD KANDEV_TEST_POSTGRES_DSN
run() { KANDEV_USAGE_BENCH_EVIDENCE_DIR="/tmp/usage-gate-evidence/$1" go test -p 1 -trimpath -tags 'sqlite_trace usage_history_scanstatus' ./internal/analytics/repository/sqlite -run '^$' -bench "^$2$" -benchtime=10x -benchmem -count=1 -timeout=0 > "/tmp/usage-gate-$1.log" 2>&1; echo "$1 exit $?" >> /tmp/usage-gate-status.txt; }
mkdir -p /tmp/usage-gate-evidence/sqlite /tmp/usage-gate-evidence/postgres
run sqlite BenchmarkUsageHistoryFiveYears
export KANDEV_TEST_POSTGRES_DSN='postgres://postgres:gate@127.0.0.1:55366/usage_history?sslmode=disable'
(
  cd /tmp/usage-gate/apps/backend
  go test -trimpath ./internal/task/repository/sqlite/ -run '^TestPreUpgradeNativeUsageAppearsAfterMigration$' -count=1 > /tmp/usage-gate-task01-postgres.log 2>&1
  echo "task01-postgres exit $?" >> /tmp/usage-gate-status.txt
  go test -trimpath ./internal/task/repository/sqlite/ -run 'TestUsageHistoryForkedNativeUsageDeletesWithGap|TestUsageHistoryUnprovenNativeBlocksDeletionBeforeAdoption|TestPreUpgradeNativeUsageAppearsAfterMigration' -count=1 > /tmp/usage-gate-task02-postgres.log 2>&1
  echo "task02-postgres exit $?" >> /tmp/usage-gate-status.txt
)
run postgres BenchmarkUsageHistoryPostgresFiveYears
echo done >> /tmp/usage-gate-status.txt
```

Start it with `setsid nohup bash /tmp/usage-gate-run.sh >/tmp/usage-gate-run.out 2>&1 < /dev/null &`. Check `/tmp/usage-gate-status.txt` occasionally. Expect several hours: the PostgreSQL run alone took 6,860 seconds on C12.

## Out of scope

- Changing fixtures, oracle, samples, deadlines, or query code.
- Committing raw evidence. Store it under `/tmp`, and put only a summary in the repository.

## Acceptance

- Both engines report exit 0 on one recorded head, with no `--- FAIL` in either log.
- Each evidence directory contains receipts for all 32 scenarios.
- `docs/plans/usage-history/plan.md` records the SHA, duration, and slowest scenario per engine.

If any scenario fails, or a run is killed twice, STOP. Record the failing scenarios and report them. Don't tune queries. That needs a design pass.

## Verification

```bash
cat /tmp/usage-gate-status.txt
grep -c -- '--- FAIL' /tmp/usage-gate-sqlite.log /tmp/usage-gate-postgres.log
ls /tmp/usage-gate-evidence/sqlite | wc -l
ls /tmp/usage-gate-evidence/postgres | wc -l
```

## Files likely touched

- `docs/plans/usage-history/plan.md`
- `docs/plans/usage-history-merge-readiness/task-04-database-gate.md`

## Dependencies

Tasks 01 and 02, plus the final analytics code on the current #3660 head. Task 03 was cancelled and does not block this gate. A later change to report or projection SQL invalidates this run.

## Risks

- Fixture construction may need updating for adoption-first migration. If setup fails before any scenario runs, STOP and report it.
- Remove the container and `/tmp/usage-gate` afterwards, and keep the logs until the report is delivered.

## Parallelism

`sequential`

## Inputs

- `git show a0d13b9:docs/plans/usage-history/evidence/usage-history-luna-C12-full-normal-20261031T073420Z/usage-history-C12-run-full-normal-gates.sh`, the original runner and its receipt validator.
- `apps/backend/internal/analytics/repository/sqlite/usage_history_bench_run_test.go`.

## Results

The detached command `setsid nohup bash /tmp/usage-gate-run.sh >/tmp/usage-gate-run.out 2>&1 < /dev/null &` used then-current #3660 head `a39942aa5e74be35461bdc09961f44199eb79e0f` and PostgreSQL 18.6. The SQLite command `go test -p 1 -trimpath -tags 'sqlite_trace usage_history_scanstatus' ./internal/analytics/repository/sqlite -run '^$' -bench '^BenchmarkUsageHistoryFiveYears$' -benchtime=10x -benchmem -count=1 -timeout=0` exited 1 after 2,052.861 seconds during `fixture.seedOlder`: bounded fixture projection timed out at `scope_index=0`, page 12,403 with `analytics busy: context deadline exceeded`. Sixteen one-year evidence receipts exist; the five-year phase did not run. `grep -c -- '--- FAIL' /tmp/usage-gate-sqlite.log` returned 1, and the SQLite/PostgreSQL receipt counts were 16/0. The runner stopped before the PostgreSQL benchmark, so no PostgreSQL full-scale result is claimed.

The Task 01 PostgreSQL-focused test passed in 0.245 seconds, and the Task 02 focused tests passed in 0.693 seconds. Their logs are `/tmp/usage-gate-task01-postgres.log` and `/tmp/usage-gate-task02-postgres.log`. The benchmark log and receipts remain under `/tmp/usage-gate-sqlite.log` and `/tmp/usage-gate-evidence/sqlite/`.

Commit `3d43b09752a` later changed PostgreSQL report SQL. No full-scale SQLite or PostgreSQL result for that updated source is recorded; the gate remains blocked.
