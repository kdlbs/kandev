---
title: "Operations"
description: "Operate, back up, update, monitor, and recover a local or self-hosted Kandev installation."
---

# Operations

Kandev can run as a desktop app, an interactive CLI process, an OS-managed service, or a container. All modes run the same backend and web application. Pick one owner for a given database and workspace root; do not start a second backend against the same SQLite file.

Kandev does not currently provide a user-login boundary for the web application, HTTP API, WebSocket, or external MCP routes. Treat anyone who can reach the backend as an operator. Keep it on a trusted host or private network, or put the entire origin behind an authenticated TLS reverse proxy.

## Choose an operating model

| Mode | Start and stop | Durable state | Update path |
| --- | --- | --- | --- |
| Desktop | Launch or quit Kandev | `~/.kandev` by default | **Settings > System > Updates** uses the signed desktop updater when supported |
| Interactive CLI | `kandev`, then `Ctrl-C` | `~/.kandev` by default | Upgrade the Homebrew or npm package, then restart |
| Managed service | `kandev service {start,stop,restart,status}` | `~/.kandev` for a user service, `/var/lib/kandev` for a system service, or the install-time `--home-dir` | Upgrade the package, reinstall the unit with the same flags, and restart; the current native installer does not enable in-app apply |
| Docker or Kubernetes | Container or workload manager | Mounted Kandev home plus any external database/provider state | Replace the image and recreate the container or pod |

See [Desktop app](desktop-app.md), [CLI](cli.md), [Run as a service](run-as-a-service.md), [Docker](docker.md), and [Kubernetes](k8s.md) for mode-specific prerequisites and commands.

## Health and readiness

Use the top-level readiness endpoint for supervisors and probes:

```bash
curl -fsS http://127.0.0.1:38429/health
```

After startup it returns HTTP 200 with:

```json
{"status":"ok","service":"kandev","mode":"websocket+http"}
```

It returns HTTP 503 with `status: "starting"` until routes, the agent registry, and the listener are ready. The supplied Kubernetes probes use this endpoint.

For application diagnostics, open **Settings > System > Status** or request:

```bash
curl -fsS http://127.0.0.1:38429/api/v1/system/health
```

This diagnostic checks the Git executable, GitHub authentication/rate limits, agent discovery, and Linux inotify pressure. It returns a JSON `healthy` field and issue list, but normally uses HTTP 200 even when `healthy` is false; do not substitute it for `/health` in a status-only probe.

For a managed service, also check its process manager:

```bash
kandev service status
kandev service logs -f
```

Add `--system` to both commands for a system service.

## State and storage

`KANDEV_HOME_DIR` relocates the Kandev home. Its default is `~/.kandev`; the derived data directory is always `<home>/data`.

| Path under Kandev home | Contents and recovery significance |
| --- | --- |
| `data/kandev.db`, `-wal`, `-shm` | Default SQLite database and transient WAL files |
| `data/master.key` | Owner-only AES-256 key used to decrypt secrets stored in the database; a database copy without the matching key cannot recover those secret values |
| `data/backups/` | SQLite manual, pre-upgrade, and pre-reset snapshots |
| `tasks/` and legacy `worktrees/` | Managed Git worktrees and per-task files; may contain uncommitted or untracked work |
| `repos/` | Kandev-managed source clones |
| `sessions/`, `quick-chat/`, `agent-sessions/` | Session history, ephemeral workspaces, and isolated agent homes when used |
| `logs/` | Service and optional ACP debug logs |
| `service/` | Update/helper files when present; the current native service installer does not create `install.json` |
| `lsp-servers/`, `runtime/`, `workspaces/` | Installed tools and feature-specific materialized state |

Database snapshots do not contain Git worktrees, clones, the master key, service metadata, or provider-side objects. Native agent and `gh` login files also normally live in the service user's home outside `~/.kandev` (for example `~/.codex` and `~/.config/gh`). The official container instead sets `HOME=/data/home`, so those CLI credentials live on its mounted volume.

The supported SQLite layout for the System database and restore pages is the derived `<home>/data/kandev.db`. `database.path` can point the persistence layer elsewhere, but the current System page still derives its displayed path, WAL path, and restore destination from `<home>/data/kandev.db`. Treat a custom SQLite path as operator-managed: back it up and restore it with SQLite-aware tooling while Kandev is stopped.

