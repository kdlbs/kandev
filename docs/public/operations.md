---
title: "Operations"
description: "Operate, back up, update, monitor, and recover a local or self-hosted Kandev installation."
---

# Operations

Kandev can run as a desktop app, an interactive CLI process, an OS-managed service, or a container. All modes run the same backend and web application. Pick one owner for a given database and workspace root; do not start a second backend against the same SQLite file.

Kandev does not currently provide a user-login boundary for the web application, HTTP API, WebSocket, or external MCP routes. Treat anyone who can reach the backend as an operator. Keep it on a trusted host or private network, or put the entire origin behind an authenticated TLS reverse proxy.

## Quick path

1. Choose one process owner for the database and Kandev home.
2. Check `/ready` for readiness and **System > Status** for diagnostics.
3. Back up the database and `master.key` before upgrades, resets, or recovery work.
4. Preserve Git branches and external provider state separately from database backups.

## Choose an operating model

| Mode | Start and stop | Durable state | Update path |
| --- | --- | --- | --- |
| Desktop | Launch or quit Kandev | `~/.kandev` by default | **Settings > System > Updates** uses the signed desktop updater when supported |
| Interactive CLI | `kandev`, then `Ctrl-C` | `~/.kandev` by default | Upgrade the Homebrew or npm package, then restart |
| Managed service | `kandev service {start,stop,restart,status}` | `~/.kandev` for a user service, `/var/lib/kandev` for a system service, or the install-time `--home-dir` | A verified npm/npx user service can select and apply Stable or Nightly; other services use Stable and may require a package upgrade, reinstall, and restart |
| Docker or Kubernetes | Container or workload manager | Mounted Kandev home plus any external database/provider state | Replace the image and recreate the container or pod |

See [Desktop app](desktop-app.md), [CLI](cli.md), [Run as a service](run-as-a-service.md), [Docker](docker.md), and [Kubernetes](k8s.md) for mode-specific prerequisites and commands.

## Prevent host sleep during active tasks

Administrators can open **Settings > General > Task Actions** and enable
**Prevent host sleep while tasks run**. The install-wide setting is off by
default and is saved with the Kandev database. It applies only to the machine
running the backend; it does not change executor or node power policy.

When enabled, Kandev holds one host sleep-prevention request while any task
session is `STARTING` or `RUNNING`. It releases the request when the last such
session reaches `WAITING_FOR_INPUT`, `IDLE`, or a terminal state, when an
administrator disables the setting, and during backend shutdown. Missed session
events are repaired by periodic reconciliation against the session repository.

The request prevents idle system sleep but does not keep the display awake or
override an explicit user sleep action, lid-close behavior, shutdown, low-power
emergency, or another administrator/platform policy. It is supported through
macOS `caffeinate`, Windows execution state, and Linux `systemd-logind` over the
system D-Bus. A missing or denied Linux system bus, an unavailable service, or
another native failure is shown in the setting status and never blocks task
execution.

Do not enable this setting by default for Docker, Kubernetes, remote-executor,
or always-on deployments. A backend inside a container or pod cannot inhibit
sleep on the physical node, and an SSH or other remote executor runs on a
different host. Those deployments should use their normal host power and
availability policy instead. Enabling the preference is safe when moving the
database between hosts: Kandev reacquires a native request only after startup
if the new backend can provide it and a working task still exists.

## Health and readiness

Use the top-level liveness endpoint for process supervisors:

```bash
curl -fsS http://127.0.0.1:38429/health
```

It returns HTTP 200 as soon as the listener accepts connections, with:

```json
{"status":"ok","service":"kandev","mode":"websocket+http","version":"1.2.3"}
```

`/health` never returns a non-2xx status while the process is alive, even mid-startup: it confirms the process is up, not that it can serve real traffic. Use the readiness endpoint instead when you need to know the backend can actually serve requests:

```bash
curl -fsS http://127.0.0.1:38429/ready
```

