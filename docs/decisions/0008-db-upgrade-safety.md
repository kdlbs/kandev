# 0008: DB upgrade safety - meta table, pre-migration backup, migration logging

**Status:** accepted
**Date:** 2026-05-16
**Area:** backend

Spec: [db-upgrade-safety](../specs/db-upgrade-safety/spec.md)

## Context

Kandev ships a single binary that auto-applies SQLite schema changes on every boot. Migrations are imperative `runMigrations()` functions across four repositories (`task`, `office`, `workflow`, `agent/settings/store`), each doing dozens of `_, _ = r.db.Exec("ALTER TABLE ...")` with errors swallowed. Today this has three concrete problems:

1. **No observability.** A successful ALTER and an idempotent re-attempt both produce zero log output. When a user reports a post-upgrade issue we cannot tell from their logs which migrations ran on their box.
2. **No safety net.** A bad release can drop a column, half-rebuild a table, or corrupt the file. There is no pre-migration snapshot and the data dir holds the user's entire kandev history.
3. **No version tracking.** The DB has no record of which kandev binary version last touched it, so we cannot key any of the above off "is this an upgrade boot?".

The merge of `feature/orchestrate` into main is the immediate trigger: it ships ~30 new tables plus ~20 column ALTERs on existing main tables. Doing it without a snapshot and without an observable trail is unacceptable for a tool storing weeks of agent work.

We considered four shapes:

- **Adopt golang-migrate (or similar).** Numbered SQL files, embedded migrations, `schema_migrations` table. Cleanest long-term but invasive: every existing migration must be converted, and the current "idempotent ALTER per release" pattern is well-matched to the codebase's pace.
- **Backup only, no logging changes.** Solves the data-loss risk but leaves us blind to which release introduced which schema change. Cheap but punts.
- **Logging only, no backup.** Improves observability but doesn't prevent loss when something does go wrong.
- **Meta + snapshot + per-statement logging.** Keeps the existing migration code shape, adds one tracking table, one VACUUM INTO call, and one logging wrapper per `r.db.Exec` line. This is what we picked.

## Decision

Add a `kandev_meta` table owned by the persistence layer, take a `VACUUM INTO` snapshot before any boot that detects an upgrade, and wrap every swallow-error `db.Exec` in a `MigrateLogger.Apply(name, stmt)` helper that logs applied/failed/silent classifications.

### kandev_meta table

```sql
CREATE TABLE IF NOT EXISTS kandev_meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);
```

Keys:
- `kandev_version` - the binary version that last successfully completed boot. Empty on fresh installs.
- `schema_initialized_at` - RFC3339 timestamp of the first-ever init.

The persistence provider creates the table and reads `kandev_version` between opening the writer connection and handing the pool to repositories. After all repositories finish `initSchema`, `cmd/kandev/storage.go` writes the current binary version into `kandev_version`.

The "write the version only after every repo init succeeds" rule is intentional: if a repo's `initSchema` panics or errors, the stored version stays at the previous release, and the *next* boot will detect the upgrade again and re-attempt - safe because every migration is idempotent.

### Pre-migration backup

When the persistence provider detects an upgrade boot (stored version differs from current binary version, or stored is empty and user tables exist), it runs:

```sql
VACUUM INTO '<dataDir>/backups/kandev-<oldVersion>-<UTCRFC3339>.db'
```

Properties:
- atomic and consistent (includes WAL frames)
- runs on the writer connection while no repo has touched the DB yet
- size proportional to live data
- single file, openable directly with the `sqlite3` CLI

Backups live at `<KANDEV_HOME_DIR>/data/backups/`. Retention: **last 2** files; older ones are deleted after a successful snapshot. Restore is manual: stop the backend, copy the snapshot over `kandev.db`, delete `kandev.db-wal` and `kandev.db-shm`, restart.

**Backup failure is fatal.** On `VACUUM INTO` error, `persistence.Provide` closes the pool and returns the error. The backend refuses to boot. Rationale: the dominant failure mode is disk-full or permission-denied, both of which would also leave a half-applied migration in an unrecoverable state. Surfacing the failure is better than charging ahead.

Two backups (not five): a SQLite kandev DB on a healthy install is single-digit MB to low tens of MB. Two snapshots is enough to cover "current release introduced a bug" and "previous release also had a bug" without unbounded disk growth on machines that pull new builds daily.

### Migration logging

A `MigrateLogger` helper lives in `internal/db/migratelog.go`:

```go
func (m *MigrateLogger) Apply(name, stmt string) {
    if _, err := m.db.Exec(stmt); err != nil {
        if isAlreadyExists(err) {
            return
        }
        m.log.Warn("migration failed", zap.String("name", name), zap.Error(err))
        return
    }
    m.log.Info("migration applied", zap.String("name", name))
}
```

