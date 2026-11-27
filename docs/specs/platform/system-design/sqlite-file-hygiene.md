---
status: current
system: platform
requirements:
  - REQ-PLATFORM-SQLITE-FILE-HYGIENE-001
  - REQ-PLATFORM-SQLITE-FILE-HYGIENE-002
---

# SQLite File Hygiene System Design

## Purpose and boundaries

This design bounds two on-disk files that the shared SQLite persistence layer
creates: the writer's WAL file and the private staging directory of a
pre-migration backup. It changes no schema, no transaction mode, and no backup
retention rule.

The design uses but does not own:

- The writer and reader factories in `internal/db` and their transaction
  admission, described in
  [ADR 2026-10-05](../../../decisions/2026-10-05-sqlite-writer-transaction-admission.md).
- The pre-migration backup and its retention, described in
  [ADR 0008](../../../decisions/0008-db-upgrade-safety.md).
- The system backups service (`internal/system/backups`), which owns manual and
  retention backups, their `.snapshot-` staging, and `.tmp` sidecars.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-SQLITE-FILE-HYGIENE-001` | [Writer WAL bound](#writer-wal-bound) |
| `REQ-PLATFORM-SQLITE-FILE-HYGIENE-002` | [Boot backup staging reclaim](#boot-backup-staging-reclaim) |

## Writer WAL bound

A checkpoint copies WAL frames into the database but rewrites the WAL in
place. Without `journal_size_limit`, SQLite never truncates the file, so it
keeps its high-water mark. With the limit set, SQLite truncates the WAL to at
most the limit when it resets the log.

`journal_size_limit` is per-connection state and has no go-sqlite3 DSN
parameter. Setting it once on a `*sql.DB` would not survive the pool replacing
its connection. `internal/db` therefore registers a separate writer driver
name whose `sqlite3.SQLiteDriver.ConnectHook` runs
`PRAGMA journal_size_limit = 67108864` on every new writer connection.
`OpenSQLite` opens that driver name with an unchanged DSN. `OpenSQLiteReader`
keeps the stock `sqlite3` driver because read-only connections never reset the
WAL.

The hook calls the pragma through the standard `driver.ExecerContext`
interface. The cgo `SQLiteConn` implements it. The non-cgo go-sqlite3 stub does
not implement it, and its driver cannot open a database, so the hook does
nothing there. This keeps one code path and no build tags, and the
`CGO_ENABLED=0` agentctl cross-builds keep compiling.

Callers wrap the returned `*sql.DB` with `sqlx.NewDb(..., "sqlite3")`. sqlx
takes the dialect name from that argument, not from the registered driver
name, so dialect detection and query rebinding are unchanged. The pragma is a
SQLite-only statement inside the SQLite factory, so the SQL guard records one
exact `sqlite-catalog` exemption for the hook symbol.

64 MiB leaves room for a write burst well over the default 1,000-page
autocheckpoint (about 4 MiB) while bounding the file that remains on disk.
The limit is a size bound at reset time, not a per-write budget.

## Boot backup staging reclaim

`createPreMigrationBackup` writes its snapshot copy into a private
`.kandev-backup-*` directory and removes it with a deferred `os.RemoveAll`. A
crash or kill between `os.MkdirTemp` and that cleanup leaves the directory and
a full database copy in `<database dir>/backups`. The existing system-backups
sweep only matches `.snapshot-*` directories and `.tmp` sidecars.

`persistence` owns the prefix and exposes one sweep. It lists the backups
directory and, for each directory with the `.kandev-backup-` prefix, takes the
newest modification time of the directory and its direct entries. It removes
the directory only when that time is more than six hours old.

The newest-child rule exists because a directory's own modification time does
not change while the snapshot copy grows a file inside it. The six-hour window is
the liveness guard. The backups directory sits beside the database file, so
two backend processes that use different database files in one directory
share it, and the runtime-state ownership lock does not exclude the other
process's live backup. A boot backup is one pre-migration step, so six hours
is far beyond any plausible run.

The sweep runs at two call sites:

1. `provideSQLite`, on every SQLite boot, after the meta read and before the
   backup decision. It is skipped when the backups directory does not exist.
2. `backups.Service.sweepStaleTmpFiles`, before it applies its own `.snapshot-`
   and `.tmp` rules, while snapshot creation holds maintenance admission.

Running on every boot, not only on upgrade boots, matters because a crash
during the backup leaves the stored version unchanged. The restart is again
an upgrade boot, but that debris is minutes old and the guard keeps it. The
next boot or backup creation after the window removes it.

## Failure and recovery

The WAL pragma runs while the connection opens. If it fails, the driver
returns the error from `Open`, and the pool reports it like any other
connection failure. Startup fails instead of running without the bound.

A sweep never fails boot or backup creation. An absent backups directory is a
quiet no-op. Other listing failures log a warning with the directory path and
error, then return. If it cannot inspect or remove one staging
directory, it logs a warning with the path and the error, then continues with
the other entries. A directory or direct entry that disappears during inspection
is a quiet no-op. Symlinks with the staging prefix are skipped; direct-entry
timestamps are read with `Lstat` without following symlink targets. A partially
removed directory keeps its prefix, so a later sweep retries it after the
liveness window.

## Persistence

Neither change adds tables, migrations, or settings. The limit and the
six-hour window are compile-time constants. Published backups keep their
existing names and their retention rule (the last two pre-migration backups).

## Observability

A reclaimed or already vanished staging directory is not logged. Listing,
inspection, and removal failures have distinct warnings with the affected path
and error. The WAL limit can be read back with
`PRAGMA journal_size_limit` on a writer connection.

## Related decisions

- [ADR 0008: DB upgrade safety](../../../decisions/0008-db-upgrade-safety.md)
- [ADR 2026-10-05: SQLite writer transaction admission](../../../decisions/2026-10-05-sqlite-writer-transaction-admission.md)
