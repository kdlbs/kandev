---
id: "01-contention-benchmark"
title: "Benchmark delivery writer contention"
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-PLATFORM-DELIVERY-WRITE-COST-001
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-WRITE-COST-001.1
system_design:
  - ../../specs/platform/system-design/durable-delivery-write-cost.md
---

# Task 01: Benchmark delivery writer contention

## Summary

Commit the contention benchmark used as the AC-PLATFORM-DELIVERY-WRITE-COST-001.1 evidence boundary. It
records the pre-change commit pattern as a permanent `baseline` variant, so
later work orders compare against it in the same run.

## In scope

- Finalize `agent_delivery_writer_contention_bench_test.go` from the draft:
  file-backed SQLite through the production factory, N synthetic sessions with
  full owner state, 100 events/s per session, batches matching the 20 ms window,
  and timed foreground `Repository.CreateTask`.
- Variants keyed by name: `baseline` (separate receive and projection commits,
  writer forced to FULL) and `current` (the code as it is). Session counts 0, 4,
  and 8.
- Metrics: p50, p95, and p99 in ms, sustained `stream-events/s`, and
  `writer-wait-ms/s` from `sql.DBStats`.

## Out of scope

- Production code changes.
- Running the benchmark in CI.

## Acceptance

- `go test -run '^$' -bench BenchmarkDeliveryWriterContention -benchtime=100x`
  completes for every variant, and its output contains every metric above.
- The `baseline` variant asserts `PRAGMA synchronous` is 2 (FULL) before it
  measures.

## Verification

```bash
(cd apps/backend && go vet -trimpath ./internal/task/repository/sqlite)
(cd apps/backend && go test -trimpath -run '^$' -bench 'BenchmarkDeliveryWriterContention' -benchtime=100x -count=3 ./internal/task/repository/sqlite)
bash scripts/lint-go-changed apps/backend/internal/task/repository/sqlite/agent_delivery_writer_contention_bench_test.go
```

Record the median table in Results.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/agent_delivery_writer_contention_bench_test.go` (new)

## Dependencies

None.

## Risks

Absolute timings depend on host load and disk fsync cost. Compare variants
within one run only.

## Results

Done. `BenchmarkDeliveryWriterContention` now runs four named variants against a
file-backed database opened through the production factory:

- `baseline`: separate receipt and projection commits with the writer forced to
  FULL. The variant asserts `PRAGMA synchronous` is 2 before it measures.
- `current`: the same commit pattern with the NORMAL writer.
- `combined`: one receipt-and-projection transaction per batch with the NORMAL
  writer.
- `final`: `combined` plus one ACK worker per stream that awaits the durability
  barrier every 20 ms.

Every variant runs with 0, 4, and 8 sessions. Each session offers 100 events/s
from a decoupled producer, and the benchmark reports p50, p95, and p99 in ms,
`stream-events/s`, `offered-events/s`, `delivery-%`, `writer-wait-ms/s`, and
`barrier-awaits/s`. Baseline and final measurements from the verification run
are recorded in [the plan](plan.md#verification-results).

The run completed for every variant:
`go test -run '^$' -bench BenchmarkDeliveryWriterContention -benchtime=100x`.

