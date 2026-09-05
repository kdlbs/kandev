---
id: "01-canonical-watch-migration"
title: "Migrate PR watches to task ownership"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/pr-watch-and-storage-bounds.md"
---

# Task 01: Migrate PR watches to task ownership

## Intent

Make `github_pr_watches` canonical at task/repository scope and safely collapse legacy session duplicates without breaking Review monitoring.

## Acceptance

- Searching and discovered watch identities enforce the spec keys; provenance `session_id` cannot create a second canonical watch.
- Upgrade migration snapshots before destructive work, is transactional/idempotent, removes only duplicates/orphans, prefers discovered rows, and preserves newest status/check/review/comment fields.
- Active listing excludes invalid/orphaned watches but retains a non-archived Review task whose provenance session is completed.

## Files likely touched

- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/store_multi_repo_test.go`
- `apps/backend/internal/github/store_watch_reset_test.go`
- `apps/backend/internal/github/store_task_cleanup_qa_test.go`
- `apps/backend/internal/github/service_task_events.go`
- `apps/backend/internal/persistence/snapshot.go`
- New focused GitHub migration tests beside `store.go`

## Dependencies

None.

## Parallelism

Sequential. This task owns the shared watch schema and migration contract.

## Verification

Write the duplicated-legacy and completed-session Review regressions first. Run targeted tests before and after implementation:

```bash
cd apps/backend && go test ./internal/github -run 'Test.*(PRWatch|WatchMigration|BackfillPRWatches|Review).*' -count=1 -v
cd apps/backend && go test ./internal/persistence ./internal/github -count=1
git diff --check
```

## Output contract

Report migration before/after fixture counts, the expected red failure, backup boundary, exact files, and test results. Update task and plan status in the same conversation.

## Results

**Status: done.**

`github_pr_watches` now uses task-owned canonical identities. Searching rows
are unique by `(task_id, repository_id, branch)` and discovered rows by
`(task_id, repository_id, pr_number)` through partial unique indexes;
`session_id` remains optional provenance and cannot create another watch.
Service lookups, create/ensure behavior, and collision-safe PR-number updates
all use those task-owned keys, so concurrent and resumed sessions converge on
one row.

The startup migration runs transactionally after the existing SQLite snapshot
boundary. It removes watches for missing tasks or detached repositories,
clears missing-session provenance, prefers a discovered row while merging
newest status/check/review/comment watermarks, resolves cross-branch PR-number
collisions, and is idempotent on a second boot. A completed provenance session
does not make a non-archived Review task ineligible for monitoring.

Focused migration and service regressions cover 50 historical sessions
collapsing to one watch, discovered-row preference, PR-number collisions,
orphan cleanup, provenance clearing, database-enforced uniqueness,
create/ensure deduplication, and completed-session Review monitoring. The
later sustained-load fixture additionally proves those 50 historical sessions
still produce one canonical polling target per cycle.

Verification:

~~~text
go test ./internal/github -run 'Test.*(PRWatch|WatchMigration|BackfillPRWatches|Review).*' -count=1 -v
go test ./internal/persistence ./internal/github -count=1
golangci-lint run ./internal/github/...
~~~

All commands passed when this wave was delivered; Task 07's integrated load
validation and the final PR remediation rerun the affected canonical-watch
coverage against the current branch head.