## Storage maintenance

Open **Settings > System > Storage** to inspect Kandev-managed disk usage and configure cleanup.
**Analyze** is read-only. **Run now** applies only the enabled cleanup rules and refuses to start
while task resources are active or another maintenance run owns the cleanup gate.

Scheduled cleanup is disabled by default and runs only after the configured resource-idle quiet
period. Orphaned task workspaces move into Kandev's quarantine before permanent deletion; review
the quarantine list to restore an entry or request deletion as a background job. Host-wide Docker
build-cache and unused-image cleanup remain disabled until you confirm that Kandev owns a dedicated
Docker daemon.
Do not enable those rules on a daemon shared with unrelated workloads.

Host-local agents inherit the Kandev service's `TMPDIR`, `TMP`, and `TEMP` values unchanged. If the
service leaves them unset, agent tools use their normal operating-system defaults; if an operator
sets them on the service, all host-local agents share those configured locations. This permits
tools whose caches are derived from the temporary directory, such as Node or Playwright tooling, to
reuse cache data across agent instances. Kandev does not create a per-instance temporary root or
claim ownership of arbitrary files in the shared temporary directory. On Unix, keep an explicit
service temp path short enough for local Unix-domain socket paths; an excessively long path can
exceed the platform socket-address limit.

Persistent Go build caching is separate: Go's default `GOCACHE` is not derived from `TMPDIR`, and
Kandev injects its managed Go-cache location only when that opt-in Storage setting is enabled.
Scratch cleanup in the inherited temporary directory belongs to the operating system or the host's
temporary-file policy. Archive and delete stop and reap the task's host-local processes, but do not
recursively delete shared temporary files.

Older versions may have left inactive directories under `/tmp/kandev-agent/*`. Storage analysis and
cleanup do not delete this legacy data by name or age. Remove it only through a deliberate
host-administrator procedure after confirming that no live process references the target; stopping
the Kandev service first provides the clearest maintenance boundary.

## Database operation

### SQLite

SQLite is the default and is appropriate for a desktop, CLI, service, or single-replica container installation. Kandev uses one writer connection and a read pool in WAL mode. Only one Kandev backend should own the file.

Open **Settings > System > Database** to see database size, WAL size, schema version, path, and the newest modification time among regular entries in `data/backups`. That timestamp is a filesystem hint, not proof of a valid snapshot: an unrelated or temporary file in the directory can affect it. SQLite exposes three maintenance actions:

- **Optimize** runs `PRAGMA optimize`. It is quick and updates planner statistics.
- **Vacuum** runs `VACUUM`, compacts the file, and reports bytes reclaimed. It can need substantial temporary disk and can block writes, so run it during a quiet period.
- **Factory reset** is destructive and is described below.

These actions run as background system jobs. Closing the browser does not cancel a job. Check the page or logs for its terminal state before starting another maintenance operation.

### PostgreSQL

PostgreSQL is an external, operator-managed database. The Database page shows driver, database-level size, and schema version, but hides SQLite path, WAL, backup time, and maintenance controls. Kandev does not take a pre-migration PostgreSQL dump and the System Backups page is not a PostgreSQL backup/restore facility. Use your platform backup policy or `pg_dump`/`pg_restore`.

One executable pattern, after setting standard `PGHOST`, `PGPORT`, `PGUSER`, `PGDATABASE`, and a secure password source such as `.pgpass`, is:

```bash
pg_dump --host "$PGHOST" --port "${PGPORT:-5432}" \
  --username "${PGUSER:-kandev}" --format=custom \
  --file "kandev-$(date -u +%Y%m%dT%H%M%SZ).dump" \
  "${PGDATABASE:-kandev}"
```

Switching `database.driver` does not migrate data. PostgreSQL and shared NATS remove two single-process data constraints, but they do not make Kandev horizontally scalable: WebSocket subscriptions, execution lifecycle/control state, and task workspaces remain process- or filesystem-local. The current product and supplied deployment validate one backend replica only; do not add replicas based on the database and event bus alone.

## SQLite backups

Open **Settings > System > Backups**.

