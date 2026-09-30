---
id: "01-bound-preparation"
title: "Bound SQLite preparation and scratch lifetime"
status: done
wave: 2
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-002
acceptance_criteria:
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.1
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.3
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.4
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.5
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.7
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.10
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.14
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.19
system_design:
  - ../../specs/ui/system-design/sidebar-archived-filter.md
---

# Task 01: Bound SQLite preparation and scratch lifetime

## Summary

Separate candidate evaluation from recursive page preparation on SQLite, within one read snapshot.
Preserve all view semantics and make scratch cleanup safe for reader-pool reuse.

## In scope

- Split candidate/filter SQL and arguments from visibility/paging SQL and arguments.
- Pin one reader connection and use the existing read transaction for every stage and selected-row hydration.
- Create the temporary filtered relation and ID/parent indexes without changing the main database.
- Reuse the relation for page selection, empty results, and collapsed headers.
- Close rows, drop scratch state, commit or roll back, and release or discard the connection.
- Preserve PostgreSQL's existing path and the current timestamp expression.
- Add native-memory and lifecycle regressions through the real repository method.

## Out of scope

Frontend changes, caches, saved settings, allocator trimming, migrations, browser-crash remediation, and changes to shared SQLite driver configuration.

## Acceptance

1. `TestSidebarQueryPreparationMemory` fails on the old query and passes within the design's 64 MiB sequential native budget.
2. Success, errors, cancellation, cleanup timeout, and connection reuse leave no request-owned rows available to another request.
3. Complete-view results retain chronological precision, grouping, filtering, pin/manual precedence, page metadata, and read-only-main behavior.

## TDD and regression matrix

Add `sidebar_task_query_memory_test.go` with a subprocess helper that invokes `QuerySidebarTaskPage`.
Use synthetic migrated databases with zero and 101 tasks, never an installation database.
Expose native SQLite counters through a test-only package at `internal/testutil/sqlitememory`.
Use a normal `.go` cgo bridge imported only by tests. Cgo declarations cannot live directly in `_test.go` files.
Link the bridge against the same go-sqlite3 build, rather than a different system SQLite library.
Use OS-specific test files or explicit skips for unsupported platforms, with Linux/cgo required for delivery evidence.
Reset counters only inside the isolated child. Record baseline, peak, retained bytes, and process RSS separately.
Assert the native budget before cleanup of the database; do not measure Go allocation alone.

The expected RED case is Last activity descending with State grouping on an empty database.
Its native allocation exceeds 64 MiB before the correction.
Run every sort/group pair in both directions after correction, including maximum accepted input collections.
The fixtures must preserve nanoseconds, timezone offsets, malformed-value fallback, equal instants, and missing summaries.
Reuse `TestQuerySidebarTaskPageOrdersActivityByInstant` and complete-tree fixtures rather than replacing their expected results.

Add `sidebar_task_query_snapshot_test.go` with `TestSidebarQueryScratchLifecycle` and `TestSidebarQueryScratchWorkspaceIsolation`.
Cover failures after relation creation, index creation, page read, empty-count read, header read, hydration, and commit.
Cover pre-cancelled context, in-flight cancellation, rollback error, and cleanup deadline expiry.
Assert an uncertain connection is discarded and replaced, not returned with stale temporary tables.
Use a four-reader pool and mixed concurrent workspace queries with disjoint task IDs.
Cover a concurrent WAL writer, transaction snapshot consistency, and a main-database write rejection on the reader.

A constant internal scratch name is safe only while a connection is exclusively pinned.
Do not use `IF NOT EXISTS`, share scratch data across requests, or fall back to the expensive original query on failure.
Preserve SQLite scalar values and collation through the temporary relation.

## Verification

Run from the repository root. Use a disposable PostgreSQL database for the required dialect cases.
A PostgreSQL skip is not a pass. Run product commands sequentially.

```bash
: "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable PostgreSQL test DSN}"
(cd apps/backend && go test -tags sqlite_fts5 -p 1 ./internal/task/repository/sqlite -run '^TestSidebarQueryPreparationMemory$' -count=1 -v)
(cd apps/backend && go test -tags sqlite_fts5 -p 1 ./internal/task/repository/sqlite -run '^TestSidebarQueryScratch' -count=1 -v)
(cd apps/backend && go test -tags sqlite_fts5 -p 1 ./internal/task/repository/... ./internal/task/handlers ./internal/task/service -run 'SidebarTask|SidebarQuery|OnlyArchived' -count=1)
git diff --check
```

