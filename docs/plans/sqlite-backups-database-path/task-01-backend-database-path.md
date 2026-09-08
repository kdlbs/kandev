---
id: "01-backend-database-path"
title: "Route System backups to the SQLite path"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/system-page/requirements/system-page.md"
---

# Task 01: Route System backups to the SQLite path

## Intent

Make the System Database and Backups services use the live SQLite file path. Keep all snapshot families in one sibling `backups/` directory.

## Acceptance

- A custom SQLite filename is the reported database path and the restore destination.
- Manual, pre-reset, and pre-migration snapshots use the `backups/` directory beside that file.
- Restore quiesces scheduling, active executions, and database-backed workers, validates the checkpoint result, closes the SQLite pool, and uses rollback-capable quarantine replacement. PostgreSQL restore is rejected before staging or shutdown. A restart is required before database-backed work resumes.
- The default `<home>/data/kandev.db` behavior and existing backup safety rules remain unchanged.

## Files likely touched

- `apps/backend/internal/system/system.go`
- `apps/backend/internal/system/system_database_path_test.go`
- `apps/backend/internal/system/backups/store.go`
- `apps/backend/internal/system/backups/store_test.go`
- `apps/backend/internal/system/backups/path_test.go`
- `apps/backend/internal/system/database/stats.go`
- `apps/backend/internal/system/database/stats_test.go`
- `apps/backend/internal/system/database/reset.go`
- `apps/backend/internal/system/database/path_test.go`
- `apps/backend/internal/backendapp/restore_quiesce_test.go`

## Dependencies

None.

## Parallelism

Sequential. The constructors and the System composer share one path contract.

## Inputs

- Spec: `docs/specs/system-page/requirements/system-page.md`, Database, Backups, scenarios, failure modes, and persistence guarantees.
- Plan: `plan.md`, confirmed root cause and backend sections.
- Existing pre-migration rule: `apps/backend/internal/persistence/provider.go` derives `backups/` from the live database path.
- Issue: `https://github.com/kdlbs/kandev/issues/2679`.

## Verification

Write the custom-path regression cases first. Run them before production changes and make sure that they fail for the path split:

```bash
cd apps/backend && go test ./internal/system/... -run 'Test.*(Configured|Custom)DatabasePath' -count=1 -v
```

After the fix, run the custom-path regressions and all affected System packages:

```bash
cd apps/backend && go test ./internal/system/... -run 'Test.*(Configured|Custom)DatabasePath' -count=1 -v
cd apps/backend && go test ./internal/system/... -count=1
git diff --check
```

## Output contract

Report the exact changed files and test counts. Include the expected red failure, green results, and cleanup evidence. Update this task and `plan.md` in the same conversation.

## Results

- Red phase: the four custom-path regressions failed on the old path split. The
  composer listed the old default backup, restore could not reach the custom
  filename, stats missed the custom WAL and backup, and reset used the wrong
  parent directory.
- Green phase: `cd apps/backend && go test -race ./internal/system/backups
  ./internal/system ./internal/backendapp -run 'TestRestore|TestConfiguredDatabasePath|TestProvideWiresRestoreQuiesce|TestQuiesceForRestore' -count=1`
  passed 11 restore-safety tests across 3 packages, including restore quiescing.
- Full affected suite: `cd apps/backend && go test ./internal/system/... ./internal/backendapp -count=1`
  passed 844 tests across 20 packages.
- `cd apps/backend && make lint` passed with 0 issues.
- Changed files: `system.go`, `system_database_path_test.go`,
  `backups/store.go`, `backups/store_test.go`, `backups/path_test.go`,
  `database/stats.go`, `database/stats_test.go`, `database/reset.go`,
  `database/maintenance_test.go`, and `database/path_test.go`.
- `git diff --check` passed.

### Restore safety remediation

- Red phase: the PostgreSQL path closed the pool, busy checkpoint rows were ignored, and replacement failures deleted live sidecars.
- Green phase: restore safety tests pass for PostgreSQL rejection, busy checkpoint preservation, rollback of the main file and sidecars, and the complete quiescence callback.
