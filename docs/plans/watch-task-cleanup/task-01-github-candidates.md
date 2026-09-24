---
id: "01-github-candidates"
title: "Filter GitHub cleanup candidates"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-WATCH-CLEANUP-001
acceptance_criteria:
  - AC-INTEGRATIONS-WATCH-CLEANUP-001.1
  - AC-INTEGRATIONS-WATCH-CLEANUP-001.2
  - AC-INTEGRATIONS-WATCH-CLEANUP-001.3
  - AC-INTEGRATIONS-WATCH-CLEANUP-001.4
  - AC-INTEGRATIONS-WATCH-CLEANUP-001.5
system_design:
  - ../../specs/integrations/system-design/watch-task-cleanup.md
---

# Task 01: Filter GitHub cleanup candidates

## Summary

Filter GitHub cleanup candidates. Implement with TDD and record the regression results.

## In scope

Add mixed-state regressions in `store_task_cleanup_test.go` and
`service_review_cleanup_test.go`. Name the new tests
`TestCleanupCandidateListsExcludeHistoricalTasks` and
`TestCleanupMergedReviewTasksSkipsHistoricalTasks`.
Cover by-watch, global, orphan, and workspace cleanup for reviews and issues.
Assert zero upstream calls for archived and missing tasks. Cover terminal and
nonterminal empty reservations, all policies, engagement, and lifecycle prompts.

Apply the candidate predicate to all four GitHub list methods. Preserve
explicit watch deletion with unfiltered task-ID queries in `service_reviews.go`
and `service_issues.go`. Add archived-child deletion assertions to existing
watch deletion tests. Assert deduplication remains present through unfiltered
lookups, not filtered cleanup queries.

Update obsolete missing-task cleanup expectations to zero provider calls and
zero deletions. Keep task-not-found race coverage by deleting an initially
eligible task after selection.

## Out of scope

New policies, UI, schema migrations, and unrelated provider changes.

## Acceptance

- Mixed candidate lists and store-backed cleanup satisfy AC 001.1 through 001.4.
- Explicit watch deletion, reset, scope, and deduplication satisfy AC 001.5.
- Regression tests fail before the query changes and pass afterward.

## Verification

Run from the repository root.

```bash
(cd apps/backend && go test ./internal/github -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/service_reviews.go`
- `apps/backend/internal/github/service_issues.go`
- `apps/backend/internal/github/store_task_cleanup_test.go`
- `apps/backend/internal/github/service_review_cleanup_test.go`
- `apps/backend/internal/github/service_cleanup_policy_test.go`
- `apps/backend/internal/github/store_watch_reset_test.go`
- `apps/backend/internal/github/store_watch_disable_test.go`

## Dependencies

None.

## Risks

Preserve task inventory outside cleanup. Seed actual active tasks in fixtures.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/watch-task-cleanup.md)
- [Design](../../specs/integrations/system-design/watch-task-cleanup.md)
- Existing source and tests listed above.

## Results

Implemented the task eligibility predicate for all four GitHub cleanup list
queries. Explicit watch deletion keeps its unfiltered task-ID inventory, so
archived and missing task rows are preserved for deduplication while active
tasks and empty reservations remain cleanup candidates. Added mixed-state
store and service regressions, updated active-task fixtures, and retained a
post-selection task-not-found race regression.

Verification: `go test ./internal/github -count=1` passed.
