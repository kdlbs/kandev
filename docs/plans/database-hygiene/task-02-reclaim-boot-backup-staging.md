---
id: "02-reclaim-boot-backup-staging"
title: "Reclaim abandoned boot backup staging directories"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-SQLITE-FILE-HYGIENE-002
acceptance_criteria:
  - AC-PLATFORM-SQLITE-FILE-HYGIENE-002.1
  - AC-PLATFORM-SQLITE-FILE-HYGIENE-002.2
  - AC-PLATFORM-SQLITE-FILE-HYGIENE-002.3
  - AC-PLATFORM-SQLITE-FILE-HYGIENE-002.4
system_design:
  - ../../specs/platform/system-design/sqlite-file-hygiene.md
---

# Task 02: Reclaim boot backup staging

## Summary

Add one exported sweep in `internal/persistence` for `.kandev-backup-*`
directories that have shown no modification for six hours. Call it on every
SQLite boot and from the system backups sweep.

## In scope

- `internal/persistence/snapshot.go`: the prefix constant, the six-hour
  constant, `SweepStaleBootStaging(backupDir, log)`, and the newest
  modification time over a directory and its direct entries.
- `internal/persistence/provider.go`: use the prefix constant in
  `createPreMigrationBackup`. Call the sweep once per SQLite boot when the
  backups directory exists, before the backup decision.
- `internal/system/backups/store.go`: call the sweep at the start of
  `sweepStaleTmpFiles`.
- Tests next to both packages:
  - an old staging directory is removed and a published backup beside it stays;
  - a staging directory whose child file is recent stays even when the
    directory itself is old;
  - a recent staging directory and an old `.snapshot-` directory are not
    removed by the boot sweep;
  - a boot that takes no new backup still removes an old staging directory;
  - creating a system backup removes an old staging directory.

## Out of scope

Backup retention counts, `.snapshot-` and `.tmp` rules, restore, and the
startup progress steps.

## Acceptance

- The new tests fail on the base branch (the staging directory survives) and
  pass after the change.
- A sweep failure logs a warning and never fails boot or backup creation.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/persistence -run '^Test(SweepStaleBootStaging|ProvideSQLiteReclaimsBootStaging)' -count=1 -v)
(cd apps/backend && go test -tags fts5 ./internal/system/backups -run '^TestCreate_SweepsStaleBootStaging' -count=1 -v)
(cd apps/backend && go vet ./internal/persistence/... ./internal/system/backups/...)
```

## Likely files

- `apps/backend/internal/persistence/snapshot.go`
- `apps/backend/internal/persistence/provider.go`
- `apps/backend/internal/persistence/snapshot_test.go`
- `apps/backend/internal/system/backups/store.go`
- `apps/backend/internal/system/backups/store_test.go`

## Results

Implemented on 2026-10-12, verified on Windows 11 with Go 1.26.

- Red: on the base commit, `TestProvideSQLiteReclaimsBootStagingWithoutBackup`
  failed with `.kandev-backup-111 survived`, and
  `TestCreate_SweepsStaleBootStaging` failed with `abandoned boot staging not
  swept`.
- Green: both pass, together with `TestSweepStaleBootStagingUsesNewestEntry`
  (an old directory with a recent staged file stays),
  `TestSweepStaleBootStagingToleratesMissingDirectory`, and the existing
  `TestCreate_SweepsStaleTmpSidecar` and `TestCreate_PreservesRecentTmpSidecar`.
- `TestProvide_TakesSnapshotOnUpgrade` still creates the upgrade backup through
  the prefix constant. On Windows it stops at its POSIX `0600` mode assertion
  (`snapshot mode = 0666`), which this change does not touch.
- `go vet ./internal/persistence/... ./internal/system/backups/...` passes.
- Limits: debris younger than six hours stays, including debris from a crash
  just before the restart; a later boot or backup creation reclaims it.

### Review remediation

Added non-root POSIX filesystem regressions for listing, inspection, and removal
warnings, including the path and underlying error. Listing failures now warn;
an absent directory remains quiet. Additional tests cover concurrent sweeps and
staging-prefix symlinks without following their targets. The permission cases
skip on Windows and when run as root; the local contained proof exercises them
as UID 1000. The listing-error and missing-directory tests also run as root.

Focused command:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/persistence -run '^TestSweepStaleBootStaging' -count=1 -v)
```

Local verification uses exact production function slices and these regression
tests in the isolated task sandbox, with only the logger boundary replaced.
Full package, SQL guard, lint, and database verification run in hosted CI.
