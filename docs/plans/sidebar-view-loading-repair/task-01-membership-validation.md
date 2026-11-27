---
id: "01-membership-validation"
title: "Correct sidebar membership validation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-002
acceptance_criteria:
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.3
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.14
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.17
system_design:
  - ../../specs/ui/system-design/sidebar-archived-filter.md
---

# Task 01: Correct sidebar membership validation

## Summary

Remove the aggregate 256-byte membership limit while retaining bounded input validation.
Return structured, safe validation details for localized consumer recovery.

## In scope

Separate scalar UTF-8 byte limits from membership cardinality (1,000 elements).
Preserve types, operators, empty arrays, 20 clauses, and 256 KiB body limits.
Add typed errors with reason/index/limit and backward-compatible `error` text.
Exercise combined parameter counts against real SQLite and PostgreSQL.

## Out of scope

Frontend presentation, caching, repository-order changes, settings migrations.

## Acceptance

1. Red first: `TestSidebarTaskViewQueryMembershipLimits` rejects the current
   implementation on seven workflow UUIDs and on repository-name arrays totaling
   over 256 encoded bytes; 1,000 entries then pass and
   1,001 fail explicitly. Test scalar 256/257 decoded bytes, escaped characters,
   Unicode, wrong types, empty arrays, 20/21 clauses, and oversized bodies.
2. `TestHTTPQuerySidebarTasksStructuredValidation` checks additive error shape,
   safe fields, malformed-input fallback, and existing authorization tests.
3. `TestQuerySidebarTaskPageLargeMembership` covers both dialects, `in` and
   `not_in`, maximum combined valid bindings, all selected memberships, and
   bounded/global page results. No truncation or query broadening.

## Verification

From repository root, with Go installed and a disposable PostgreSQL service:

```bash
: "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable PostgreSQL test DSN}"
(cd apps/backend && go test -tags fts5 ./internal/task/models -run TestSidebarTaskViewQuery -count=1)
(cd apps/backend && go test -tags fts5 ./internal/task/handlers -run TestHTTPQuerySidebarTasks -count=1)
(cd apps/backend && go test -tags fts5 ./internal/task/repository/sqlite -run 'TestQuerySidebarTaskPage|TestSidebarTaskViewConformance' -count=1)
git diff --check
```

Browser evidence is owned by Task 03. Do not classify skipped PostgreSQL tests as passed.

## Files likely touched

- `apps/backend/internal/task/models/sidebar_task_view.go`
- `apps/backend/internal/task/models/sidebar_task_view_test.go`
- `apps/backend/internal/task/handlers/sidebar_task_http.go`
- `apps/backend/internal/task/handlers/task_http_lifecycle_test.go`
- `apps/backend/internal/task/repository/sqlite/sidebar_task_query_test.go`

## Dependencies

None.

## Risks

Scalar byte interpretation differs from raw JSON length. Prove Unicode and escape
boundaries. Maximum filter count times collection cardinality affects SQL bindings.

## Parallelism

`sequential`

## Inputs

- [Design: Query contract](../../specs/ui/system-design/sidebar-archived-filter.md#query-contract).
- [Pagination requirement](../../specs/ui/requirements/sidebar-task-pagination.md).
- Existing model, handler, and `newRepoForSidebarConformance` test patterns.
- Backend scoped guidance and `.agents/skills/tdd/references/backend-tests.md`.

## Results

Passed: model membership regression failed before the fix; structured handler errors also failed before implementation. After correction, the combined `go test -p 2 -tags fts5` command for the three listed packages and named patterns passed, including SQLite and disposable PostgreSQL (9.379s repository run). Go 1.26.3 was available at `/usr/local/go/bin/go`; the earlier diagnostic PATH limitation is resolved. Added dedicated `sidebar_task_http_validation_test.go` and `sidebar_task_query_membership_test.go` files to keep existing files bounded.