1. Click **Create snapshot**. Kandev runs SQLite `VACUUM INTO`, including committed WAL frames, writes a temporary sidecar, then atomically renames it to `manual-<nanoseconds>.db`.
2. Wait for the manual row to appear. The browser waits up to 15 seconds; on a large database the backend job can continue after that UI timeout, so reload before retrying.
3. Download the snapshot and copy it off the host.
4. Back up `<home>/data/master.key` with owner-only access if you need encrypted secrets to remain usable.
5. Separately preserve unpushed Git work, executor/provider state, service configuration, and required CLI login files.
6. Restore into an isolated instance and verify tasks, workflows, secrets, and repository references before calling the backup tested.

Manual snapshots are never automatically pruned. When the recorded Kandev application version changes, or when a legacy database has user tables but no stored application-version metadata, Kandev takes a pre-migration `kandev-<stored-version-or-pre-meta>-<timestamp>.db` before repository schema initialization. Snapshot failure aborts SQLite startup, so keep the backup directory writable and leave enough free space. Kandev then attempts to retain the two newest `kandev-*.db` files, but pruning is best-effort and a failed delete does not abort startup. That two-file retention applies to automatic files, including older `kandev-pre-reset-*` snapshots; it does not apply to `manual-*` files. Monitor and delete obsolete manual files yourself.

For a cold copy of the complete default Kandev home on a user-service installation:

```bash
kandev service stop
tar -C "$HOME" -czf "$HOME/kandev-state-$(date -u +%Y%m%dT%H%M%SZ).tar.gz" .kandev
kandev service start
```

Adapt the source for a custom `--home-dir`. This archive still does not include external PostgreSQL, remote executors, provider objects, or CLI credentials stored elsewhere in the operating-system home.

## Restore and recovery

### Restore a System snapshot

The supported UI flow applies to the default SQLite path:

1. Stop or finish active agent sessions and preserve unpushed work.
2. Open **Settings > System > Backups**, choose **Restore**, type `RESTORE`, and confirm.
3. Kandev copies the selected snapshot to `data/kandev.db.new` and atomically renames it over `data/kandev.db`.
4. Quit and relaunch Kandev immediately. The running backend retains connections to the old database inode and can otherwise serve or write stale state.
5. Check `/health`, **System > Status**, database schema version, secrets, and representative tasks.

Restore does not roll back worktrees or remote/provider state. A database may therefore refer to files, containers, pull requests, or credentials from a different point in time. Reconcile them before restarting automation.

### Restore PostgreSQL

Stop every Kandev backend using the database, then follow your database provider's point-in-time recovery process or restore a verified dump. With the standard PostgreSQL environment variables configured, a destructive replacement pattern is:

```bash
pg_restore --clean --if-exists --no-owner \
  --host "$PGHOST" --port "${PGPORT:-5432}" \
  --username "${PGUSER:-kandev}" --dbname "${PGDATABASE:-kandev}" \
  kandev-YYYYMMDDTHHMMSSZ.dump
```

Restart one backend, allow schema initialization to complete, then validate it as a single-replica deployment. Match the restored data with a compatible Kandev version; Kandev has no automatic database downgrade or validated multi-replica operating path.

## Factory reset

In **Settings > System > Database**, click **Factory reset**, type `RESET`, and confirm. This is SQLite-only. The job:

1. stops the orchestrator;
2. creates `data/backups/kandev-pre-reset-<unix>.db`;
3. drops every SQLite user table while retaining `kandev_meta`;
4. removes managed `worktrees/`, `repos/`, `sessions/`, `tasks/`, and `quick-chat/` trees;
5. requires a manual quit and relaunch.

It does not erase the entire Kandev home: backups, `master.key`, logs, service metadata, installed tools, and external provider state can remain. Pre-reset files cannot be deleted through the Backups delete action, but older automatic snapshots can later age out under the two-file automatic retention policy. Download a recovery copy before further upgrades.

## Logs and diagnostics

Open **Settings > System > Logs** for recent structured backend events. Kandev maintains an in-memory ring of exactly 2,000 events that passed the configured log level; the page requests the newest 1,000. This buffer disappears on restart.

Default interactive launcher output is warning level. `kandev --verbose` selects info level and shows backend output; `kandev --debug` selects debug level and also enables ACP message dumps. An explicit `KANDEV_LOG_LEVEL` overrides the flag-selected level. Those dumps can contain full prompts, file contents, and tool calls. Use debug mode only on a trusted machine, collect the minimum needed, then disable and remove the files.

