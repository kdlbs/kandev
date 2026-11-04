---
id: "01-measure-creation"
title: "Measure full creation under load"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CREATE-RESPONSIVENESS-001
acceptance_criteria:
  - AC-TASKS-CREATE-RESPONSIVENESS-001.1
  - AC-TASKS-CREATE-RESPONSIVENESS-001.2
system_design:
  - ../../specs/tasks/system-design/task-create-responsiveness.md
---

# Task 01: Measure full creation under load

## Summary

Capture a repeatable full HTTP creation baseline with concurrent durable output
and unrelated color writes. Retain the original benchmark binary before changes.

## In scope

- Add `BenchmarkTaskCreateContention` to task-handler tests.
- Use production SQLite factories and real services through session preparation
  and user-settings mutation. Substitute only asynchronous provider/process launch.
- Offer zero, four, and eight independent streams at 100 text events/s each.
- Measure immediate-start and no-agent requests separately.
- Report at least 100 requests per point, latency percentiles, errors, writer
  acquisitions/commits, offered events, projected events, and settings CAS failures.
- Add focused phase instrumentation only if it identifies an unresolved delay.
  Keep prompts and SQL parameter values out of output.

## Out of scope

Production optimization, schema changes, broad profiling, and live mutations.

## Acceptance

1. The baseline reaches the real create and color-save boundaries.
2. The fixture verifies all sessions, submissions, colors, and delivery progress
   after the run. Initialization and migrations remain outside measurement.
3. The plan records the original revision, host conditions, exact command,
   original binary path, and complete baseline results for the later comparison.

## Verification

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/handlers -run '^TestTaskCreateContentionFixture$' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/handlers -run '^$' -bench '^BenchmarkTaskCreateContention$' -benchtime=100x -count=1)
```

Before production changes, compile this fixture with `go test -c` into a private
temporary directory. Retain its path and build identity in the Results section.
The final comparison runs this binary and the candidate sequentially.

## Files likely touched

- `apps/backend/internal/task/handlers/task_create_contention_bench_test.go` (new)
- `apps/backend/internal/task/handlers/task_create_contention_fixture_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/agent_delivery_writer_contention_bench_test.go` (reference)
- This package's verification results.

## Dependencies

None.

## Risks

Test doubles before SQL preparation can conceal the actual cost.
Backpressured producers can report low offered load as apparent success.
The original binary is temporary and must be rebuilt from the original source
if the environment disappears.

## Parallelism

`sequential`

## Inputs

The system design's Measurement boundary and the existing production-factory
delivery contention benchmark.

## Results

Completed. `TestTaskCreateContentionFixture` and
`BenchmarkTaskCreateContention` live beside the real task HTTP handlers. The
fixture uses file-backed SQLite through `OpenSQLite` and `OpenSQLiteReader`,
the production task/user repositories and services, the real orchestrator
preparation path, a real local Git repository on the create payload, and the
production durable-delivery receive/projection, cursor, and durability-barrier
paths. Only the asynchronous provider/process launch is substituted.

Baseline source: revision `a0f2e665701` in temporary worktree
`/tmp/opencode/kandev-baseline-fixed`. The same corrected fixture and benchmark
are present there. Retained binaries:

- Baseline: `/tmp/opencode/task-create-baseline-fixed.test`, SHA-256
  `abb83fd3451bd96b8db93e0b9bd74b01a9484c940f8d12d5d4ac709c76f19f92`.
- Candidate: `/tmp/opencode/task-create-candidate-fixed.test`, SHA-256
  `1ff15ed60fbb1637c61379bbb0df416d4db81144df1b0f3ce64d36a9552e3a6c`.

Verification:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/handlers -run '^TestTaskCreateContentionFixture$' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/handlers -run '^$' -bench '^BenchmarkTaskCreateContention$' -benchtime=100x -count=1)
```

The fixture passed. The benchmark reports create p50/p95/p99, create failures,
color p50/p95, color sample count and failures, offered/projected/dropped
stream events, actual offered/projected rates, timely delivery percentage,
stream and ACK errors, writer wait count, and writer wait duration. Events
projected after the timed interval are cleanup only and do not count as timely
delivery. A failed color, too few color samples, or less than 90% timely
delivery fails the benchmark.

Five eight-stream runs per binary on the same host produced unstable p95
values under unrelated host load; the candidate does not consistently beat the
baseline. The complete comparison and the unproven AC .1 result are recorded in
`plan.md`. Writer acquisitions, commits, and individual SQL statements are not
counted separately; pool wait counters are the contention evidence.