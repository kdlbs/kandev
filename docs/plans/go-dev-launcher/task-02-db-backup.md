---
id: "02-db-backup"
title: "Production database backup in Go"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/go-dev-launcher.md"
---

# Task 02: Production database backup in Go

Port `apps/cli/src/backup.ts` so `make dev-prod-db` stays safe on the Go path.

## Acceptance

- `isProductionDb(path)` is false only when the normalized path contains a
  `.kandev-dev` segment; everything else is treated as production.
- `backupProductionDb(dbPath, homeDir, now)` copies the DB to
  `<homeDir>/.kandev/data/backups/dev-prod-db-<RFC3339-without-separators>.db`, stamps
  the file's mtime to `now` so pruning is deterministic, and returns the created path;
  it returns an empty path and no error when the source DB does not exist.
- Pruning keeps the five newest `dev-prod-db-*.db` files by mtime and never touches
  files outside that prefix/suffix pair (notably the backend's own `kandev-*` and
  `manual-*` snapshot families).

## Verification

Use TDD. Add `apps/backend/internal/launcher/backup_test.go` with an injectable
`homeDir` and `now` so back-to-back calls produce distinct names without sleeping, then:

~~~bash
cd apps/backend && go test ./internal/launcher/ -run 'TestIsProductionDb|TestBackupProductionDb|TestPruneBackups' -race
~~~

## Files

- `apps/backend/internal/launcher/backup.go` (new)
- `apps/backend/internal/launcher/backup_test.go` (new)

## Inputs

- `apps/cli/src/backup.ts` — the reference implementation, including the comment
  explaining why backups always land under `~/.kandev/data/backups/` even when `dbPath`
  points elsewhere (custom `KANDEV_DATABASE_PATH` is advanced usage where the user owns
  the backup location).
- `apps/cli/src/backup.test.ts` — existing cases worth mirroring.

## Risks

- Failing to remove one old backup must not fail the launch; failing to *create* the
  backup must. Keep the two error paths distinct.
- A plain file copy of a live SQLite DB is what the TypeScript version does and what
  this task reproduces; do not silently upgrade it to `VACUUM INTO` here — that is the
  backend's boot-time backup path and a different guarantee.
