---
status: current
system: platform
requirements:
  - REQ-PLATFORM-SQLITE-SNAPSHOT-001
  - REQ-PLATFORM-STARTUP-LIFECYCLE-001
---

# SQLite snapshot copy design

## Decision and ownership

Use the existing mattn/go-sqlite3 native Backup API for backup-only consumers.
[The decision](../../../decisions/2026-10-12-separate-sqlite-backups-from-compaction.md)
partially supersedes ADR 0008's snapshot mechanism. Persistence owns verified
publication; internal/db owns the driver-specific bounded copy primitive.

## Copy lifecycle

Add CopySQLiteSnapshot and CopySQLiteSnapshotContext in persistence, backed by
internal/db/sqlite_backup.go. Pin source and destination sql.Conn handles and use
Raw to obtain SQLiteConn values. Create the destination exclusively in caller-owned
private staging, with DELETE journaling and FULL synchronization. Reject an existing
destination without truncation. Keep source and destination distinct.

Call destination.Backup("main", source, "main"). Bound each Step by approximately
4 MiB using the measured source page size. Check context between steps. The driver
returns false,nil for both progress and BUSY/LOCKED; compare remaining/page counts
and use a cancellable retry delay for no progress. Abort after five consecutive
seconds without progress, using the existing SQLite busy-timeout bound, even when
the caller provides no deadline. Progress resets the stall timer. Completion requires the
API's done result, not a zero sampled counter. Always Finish and propagate its error;
close pinned handles before validation or publication. Cancellation and copy errors
remove only owned incomplete output. Driver mismatch fails closed. Native copy is built only with CGO; non-CGO helper
binaries retain a stub that reports SQLite backup unavailable without creating a file.

Online callers use a reader/dedicated source connection, avoiding holding the single
application writer for the entire backup. Startup and offline callers may use their
existing pool while application mutation is gated. The native backup API owns WAL
consistency and writer coordination. Never copy the live main/WAL files directly.
Keep existing shared maintenance admission for backup versus retention.

For an autocommit WAL source connection, establish a deferred read transaction
and read the schema before starting the copy. This pins one committed snapshot
while independent WAL writers continue to commit. Without the read transaction,
each independent commit can restart the incremental copy, leaving the same page
counts until the existing stall bound expires. Release only the transaction opened
by the copy, using a cleanup context independent of cancellation, before returning
the connection to its pool. Preserve already-open caller transactions, non-WAL
journal modes, the bounded step size and the existing contention timeout.

## Consumer migration and compaction

Switch backup-only calls in persistence/provider.go, persistence/sqlite_selection.go,
system/backups/retention_preparation.go, system/database/reset.go,
maintenance/backup.go and task/inventoryrepair/journal.go to the new copy helper.
Audit all SnapshotSQLite references when implementing to catch callers added on main.

Keep SnapshotSQLite(Context) and its existing VACUUM INTO implementation for
maintenance/run.go's explicit compact callback. Do not globally substitute page copy
for that API: a page copy preserves free pages and cannot implement --compact.
Legacy adoption retains its existing private copy, verification and atomic install.

## Verification and readiness

Existing inspectSQLiteCandidate full integrity_check remains mandatory. Preserve
0600 artifacts, private staging, atomic no-replace installation, cleanup and backup
rotation. Copy success alone cannot release startup's migration/readiness gate.
No new schema, settings, runtime flag or backup format is needed. Existing startup
phase reporting continues; do not invent percentages or ETA from file size.

## Compatibility and requirement mapping

| Boundary | Behavior and evidence |
| --- | --- |
| SQLite file/WAL/in-memory (.001.1) | Small native-copy fixtures include committed WAL rows and exact schema/data |
| Context/contention (.001.2) | Controlled step/retry cancellation, concurrent source writer and handle release |
| Publication (.001.3) | Existing-target sentinel, failed copy/Finish/integrity, permissions and source preservation |
| Startup (.001.4; STARTUP-LIFECYCLE-001.7) | Upgrade snapshot before migration, failure blocks, same-version boot skips |
| Explicit compaction (.001.4) | Deleted-page fixture proves VACUUM path still shrinks |
| PostgreSQL | Unchanged operator-managed pg_dump; SQLite helper rejects unsupported drivers |

## Implementation plans

[Database maintenance throughput](../../../plans/database-maintenance-throughput/plan.md).