It returns HTTP 503 with `status: "starting"` (plus the same `version`) until routes, the agent registry, and the listener are ready, then HTTP 200. The supplied Kubernetes liveness probe uses `/health`; the readiness probe uses `/ready`. Both are unauthenticated even when auth is enabled, so they're also a credential-free way for monitoring to read the running version; there is no need to authenticate to **System > About** just to check what build is deployed.

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

## Message queue settings

Open **Settings > Task Behavior > Message Queue** to manage install-wide queue behavior. The default capacity is `10`; `0` means unlimited. Admin saves apply immediately to later admissions. Lowering the limit does not prune rows already waiting, so a queue at or above the new limit rejects new work until messages run or are removed. Delivery retries for work accepted before the change are not discarded by the lower cap.

`KANDEV_QUEUE_MAX_PER_SESSION` has higher precedence than the saved capacity. A valid environment value makes only that field read-only; zero or a negative value means unlimited. Invalid text is logged and ignored in favor of the saved setting or default. Environment changes require a backend restart, while UI changes do not.

**Automatically merge consecutive messages** is on by default. Capacity is checked before any fold, so a full queue still rejects a compatible message. After admission, a new row folds only into its immediate pending predecessor when both rows have the same strict source and compatible task, model, mode, metadata, attachments, and references. Any mismatch or combined limit leaves the new row separate. The earlier row survives and its ID is returned. Turning the switch on does not compact existing rows. This setting is independent from **Enable queued message merging**, which controls the manual queue action.

To recover capacity in one session, expand its queue chip in the task workbench. **Remove** deletes one visible pending row and **Clear all** deletes all visible pending rows, including user-, agent-, workflow-, and server-origin work. After removal, merge, or drain, displayed positions immediately compact to `#1` through `#N`; durable FIFO ordering is unchanged. A row already reserved for delivery is hidden and is not cancelled by either action.

## State and storage

`KANDEV_HOME_DIR` relocates the Kandev home. Its default is `~/.kandev`; the derived data directory is always `<home>/data`.

| Path under Kandev home | Contents and recovery significance |
| --- | --- |
| `data/kandev.db`, `-wal`, `-shm` | Default SQLite database and transient WAL files |
| `data/master.key` | Owner-only AES-256 key used to decrypt secrets stored in the database; a database copy without the matching key cannot recover those secret values |
| `data/backups/` | SQLite snapshots when the database uses the default path |
| `tasks/` and legacy `worktrees/` | Managed Git worktrees and per-task files; may contain uncommitted or untracked work |
| `repos/` | Kandev-managed source clones |
| `sessions/`, `quick-chat/`, `agent-sessions/` | Session history, ephemeral workspaces, and isolated agent homes when used |
| `logs/` | Service and optional ACP debug logs |
| `service/` | Owner-only managed-service install metadata plus update intents and helper files |
| `lsp-servers/`, `runtime/`, `workspaces/` | Installed tools and feature-specific materialized state |

Database snapshots do not contain Git worktrees, clones, the master key, service metadata, or provider-side objects. Native agent and `gh` login files also normally live in the service user's home outside `~/.kandev` (for example `~/.codex` and `~/.config/gh`). The official container instead sets `HOME=/data/home`, so those CLI credentials live on its mounted volume.

The System Database and Backups pages use the configured SQLite file path. They use `backups/` under the parent directory of that file. The default remains `<home>/data/kandev.db` with snapshots in `<home>/data/backups/`. A custom path can place the database and snapshots outside the Kandev home. Kandev does not move snapshots from another directory automatically.

## Storage maintenance

> **Destructive:** **Force clear all** permanently removes eligible quarantine entries and discards their restore windows.

<details>
<summary>Storage maintenance details</summary>

Open **Settings > System > Storage** to inspect Kandev-managed disk usage and configure cleanup.
**Analyze** is read-only. **Run now** applies only the enabled cleanup rules and refuses to start
while another maintenance run owns the cleanup gate. If task resources are active, the page names
the active work and offers **Run anyway** after an explicit disruption warning. Use that override
only when the active task work can tolerate cleanup running alongside it.

