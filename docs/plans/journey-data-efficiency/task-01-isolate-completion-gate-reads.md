---
id: "01-isolate-completion-gate-reads"
title: "Keep completion-gate inspection off the writer"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERACTIVE-READS-006
acceptance_criteria:
  - AC-PLATFORM-INTERACTIVE-READS-006.1
  - AC-PLATFORM-INTERACTIVE-READS-006.2
  - AC-PLATFORM-INTERACTIVE-READS-006.3
system_design:
  - ../../specs/platform/system-design/journey-data-loading.md
---

# Task 01: Keep completion-gate inspection off the writer

## Summary

A held writer does not prevent standalone gate inspection, warm homepage, or board snapshot completion. Tests prove reader release after errors and cancellation.

## In scope

Move only standalone `GetTaskCompletionGate` to a native reader snapshot. Preserve the existing helper, missing-task errors, and mutation-owned transaction paths.
Use real factory-created separate SQLite handles. Add PostgreSQL read-snapshot coverage without changing mutation isolation or evidence lock order.
Create a reusable deterministic journey fixture and `BenchmarkJourneyReadLoad` before the production change. Capture ten baseline warm samples, then ten candidate samples. Include idle and existing large-message-write workload variants. Record raw output and machine/fixture identity in `implementation-evidence.md`.

## Out of scope

Other work orders, live installation mutation, unrelated refactors, pool increases, new runtime flags, and deployment.

## Acceptance

- A held writer does not prevent standalone gate inspection, warm homepage, or board snapshot completion. Tests prove reader release after errors and cancellation.
- Gate edits and completion races still reject stale evidence. Real PostgreSQL execution passes or remains an explicit incomplete engine gate.
- The same seeded benchmark runs before and after the change; structural assertions cannot be replaced by timing claims.

## Verification

Use TDD. New test filenames and named methods below are planned deliverables, not existing passing evidence.
Run from the repository root. Install `apps/` dependencies first only if absent. Each command is independently rooted.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite ./internal/task/service ./internal/backendapp -run 'Test(CompletionGate|JourneyRead)' -count=1)
(cd apps/backend && test -n "${KANDEV_TEST_POSTGRES_DSN:-}" && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run '^TestCompletionGateReadPostgres$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/backendapp -run '^$' -bench '^BenchmarkJourneyReadLoad$' -benchtime=1x -count=10)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use a disposable PostgreSQL database with `KANDEV_TEST_POSTGRES_DSN`. Never use the live database. A missing server or skipped test is an incomplete engine gate.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/completion_gates.go`
- `apps/backend/internal/task/repository/sqlite/completion_gate_read_test.go (new)`
- `apps/backend/internal/backendapp/journey_read_test.go (new shared fixture and route control)`
- `apps/backend/internal/backendapp/journey_read_bench_test.go (new)`
- `apps/backend/internal/task/service/completion_gates_test.go`

## Dependencies

None. Execute in plan order by default.

## Risks

A writer-backed alias fixture gives false isolation evidence. ReadOnly options on the writer do not change SQLite BEGIN IMMEDIATE.

## Parallelism

`sequential`

## Inputs

- [Plan, contract inventory, and test mapping](plan.md).
- [Journey loading design](../../specs/platform/system-design/journey-data-loading.md).
- [Measured baseline](evidence.md) and `evidence/` artifacts.
- Read the owned source and nearby tests before the first edit. Preserve existing user changes.

## Results

Implemented `GetTaskCompletionGate` on the separate native reader transaction. PostgreSQL inspection uses read-only repeatable-read; completion mutation transactions and evidence lock order are unchanged. The held-writer gate and warm homepage/board snapshot tests failed before the change and passed after it. Reader release after missing-task errors and cancellation passed.

The required race-enabled SQLite/service/backend-app command passed. `TestCompletionGateReadPostgres` passed against the disposable PostgreSQL 17 instance and verified both transaction settings. The exact commands and matched ten-sample baseline/candidate measurements are in [implementation evidence](implementation-evidence.md), with raw logs in `runs/01-gate-baseline.log` and `runs/02-gate-candidate.log`.

Under eight concurrent 16 MiB message writers, route-read median/p95 moved from 858.458/2,898.051 ms to 1.482/1.987 ms. The idle candidate p95 was higher than baseline, and other processes shared the host. These observations are directional; the held-writer tests, not timings, establish reader isolation. This route benchmark did not measure health deadlines or pool wait/occupancy; Task 08 owns those measurements.