Record the original allocation failure, corrected peaks, cleanup failures exercised, dialect results, and exact test counts.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/sidebar_task_query.go`
- `apps/backend/internal/task/repository/sqlite/sidebar_task_query_base.go`
- `apps/backend/internal/task/repository/sqlite/sidebar_task_query_page_sql.go`
- `apps/backend/internal/task/repository/sqlite/sidebar_task_query_snapshot.go` (new)
- `apps/backend/internal/task/repository/sqlite/sidebar_task_query_snapshot_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/sidebar_task_query_memory_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/sidebar_task_query_memory_linux_test.go` (new)
- `apps/backend/internal/testutil/sqlitememory/memory.go` (new test-support package)
- `apps/backend/internal/task/repository/sqlite/sidebar_task_query_presentation_test.go`
- `apps/backend/internal/task/repository/sqlite/sidebar_task_query_test.go`

## Dependencies

None.

## Risks

SQLite TEMP writes must work through the actual `mode=ro` connection and driver transaction path.
Cancellation and automatic `database/sql` rollback can race explicit cleanup.
Unexpected cleanup errors require connection discard through `sql.Conn.Raw` and `driver.ErrBadConn`.
Do not silently broaden the change to the writer pool or persistent schema.

## Parallelism

`sequential`

## Inputs

- [Pagination requirements](../../specs/ui/requirements/sidebar-task-pagination.md), .1, .3–.5, .7, .10, .14, .19.
- [Browsing design](../../specs/ui/system-design/sidebar-archived-filter.md), Bounded SQLite query preparation.
- [Decision](../../decisions/2026-09-29-sidebar-query-scratch-relation.md).
- Existing reader-pool snapshot and cross-dialect sidebar tests.
- [Plan](plan.md), evidence and test matrix.

## Results

The SQLite query now pins one read-only reader, stages filtered scalar rows, and
uses them for recursive selection, counts, and headers within the same transaction.
Cleanup rolls back errors/cancellation, verifies scratch removal with an independent
bounded context, and discards uncertain connections, including failed commits.
New synthetic production-pool tests cover native allocation, repeated concurrent
reads, stage failures, cancellation, cleanup expiry, snapshot consistency, and workspace
isolation. Tests are authored for CI and have not run locally, per the user.
Scratch preparation now adds a unique ID index and analyzes only the temporary
candidate relation. Cleanup regressions check that its statistics retain no rows
for the next borrower. The main reader remains read-only.

CI on the earlier delivery head exposed a 75,247,976-byte native peak for the
maximum collapsed-input case in the PostgreSQL job, backend shard 1, and isolated
memory job. The existing maximum-input regression supplies the failing evidence.
Collapse IDs, collapsed group exclusions, and collapsed header requests now bind
JSON string arrays through SQLite/PostgreSQL set expansion instead of expanding
thousands of SQL parameters. Cross-engine coverage preserves duplicate/missing
membership, quoted IDs, counts, headers, and page clamping.

Exact SQL head `e5ea04469` passed isolated resource CI run `36701226446`,
attempt 1, memory job `109841594547`. All 144 preparation samples remained
below 64 MiB (peak 64,299,672 bytes). The five maximum-input cases peaked at
15,684,584 bytes; collapse preparation fell to 5,850,472 bytes. The 73 pooled
cases (100 reads each, concurrency four) peaked at 256,519,800 native bytes,
98,784 retained bytes, and 450,637,824 bytes RSS delta, meeting every budget.
Full backend and cross-engine confirmation is recorded below.

Before the final browser-fixture correction, full backend CI run `36707127038`
passed at `a0bf2671f` (attempt 1), including both test shards, both PostgreSQL
versions, Windows, ambient integration, and static checks. Memory job
`109861941457` independently confirmed all 144 preparation samples (72 each
at 0/101), five maximum-input cases, and 1,825 pooled samples across 73 cases.
Preparation peak was 64,299,672 bytes; maximum-input peak was 15,684,584 bytes;
pooled peak was 256,176,304 bytes, retained 98,784 bytes, and RSS delta
504,885,248 bytes. Every native budget passed. The remaining correction changes
only browser test ordering and delivery records; SQL implementation is unchanged.