Storage analysis results are cached in the running backend for 15 minutes, so page reloads and
policy saves reuse the displayed snapshot instead of scanning disk again. The page shows when that
snapshot was last analyzed. Select **Analyze** to force a fresh scan; restarting the backend also
clears the in-memory snapshot.

The page also shows the current percentage used on the filesystem containing Kandev's storage,
along with used, available, and total capacity. This is host-volume capacity, not just the bytes
that Kandev can classify. The card warns at 80% full and uses a critical style at 90%; these
thresholds do not trigger cleanup.

Scheduled cleanup is disabled by default and runs only after the configured resource-idle quiet
period. Orphaned task workspaces and rotated Go caches move into Kandev's quarantine before
permanent deletion. Each entry shows its `delete_after` retention deadline: **Delete** and
**Clear eligible** cannot remove it before that time. The deadline is the earliest safe deletion
time, not an exact promise, the first successful scheduled or full manual maintenance run after the
deadline performs the purge, subject to the idle gate and any preemption.

Use **Clear eligible** to remove only entries whose deadlines have passed. It reports protected
entries that remain. **Force clear all** requires typing `DELETE ALL NOW` and attempts to permanently
remove every active quarantine entry, discarding restore windows for entries that are successfully
deleted. Safety-validation or deletion failures may leave entries visible and retryable. This
override bypasses only the retention timestamp; path, ownership, state, and filesystem safety
checks still apply.

Kandev keeps at most one restorable Go-cache generation for each original cache path. If that
generation is still active when the replacement cache exceeds its limit, the next rotation is
deferred. The maintenance run succeeds and reports `active_quarantine`; both the live cache and
the retained generation stay unchanged.

If a Go-cache quarantine payload is already missing, **Delete**, **Clear eligible**, or **Force
clear all** can close its durable entry without changing the live replacement cache. The purge
reports zero deleted bytes for that entry. **Restore** remains unavailable because Kandev cannot
prove which cache generation is currently live.

If scheduled cleanup is disabled, no independent quarantine sweeper runs: use a full **Run now** or
one of the quarantine actions when you want cleanup.

The Workspaces policy includes an off-by-default **Remove dependencies from archived or deleted
task workspaces** option. When enabled, a scheduled cleanup or the matching manual action can
recursively remove only these directories from eligible, unprotected Kandev task workspaces:

`node_modules`, `bower_components`, `.pnpm-store`, `.yarn/cache`, `.yarn/unplugged`, `.venv`,
`venv`, `.tox`, `.nox`, `__pypackages__`, `Pods`, and `.gradle`.

The Storage page always shows this exact list next to an information control. Kandev skips missing
directories, never follows symlinks, and does not target ambiguous names such as `vendor`, `target`,
`build`, `dist`, `.cache`, or `.git`. Only archived or deleted workspaces without active or
protected references qualify. This does not change the existing full-workspace lifecycle (seven
days of orphan grace followed by seven days of quarantine retention), and restoring a workspace
after dependency pruning may require reinstalling its dependencies.

Host-wide Docker build-cache and unused-image cleanup remain disabled until you confirm that Kandev
owns a dedicated Docker daemon.
Do not enable those rules on a daemon shared with unrelated workloads.

The Storage page also reports **Kandev temporary artifacts** created by services that need a
short-lived directory under the host temporary root. Each current artifact is registered in the
Kandev database and carries an owner-only marker in its exact directory. Active artifacts and
artifacts created within the last 24 hours are protected. **Clean stale artifacts** is a manual-only
action: it moves eligible, registered artifacts into Kandev quarantine on the same filesystem so
they can be restored during the configured retention period. It does not permanently delete them.
The action does not inspect or claim arbitrary `/tmp` entries, shared caches, Node or Playwright
caches, preview/CI/dev-harness directories, or temporary data belonging to another Kandev
installation. A missing registry or failed measurement is shown as unavailable rather than as
zero usage. The inherited `TMPDIR`, `TMP`, and `TEMP` behavior below is unchanged.

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

