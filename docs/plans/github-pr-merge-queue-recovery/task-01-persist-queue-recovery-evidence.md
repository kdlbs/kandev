---
id: "01-persist-queue-recovery-evidence"
title: "Persist queue recovery evidence"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-001.1
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-001.2
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-001.3
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-001.4
system_design:
  - ../../specs/integrations/system-design/github-pr-merge-queue-recovery.md
---
# Task 01: Persist Queue Recovery Evidence

## Summary

Extend the GitHub PR snapshot with a stable head SHA, active queue identity,
and the latest removal event. Persist complete observations across every
TaskPR write path.

## In scope

- Extend the batched GraphQL query and conversion.
- Add populated guards for queue entry and removal history.
- Add the TaskPR and SQLite fields from the system design.
- Preserve fields across create, replace, restore, and sync paths.
- Add focused GraphQL, service, and real-store tests.

## Out of scope

- Auto-fix classification or dispatch.
- Auto-merge and requeue decisions.
- Frontend rendering.

## Acceptance

- A poll can detect a removal after the active entry disappears.
- Queue-unaware reads preserve the complete recovery snapshot.
- The snapshot survives every TaskPR write path and a backend restart.

## Verification

```bash
go test -tags fts5 ./internal/github -run 'Test(PRFieldsBlockRequestsMergeQueueRecovery|ConvertBatchedPRResultPreservesMergeQueueRecovery|SyncTaskPR.*MergeQueueRecovery|TaskPRMergeQueueRecoverySchema)'
```

Run the command from `apps/backend`.

## Files likely touched

- `apps/backend/internal/github/graphql.go`
- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/service_pr_watch.go`
- `apps/backend/internal/github/graphql_merge_queue_recovery_test.go`
- `apps/backend/internal/github/service_pr_merge_queue_recovery_test.go`
- `apps/backend/internal/github/store_merge_queue_recovery_test.go`

## Dependencies

None.

## Risks

- A missing populated guard can erase a valid timeline observation during a
  REST feedback refresh.
- Schema column lists have several create, replace, and restore paths.

## Parallelism

`sequential`

## Inputs

- Integration requirements 001 and system-design provider sections.
- Existing merge-queue status persistence and outcome-attribution patterns.

## Results

- Added GraphQL head, active queue-entry, and removal-event observations with
  populated guards.
- Persisted and restored the recovery snapshot through TaskPR create, replace,
  update, restore, and sync paths, including stale-event protection.
- Added GraphQL, service, and SQLite regression coverage.
- Verification: `go test -tags fts5 ./internal/github` passed (1,660 tests).
- Verification: `go test -race -tags fts5 ./internal/github` passed (1,660 tests).
