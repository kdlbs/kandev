---
id: "01-query-adapter"
title: "Shared query and temporary result adapter"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MOBILE-LIST-VIEWS-001
  - REQ-UI-MOBILE-LIST-VIEWS-002
  - REQ-TASKS-WORKSPACE-SEARCH-001
acceptance_criteria:
  - AC-UI-MOBILE-LIST-VIEWS-001.3
  - AC-UI-MOBILE-LIST-VIEWS-001.5
  - AC-UI-MOBILE-LIST-VIEWS-002.2
  - AC-UI-MOBILE-LIST-VIEWS-002.3
  - AC-UI-MOBILE-LIST-VIEWS-002.6
  - AC-UI-MOBILE-LIST-VIEWS-002.7
  - AC-UI-MOBILE-LIST-VIEWS-002.8
  - AC-TASKS-WORKSPACE-SEARCH-001.1
  - AC-TASKS-WORKSPACE-SEARCH-001.2
  - AC-TASKS-WORKSPACE-SEARCH-001.3
  - AC-TASKS-WORKSPACE-SEARCH-001.5
system_design:
  - ../../specs/ui/system-design/mobile-list-saved-views.md
  - ../../specs/tasks/system-design/workspace-search.md
---

# Task 01: Shared query and temporary result adapter

## Summary

Support phone List refinements through the existing bounded sidebar query.
Preserve literal workspace search and full shared ordering before pagination.
This work introduces no visible control changes.

## In scope

- Add optional request-only `query` to Go/TS sidebar query types and validation.
- Reuse workspace search fields, escaping, repository deduplication, and dialect handling.
- Include search/refinements in count, rank, page, cache identity, and request ownership.
- Add optional consumer scope through the sidebar hooks without changing default callers.
- Add a pure mobile query adapter for saved scope, ordinary refinements, and explicit links.
- Keep nonempty workspace search on bounded server evaluation; preserve empty-query local coverage.
- Preserve archive command intent and legacy sort/group/facet validation without saved-view writes.

## Out of scope

Phone markup, saved schema changes, new comparators, controller duplication,
startup backfills, and a new global search abstraction.

## Acceptance

1. Literal search and the complete running/color/activity chain produce correct
   totals and pages, including a match outside the initial 100 rows.
2. Temporary/link scope changes query identity without mutating saved views,
   drafts, unrelated settings, or another consumer's active query.
3. Existing default sidebar consumers retain their local/server behavior and
   authorization guards. Real PostgreSQL behavior has its own reported result.

## Verification

Use TDD. Record a behavioral RED assertion before production edits. A missing
file or selector is not the intended RED evidence.

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/task/models ./internal/task/handlers -run 'TestSidebarTask(ViewSearchValidation|QuerySearch)' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run 'Test(SidebarTaskPageWorkspaceSearch|PostgresSidebarTaskPageWorkspaceSearch|SidebarColorQueryKey)' -count=1)
pnpm --dir apps/web exec vitest run lib/tasks/mobile-list-query.test.ts hooks/domains/kanban/use-sidebar-task-page.test.tsx hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts
git ls-files --modified --others --exclude-standard -z -- 'apps/backend/**/*.go' | xargs -0 -r bash scripts/lint-go-changed
git diff --check
```

Planned Go names are `TestSidebarTaskViewSearchValidation`,
`TestSidebarTaskQuerySearch`, `TestSidebarTaskPageWorkspaceSearch`, and
`TestPostgresSidebarTaskPageWorkspaceSearch`. Require actual matching tests;
a zero-test result cannot pass. Use existing real database fixtures.
An unset `KANDEV_TEST_POSTGRES_DSN` is a skip, not a PostgreSQL pass.

Unit cases include `%`, `_`, backslash, description/repository-only matches,
and multiple repository matches. Also cover contradictory saved/result filters,
link overrides, missing capabilities, page reset, stale replies, and no settings writes.

## Files likely touched

- `apps/backend/internal/task/models/sidebar_task_view.go` and a focused search test file.
- `apps/backend/internal/task/handlers/sidebar_task_http.go` and a focused search test file.
- `apps/backend/internal/task/repository/sqlite/sidebar_task_query.go`, `sidebar_task_query_base.go`, and focused search tests.
- `apps/backend/internal/task/repository/sqlite/task.go` only for local predicate reuse.
- `apps/web/lib/types/http.ts`.
- `apps/web/hooks/domains/kanban/use-sidebar-page-context.ts`.
- `apps/web/hooks/domains/kanban/use-sidebar-task-page.ts`.
- `apps/web/hooks/domains/kanban/use-workspace-sidebar-tasks.ts`.
- `apps/web/hooks/domains/kanban/use-local-sidebar-page.ts` and affected coverage/cache helpers.
- New `apps/web/lib/tasks/mobile-list-query.ts` and `mobile-list-query.test.ts`.

## Dependencies

None. Existing workspace view storage and composable sidebar ranking are prerequisites already in the source.

## Risks

A title-only replacement weakens search. A client-only page filter misses later
matches. A missing query-key field can show another scope's rows. Preserve
SQLite/PostgreSQL behavior and authenticated preference ownership.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/mobile-list-saved-views.md), 001 and 002.
- [Design](../../specs/ui/system-design/mobile-list-saved-views.md), Shared query through Search transport and Compatibility.
- [Workspace search](../../specs/tasks/system-design/workspace-search.md), planned phone adapter.
- Existing literal-search tests and sidebar page/coverage tests.

## Results

Completed. RED evidence: the HTTP decoder rejected `query`, and the repository result count included nonmatching rows. GREEN: literal workspace search now reaches the bounded sidebar query before counting/ranking/paging; punctuation, descriptions, repository metadata, duplicates, late-page matches, and the full running/color/activity chain are covered. Query identity and consumer scoping prevent stale or cross-consumer results; the pure adapter preserves saved settings.

Checks passed:
- Backend model/HTTP focused tests.
- SQLite query and ordering tests; PostgreSQL counterpart skipped because `KANDEV_TEST_POSTGRES_DSN` is unset.
- Four focused Vitest files, 40 tests.
- Changed Go lint: 0 issues.
- `git diff --check`.

### Review remediation (2026-10-27)

- Row projection now uses the resolved request filters, keeping explicit
  archive scope separate from saved-view state.
- Local inventory coverage now proves the composed request scope before it
  skips the bounded page request. Workflow and archive overrides fall back to
  the server when existing snapshots cannot prove completeness.
- `archive=all` now sends boolean array values accepted by the shared filter
  contract. The first E2E run exposed the invalid string-boolean payload.
- Focused hook and adapter tests pass, including changed workflow coverage and
  active-only inventory with an archived override. The phone E2E archive-link
  regression passes for `archived`, `all`, and `active` over a differently
  scoped saved view.

These are request and projection changes only; saved views and drafts are not
modified by a link override.