</details>

## Database operation

> **Single-owner rule:** SQLite uses one writer connection in WAL mode; only one Kandev backend should own the file. **Factory reset** is destructive and removes managed data after creating a pre-reset backup.

<details>
<summary>Database operation details</summary>

### SQLite

SQLite is the default and is appropriate for a desktop, CLI, service, or single-replica container installation. Kandev uses one writer connection and a read pool in WAL mode. Only one Kandev backend should own the file.

Open **Settings > System > Database** to see database size, WAL size, schema version, path, and the newest modification time among regular entries in the sibling `backups/` directory. That timestamp is a filesystem hint, not proof of a valid snapshot: an unrelated or temporary file in the directory can affect it. SQLite exposes three maintenance actions:

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

Some releases perform a one-time schema cutover that rewrites ownership tables and drops legacy schema (for example the task-worktree ownership normalization). Before such an upgrade:

1. Take and **verify** a backup. For PostgreSQL use the pattern above. For SQLite, create a manual snapshot from **Settings > System > Backups** (or `sqlite3` `VACUUM INTO`) and verify it: the automatic pre-migration snapshot is taken during startup of the new binary, so it cannot be verified before the upgrade starts.
2. Stop all backend writers during the cutover. Do not run a mixed-version fleet across the upgrade; the migration takes a database advisory lock and aborts on lock timeout without changing the schema or data.
3. Let exactly one schema initializer run and reach a healthy state before starting additional instances.

If the new binary reports a worktree-ownership conflict and exits, stop the
rollout. The cutover is transactional, so the legacy database remains
authoritative. Do not delete ownership rows by hand. Start a compatible
pre-cutover binary to restore service, or deploy the migration hotfix and retry
the upgrade against the unchanged database.

The normalized schema is intentionally incompatible with older binaries. To downgrade, stop all instances and restore the pre-upgrade backup; never start an older binary against a post-cutover database. SQLite restores from the verified manual snapshot created before the upgrade; PostgreSQL restores your verified `pg_dump` backup.

Switching `database.driver` does not migrate data. PostgreSQL and shared NATS remove two single-process data constraints, but they do not make Kandev horizontally scalable: WebSocket subscriptions, execution lifecycle/control state, and task workspaces remain process- or filesystem-local. The current product and supplied deployment validate one backend replica only; do not add replicas based on the database and event bus alone.

</details>

## SQLite backups

<details>
<summary>Backup details</summary>

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

</details>

## Restore and recovery

> **Restore warning:** Stop or finish active sessions, relaunch Kandev after restoring, and reconcile worktrees, containers, pull requests, credentials, and other external state before restarting automation. A database restore does not roll those systems back.

<details>
<summary>Restore details</summary>

### Restore a System snapshot

The UI flow applies to the configured SQLite path:

1. Stop or finish active agent sessions and preserve unpushed work.
2. Open **Settings > System > Backups**, choose **Restore**, type `RESTORE`, and confirm.
3. Kandev stops scheduling, active executions, and database-backed workers. It copies the selected snapshot to `<configured-database-path>.new`, validates the SQLite checkpoint result, and closes the pool. It then quarantines the configured database and its `-wal`/`-shm` sidecars before installing the staged file. If installation fails, Kandev restores the quarantined files.
4. Click **Restart Kandev** in the success dialog. If automatic restart is unavailable, quit and relaunch Kandev manually. The backend must restart before database-backed work resumes.
5. Check `/ready`, **System > Status**, database schema version, secrets, and representative tasks.

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

</details>

## Factory reset

In **Settings > System > Database**, click **Factory reset**, type `RESET`, and confirm. This is SQLite-only. The job:

1. stops the orchestrator;
2. creates `<database-dir>/backups/kandev-pre-reset-<unix>.db` beside the configured SQLite file;
3. drops every SQLite user table while retaining `kandev_meta`;
4. removes managed `worktrees/`, `repos/`, `sessions/`, `tasks/`, and `quick-chat/` trees;
5. requires a manual quit and relaunch.

It does not erase the entire Kandev home: backups, `master.key`, logs, service metadata, installed tools, and external provider state can remain. Pre-reset files cannot be deleted through the Backups delete action, but older automatic snapshots can later age out under the two-file automatic retention policy. Download a recovery copy before further upgrades.

## Logs and diagnostics

Open **Settings > System > Logs** and select **Customize bundle** to create and
download a diagnostic ZIP. Backend and frontend diagnostic evidence are
selected by default; you can add the allow-listed runtime index, and in debug
mode raw and normalized agent-protocol (ACP) evidence. Adding ACP opens an
explicit session picker, with each session's Kandev task title linked for
identification. The archive has source directories and `manifest.json`; inspect
the manifest for requested sources, warnings, captured ranges, truncation, and
loss before treating it as complete.

Standard bundles do not read stored chat transcripts, session messages, or agent messages. Backend evidence covers startup, lifecycle, API, executor, and error events; frontend evidence covers bounded browser console errors/warnings and diagnostic toast reports. Incidental text already emitted into a backend/frontend log entry is not automatically redacted, so review the ZIP before sharing. The runtime index contains only allow-listed session status and executor metadata.

ACP evidence is a separate, debug-only opt-in. It is limited to one to ten sessions the caller is authorized to view and may contain prompts, responses, tool calls, file contents, MCP data, environment-derived values, and secrets. Collection is on demand from retained host files or reachable executors; unavailable, expired, invalid, or truncated sessions make the archive partial instead of broadening the request or continuously copying ACP frames.

Every launch writes info and above to `<home>/logs/backend-logs.log`; normal and debug stdout show warn and above. `kandev --verbose` also shows info on stdout. `kandev --debug` records debug events in the file and enables ACP message dumps.

The active backend file accepts at most 16 MiB. Before a new entry would exceed that limit, Kandev closes it as `backend-logs-YYYY-MM-DD-NNNNNN.log` and opens a new active file. Active and closed backend files use at most 256 MiB in total. Kandev removes the oldest closed segments when needed, so logging continues and high-volume periods keep the newest evidence. Three UTC days is the maximum file age, not a reserved allocation for each day. Same-day restarts append to the active file, and UTC midnight starts a new active file.

The browser keeps a bounded three-day console history locally and sends it only when an authenticated bundle request asks for frontend evidence. Console calls are not continuously uploaded. Error toasts are reported immediately, but automatic reports remove URL query strings and fragments and use count and byte rate limits. A bundle can be partial if no browser responds, IndexedDB is unavailable, queues shed entries, or byte/profile limits truncate data. Ready bundles expire after 15 minutes; one active job per user and bounded global capacity can return `429` or `503`. Standard bundles cap backend/frontend/runtime sources at 160/80/2 MiB; ACP-inclusive bundles use 96/48/2 MiB plus 96 MiB ACP, with no budget transfer between sources.

Process-manager logs remain useful for live output:

- Linux service: `kandev service logs -f` reads the systemd journal.
- macOS service: the same command tails `<home>/logs/service.out` and `service.err`.
- Docker: `docker logs -f kandev`.
- Kubernetes: `kubectl logs -f deployment/kandev`.

When reporting an incident, record timestamp/timezone, Kandev version and commit from **System > About**, task ID, session ID, executor type, repository/branch, and relevant provider request IDs. Bundles can contain install-wide backend events, full browser URLs, console arguments, and stacks; review them as sensitive data before sharing. Agents can request backend, frontend, or all sources and should search a known task ID exactly before broadening. **System > Licenses** is a generated inventory of shipped npm and Go dependencies, not a runtime health check.