`isAlreadyExists` matches the SQLite error strings `"duplicate column name"` and `"already exists"`. Anything else is a real failure and logs at WARN.

Each of the four repositories gains a `*logger.Logger` field threaded through its `NewWithDB`. Existing swallow-error ALTERs are rewritten mechanically:

```go
// before
_, _ = r.db.Exec(`ALTER TABLE tasks ADD COLUMN origin TEXT DEFAULT 'manual'`)

// after
r.migrate.Apply("tasks.origin", `ALTER TABLE tasks ADD COLUMN origin TEXT DEFAULT 'manual'`)
```

The four recreate-table migrations (`migrateTaskPriorityToText`, `migrateSessionsRemoveAgentExecutionID`, `migrateTaskEnvironmentsRemoveAgentExecutionID`, `recreateAgentProfilesWithoutModelCheck`) already gate on a trigger phrase in the stored DDL; they get a single `log.Info("migration applied", ...)` at the point the gate fires.

### Postgres path

Postgres deployments skip backup with a log line:

```
INFO  pre-migration backup skipped: postgres driver (use pg_dump)
```

Postgres callers are by definition operators running their own DBA flow. No `kandev_meta` row is written on the Postgres path; version tracking is SQLite-only.

## Consequences

### Wins

- A user reporting a post-upgrade bug attaches their log; we can read off the exact list of `migration applied` lines to know what changed on their machine.
- A bad release is no longer destructive: the user has the previous-shape DB sitting in `backups/` and a documented one-line restore.
- The `kandev_meta` table is also the obvious place to land future boot-state tracking (last-clean-shutdown, feature-flag overrides per DB, etc.) without re-litigating the schema choice.
- The migration code keeps its current shape - no framework, no migration files, no numbering scheme to maintain.

### Costs

- Boot time on an upgrade grows by one `VACUUM INTO` (proportional to DB size; ~50ms on a typical kandev DB).
- Disk usage on the data volume grows by ~2x DB size (two backups). On a 20MB kandev DB this is 40MB. Acceptable.
- Refuse-to-boot on backup failure is a new failure mode users can hit. Mitigated by logging the underlying error clearly (disk full / permission denied / data dir read-only) and documenting it in the upgrade troubleshooting section.
- Every new ALTER ADD COLUMN going forward must be added via `migrate.Apply` rather than the bare `db.Exec` shortcut. This is a 1-character difference at the callsite; trivial to enforce in code review.

### Reversibility

Reverting this ADR means deleting the meta table, the snapshot helper, and the `MigrateLogger`. The four repositories revert to their swallow-error pattern. No data migration is needed - `kandev_meta` is read-only opaque to every other repo and SQLite tolerates leftover tables.

## Alternatives considered

### Adopt golang-migrate

Cleaner long-term: numbered SQL files, embedded migrations, atomic per-file transactions, automatic `schema_migrations` table. Rejected for this round because:
- migrating ~80 existing ALTERs into numbered files is a multi-day refactor for marginal benefit
- the per-statement granularity we get from idempotent ALTERs is already fine for kandev's pace (one release a week, two or three new columns)
- the `MigrateLogger` gets us the observability win without the refactor

We may revisit this once kandev has more contributors writing migrations.

### Backup only, no logging changes

Solves data loss. Leaves us blind to which release introduced a schema bug. The marginal cost of adding logging is one helper file and a mechanical sed across four repos; not enough to defer.

### Logging only, no backup

Solves observability. Leaves the user one bad release away from losing weeks of agent work. Not acceptable for a tool that stores irreplaceable conversation history.

### Five backups instead of two

Suggested in the original draft. Pulled back to two: kandev users typically pull new builds daily (npm / Homebrew autoupdate), and five snapshots at that cadence is 5x DB size on disk for a week of redundancy that nobody asked for. Two covers the "bad release lands, user notices on the next launch, wants to roll back" case which is the only realistic restore path.

### Restore command in the backend

Out of scope. The restore is a three-line shell procedure and lives in upgrade docs. Adding a `kandev restore --from <snapshot>` subcommand is solvable later if users actually need it.

### UI surface for upgrade history

A "Last upgrade" panel in settings showing the version pair, the migration names that applied, and a "Download backup" link is appealing. Out of scope for this ADR - backend logs are sufficient for the first cut, and the UI surface is opt-in work that doesn't change the persistence-layer contract.

## References

- `internal/persistence/provider.go` - where the backup hook lands
- `internal/task/repository/sqlite/base.go` - representative target for migration logging
- SQLite docs: [VACUUM INTO](https://www.sqlite.org/lang_vacuum.html#vacuuminto)
