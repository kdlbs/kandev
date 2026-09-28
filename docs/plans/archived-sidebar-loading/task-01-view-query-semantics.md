---
id: "01-view-query-semantics"
title: "Define complete sidebar view queries"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-001
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-002
acceptance_criteria:
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-001.2
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-001.3
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-001.4
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-001.5
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-001.8
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-001.14
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.3
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.4
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.5
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.10
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.11
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.12
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.13
system_design:
  - ../../specs/ui/system-design/sidebar-archived-filter.md
---

# Task 01: Define complete sidebar view queries

## Summary

Define and implement relational sidebar-view evaluation before any client switches to it.
This establishes the semantics needed to paginate complete views without silently losing matches.

## In scope

- Add typed query validation and SQL query construction in focused task repository modules.
- Create shared JSON conformance fixtures and a frontend harness using the current `applyView` engine.
- Implement all dimensions/operators, groups, sorts, complete tree aggregation, collapse, pins,
  stable input ordinals, and manual child order. Include SQLite and PostgreSQL dialect handling.
- Produce ordered entry identities and context before row enrichment. Use literal matching,
  strict timestamp comparison, and the documented database text ordering. Keep recursive traversal cycle-safe.

## Out of scope

HTTP registration, client page loading, saved-view migrations, and agent lifecycle changes.

## Acceptance

1. Database results match shared expected fixtures for every existing view operation, including mixed-state trees and filtered ancestors.
2. A descendant after source row 100 affects its parent's rank before the first page is selected; cycle fixtures terminate deterministically.
3. The implementation performs filtering/ranking in the database and exposes no all-task hydration fallback.

## TDD evidence

Add `TestSidebarTaskViewConformance` and frontend `sidebar-view-conformance.test.ts`.
First prove that applying `applyView` to only the first 100 source rows misses a matching
later task and misranks a parent. Keep that fixture as the complete-view expected result.
The production query test must fail before the new implementation is connected.
Cover Unicode titles, every negative operator, missing summaries, nanosecond ties,
repository combinations, filtered parents, deep descendants, custom order, and collapsed groups.
Fixtures specify canonical source order; do not derive expected values from the new SQL.

## Risks

SQLite/PostgreSQL JSON and collation differences can change membership or ordering.
Do not approve a dialect implementation from SQL-string assertions alone.
Use the repository's existing database test harness for executable dialect coverage.

## Verification

Run from the repository root. Install workspace dependencies first in a fresh worktree.
Set `KANDEV_TEST_POSTGRES_DSN` to a disposable PostgreSQL test database before this block.
Use `internal/testutil.OpenIsolatedPostgres` for isolated schemas and cleanup.
The new conformance/page tests and benchmark must run SQLite and PostgreSQL subcases.
A skipped PostgreSQL subcase is not a passing result.

```bash
: "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable PostgreSQL test DSN}"
(cd apps/backend && go test ./internal/task/repository/... -run 'SidebarTaskViewConformance' -count=1)
(cd apps/web && pnpm exec vitest run lib/sidebar/sidebar-view-conformance.test.ts lib/sidebar/apply-view.test.ts)
git diff --check
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/sidebar_task_query.go (new)`
- `apps/backend/internal/task/repository/sqlite/sidebar_task_query_test.go (new)`
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/repository/sidebar_task_query.go (new)`
- `apps/web/lib/sidebar/sidebar-view-conformance.test.ts (new)`
- `apps/web/lib/sidebar/apply-view.ts`
- `apps/web/lib/sidebar/task-tree-activity.ts`
- `apps/web/lib/sidebar/effective-task-tree-state.ts`
- `apps/backend/internal/task/repository/testdata/sidebar-task-views.json (new)`

New files named in this work order are planned; existing parent directories are present.

## Dependencies

None.

## Parallelism

`sequential`

## Inputs

- [Shared size-based pagination](../../specs/ui/requirements/sidebar-task-pagination.md).

- [Requirements](../../specs/ui/requirements/sidebar-archived-filter.md).
- [System design](../../specs/ui/system-design/sidebar-archived-filter.md).
- [Plan and evidence](plan.md).

## Results

The shared JSON conformance fixture is consumed by both the frontend `applyView` harness and
the repository query test. The frontend assertion confirms that evaluating the full source
finds the late title match that evaluating only the first 100 source rows misses. The backend
conformance test passed against SQLite and a disposable PostgreSQL schema.

Passed: `go test -p 1 ./internal/task/repository/... ./internal/task/service ./internal/task/handlers -run 'SidebarTask|OnlyArchived' -count=1`, the focused sidebar conformance/frontend tests, and `git diff --check`. The production query and its regression tests were present before this implementation session; no new TDD-red result is claimed here.

## View-independent pagination contract

Use one query/controller for built-in, saved, and draft views, active or archived.
Count displayable task rows after filters/collapse, excluding headings and context.
For 0 through 100 rows, hide paging controls. At 101 rows, show two pages.
Do not download all tasks to discover the threshold. Return the count with page one.
Successful Next/Previous replaces rows and scrolls only the task list to its top.
Failure preserves rows, page number, scroll position, and the selected conversation.
Live refresh can cross the threshold without selecting another task.