## Disk use and environment cleanup

**Settings > System > Status** walks `data`, worktrees, repositories, sessions, tasks, quick chat, and the default `data/backups` directory. Results are cached for two hours; **Refresh** forces a new single-flight walk. Permission failures appear as warnings. Backup files outside the resolved home are not included in the total. The displayed total intentionally counts `data/backups` both inside the `data` row and again as the separate `backups` row, so use filesystem or volume metrics for quota enforcement.

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

Stable is the default update channel. It resolves signed releases from the public GitHub Releases
API. A verified Kandev-managed npm or npx user service can select **Nightly** in
**Settings > System > Updates**, then use the shared **Save changes** action. Nightly resolves the
public npm `kandev@nightly` dist-tag. The install-wide selection survives restart. Desktop,
Homebrew, system-service, unmanaged, local-checkout, unknown, and invalid-metadata installs stay on
Stable and the page explains why Nightly is unavailable.

The backend checks the selected source once at startup and every six hours, with a 30-second HTTP
timeout, and persists the last successful Stable and Nightly results separately. **Check now**
performs a synchronous request and permits one manual check per process every 30 seconds. Offline,
rate-limited, or registry-failing installations continue to show that channel's cached state.

Nightlies are best-effort daily snapshots. A new version appears only when `main` contains commits
after the latest Stable release and that exact commit has not already been published. Nightlies do
not move npm `latest` and do not create Homebrew, Scoop, Desktop, GHCR, Git tag, or GitHub Release
artifacts.

Configure update alerts in **Settings > General > Notifications > Notification Events**. Select **Kandev update available** for each notification provider that should receive it: Local delivers the in-app update indication and can use an already-granted browser or native desktop notification; System and Apprise use their configured provider transports. Notification routing is independent of the Stable/Nightly release selector. New Local and System providers include this event by default; existing Local and System providers receive it once on upgrade, while existing Apprise providers remain unchanged. Disabling the event for a provider stops that provider's delivery without disabling release checks. Each provider receives a release version at most once; a later version is a new occurrence. Returning from Nightly to a Stable version is an explicit action and does not produce a normal upgrade notification.

Before any update, finish or stop active sessions, create and export a database backup plus its master key, preserve unpushed Git work, and read release notes.

- Desktop: use **Settings > System > Updates** when signed updater assets are available; otherwise install the new desktop package.
- Managed npm/npx user service: choose Stable or Nightly, save, check the resolved exact version,
  then use **Apply update**. Apply submits the exact immutable version shown; if a newer channel
  target was cached meanwhile, the backend rejects the stale request so the page can refresh. To
  recover from Nightly, select Stable and apply the shown release. For terminal recovery, an
  npm-managed service runs `npm install -g kandev@latest` and then `kandev service install` with the
  same flags; an npx-managed service runs
  `npx -y kandev@latest service install` with the same flags. Restart afterward. An npx-managed
  service points into npm's transient `_npx` cache and stops working if that cache is pruned; prefer
  `npm install -g` for a durable service.
- Managed Homebrew user service: Stable only. Use **Apply update** when available; otherwise run
  `brew upgrade kandev`, reinstall the service with the same flags, and restart.
- System service: Stable only. Upgrade with the required privileges, reinstall with the same
  service flags, and restart; the UI never applies it.
- Unmanaged CLI: run `brew upgrade kandev`, `scoop update kandev`, or `npm install -g kandev@latest`, then restart the process.
- Transient npx: start the desired release with `npx -y kandev@latest`; this does not update a persistent package.
- Docker/Kubernetes: replace the image and recreate the workload. Do not treat an in-container package install as a durable update.

After restart, verify `/ready`, **System > About**, **System > Status**, the database page, and one non-destructive agent session. Kandev does not perform automatic binary or database rollback. If rollback is necessary, restore a compatible pre-upgrade database and matching application release together.

## Resource metrics

