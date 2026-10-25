---
id: "01-stabilize-github-pr-refresh"
title: "Stabilize GitHub task-PR refreshes"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/sidebar-last-activity-sort.md"
---

# Task 01: Stabilize GitHub task-PR refreshes

Make REST feedback and batched GraphQL polling converge on one task-PR status.
Add safe evidence for any future semantic update.

## Acceptance

- GraphQL check-rollup values map to the documented task-PR check states.
- Equivalent REST and GraphQL snapshots do not persist or publish a second
  semantic task-PR update.
- A real update logs only changed field names with task and PR identity, then
  publishes the existing event once.

## Verification

```bash
cd apps/backend && go test ./internal/github -run 'CheckRollup|Equivalent.*Status|SyncTaskPR.*ChangedFields'
```

## Files likely touched

- `apps/backend/internal/github/graphql.go`
- `apps/backend/internal/github/graphql_test.go`
- `apps/backend/internal/github/service_pr_watch.go`
- `apps/backend/internal/github/service_pr_feedback_sync_test.go`
- `apps/backend/internal/github/service_pr_watch_batched_budget_test.go`

## Dependencies

None.

## Risks

- The regression must call both conversion paths before `SyncTaskPR`.
- Logs must not include provider field values, comments, titles, or tokens.
- Normalization must preserve the existing empty state for absent checks.

## Result

- RED: the focused tests failed because the normalization and changed-field
  helper were not present.
- GREEN: `go test ./internal/github -run
  'CheckRollup|Equivalent.*Status|SyncTaskPR.*ChangedFields'` passed (10 tests).
- Regression: `go test ./internal/github` passed (1572 tests).
