---
id: "02-batch-completion-summary-reads"
title: "Batch completion-gate summary observations"
status: done
wave: 2
depends_on:
  - "01-isolate-completion-gate-reads"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERACTIVE-READS-006
acceptance_criteria:
  - AC-PLATFORM-INTERACTIVE-READS-006.2
  - AC-PLATFORM-INTERACTIVE-READS-006.3
  - AC-PLATFORM-INTERACTIVE-READS-006.4
  - AC-PLATFORM-INTERACTIVE-READS-006.5
system_design:
  - ../../specs/platform/system-design/journey-data-loading.md
---

# Task 02: Batch completion-gate summary observations

## Summary

A 1,000-task fixture uses at most ten gate snapshots and sixty data queries. More criteria or sessions do not introduce per-row queries.

## In scope

Add `GetTaskCompletionGateSummaries` as a typed repository read. Use chunks of at most 100 tasks and no more than six data queries per chunk.
Read task identity, criteria/revisions, and supported evidence kinds in one reader snapshot per chunk. Reuse criterion evaluation rather than maintaining another truth table.
Pass keyed observations to existing and missing status-summary reconciliation. Preserve CAS retries, failure classification, and all current completion evidence semantics.

## Out of scope

Other work orders, live installation mutation, unrelated refactors, pool increases, new runtime flags, and deployment.

## Acceptance

- A 1,000-task fixture uses at most ten gate snapshots and sixty data queries. More criteria or sessions do not introduce per-row queries.
- Batch results match standalone gates for all evidence kinds, absent tasks, and stale evidence on SQLite and PostgreSQL.
- Missing/stale summary repair converges without overwriting newer revisions. Warm no-op repair performs no summary mutation and no writer reservation.

## Verification

Use TDD. New test filenames and named methods below are planned deliverables, not existing passing evidence.
Run from the repository root. Install `apps/` dependencies first only if absent. Each command is independently rooted.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite ./internal/task/service ./internal/backendapp -run 'Test(CompletionGate|TaskStatusSummary|JourneyRead)' -count=1)
(cd apps/backend && test -n "${KANDEV_TEST_POSTGRES_DSN:-}" && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run '^TestCompletionGateBatchPostgres$' -count=1 -v)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use a disposable PostgreSQL database with `KANDEV_TEST_POSTGRES_DSN`. Never use the live database. A missing server or skipped test is an incomplete engine gate.

## Files likely touched

- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/completion_gates.go`
- `apps/backend/internal/task/repository/sqlite/completion_gate_batch_test.go (new)`
- `apps/backend/internal/task/service/service_status_summary_rebuild.go`
- `apps/backend/internal/task/service/service_status_summary_batch_test.go (new)`
- `apps/backend/internal/backendapp/journey_read_test.go`

## Dependencies

[Task 01](task-01-isolate-completion-gate-reads.md)

## Risks

Gate summaries cannot infer unblocked status from missing evidence reads. No single board-wide writer transaction is permitted.

## Parallelism

`sequential`

## Inputs

- [Plan, contract inventory, and test mapping](plan.md).
- [Journey loading design](../../specs/platform/system-design/journey-data-loading.md).
- [Measured baseline](evidence.md) and `evidence/` artifacts.
- Read the owned source and nearby tests before the first edit. Preserve existing user changes.

## Results

Implemented `GetTaskCompletionGateSummaries` with 100-task reader snapshots. Each chunk loads task identity, set revisions, criteria, and supported mutable evidence in at most six data queries. The batch reuses the standalone criterion validity rules and returns explicit missing task IDs. Status-summary repair now loads keyed observations once and supplies the same snapshot to existing-summary comparison and missing-summary rebuild.

TDD evidence: the 1,000-task repository fixture first failed because the reader was absent. After implementation it passed with ten reader transactions and no more than sixty data queries while evaluating ten criteria per task. Standalone parity covers immutable artifacts, current and stale task revisions, changed pull-request heads, incomplete execution evidence, unverified criteria, tasks without a gate, and missing tasks. The service regression first failed with zero batch-reader calls, then passed for 130 missing summaries and a warm no-op repair that retained revisions and published no events.

The required race-enabled SQLite, service, and backend-app command passed. `TestCompletionGateBatchPostgres` passed against the disposable PostgreSQL 17 database. `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed. No timing claim is made for this structural work order; integrated route and pool measurements remain in Task 08.