Logging configuration defaults are `outputPath: stdout`, 100 MB rotation size, five rotated files, 30-day rotated-file age, and gzip compression. Rotation applies only when `logging.outputPath` is a file, and active files are created owner-readable. The Logs page can list/download the exact active filename and timestamped rotations when that filename has an extension; for an extensionless output path it does not enumerate rotated siblings. With stdout logging, use the in-memory tail plus the process manager:

- Linux service: `kandev service logs -f` reads the systemd journal.
- macOS service: the same command tails `<home>/logs/service.out` and `service.err`.
- Docker: `docker logs -f kandev`.
- Kubernetes: `kubectl logs -f deployment/kandev`.

When reporting an incident, record timestamp/timezone, Kandev version and commit from **System > About**, task ID, session ID, executor type, repository/branch, and relevant provider request IDs. **System > Licenses** is a generated inventory of shipped npm and Go dependencies, not a runtime health check.

## Disk use and environment cleanup

**Settings > System > Status** walks `data`, worktrees, repositories, sessions, tasks, quick chat, and backups. Results are cached for two hours; **Refresh** forces a new single-flight walk. Permission failures appear as warnings. The displayed total intentionally counts `data/backups` both inside the `data` row and again as the separate `backups` row, so use filesystem or volume metrics for quota enforcement.

Archiving or deleting a task stops active sessions and starts durable asynchronous cleanup. Depending on executor, cleanup can delete a managed worktree and its local branch, remove a container, destroy a Sprite, reap a host-local agent's process tree, or attempt to stop the remote SSH controller and remove only its per-session runtime directory. Failed cleanup remains retryable across a backend restart. Kandev does not sweep arbitrary files from the shared temporary directory during archive or delete. SSH process/session cleanup is best-effort when the connection is failing, and the task directory always remains for deliberate, audited cleanup; there is no automatic sweeper for it today. The task can disappear from the UI before cleanup finishes.

