---
id: "02-query-aggregation"
title: "Remove multiplicative statistics queries"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERACTIVE-READS-001
acceptance_criteria:
  - AC-PLATFORM-INTERACTIVE-READS-001.1
  - AC-PLATFORM-INTERACTIVE-READS-001.2
  - AC-PLATFORM-INTERACTIVE-READS-001.3
system_design:
  - ../../specs/platform/system-design/interactive-read-availability.md
---

# Task 02: Remove multiplicative statistics queries

## Summary

Rewrite the three heavy statistics aggregates while preserving their numeric meaning.
Establish repeatable performance evidence before adding concurrency limits.

## In scope

- Own task/repository/daily SQL, measured query helpers/indexes, dual-engine regression fixtures, and opt-in reference benchmarks.
- Capture baseline before edits and optimized results on the same disposable dataset.

## Out of scope

Other work orders, new product metrics, health-policy changes, and live-instance mutation.

## Acceptance

- All metric fixtures match on SQLite and PostgreSQL, including zero-turn sessions, multi-repository tasks, range boundaries, hidden task origins, and message-only days.
- No raw turns-by-messages product remains; task page/workspace/date scope applies before the large aggregation.
- Record the minimum fivefold heavy-fixture improvement and per-section/all-seven timings; Task 03 owns the final admission-enabled latency gate.

## Regression tests

Implement `TestStatsAggregateParity` using the repository's real queries and shared assertions for both drivers; use the established isolated PostgreSQL test helper. Do not treat a skipped PostgreSQL subtest as evidence. Cases must include naive UTC midnight boundaries, equal timestamps, excluded tasks, two workspaces, two repositories per task, sessions without turns, messages without turn IDs, and days with messages but no turns. Add `BenchmarkStatsReference` sub-benchmarks for each query and all seven operations, with fixtures outside measured setup. Keep a small oracle fixture to compare expected values without retaining production copies of old SQL.

## Verification

Run from the repository root. Before the first pnpm command in a fresh worktree,
run `(cd apps && pnpm install --frozen-lockfile)`. New test paths below must be
created by this work order; verify test discovery before claiming a pass.

```bash
(cd apps/backend && go test ./internal/analytics/repository/sqlite ./internal/analytics/handlers)
(cd apps/backend && go test ./internal/analytics/repository/sqlite -run '^TestStatsAggregateParity$' -count=1)
(cd apps/backend && go test ./internal/analytics/repository/sqlite -run '^$' -bench '^BenchmarkStatsReference' -benchtime=10x -count=1)
(cd apps/backend && go run ./cmd/sqlguard)
# Require an isolated PostgreSQL DSN in the environment; do not print credentials.
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && go test ./internal/analytics/repository/sqlite -run '^TestStatsAggregateParity$' -count=1)
```

If additional test files are changed during extraction, add their exact commands
here and run them before completion. Record skipped external services as blockers
to that validation, never as passing evidence.

## Files likely touched

- `apps/backend/internal/analytics/repository/sqlite/stats.go`
- `apps/backend/internal/analytics/repository/sqlite/repository.go` only for measured indexes
- `apps/backend/internal/analytics/repository/sqlite/repository_test.go`
- New `apps/backend/internal/analytics/repository/sqlite/stats_aggregate_parity_test.go`
- New `apps/backend/internal/analytics/repository/sqlite/stats_reference_bench_test.go`
- Same-package query helpers if needed for code-quality limits.

## Dependencies

None. Follow the manifest order for delivery.

## Risks

Metric compatibility, portable SQL, cancellation cleanup, and misleading performance claims. See the manifest risks.

## Parallelism

`sequential`

## Inputs

- Applicable requirements and system designs from frontmatter, read in full.
- Investigation evidence and current source pointers in [plan.md](plan.md).
- Existing tests adjacent to owned files; E2E uses `test-base`, API seeding, and causal waits.

## Results

Implemented the task, repository, and daily aggregates with independent child
aggregations. Task and repository scopes are reduced before child rows are read;
daily messages are counted through distinct session/day pairs that have an
eligible turn. No turns-by-messages product remains in these three queries.

RED evidence was captured before the production rewrites:

- `cd apps/backend && go test ./internal/analytics/repository/sqlite -run '^TestStatsTaskAggregateAvoidsTurnsMessagesProduct$' -count=1` failed with `context deadline exceeded` while the old task query formed the 200-turn by 10,000-message intermediate result.
- `cd apps/backend && go test ./internal/analytics/repository/sqlite -run '^TestStatsRepositoryAndDailyAggregatesAvoidTurnsMessagesProduct$' -count=1` failed with `context deadline exceeded` while the old repository query ran; the daily assertion was not reached in that run.

GREEN and parity evidence:

- Both heavy-join regression commands above pass after the rewrites.
- `cd apps/backend && go test -v ./internal/analytics/repository/sqlite -run '^TestStatsAggregateParity$' -count=1` passes the SQLite subtest. The PostgreSQL subtest is skipped because `KANDEV_TEST_POSTGRES_DSN` was not set, so this is not PostgreSQL evidence.
- `cd apps/backend && go test ./internal/analytics/repository/sqlite ./internal/analytics/handlers` passes.
- `cd apps/backend && go test -race ./internal/analytics/repository/sqlite -run '^(TestStatsTaskAggregateAvoidsTurnsMessagesProduct|TestStatsRepositoryAndDailyAggregatesAvoidTurnsMessagesProduct|TestStatsAggregateParity)$' -count=1` passes.
- `cd apps/backend && go run ./cmd/sqlguard ./internal` passes.

Reference benchmark evidence uses the required disposable shape: 700 tasks,
800 sessions, 5,000 turns, 650,000 messages, 2,200 commits, and five
repositories in each of two workspaces. The fixture and inserts are outside the
benchmark timer. The machine was an AMD Ryzen 5 7640HS, Linux amd64, with 12
logical CPUs, 22 GiB RAM, about 15 GiB available during the run, and local ext4
storage.

The optimized command was:

```text
cd apps/backend && go test ./internal/analytics/repository/sqlite -run '^$' -bench '^BenchmarkStatsReference' -benchtime=10x -count=1
```

The reported warm `ns/op` averages were:

| Section | Average |
| --- | ---: |
| global | 105.5 ms |
| tasks | 366.6 ms |
| daily | 1.663 s |
| completed | 1.40 ms |
| models | 9.67 ms |
| repositories | 1.814 s |
| git | 1.44 ms |
| all-seven | 3.420 s |

On the same fixture and machine, a cold single-iteration run from the pre-change
checkout measured 16.44 s for tasks, 6.42 s for daily, 54.63 s for repositories,
and 80.64 s for all seven. The corresponding optimized cold samples were 2.59 s,
3.78 s, 2.38 s, and 4.74 s, respectively. This is approximately 6.4x faster
for tasks, 1.7x for daily, 23x for repositories, and 17x for all seven. The
fivefold improvement is therefore established for the combined workload and the
heavy task/repository paths; the daily sample is recorded without overstating
its smaller improvement.

The PostgreSQL parity run remains a validation limitation because no isolated DSN
was available. Benchmark values are SQLite-only, the ten-iteration output is an
average rather than a p95, and the cold comparison is a single sample; these
figures do not claim production latency or cross-engine performance. These
sequential repository timings are query baselines owned by Task 02. The
admission-enabled concurrent HTTP p95 gate is recorded in Task 03.
