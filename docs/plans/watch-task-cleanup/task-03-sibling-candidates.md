---
id: "03-sibling-candidates"
title: "Filter sibling provider cleanup candidates"
status: done
wave: 3
depends_on: ["02-github-rate-limits"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-WATCH-CLEANUP-001
acceptance_criteria:
  - AC-INTEGRATIONS-WATCH-CLEANUP-001.1
  - AC-INTEGRATIONS-WATCH-CLEANUP-001.2
  - AC-INTEGRATIONS-WATCH-CLEANUP-001.4
  - AC-INTEGRATIONS-WATCH-CLEANUP-001.5
system_design:
  - ../../specs/integrations/system-design/watch-task-cleanup.md
---

# Task 03: Filter sibling provider cleanup candidates

## Summary

Filter sibling provider cleanup candidates. Implement with TDD and record the regression results.

## In scope

Add `TestCleanupCandidateListsExcludeHistoricalTasks` and
`TestCleanupSkipsHistoricalTasks` to the GitLab and Azure DevOps packages.
Create `gitlab/service_cleanup_test.go` beside the existing
`service_cleanup_reason_test.go` and `service_cleanup_mr_automation_test.go`.
Use their existing store and cleanup test fixtures. Cover active, archived,
missing, and empty-task rows together. Assert no remote status calls for
historical tasks and preserve active-task policies.

Filter GitLab review and issue queries in `store_watches.go`, including global,
workspace, and by-watch methods. Filter Azure DevOps work-item and PR list
queries in `watch_store.go`. Preserve workspace and generation constraints.
GitLab and Azure DevOps continue to skip empty reservations in cleanup.
Do not change sibling rate-limit handling or task deletion adapters.

## Out of scope

New policies, UI, schema migrations, and unrelated provider changes.

## Acceptance

- Both provider packages exclude historical tasks from upstream cleanup calls.
- Active policy behavior and workspace/generation boundaries remain intact.
- Empty reservations and reset inventory preserve existing semantics.

## Verification

Run from the repository root.

```bash
(cd apps/backend && go test ./internal/gitlab ./internal/azuredevops -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/gitlab/store_watches.go`
- `apps/backend/internal/gitlab/store_task_cleanup_test.go`
- `apps/backend/internal/gitlab/service_cleanup_test.go`
- `apps/backend/internal/azuredevops/watch_store.go`
- `apps/backend/internal/azuredevops/watch_store_test.go`
- `apps/backend/internal/azuredevops/watch_cleanup_test.go`

## Dependencies

02-github-rate-limits.

## Risks

Preserve task inventory outside cleanup. Seed actual active tasks in fixtures.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/watch-task-cleanup.md)
- [Design](../../specs/integrations/system-design/watch-task-cleanup.md)
- Existing source and tests listed above.

## Results

Implemented the shared task eligibility predicate for GitLab review and issue
candidate queries, including global, workspace, and by-watch reads. Azure
DevOps work-item and pull-request reads now apply the same archived-or-missing
task filter while preserving watch generation scoping. Empty reservations stay
visible to cleanup, which continues to skip them before provider status calls.

Added mixed-state store regressions and instrumented cleanup tests for both
providers. Active task policies still delete terminal records, while archived
and missing task IDs generate zero provider calls. Updated SQLite fixtures to
provide the central task table and seed active task rows where cleanup reads
task eligibility.

Verification: `go test ./internal/gitlab ./internal/azuredevops -count=1`
passed.
