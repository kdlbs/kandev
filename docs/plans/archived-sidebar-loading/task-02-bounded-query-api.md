---
id: "02-bounded-query-api"
title: "Expose bounded sidebar pages"
status: done
wave: 2
depends_on:
  - "01-view-query-semantics"
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-001
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-002
acceptance_criteria:
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-001.8
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-001.9
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.1
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.2
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.3
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.4
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.5
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.10
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.6
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.7
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.8
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.11
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.12
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.13
system_design:
  - ../../specs/ui/system-design/sidebar-archived-filter.md
---

# Task 02: Expose bounded sidebar pages

## Summary

Expose the bounded query through the existing authenticated workspace boundary.
Prove page correctness and query cost before changing the sidebar consumer.

## In scope

- Register the read-only POST route and typed request/response DTOs.
- Read authenticated user preferences, validate inputs, and apply query semantics from Task 01.
- Return at most 100 task rows, bounded row projections including filtered descendant counts,
  continuation context, totals, and clamped page metadata.
- Keep totals and entries in a consistent read transaction; batch row enrichment after page selection.
- Add measured indexes through normal migrations only when query-plan evidence requires them.
- Keep the existing task-list GET endpoint, archive flags, and its callers compatible.
- Use `/docs-maintainer` to document the additive query contract in the existing API reference at implementation.

## Out of scope

Client integration, transcript loading, persisted query snapshots, and new runtime flags.

## Acceptance

1. Authorized queries return complete filtered page semantics; invalid or unauthorized requests cannot broaden the result or expose rows.
2. A 10,000-task fixture returns at most 100 task rows with no per-task enrichment queries and no transcript reconstruction.
3. First and deep-page queries satisfy the design's benchmark gate on both supported database backends; record plans, timings, bytes, and allocations.

## TDD evidence

Add `TestSidebarTaskQuery` and `TestSidebarTaskPage` cases for page one,
Next/Previous, empty active/archived views, 99/100/101 task counts with extra headers, invalid input, oversized body, conflicting archive clauses,
missing summaries, page clamping after deletion, user isolation, and cross-workspace IDs.
Concatenate stable pages and compare every identity with Task 01's expected complete order.
Add `BenchmarkSidebarTaskPage100K` with first, middle, and final-page sub-benchmarks.
Seed realistic parent chains, repository combinations, summaries, and filters.
The default workspace route must fail the new full-view/page expectation before this endpoint exists.

## Risks

Offset cost and recursive ranking can dominate large queries. A bounded HTTP payload alone
is insufficient evidence. Never hide a slow plan by benchmarking only 100 independent roots.

## Verification

Run from the repository root. Install workspace dependencies first in a fresh worktree.
Set `KANDEV_TEST_POSTGRES_DSN` to a disposable PostgreSQL test database before this block.
Use `internal/testutil.OpenIsolatedPostgres` for isolated schemas and cleanup.
The new conformance/page tests and benchmark must run SQLite and PostgreSQL subcases.
A skipped PostgreSQL subcase is not a passing result.

```bash
: "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable PostgreSQL test DSN}"
(cd apps/backend && go test ./internal/task/repository/... ./internal/task/service ./internal/task/handlers -run 'SidebarTask|OnlyArchived' -count=1)
(cd apps/backend && go test ./internal/task/repository/sqlite -run '^$' -bench BenchmarkSidebarTaskPage100K -benchmem -benchtime=5x)
git diff --check
```

## Files likely touched

- `apps/backend/internal/task/handlers/task_handlers.go`
- `apps/backend/internal/task/handlers/sidebar_task_handlers.go (new)`
- `apps/backend/internal/task/handlers/sidebar_task_handlers_test.go (new)`
- `apps/backend/internal/task/service/sidebar_task.go (new)`
- `apps/backend/internal/task/dto/sidebar_task.go (new)`
- `apps/backend/internal/task/repository/sqlite/sidebar_task_query.go` (created by Task 01)
- `apps/backend/internal/task/repository/sqlite/sidebar_task_page_test.go (new)`
- `apps/backend/internal/task/repository/interface.go`

New files named in this work order are planned; existing parent directories are present.

## Dependencies

01-view-query-semantics.

## Parallelism

`sequential`

## Inputs

- [Shared size-based pagination](../../specs/ui/requirements/sidebar-task-pagination.md).

- [Requirements](../../specs/ui/requirements/sidebar-archived-filter.md).
- [System design](../../specs/ui/system-design/sidebar-archived-filter.md).
- [Plan and evidence](plan.md).

## Results

The authenticated query endpoint returns the bounded page projection and totals used by all
sidebar views. Repository, service, and handler cases passed against SQLite and PostgreSQL,
including filtering before paging and the shared view conformance fixture. The 100,000-task
first, middle, and final page benchmark passed in the prior session; warm reads measured about
0.85-0.95 seconds with responses around 60 KB.

Passed: `go test -p 1 ./internal/task/repository/... ./internal/task/service ./internal/task/handlers -run 'SidebarTask|OnlyArchived' -count=1` and `TMPDIR=/root/.cache/kandev-sidebar-pagination-go-tmp make -C apps/backend build`.

## View-independent pagination contract

Use one query/controller for built-in, saved, and draft views, active or archived.
Count displayable task rows after filters/collapse, excluding headings and context.
For 0 through 100 rows, hide paging controls. At 101 rows, show two pages.
Do not download all tasks to discover the threshold. Return the count with page one.
Successful Next/Previous replaces rows and scrolls only the task list to its top.
Failure preserves rows, page number, scroll position, and the selected conversation.
Live refresh can cross the threshold without selecting another task.
