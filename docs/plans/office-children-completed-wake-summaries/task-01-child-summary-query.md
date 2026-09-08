---
id: "01-child-summary-query"
title: "Make the child-summary query deterministic and live-only"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-WAKE-CHILD-SUMMARIES-003
  - REQ-OFFICE-WAKE-CHILD-SUMMARIES-004
acceptance_criteria:
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-001.5
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-003.1
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-003.1a
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-003.2
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-003.3
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-003.4
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-003.5
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-003.6
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-003.9
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-004.3
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-004.8
system_design:
  - ../../specs/office/system-design/children-completed-wake-summaries.md
---

# Task 01: Make the child-summary query deterministic and live-only

## Summary

Rewrite `Repository.GetChildSummaries` as a single statement that returns the
parent's live direct children in a deterministic order with a snapshot-consistent
live-child count, guards on the parent existing, and reads one code point past the
comment display limit so a cut comment is detectable. The query becomes the sole
source of the wake prompt's child list section.

## In scope

- Collapse the separate `COUNT(*)` and `SELECT` into one statement, with the count
  as a scalar subquery over the same predicate.
- Exclude archived children (`archived_at IS NULL`) from both the count subquery
  and the row predicate. Add no state predicate.
- Add `EXISTS (SELECT 1 FROM tasks p WHERE p.id = ?)` so a deleted parent yields
  no rows and a zero count.
- Order rows `t.created_at ASC, t.id ASC`; order the last-comment subquery
  `c.created_at DESC, c.id DESC`.
- Read `SUBSTR(c.body, 1, maxCommentChars+1)` instead of `maxCommentChars`.
- SQLite and PostgreSQL coverage for membership, order, cap, and truncation.

## Out of scope

- Rendering. This work order changes what the rows are, not how they read.
- The `PRLinks` data, which comes from `ListTaskPRsByTaskIDs`, not this query.
- Removing `ChildSummary.AssigneeAgentProfileID` or its `RunnerProjection` join.
  It has no consumer after Task 04, but deleting it is unrelated churn.
- Any change to `AreAllChildrenTerminal`, `GetChildSetKey`, or `ListChildStates`.

## Acceptance

- One statement returns rows and count together, so an archived child cannot
  leave a total above the cap while the capped select already covers every live
  child, and a parent with 20 or fewer live children never reports truncation.
- Rows are ordered by `created_at` then `id`, and each row's comment is the most
  recent by `created_at` then `id`, regardless of author; a non-terminal live
  child still appears and an archived child never does.
- A parent id with no `tasks` row returns no rows and a zero count even when child
  rows still carry that `parent_id`; the same statement runs unmodified on SQLite
  and PostgreSQL.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/office/repository/sqlite/ -run 'TestGetChildSummaries' -count=1
```

```bash
cd apps/backend && KANDEV_TEST_POSTGRES_DSN="$KANDEV_TEST_POSTGRES_DSN" go test -tags fts5 ./internal/office/repository/sqlite/ -run 'TestPostgresGetChildSummaries' -count=1
```

```bash
gofmt -l internal/office/repository/sqlite/blockers.go
```

## Files likely touched

- `apps/backend/internal/office/repository/sqlite/blockers.go`
- `apps/backend/internal/office/repository/sqlite/blockers_test.go`
- new: `apps/backend/internal/office/repository/sqlite/child_summaries_postgres_test.go`

## Dependencies

None.

## Risks

- `SUBSTR` counts code points on both backends while Go counts bytes. This work
  order owns the `+1` only; the comparison that consumes it is Task 02's, and both
  must treat 500 as a code-point limit or a complete comment gets marked truncated.
- The existing `TestGetChildSummaries` seeds no `archived_at` and no second
  comment per child, so it passes under both the old and new predicates. New
  fixtures are required for the archived, tiebreak, and orphaned-parent cases, or
  the work order ships green tests that prove nothing.
- The PostgreSQL twin needs the task repository's schema init before the office
  repository, mirroring `runs_inflight_postgres_test.go`; it skips without
  `KANDEV_TEST_POSTGRES_DSN`, so a local green run is not evidence the dialect
  parity criterion holds.

## Parallelism

`parallel-safe` with Task 02. Files are disjoint and no schema, migration, or
generated contract is shared; the only coupling is the shared 500-code-point
comment limit, stated in both work orders.

## Inputs

- System design, [Child query contract](../../specs/office/system-design/children-completed-wake-summaries.md#child-query-contract).
- `apps/backend/internal/office/repository/sqlite/blockers.go:287-336`.
- Postgres test pattern: `apps/backend/internal/office/repository/sqlite/runs_inflight_postgres_test.go`.
- Ordering precedent: `RunnerProjection` callers tiebreaking `position ASC, id ASC`.

## Results

Pending.
