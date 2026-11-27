---
id: "01-bound-clarification-reads"
title: "Bound clarification database work"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERACTIVE-READS-004
acceptance_criteria:
  - AC-PLATFORM-INTERACTIVE-READS-004.1
  - AC-PLATFORM-INTERACTIVE-READS-004.2
  - AC-PLATFORM-INTERACTIVE-READS-004.3
system_design:
  - ../../specs/platform/system-design/interactive-read-availability.md
---

# Task 01: Bound clarification database work

## Summary

Keep clarification list/count work from occupying every reader. Preserve all
bundle predicates and retain actual persistence failures in the health gate.

## In scope

- Add a task-repository admission gate for list/count methods, using the analytics
  admission pattern. One operation executes; queued work uses no database connection.
- Bound each operation and each HTTP inbox read to ten seconds, respecting earlier cancellation.
- Map internal HTTP read expiry to sanitized 503 and `Retry-After: 2`.
- Compare query plans and execution time on disposable substantial history.
  Optimize measured access paths without changing results or introducing unverified indexes.
- Add slow/failed operation timings without SQL, metadata, transcripts, or credentials.

## Out of scope

Health-state changes, additional pools, public configuration, mutation admission,
and attribution of unrelated slow reads to Inbox.

## Acceptance

1. `TestClarificationReadsPreserveInteractiveCapacity` fails before the gate and
   passes afterward. Hold admitted reads at barriers; prove normal reads and
   `Health.Check` progress through a factory-created four-reader SQLite pool.
2. Cancellation, expiry, and SQL errors release admission. Authorized HTTP overload
   is retryable, and auth errors and genuine database failures preserve their semantics.
3. List/count fixtures retain parity across SQLite and PostgreSQL. Record plans,
   baseline/improved timings, fixture size, and machine. Explain every query/index change.

## Verification

```bash
(cd apps/backend && go test ./internal/task/repository/sqlite -run 'Clarification|InboxReadAvailability' -count=1)
(cd apps/backend && go test ./internal/clarification ./internal/persistence/requiredstores ./internal/backendapp -run 'Inbox|Persistence|Health' -count=1)
(cd apps/backend && go test ./internal/mcp/handlers -run 'ListPending|Clarification' -count=1)
(cd apps/backend && go test -race ./internal/task/repository/sqlite -run '^TestClarificationReadsPreserveInteractiveCapacity$' -count=1)
(cd apps/backend && go test ./internal/task/repository/sqlite -run '^$' -bench '^BenchmarkClarificationInboxReference$' -benchtime=1x -count=3)
```

The benchmark is new and uses disposable generated data. Run PostgreSQL tests
with the repository's isolated PostgreSQL test setup and its configured DSN.
Skipped PostgreSQL tests do not establish parity; record them as pending until run.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/repository.go`
- `apps/backend/internal/task/repository/sqlite/clarification_bundle_query.go`
- `apps/backend/internal/task/repository/sqlite/clarification_read_admission.go` (new)
- `apps/backend/internal/task/repository/sqlite/clarification_read_admission_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/clarification_read_reference_bench_test.go` (new)
- Existing clarification list/count and PostgreSQL tests in that package.
- `apps/backend/internal/task/repository/sqlite/base_schema.go`, only for a measured index.
- `apps/backend/internal/clarification/inbox_handlers.go` and adjacent tests.

## Dependencies

None.

## Risks

Avoid nested gate acquisition. Keep row cleanup before release. Preserve current-turn
and mixed-status predicates during early workspace filtering. No shared private cache.

## Parallelism

`sequential`

## Inputs

- REQ-PLATFORM-INTERACTIVE-READS-004 and the clarification workload design.
- `internal/analytics/repository/sqlite/admission.go`.
- Existing current-turn clarification ownership and bounded response ADRs.

## Results

Implemented one shared repository slot for clarification list/count SQL with a
ten-second queue-plus-query context. Cancellation, expiry, and SQL failures
release the slot. Inbox HTTP reads now pass one ten-second context through
authorization, listing, message hydration, enrichment, and hidden count; internal
deadlines return a sanitized 503 with `Retry-After: 2`. Authorization failures
and genuine database errors keep their existing mappings. Fixed-name warning
fields record admission wait, execution time, error class, and reader-pool stats
without SQL or row data.

The disposable 100,001-message SQLite reference showed a broad
`idx_messages_metadata_pending_id` scan for both queries. A partial expression
index on `(metadata.pending_id, task_session_id)` for
`type = 'clarification_request'` changed both plans to scan only clarification
rows. The list query measured 23.6–24.0 ms before the candidate index and
0.94–0.99 ms with it in the first three-run comparison; final indexed runs were
0.80–1.64 ms. The empty hidden-count fixture measured 0.25–0.62 ms before and
0.47–0.69 ms after, so no count latency improvement is claimed. No SQL shape
changed. The index is dialect-generated and idempotently created by the existing
`ensureMessageMetadataIndexes` schema replay.

Fixture and machine: 100,000 ordinary history messages plus one clarification,
75% in an unrelated workspace; Linux/amd64, Go 1.26.0, Ryzen 5 7640HS, 11 CPUs.
The measurements use the prescribed `-benchtime=1x -count=3` runs and are
reference evidence, not a latency guarantee.

Verification passed:

- `(cd apps/backend && go test ./internal/task/repository/sqlite -run 'Clarification|InboxReadAvailability' -count=1)`
- `(cd apps/backend && go test ./internal/clarification ./internal/persistence/requiredstores ./internal/backendapp -run 'Inbox|Persistence|Health' -count=1)`
- `(cd apps/backend && go test ./internal/mcp/handlers -run 'ListPending|Clarification' -count=1)`
- `(cd apps/backend && go test -race ./internal/task/repository/sqlite -run '^TestClarificationReadsPreserveInteractiveCapacity$' -count=1)`
- `(cd apps/backend && go test ./internal/task/repository/sqlite -run '^$' -bench '^BenchmarkClarificationInboxReference$' -benchtime=1x -count=3)`
- SQLite and PostgreSQL clarification/index tests passed. PostgreSQL ran against a disposable local PostgreSQL 16 container, which was stopped after verification.
