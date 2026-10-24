---
id: "01-hold-blocked-issues"
title: "Hold Linear watch issues until blockers complete"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-LINEAR-BLOCKERS-001
acceptance_criteria:
  - AC-INTEGRATIONS-LINEAR-BLOCKERS-001.1
  - AC-INTEGRATIONS-LINEAR-BLOCKERS-001.2
  - AC-INTEGRATIONS-LINEAR-BLOCKERS-001.3
  - AC-INTEGRATIONS-LINEAR-BLOCKERS-001.4
  - AC-INTEGRATIONS-LINEAR-BLOCKERS-001.5
system_design:
  - ../../specs/integrations/system-design/linear-watch-blockers.md
---

# Task 01: Hold Linear Watch Issues Until Blockers Complete

## Summary

Store `requireBlockersDone` in the watch filter, read each issue's blockers
only for such watches, and drop issues with open or truncated blocker lists
before they are published.

## In scope

- `SearchFilter.RequireBlockersDone`, `LinearIssue.Blockers`,
  `LinearIssue.BlockersTruncated`, `LinearIssueBlocker.Done`, and
  `normalizeFilter`.
- `searchIssuesWithBlockersQuery` and `blockersFromRelations` in the GraphQL
  client; the mock client returns blockers only for blocker-aware searches.
- `CheckIssueWatch` pagination for blocker-aware watches,
  `holdBlockedIssues`, and the `linear_issue_watch_pending_issues` table with
  re-checks of pending issues by ID (`SearchFilter.IssueIDs`).

## Out of scope

- The dialog setting (Task 02).

## Acceptance

1. Plain searches send the existing query; blocker-aware searches add a
   bounded `inverseRelations` block and never send the flag to Linear.
2. A poll publishes only issues whose blockers are all completed, holds
   canceled, cross-team, and truncated cases, releases an issue on the first
   poll after completion, and pages past held issues. A held issue outside
   the search window is re-checked by ID and released, in bounded batches,
   and a released issue stays pending until dispatch reserves it.
3. A flag-only filter is rejected as empty; a watch with the flag off keeps
   its single plain search.

## Verification

```bash
(cd apps/backend && go test -race -tags=fts5 -count=1 ./internal/linear/)
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/integrations/linear-watcher-blockers.spec.ts
```

## Files likely touched

- `apps/backend/internal/linear/{models,graphql_client,mock_client,store,store_issue_watch,service_issue_watch,service_issue_watch_blockers}.go`
- `apps/backend/internal/linear/blockers_test.go`, `blockers_revisit_test.go`
- `apps/web/e2e/helpers/api-client.ts`, `apps/web/e2e/tests/integrations/*blockers*`

## Dependencies

None.

## Parallelism

`sequential`

## Results

Author-reported: `go test -race -tags=fts5 ./internal/linear/` passes,
including the GraphQL request, mapping, hold, release, pagination, and
default-path tests in `blockers_test.go` and the pending-issue re-check and
store tests in `blockers_revisit_test.go`. The desktop Playwright dispatch case
creates a task for the unblocked mock issue only, then for the held issue after
its blocker is marked completed.