Configure sampling at **Settings > Preferences > Appearance > Resource Metrics**. Defaults are CPU, memory, and disk percentage every five seconds, backend disk path `/`, and execution-environment collection off. Valid intervals are 1–300 seconds; at least one of CPU, memory, disk, CPU temperature, or 1-minute system load remains selected. System load is the average number of tasks running or waiting for CPU during the last minute; compare it with the host's CPU core count. Enable **Simplified metrics** to show only each metric icon and value in the status bar, fallback top bar, or phone Status drawer, without the Host marker or percentage progress bars.

Collection starts only while at least one connected client displays metrics in the status bar, fallback top bar, or an open phone Status drawer. Phone clients subscribe only while their Status drawer is open. The built-in status surface renders the Kandev host source only. Enabling execution metrics also adds active Docker, SSH, and Sprites `agentctl` sources to the metrics stream for separately owned consumers such as plugins; execution disk sampling uses `/`. A provider hook also exists for remote Docker, but creating that runtime currently returns a not-implemented error. Missing platform APIs, container permissions, an invalid disk path, a disconnected executor, macOS/Windows temperature support, or Windows load-average support produce unavailable samples rather than quotas.

These metrics are lightweight UI observability. Set alerts, retention, CPU/memory limits, and disk quotas in the host, container platform, or external monitoring stack.

## Status bar visibility

The status surface is off by default for each user. Open **Settings > Preferences > Appearance > Status Bar**, enable **Show status bar**, and use the page's shared **Save changes** action. The portable preference applies immediately without restarting Kandev. It shows a 24 px bottom bar on desktop and fine-pointer tablets; phones use native Status controls and an inset Status drawer.

Turning it off removes ordinary status chrome. Host metrics remain controlled separately by **Show host metrics in status bar** and move to the existing top-bar fallback when enabled. A saved language-server **Status location** also falls back to the editor toolbar when the status surface is unavailable. Active WebSocket connectivity warnings remain reachable through the warning-only fallbacks described below.

## WebSocket connectivity warnings

Kandev warns when its live WebSocket connection has not recovered for three seconds: yellow means the connection is unstable and reconnecting; after ten seconds the warning turns red, meaning live updates may be stale. The warning clears as soon as the connection recovers. With **Show status bar** on, it appears in the bottom bar on desktop/tablet and through the existing Status controls on phones. When the preference is off, an active warning still appears beside the sidebar theme control on desktop/tablet and opens a connection-only Status drawer on phones.

## Feature toggles

**Settings > System > Feature Toggles** currently exposes:

- **Office mode**: experimental, medium risk, and off in the production profile by default.
- **App status bar**: stable, low risk, and off in the production profile by default. Enabling it adds the desktop/tablet bar and phone Status entry after restart; disabling it again does not stop connections, metrics collection requested by other clients, or plugins. Urgent WebSocket connectivity warnings still remain visible while the feature is off.
- **Claude background prompt handoff**: experimental, high risk, and off in every profile by default. Enabling it lets Claude Code accept another prompt after its foreground yields while recognized async subagent, `run_in_background` shell, or Monitor work remains active. ACP lifecycle gaps can misclassify activity or overlap prompts; use it only for controlled testing.
- **Unread divider**: a per-user setting at **Settings > General > Task Actions**. It defaults off, takes effect immediately, and controls both the Slack-style **New** divider and read-cursor updates while that user's transcript view is visible.
- **Debug mode**: high risk; enables diagnostic endpoints and agent-message logging that can contain sensitive content.

Each feature toggle requires restart. A value supplied explicitly by its environment variable locks the UI control; the debug toggle is also locked by explicit legacy/debug-message environment variables. Otherwise the UI stores an override in the database. The page can request restart only when the native local supervisor is available. A normal Unix `kandev` terminal launch is supervised; Desktop, a service, a container, a directly started backend, a deploy preview, or Windows requires a manual application restart.

