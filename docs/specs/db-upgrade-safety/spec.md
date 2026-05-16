---
status: draft
created: 2026-05-16
owner: cfl
---

# DB Upgrade Safety

## Why

Kandev ships a single binary that auto-applies SQLite migrations on first boot of every new release. Today there is no signal that migrations ran and no recovery path if one corrupts the DB:

1. **Silent migrations.** `runMigrations()` calls `_, _ = r.db.Exec(...)` and swallows every error. When a user reports "my DB looks weird after the upgrade", we have nothing in the logs to tell us which ALTER ran on their machine vs which was a no-op idempotent re-attempt.
2. **No backups.** A bad release can drop a column, half-rebuild a table, or corrupt the file. The user has no snapshot to fall back to. The data dir (`<KANDEV_HOME_DIR>/data/kandev.db`) holds weeks of agent work; losing it is unrecoverable.
3. **No version tracking.** Migrations rely on idempotency (`IF NOT EXISTS`, "swallow duplicate column"). The DB has no record of which kandev binary version last touched it, so we can't distinguish "first boot of upgrade" from "boot N of same release" and we can't time backups to upgrades.

The merge of `feature/orchestrate` into main is the catalyst: it ships ~30 new tables + ~20 ALTERs on existing tables. Doing it without an observable trail and without a safety net is risky.

## What

Add three coordinated pieces, all in the persistence layer, SQLite only:

### 1. `kandev_meta` table

A new table owned by the persistence package (not by any single repository). Holds key/value rows for boot-state tracking.

```sql
CREATE TABLE IF NOT EXISTS kandev_meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);
```

Keys this feature uses:

- `kandev_version` - the binary version that last successfully completed boot against this DB. Empty on fresh installs.
- `schema_initialized_at` - RFC3339 timestamp of the first-ever init. Lets us distinguish "fresh DB" from "pre-meta DB being upgraded into the meta era".

The persistence provider creates the table and writes `schema_initialized_at` on the first boot it sees an empty DB. After all repository `initSchema` calls succeed, it writes `kandev_version = currentBinaryVersion`.

### 2. Pre-migration backup

Between opening the SQLite writer connection and handing the pool to repositories, the persistence provider:

1. Reads `kandev_meta.kandev_version`.
2. Computes whether to back up:
   - **fresh DB, no user tables** - no backup, no upgrade banner.
   - **fresh DB, user tables exist** - upgrade from pre-meta release; back up.
   - **stored version differs from current binary version** - upgrade; back up.
   - **stored version == current** - same release re-launched; no backup.
3. If backing up, run `VACUUM INTO '<dataDir>/backups/kandev-<oldVersion>-<UTCtimestamp>.db'`. Logs the path and file size.
4. On backup failure, log the error and return it from `persistence.Provide`. The backend refuses to boot. The release-runner / supervisor surface that failure to the user; the user fixes the underlying issue (almost always disk full) and retries.

Backups live at `<KANDEV_HOME_DIR>/data/backups/`. Retention: keep the **last 2** backup files (newest by mtime); prune older ones after a successful snapshot. Restore is manual: stop the backend, copy the snapshot over `kandev.db`, delete `kandev.db-wal` / `kandev.db-shm`, restart.

Postgres path is unchanged. Backup is skipped with a log line `"pre-migration backup skipped: postgres driver (use pg_dump)"`.

### 3. Migration logging

Each repository gains a `*logger.Logger` field threaded through its `NewWithDB` constructor. A small helper on the repository wraps `db.Exec`:

```go
func (r *Repository) migrate(name, stmt string) {
    _, err := r.db.Exec(stmt)
    switch {
    case err == nil:
        r.log.Info("migration applied", zap.String("name", name))
    case isAlreadyExists(err):
        // duplicate column / table exists / index exists - silent no-op
    default:
        r.log.Warn("migration failed", zap.String("name", name), zap.Error(err))
    }
}
```

