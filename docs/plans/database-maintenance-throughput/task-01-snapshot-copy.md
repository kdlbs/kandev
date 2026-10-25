---
id: "01-snapshot-copy"
title: "Copy consistent SQLite backups"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-SQLITE-SNAPSHOT-001
  - REQ-PLATFORM-STARTUP-LIFECYCLE-001
acceptance_criteria:
  - AC-PLATFORM-SQLITE-SNAPSHOT-001.1
  - AC-PLATFORM-SQLITE-SNAPSHOT-001.2
  - AC-PLATFORM-SQLITE-SNAPSHOT-001.3
  - AC-PLATFORM-SQLITE-SNAPSHOT-001.4
  - AC-PLATFORM-STARTUP-LIFECYCLE-001.7
system_design:
  - ../../specs/platform/system-design/sqlite-snapshot-copy.md
---

# Task 01: Copy consistent SQLite backups

## Summary and scope

Add native incremental backup using the existing driver. Migrate all backup-only
callers while preserving full verification, atomic installation and startup gating.
Keep explicit physical compaction on VACUUM INTO. Exclude backup reuse and live repair.

## Acceptance and tests

1. TestPreMigrationBackupCopiesWithoutRebuilding in persistence/snapshot_copy_test.go
   proves WAL content, schema, free-page preservation, full integrity and private
   permissions through the actual startup backup path (.001.1/.001.3/.001.4).
2. TestCopySQLiteSnapshotMemoryAndFailureCleanup covers memory sources, escaped
   paths, existing-target preservation, cancellation and failed-copy cleanup (.001.1/.001.2/.001.3).
   Existing owning-package tests cover upgrade gating, restore/reset, repair and adoption.
3. Existing TestRunExecuteWithCompactReplacesDatabaseAndPreservesRollback and compact
   failure tests retain physical-compaction coverage (.001.4). No synthetic driver
   failure seam or additional compaction fixture is introduced.

## Files likely touched

- apps/backend/internal/db/sqlite_backup.go
- apps/backend/internal/persistence/snapshot.go, provider.go, sqlite_selection.go, snapshot_copy_test.go and existing tests
- apps/backend/internal/system/backups/retention_preparation.go and tests
- apps/backend/internal/system/database/reset.go and tests
- apps/backend/internal/maintenance/backup.go, run.go and existing compact tests
- apps/backend/internal/task/inventoryrepair/journal.go and tests
- docs/public/operations.md

## Verification

Run from repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/db ./internal/persistence ./internal/system/backups ./internal/system/database ./internal/maintenance ./internal/task/inventoryrepair)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
git diff --check
```

## Dependencies and parallelism

None. Sequential in the primary session.

## Inputs and risks

[Snapshot design](../../specs/platform/system-design/sqlite-snapshot-copy.md), its
requirements and linked ADR; existing private staging and inspection helpers.
The driver returns false,nil for BUSY as well as progress. Never publish based only
on sampled remaining pages or skip Finish errors. Copying freelist pages cannot
replace explicit compact. Preserve existing destinations on every failure path.

## Results

Passed: the listed race tests for db, persistence, backups, database, maintenance
and inventoryrepair; SQL guard; persistence store conformance (53.2 seconds); targeted
changed-package golangci-lint (0 issues). The new startup regression first failed on
free-page rebuilding, then passed with native copy. All listed documentation and diff
gates passed. Existing explicit compaction tests passed. No live database mutation.

## CI remediation

Agentctl cross-builds use CGO_ENABLED=0. The native Backup API is absent from that
driver build. Isolate the implementation behind the cgo build constraint and supply
an explicit unsupported stub for non-CGO helpers. The failing E2E/preview build was
reproduced with CGO_ENABLED=0 go build -trimpath ./internal/db. Passed: CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o /tmp/kandev-4433-agentctl ./cmd/agentctl; native persistence backup regressions under -race (1.04 seconds).

Review coverage: TestCopySQLiteSnapshotCancelsLockedCopy uses fake time and a real
SQLite write transaction to reach the native LOCKED retry, cancel the active copy,
assert partial-file removal and source-connection release, then retry successfully
and verify integrity. TestCopySQLiteSnapshotAllowsActiveWALWriter proves a committed
snapshot can complete while a separate WAL writer has uncommitted work. Both passed
under -race (1.04 seconds); fixtures contain only one or two rows.

Persistent contention now fails after five seconds with no copy progress, including
callers using context.Background. TestCopySQLiteSnapshotStopsStalledCopy first failed
by reaching its caller deadline, then passed with the finite stall bound. The failure
includes a clear no-progress reason and removes the incomplete destination.

Post-remediation verification passed: persistence and deliveryretention race suites,
non-CGO linux/amd64 agentctl build, and full backend golangci-lint against the PR base
(0 issues). Remote checks remain pending for the next pushed head.
