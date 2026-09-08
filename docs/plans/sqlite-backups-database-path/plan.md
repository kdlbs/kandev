---
spec: docs/specs/system-page/requirements/system-page.md
created: 2026-08-15
status: complete
---

# Implementation Plan: SQLite backups follow the database path

## Overview

Resolve the live SQLite path once in the System composer. Pass that exact path to the Database and Backups services. Both services derive one sibling `backups/` directory from the path.

This change aligns manual, pre-reset, and pre-migration snapshots. It also makes restore replace the configured file, including a custom filename, after quiescing the SQLite pool and removing stale WAL sidecars.

## Confirmed root cause

`persistence.Provide` opens `cfg.Database.Path` and writes pre-migration snapshots under `filepath.Dir(dbPath)/backups`. `system.Provide` instead passes `cfg.ResolvedDataDir()` to the Database and Backups services.

Both System services then reconstruct `<data-dir>/kandev.db`. They do not receive the live path. This split causes list, create, download, restore, database statistics, and pre-reset snapshots to use the wrong location.

Manual create still snapshots the live writer pool. Its contents are current, but its output location is wrong. Restore replaces a different file and cannot affect the live custom database.

A focused test configured `<custom>/custom-name.db` and placed `manual-1.db` under `<custom>/backups/`. `system.Provide(...).Backups.List()` returned an empty list. The command failed at the expected assertion:

```bash
cd apps/backend && go test ./internal/system -run '^TestReproIssue2679BackupsFollowDatabasePath$' -count=1 -v
```

The throwaway test was removed after reproduction.

## Backend

### System composition

Update `apps/backend/internal/system/system.go` to select the SQLite database path once. Use `cfg.Database.Path` when it is set. Otherwise, use `<ResolvedDataDir>/kandev.db`.

Pass the exact file path to `database.NewService` and `backups.NewService`. Keep home-owned reset directories unchanged.

### Backups service

Update `apps/backend/internal/system/backups/store.go` so `Service` owns `databasePath`. Derive the backup directory with `filepath.Join(filepath.Dir(databasePath), "backups")`.

Use `databasePath` directly for restore staging and replacement. Do not infer the filename `kandev.db`.

Before replacement, quiesce scheduling, active executions, and database-backed workers. Validate the `wal_checkpoint(TRUNCATE)` result, close the shared SQLite pool, quarantine the main file and its `-wal`/`-shm` sidecars, and restore the quarantine if staged installation fails. Reject PostgreSQL restore before staging or shutdown. The frontend requires a restart after restore, so the closed pool prevents stale WAL frames from replaying onto the restored file.

### Database service

Update `apps/backend/internal/system/database/stats.go` so `NewService` accepts the exact database path. Derive its backup directory from the path.

Use the exact path for the Database page, WAL statistics, last-backup time, and pre-reset snapshots. Update `apps/backend/internal/system/database/reset.go` to use the derived backup directory.

## Frontend

Update the restore success dialog to state that the pool is closed and database-backed work is unavailable until restart. Use the existing restart action, with quit-and-relaunch guidance when automatic restart is unavailable.

## Public documentation

Remove the warning that System Database and Backups ignore `KANDEV_DATABASE_PATH`. Document the sibling backup directory and exact restore target.

Update the CLI reference, configuration reference, operations guide, feature-status table, and introductory storage text. State that old misrouted snapshots do not move automatically.

Update `apps/backend/AGENTS.md` so the backend backup convention names the configured SQLite path. Do not change the historical ADR.

## Tests

- **What:** the composed backup handler lists snapshots beside a custom database path.
  **File:** `apps/backend/internal/system/system_database_path_test.go`.
  **How:** construct `system.Provide` with a custom filename, register the routes, request the backup list, and assert the sibling snapshot appears.
- **What:** restore stages and replaces the exact custom database filename.
  **File:** `apps/backend/internal/system/backups/path_test.go`.
  **How:** restore known bytes through the service and assert the configured file changes, the quiescence hook runs, the pool closes, the replacement sidecars are gone, and the default-home file remains absent. Add regressions for PostgreSQL rejection, busy checkpoint preservation, and rollback after replacement failure.
- **What:** restore quiescence stops every database writer before pool closure.
  **File:** `apps/backend/internal/backendapp/restore_quiesce_test.go`.
  **How:** assert scheduling, orchestrator, agents, and registered workers are all attempted and all stop errors are returned.
- **What:** the restore success dialog directs the user to restart.
  **File:** `apps/web/components/settings/system/system-confirmation-dialogs.test.tsx`.
  **How:** render a succeeded restore job and assert the restart action is present while the normal close action is absent.
- **What:** database statistics and pre-reset snapshots use the custom path and its sibling backup directory.
  **File:** `apps/backend/internal/system/database/path_test.go`.
  **How:** assert the reported path, WAL lookup, last-backup time, and pre-reset snapshot parent.
- **What:** default-path behavior remains unchanged.
  **Files:** existing Backups and Database package tests.
  **How:** update constructor inputs to exact database paths and run all `internal/system/...` tests.

## E2E Tests

No new browser test is planned. The dialog keeps the existing shared responsive composition; this remediation changes localized success copy and invokes the existing restart flow. The focused component test covers the changed rendered action.

## Verification Results

- `cd apps/backend && go test -race ./internal/system/backups ./internal/system ./internal/backendapp -run 'TestRestore|TestConfiguredDatabasePath|TestProvideWiresRestoreQuiesce|TestQuiesceForRestore' -count=1`
  passed 11 restore-safety tests across 3 packages.
- `cd apps/backend && go test ./internal/system/... ./internal/backendapp -count=1`
  passed 844 tests across 20 packages.
- `cd apps/backend && make lint` passed with 0 issues.
- `cd apps && pnpm --filter @kandev/web exec vitest run components/settings/system/system-confirmation-dialogs.test.tsx`
  passed 5 UI tests.
- `cd apps && pnpm --filter @kandev/web run i18n:check`, `i18n:ratchet`, and `typecheck`
  passed.
- `rg -n 'KANDEV_DATABASE_PATH|data/backups|database path|backup caveat' docs/public apps/backend/AGENTS.md`
  confirmed the corrected terminology and intentional default-path references.
- `node --test scripts/validate-public-docs.test.mjs` passed 61 tests.
- `node scripts/validate-public-docs.mjs` validated 41 published docs pages.
- `git diff --check` passed.

### Restore safety remediation

- Red phase: PostgreSQL restore was accepted, a busy checkpoint was treated as success, and sidecars were deleted before a replacement failure could roll back.
- Green phase: focused backend restore, system wiring, and quiescence tests pass after rejecting PostgreSQL, validating checkpoint rows, and using rollback-capable quarantine replacement.

## Implementation Waves And Parallel Candidates

Wave 1 (sequential):

- [x] [task-01-backend-database-path](task-01-backend-database-path.md)

Wave 2 (sequential):

- [x] [task-02-document-database-path](task-02-document-database-path.md)

The documentation task depends on the final backend behavior. No task is parallel-safe.

## Risks

- Existing manual snapshots can remain under the old default directory. Automatic movement is unsafe because Kandev cannot prove which database produced each file.
- A custom database filename must remain exact. Joining `kandev.db` to its parent preserves part of the bug.
- The System Status disk-usage service remains home-based. It does not count sibling backups outside the resolved home.

## Out of scope

- Moving or merging old snapshot directories.
- Changing snapshot filename families or retention rules.
- Changing PostgreSQL backup behavior.
- Changing the System Status disk-usage path model.
