---
id: "01-measure-creation"
title: "Measure full creation under load"
status: in_progress
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
  wait counters, offered events, and projected events. Writer acquisitions,
  commits, individual statements, and settings-CAS failures are deferred
  instrumentation for this in-progress work order.
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

Benchmark and fixture delivered; acquisition/commit and settings-CAS metrics
remain deferred, so this work order stays in progress.

`TestTaskCreateContentionFixture` and
`BenchmarkTaskCreateContention` live beside the real task HTTP handlers. The
fixture uses file-backed SQLite through `OpenSQLite` and `OpenSQLiteReader`,
the production task/user repositories and services, the real orchestrator
preparation path, a real local Git repository on the create payload, and the
production durable-delivery receive/projection, cursor, and durability-barrier
paths. Only the asynchronous provider/process launch is substituted.

Baseline source: revision `a0f2e665701` in temporary worktree
`/tmp/opencode/kandev-baseline-fixup`. The same corrected fixture and benchmark
are present there. Retained binaries:

- Baseline: `/tmp/opencode/task-create-baseline-final.test`, SHA-256
  `4fc967b0487c63878536044f79d80ba1e8389ca3ed9ef29569f253c6dbef90b0`.
- Candidate: `/tmp/opencode/task-create-candidate-final.test`, SHA-256
  `671390a6614d1edcfecdc30829c6d1299936ad75279c77e52454dfff62b3b014`.

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

Five eight-stream runs per binary on the same host produced a median
improvement of only about 5.8%, well below the target. The complete comparison and the unproven AC .1/AC .2
results are recorded in `plan.md`. The requested writer-acquisition, commit, and individual-statement
counters, plus settings-CAS failure counts, are not collected. They remain
deferred instrumentation work; pool wait counters and the user-service error
path are the current evidence, and this work order is not marked complete.