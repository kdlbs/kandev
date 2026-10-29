---
id: "02-cleanup-snapshot-target"
title: "Keep the completed cleanup-snapshot target on the broadened pass"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-002
acceptance_criteria:
  - AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-002.1
  - AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-002.2
  - AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-002.3
  - AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-002.4
  - AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-002.5
  - AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-002.6
  - AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-006.3
  - AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-006.4
system_design:
  - ../../specs/system-page/system-design/db-history-retention.md
---

# Task 02: Keep the completed cleanup-snapshot target on the broadened pass

## Summary

The completed cleanup-job snapshot target already exists in
`internal/system/dataretention` (`scanJobBatch`, `reduceCleanupJob` in
`scan.go`). This work order keeps it pinned on the same bounded periodic pass as
the broadened message target from work order 01 and verifies its preservation and
failure contracts with tests. No new reducer, table, or migration is added.

## Scope

- Confirm and, where needed, adjust so the cleanup-snapshot target runs in the
  same pass as the message target, sharing one batch budget so neither starves
  the other (AC-002.6).
- Candidate selection and reduction reuse the existing helpers:
  - Walk `task_resource_cleanup_jobs` in `succeeded` state with a readable
    `completed_at` at or before the cutoff (`SucceededCleanupJobEligible`,
    `NextSucceededCleanupJobID`).
  - Read the snapshot (`CleanupJobSnapshotRow`), reduce through
    `models.ReduceCleanupSnapshot`, and write through
    `WriteReducedCleanupSnapshot` under the succeeded-state compare-and-swap.
  - Preserve `archive_source_manifest`, `workspace_id`, and `worktrees`
    byte-for-byte; skip and count a malformed snapshot without deleting it.
- Pin ownership to the resource-snapshot column of a completed task-resource
  cleanup job; touch no other snapshot, session metadata, transcript, or run.

## Exclusions

- No generalization to other task snapshots, session metadata, or run records.
- No change to other `task_resource_cleanup_jobs` states or columns, or to the
  job's trigger, state, attempts, timestamps, or result summary.
- No change to the tool payload record, window, or approved state.
- No new settings record, revision stream, or settings card.
- No new table, index, or schema migration.

## Acceptance conditions

1. A succeeded in-window job is reduced with the evidence contract preserved and
   the same workspace authorization and manifest matching as before reduction; a
   non-succeeded or out-of-window job is retained.
2. A malformed snapshot is skipped and counted, never deleted, and a job that
   leaves the succeeded state is not overwritten.
3. The cleanup-snapshot target and the message target share one pass and one
   batch budget.

## Likely files

- `apps/backend/internal/system/dataretention/scan.go` (reused; shared budget)
- `apps/backend/internal/task/repository/sqlite/archived_data_retention_scan.go`
  (reused)
- `apps/backend/internal/task/models/cleanup_snapshot_retention.go` (reused,
  unchanged)
- New `*_test.go` files next to the changed sources

## Verification

```bash
cd apps/backend && go test ./internal/system/dataretention/... ./internal/task/repository/sqlite/... -count=1
cd apps/backend && go test ./internal/system/dataretention/... ./internal/task/repository/sqlite/... -race -count=1
cd apps/backend && go build ./... && go vet ./...
cd apps/backend && gofmt -l internal/system/dataretention internal/task/repository/sqlite
```

## Results

Reused without code change. The cleanup-snapshot target already runs in the same
`scanBatch` pass as the message target, sharing one `used` byte budget, and its
`ReduceCleanupSnapshot`/`WriteReducedCleanupSnapshot`/`SucceededCleanupJobEligible`
helpers are unchanged. Covered by the existing dataretention pass test (same
pass) and the reducer's own unit tests.