Every existing `_, _ = r.db.Exec(...ALTER ADD COLUMN...)` line is rewritten to `r.migrate("<short_name>", "...")`. Examples of short names: `tasks.origin`, `tasks.archived_by_cascade_id`, `workspaces.task_prefix`, `agent_profiles.workspace_id`.

Recreate-table migrations (`migrateTaskPriorityToText`, `migrateSessionsRemoveAgentExecutionID`, `migrateTaskEnvironmentsRemoveAgentExecutionID`, `recreateAgentProfilesWithoutModelCheck`) already gate on a "trigger phrase present in DDL" probe. They log `"migration applied"` with their name only when the gate fires. No additional plumbing.

The `isAlreadyExists` predicate matches the SQLite error strings:
- `duplicate column name:`
- `table … already exists`
- `index … already exists`

Anything else is logged at WARN.

### What the user sees in logs on a real upgrade

```
INFO  Database initialized (single-writer pool)        db_path=/.../kandev.db
INFO  pre-migration backup taken                       from_version=v0.42.1 to_version=v0.43.0 path=/.../backups/kandev-v0.42.1-20260516T141203Z.db size_bytes=8421376
INFO  migration applied                                name=tasks.origin
INFO  migration applied                                name=tasks.project_id
INFO  migration applied                                name=tasks.labels
INFO  migration applied                                name=tasks.identifier
INFO  migration applied                                name=tasks.archived_by_cascade_id
INFO  migration applied                                name=workspaces.task_prefix
INFO  migration applied                                name=workspaces.task_sequence
INFO  migration applied                                name=workspaces.office_workflow_id
INFO  migration applied                                name=workflows.is_system
INFO  migration applied                                name=workflows.style
INFO  migration applied                                name=tasks.priority_text_rebuild
INFO  migration applied                                name=task_sessions.recreate_drop_agent_execution_id
INFO  migration applied                                name=agent_profiles.workspace_id
INFO  migration applied                                name=agent_profiles.role
... (etc)
INFO  schema version recorded                          version=v0.43.0
```

On boot N+1 against the same DB, every `migrate` call hits `isAlreadyExists` and silences itself; the user sees just:

```
INFO  Database initialized (single-writer pool)        db_path=/.../kandev.db
```

## Boundaries

- **SQLite only.** Postgres deployments are by definition not single-user dev-tool installs; the operator runs their own backup flow. We skip with a log line.
- **No restore tooling.** Restore is documented (stop backend, copy file, drop WAL, restart). No backend-side restore endpoint.
- **No UI surface.** Backend logs are sufficient. A future "Last upgrade" panel in the web app settings page is out of scope for this spec.
- **No new migration framework.** Migrations stay imperative (`runMigrations()` per repo). The helper only adds logging - it doesn't reorder, batch, or transactionalize anything.
- **Refuse-to-boot on backup failure.** Disk full is the dominant failure mode. Better to surface it loudly than to apply migrations into an unrecoverable state.
- **Dev builds (`Version="dev"`) are not protected.** The backup trigger is a version delta. Local builds without an injected ldflags version all report `dev`, so iterative dev cycles that add a new ALTER will run the migration without taking a snapshot. Production releases each carry a unique tag and so always cross the version threshold. If a developer wants belt-and-suspenders safety during destructive schema work, the workaround is to `cp kandev.db kandev.db.bak` manually or to pass `KANDEV_VERSION=devN` via ldflags to force the delta.

## Success criteria

- After upgrading from any earlier release, a snapshot file appears at `<dataDir>/backups/`.
- The backup file is openable as a SQLite database and contains the pre-migration tables.
- The backend log lines a user can grep for - `migration applied`, `pre-migration backup taken`, `schema version recorded` - exist and carry the version pair.
- `<dataDir>/backups/` contains at most two files immediately after a boot that took a snapshot.
- A second boot of the same release writes no `migration applied` lines.
- A boot with disk full at backup time fails fast with a clear error before any ALTER runs.
- Postgres builds boot without attempting backup and log the skip reason.