Status-bar layout is a separate per-user preference. Hold Cmd on macOS or Ctrl
elsewhere while mouse-dragging an item to move it across the desktop/tablet bar.
The backend preserves the layout across reloads and restarts; the phone Status
drawer mirrors it as the saved left sequence followed by the saved right sequence.

## Troubleshooting

| Symptom | Check | Action |
| --- | --- | --- |
| `/health` cannot connect | Process-manager and launcher output | Confirm port ownership, database reachability, writable Kandev home, and required executables; then restart once |
| `/ready` stays at 503 while `/health` is 200 | Backend startup logs | Process is alive but still wiring routes, seeding the agent registry, or mounting test-harness routes; give it more time before restarting |
| Status page says unhealthy while `/ready` is 200 | `/api/v1/system/health` issue IDs | Fix Git, GitHub, agent discovery, or Linux inotify warning; readiness and application diagnostics have different meanings |
| Backups page reports a 15-second create timeout | Reload the backup list and inspect the `backup-create` job/log | Large `VACUUM INTO` jobs can still finish; avoid double-clicking and ensure free disk |
| Backup/maintenance fails on PostgreSQL | Active driver on Database page | Use `pg_dump`, provider snapshots, and PostgreSQL maintenance; System backup/vacuum/reset is SQLite-only |
| Restored data looks stale | Whether the backend was restarted immediately | Quit/restart; do not keep using the old open database connections |
| Diagnostic bundle is partial | `manifest.json` warnings, source status, and loss counters | Keep a Kandev browser open for frontend capture; use backend-only when browser evidence is unnecessary |
| Update check returns HTTP 429 | Time since last **Check now** | Wait at least 30 seconds; background checks retry every six hours |
| **Apply update** is absent | Install mode/method and `<home>/service/install.json` | Expected for system, unmanaged, local-checkout, or invalid-metadata installs. A managed npm, npx, or Homebrew user service should offer Apply; reinstall it with the same flags to refresh its identity and metadata, or use the manual package-manager flow. |
| **Nightly** is disabled | Install mode/method and `<home>/service/install.json` | Expected unless this is a verified managed npm/npx user service. Homebrew, Desktop, system-service, unmanaged, local-checkout, invalid-metadata, and unknown installs are Stable-only. |
| Nightly check or save fails | npm access and cached checked time | Verify access to `https://registry.npmjs.org/kandev`, retry after connectivity returns, or keep/select Stable. A malformed or missing npm `nightly` tag fails closed. |
| Metrics show unavailable | OS support, disk path, executor connectivity | Select supported metrics and verify permissions/network; the collector reports errors per sample |
| Disk total exceeds filesystem expectation | Separate `data` and `backups` rows | Backups are counted twice in the UI total; use volume metrics for capacity decisions |
| Legacy `/tmp/kandev-agent/*` uses disk | Process inventory and open-file references for the exact directory | This is data from older Kandev versions, not a current Storage resource. Stop Kandev, confirm no live process references the target, then remove only the confirmed-inactive legacy directory through host administration. |
| Archived task's remote resource remains | Backend, Docker/SSH/Sprites, and provider logs | Cleanup is asynchronous and bounded. SSH task directories are retained by design; for other leftovers, verify work is preserved, then remove the exact resource manually |
| Sprites reset removed the environment but not the sandbox | **Settings > Executors > Sprites.dev** | Current reset can omit the provider credential during destroy; find the old Kandev-named sandbox and destroy it explicitly |

## Related pages

- [Configuration](configuration.md); paths, database, logging, NATS, Docker, and security-sensitive environment variables
- [Executors](executors.md); runtime lifecycle, credentials, cleanup, and isolation boundaries
- [Git operations](git-operations.md); branches, worktrees, push, and pull-request behavior
- [Automation and MCP](automation-and-mcp.md); external MCP routes and their current unauthenticated trust boundary
- [Windows support](windows-support.md); Windows-native limitations and supported alternatives
