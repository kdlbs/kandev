---
id: "03-unified-history-compaction"
title: "Unified history compaction and backup"
status: completed
wave: 3
depends_on:
  - "02-durable-checkpoint-lifecycle"
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003
  - REQ-TASKS-TURN-CHANGES-004
acceptance_criteria:
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.8
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.10
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.11
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.12
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.10
  - AC-TASKS-TURN-CHANGES-004.6
  - AC-TASKS-TURN-CHANGES-004.12
system_design:
  - ../../specs/system-page/system-design/history-compaction.md
  - ../../specs/tasks/system-design/turn-change-preservation.md
---

# Task 03: Unified history compaction and backup

## Summary

Expand the existing compaction policy and job to old tool details and historical content. Preserve the existing preparation choice with complete SQL/Git backup and restore coverage.

## In scope

One versioned policy, union task discovery, shared cutoff, category cursors/results, activity/lease checks, SQL expiry, durable ref cleanup, owned-store reclamation, full-scope backup/restore, and approval migration.

## Out of scope

Separate policies, independent timers, unsupported-engine capability expansion, forced user-repository GC, and implicit VACUUM.

## Acceptance

1. One analysis and cleanup operation includes mixed-category and history-only tasks under one age/switch/schedule; protected work is retained.
2. Interrupted SQL/ref cleanup resumes without changing user refs, losing ownership, or double-counting removals. Summaries and expiry states survive.
3. The single backup preparation verifies and restores SQL plus uncommitted-only checkpoint content after source loss; old approvals require the existing preparation flow for expanded scope.

## Verification

Record a behavioral RED regression before production changes. Use real Git/database fixtures for source and cleanup claims.

```bash
cd apps/backend
go test -trimpath ./internal/system/toolretention ./internal/system/backups
go test -trimpath ./internal/task/changes ./internal/task/repository/sqlite ./internal/task/service -run 'HistoryCompaction|TurnGitHistory|TurnChangeRetention' -count=1
go test -trimpath -race ./internal/system/toolretention ./internal/system/backups ./internal/task/changes -run 'HistoryCompaction' -count=1
```

Run changed-package Go lint with actual paths. For web commands, install workspace dependencies from `apps/` if this worktree lacks them.
Use the configured PostgreSQL conformance fixtures where applicable, or record missing coverage explicitly.

## Files likely touched

- `apps/backend/internal/system/toolretention/service.go`
- `apps/backend/internal/system/toolretention/types.go`
- `apps/backend/internal/system/toolretention/scan.go`
- `apps/backend/internal/system/toolretention/runner.go`
- `apps/backend/internal/system/toolretention/scheduler.go`
- `apps/backend/internal/system/backups/store.go`
- `apps/backend/internal/system/backups/retention_preparation.go`
- `apps/backend/internal/system/maintenance/guard.go`
- `apps/backend/internal/task/repository/sqlite/turn_change_retention.go`
- `apps/backend/internal/task/service/archived_session_reconciliation.go`

Add focused adjacent regression files. Extract helpers rather than exceeding scoped size limits.

## Dependencies

02-durable-checkpoint-lifecycle

## Inputs

- [System design](../../specs/system-page/system-design/history-compaction.md)
- [System design](../../specs/tasks/system-design/turn-change-preservation.md)
- Root and scoped AGENTS guidance, relevant existing tests, and the accepted decision.

## Risks

A database-only backup is insufficient for deleted Git snapshots. Shared objects and user branches can prevent physical reclamation after ref removal.

## Parallelism

`sequential`

## Results

The single `tool_payload_retention` policy now owns both old tool details and historical content. History candidates use the shared conservative task-activity predicate, are selected as a task cohort, and are revalidated under the mutation transaction against activity, leases, and pending preservation.
Analysis and cleanup include history-only tasks under one cutoff, one switch, and the existing 24-hour schedule.
SQL expiry runs first, then exact expected-OID owned-ref cleanup with a durable receipt and bounded retry; leased reads, pending preservation, and backup preparation block destructive work. Once every ref for a backend-owned repository is released, owned packs and loose objects are reclaimed and actual bytes are reported.
The former fixed 30-day turn-content sweep is removed.
Legacy tool-only approvals migrate to the combined scope disabled and require the existing preparation choice once.
Retention and ordinary backups publish a complete paged checkpoint sidecar bound to the database image hash. Verification compares the manifest with the snapshot's own inventory, proves each pack is self-contained, and fails closed on a missing or truncated sidecar; restore re-imports it before database replacement. Download delivers a zip containing the database image and checkpoint sources. Task and session deletion stage independent owned-ref cleanup intents so cascades cannot orphan archive refs.

Behavioral evidence:

- `go test -trimpath ./internal/system/toolretention ./internal/system/backups`
- `go test -trimpath ./internal/task/changes ./internal/task/repository/sqlite ./internal/task/service -run 'HistoryCompaction|TurnGitHistory|TurnChangeRetention' -count=1`
- `go test -trimpath -race ./internal/system/toolretention ./internal/system/backups -count=1`
- `go test -trimpath -race ./internal/task/changes -run 'HistoryCompaction|PreservationRetrier|TurnGitHistory' -count=1`
- Focused regression cases cover task-cohort eligibility and mutation barriers, 1,001-item inventory paging, 300-interval multi-batch accounting, 150 receipt ref cleanup, deletion intents, owned-store reclamation, DB-image-bound backup verification, and bundle download.
- Desktop and mobile compaction E2E passed with mixed tool/history analysis details.

Remaining gaps: PostgreSQL-specific history retention cases were not run because the shared compaction policy remains SQLite-only; that engine keeps the legacy fixed-deadline path. The batch cursor remains message-primary with one bounded history batch per pass rather than a merged single cursor.