**Reset Environment** uses a separate teardown path. For Sprites, the current reset request can lose the profile credential context and report success while leaving the provider sandbox behind. After a Sprites reset, inspect **Settings > Executors > Sprites.dev** and explicitly destroy the old sandbox there if it remains. See [Executors](executors.md#spritesdev) for the executor-specific lifecycle.

Before archive, delete, reset, or manual cleanup:

1. inspect tracked, untracked, and ignored files in every attached repository;
2. commit and push work that must survive, and record its remote branch or pull request;
3. stop active sessions;
4. use the Kandev action so database and runtime inventory stay coordinated;
5. check logs and the remote provider afterward, because timeout, network, or permission failures can leave a container, directory, or sandbox behind.

Never delete a managed task directory merely because its database row looks terminal. A borrowed environment or pending asynchronous cleanup can still own it.

## Updates

The backend contacts the public GitHub Releases API once at startup and every six hours, with a 30-second HTTP timeout, and persists the last successful result. **Check now** performs a synchronous request and permits one manual check per process every 30 seconds. Offline or rate-limited installations continue to show cached state.

Before any update, finish or stop active sessions, create and export a database backup plus its master key, preserve unpushed Git work, and read release notes.

- Desktop: use **Settings > System > Updates** when signed updater assets are available; otherwise install the new desktop package.
- Managed service: the current native CLI installer neither writes `service/install.json` nor supplies the service/install metadata required by the guarded one-click updater, so its **Apply update** action is unavailable. Run `brew upgrade kandev` or `npm install -g kandev@latest`, rerun `kandev service install` with the same install flags, then `kandev service restart`. Backend support for metadata-bearing legacy services is implementation detail, not the current native installation path.
- Unmanaged CLI: run `brew upgrade kandev` or `npm install -g kandev@latest`, then restart the process.
- Transient npx: start the desired release with `npx -y kandev@latest`; this does not update a persistent package.
- Docker/Kubernetes: replace the image and recreate the workload. Do not treat an in-container package install as a durable update.

After restart, verify `/health`, **System > About**, **System > Status**, the database page, and one non-destructive agent session. Kandev does not perform automatic binary or database rollback. If rollback is necessary, restore a compatible pre-upgrade database and matching application release together.

## Resource metrics

Configure sampling at **Settings > General > Appearance > Resource Metrics**. Defaults are CPU, memory, and disk percentage every five seconds, backend disk path `/`, and execution-environment collection off. Valid intervals are 1–300 seconds; at least one of CPU, memory, disk, CPU temperature, or load average remains selected.

Collection starts only while at least one connected client shows metrics in a kanban or task topbar. Enabling execution metrics adds active Docker, SSH, and Sprites `agentctl` sources; execution disk sampling uses `/`. A provider hook also exists for remote Docker, but creating that runtime currently returns a not-implemented error. Missing platform APIs, container permissions, an invalid disk path, a disconnected executor, macOS/Windows temperature support, or Windows load-average support produce unavailable samples rather than quotas.

These metrics are lightweight UI observability. Set alerts, retention, CPU/memory limits, and disk quotas in the host, container platform, or external monitoring stack.

## Feature toggles

**Settings > System > Feature Toggles** currently exposes:

- **Office mode** — experimental, medium risk, and off in the production profile by default.
- **Debug mode** — high risk; enables diagnostic endpoints and agent-message logging that can contain sensitive content.

Both require restart. A value supplied explicitly by its environment variable locks the UI control; the debug toggle is also locked by explicit legacy/debug-message environment variables. Otherwise the UI stores an override in the database. The page can request restart only when the native local supervisor is available. A normal Unix `kandev` terminal launch is supervised; Desktop, a service, a container, a directly started backend, a deploy preview, or Windows requires a manual application restart.

## Troubleshooting

| Symptom | Check | Action |
| --- | --- | --- |
| `/health` stays at 503 or cannot connect | Process-manager and launcher output | Confirm port ownership, database reachability, writable Kandev home, and required executables; then restart once |
| Status page says unhealthy while `/health` is 200 | `/api/v1/system/health` issue IDs | Fix Git, GitHub, agent discovery, or Linux inotify warning; readiness and application diagnostics have different meanings |
| Backups page reports a 15-second create timeout | Reload the backup list and inspect the `backup-create` job/log | Large `VACUUM INTO` jobs can still finish; avoid double-clicking and ensure free disk |
| Backup/maintenance fails on PostgreSQL | Active driver on Database page | Use `pg_dump`, provider snapshots, and PostgreSQL maintenance; System backup/vacuum/reset is SQLite-only |
| Restored data looks stale | Whether the backend was restarted immediately | Quit/restart; do not keep using the old open database connections |
| Logs page has no downloadable files | `logging.outputPath` and service/container logs | `stdout` is the default; use in-memory tail or configure a file sink |
| Update check returns HTTP 429 | Time since last **Check now** | Wait at least 30 seconds; background checks retry every six hours |
| **Apply update** is absent | Install method shown on the Updates page | Expected for current native services; upgrade the package, reinstall the service with the same flags, and restart |
| Metrics show unavailable | OS support, disk path, executor connectivity | Select supported metrics and verify permissions/network; the collector reports errors per sample |
| Disk total exceeds filesystem expectation | Separate `data` and `backups` rows | Backups are counted twice in the UI total; use volume metrics for capacity decisions |
| Legacy `/tmp/kandev-agent/*` uses disk | Process inventory and open-file references for the exact directory | This is data from older Kandev versions, not a current Storage resource. Stop Kandev, confirm no live process references the target, then remove only the confirmed-inactive legacy directory through host administration. |
| Archived task's remote resource remains | Backend, Docker/SSH/Sprites, and provider logs | Cleanup is asynchronous and bounded. SSH task directories are retained by design; for other leftovers, verify work is preserved, then remove the exact resource manually |
| Sprites reset removed the environment but not the sandbox | **Settings > Executors > Sprites.dev** | Current reset can omit the provider credential during destroy; find the old Kandev-named sandbox and destroy it explicitly |

## Related pages

- [Configuration](configuration.md) — paths, database, logging, NATS, Docker, and security-sensitive environment variables
- [Executors](executors.md) — runtime lifecycle, credentials, cleanup, and isolation boundaries
- [Git operations](git-operations.md) — branches, worktrees, push, and pull-request behavior
- [Automation and MCP](automation-and-mcp.md) — external MCP routes and their current unauthenticated trust boundary
- [Windows support](windows-support.md) — Windows-native limitations and supported alternatives
