---
status: draft
created: 2026-05-18
owner: tbd
---

# System pages

## Why

Today, settings-style operational concerns (health, disk usage, current version, OSS attribution, log access, database maintenance) have no home in kandev's UI. The existing settings sidebar is product-configuration only (executors, agents, integrations); the only system-shaped entry — Changelog — sits awkwardly inside it. When something goes wrong, users have no path inside the app to see "where is my database, how big is it, what version am I on, is there an update, where are the logs." This forces them into the filesystem and into GitHub, and it makes recoverable problems (bloated SQLite, corrupt state) look unrecoverable.

Radarr and Sonarr solve this with a dedicated **System** area: a group of read-only diagnostic pages plus a small number of gated maintenance actions. This spec brings the same shape to kandev, scoped to the kandev-specific surface (SQLite + worktrees + GitHub releases + lumberjack log files).

## What (v1)

A new **System** group is added to the existing settings sidebar (`apps/web/components/settings/settings-app-sidebar.tsx`), alongside General/Workspaces/Agents/Executors/Integrations/etc. The group contains seven child pages, each on its own route under `/settings/system/*`. The existing `/settings/changelog` route is removed and its UI is absorbed into the new **Updates** page.

### Pages

1. **Status** — `/settings/system/status`
   - **Health issues** card: renders the existing `GET /api/v1/system/health` payload (warning/error/info issues with messages and links).
   - **Disk usage** card: shows total kandev data footprint with a per-subdirectory breakdown (`data dir / worktrees / repos / sessions / tasks / quick-chat / backups`), an "as of HH:MM" timestamp, and a **Refresh** button. The disk walk is lazy and asynchronous: the first visit after a cold start (or after the 2h cache expires) returns `null` immediately and the page shows a loading spinner while the walk runs in the background; subsequent visits within 2h return the cached value instantly.
   - **Version + update** card: short summary of current version and "update available" badge, with a CTA to the Updates page.
2. **Database** — `/settings/system/database`
   - Read-only: SQLite file path, file size, WAL size, schema version, last-backup timestamp (from `<data-dir>/backups/`).
   - **VACUUM** button — safe, reclaims space; shows progress and final size delta.
   - **Optimize** button — runs `PRAGMA optimize`; shows progress.
   - **Factory Reset** button — destructive. Wipes the full kandev install (database + worktrees + repo clones + session dirs + tasks dir + quick-chat dir), equivalent to deleting `~/.kandev/` and starting over. Opens a modal that requires (a) typing the literal string `RESET` to enable the confirm button, (b) acknowledging that the backend will create a pre-reset snapshot first and then restart. Server-side: stop orchestrator and running executions → snapshot DB to `<data-dir>/backups/pre-reset-<ts>.db` → drop DB tables and re-run migrations → `rm -rf` the worktrees/repos/sessions/tasks/quick-chat subdirs → restart backend process. The frontend disables the UI and waits for the backend to come back, then routes to the empty onboarding state.
3. **Backups** — `/settings/system/backups`
   - Lists existing snapshots in `<data-dir>/backups/` with name, size, and mtime.
   - Per-row actions: **Download**, **Restore** (gated like Factory Reset), **Delete** (confirm-only).
   - **Create snapshot** button at the top of the list; uses the existing `VACUUM INTO` path.
4. **Logs** — `/settings/system/logs`
   - Static log viewer: last 1000 lines of the current lumberjack log file, with a **Refresh** button. No live tail in v1.
   - List of rotated log files with name/size/mtime and a per-row **Download** button.
   - **Download current** button at the top.
5. **Updates** — `/settings/system/updates`
   - Shows running version, latest available version, and a "new version available" badge if newer.
   - **Check now** button — forces a re-poll (rate-limited 30s per process).
   - Below: the embedded changelog list (the content currently rendered at `/settings/changelog` via `@/generated/changelog.json`).
6. **Licenses** — `/settings/system/licenses`
   - Renders a JSON manifest of all third-party OSS dependencies (npm + Go), each with its license name, version, repository URL, and license text. The manifest is generated at build time and committed to the repo.
   - Searchable list (filter by package name or license type), one-line per row, expand to see full license text.
7. **About** — `/settings/system/about`
   - Version, build commit, build timestamp, Go runtime version, Node runtime version, OS/arch.
   - Links: GitHub repo, documentation, license, "Report an issue".

### Sidebar badge

The **System** group header (and the **Status** child entry) show a numeric badge equal to `count(health.issues where severity != info) + (updateAvailable ? 1 : 0)`. The badge is sourced from the existing `useSystemHealth` hook plus the new updates hook; no new WS topic is required for v1.

### Backend surface

A new package `apps/backend/internal/system/` owns these endpoints. It absorbs the existing `internal/health/` package as a sub-component. Endpoints use the same auth model as the rest of `/api/v1/settings/*` (no separate admin tier); see [Permissions](#permissions) for the destructive-action confirmation pattern.

```
GET    /api/v1/system/health                      (existing; unchanged)
GET    /api/v1/system/info                        - versions, commit, build time, OS/arch
GET    /api/v1/system/disk-usage                  - cached breakdown + computedAt; null while computing
POST   /api/v1/system/disk-usage/refresh          - kick async recompute; 202
GET    /api/v1/system/database                    - path, sizeBytes, walSizeBytes, schemaVersion, lastBackupAt
POST   /api/v1/system/database/vacuum             - 202 + jobId
POST   /api/v1/system/database/optimize           - 202 + jobId
POST   /api/v1/system/database/reset              - factory reset; body { confirm: "RESET" }
GET    /api/v1/system/backups                     - list snapshots
POST   /api/v1/system/backups                     - create snapshot; 202 + jobId
GET    /api/v1/system/backups/:name/download      - stream snapshot file
POST   /api/v1/system/backups/:name/restore       - restore; body { confirm: "RESTORE" }
DELETE /api/v1/system/backups/:name               - delete snapshot
GET    /api/v1/system/logs                        - { files: [{ name, size, mtime }] }
GET    /api/v1/system/logs/tail?n=1000            - last N lines of current log
GET    /api/v1/system/logs/:name/download         - stream log file
GET    /api/v1/system/updates                     - { current, latest, latestCheckedAt, releaseUrl, install, applySupported }
POST   /api/v1/system/updates/check               - force GitHub re-poll; rate-limited 30s
POST   /api/v1/system/updates/apply               - queue service-only self-update; body { confirm: "UPDATE" }
```

Long-running operations (vacuum, optimize, reset, restore, snapshot create, disk walk) return `202 Accepted` with a `jobId` and publish progress on the existing event bus. The frontend subscribes via WS (`system.job.update` event) to render progress and final result. On success/failure the operation flips a corresponding entry in the existing health surface (e.g., a "VACUUM completed: reclaimed X MB" info issue that auto-expires).

### Updates poller

A background goroutine in `internal/system/updates/poller.go` starts on backend boot and polls GitHub releases for `kdlbs/kandev` every 6 hours (unauthenticated; well under the 60 req/hr/IP unauth limit). It compares `tag_name` to the running version (semver) and persists the result to `kandev_meta`:

- `latest_version TEXT` — highest published semver tag (e.g., `1.2.4`)
- `latest_version_url TEXT` — URL to that GitHub release
- `latest_version_checked_at INTEGER` — unix timestamp of last successful poll

The `GET /api/v1/system/updates` handler reads from `kandev_meta` only; it never calls GitHub synchronously. It also reports the current service install state (`running_as_service`, `managed_service`, `mode`, `manager`, `kind`) so the UI can decide whether one-click apply is allowed. `POST /api/v1/system/updates/check` triggers an out-of-band refresh, rate-limited per-process to one call per 30 seconds. If the GitHub call fails (offline, rate limited, 5xx), the handler returns the last-known value and the `latestCheckedAt` exposes the staleness.

`POST /api/v1/system/updates/apply` is available only when Kandev is running as a kandev-managed user service (`systemd --user` or launchd user agent) and the latest release is newer than the current binary. It writes an update intent under `<KANDEV_HOME_DIR>/service/update-intents/`, starts a manager-owned helper (`systemd-run --user` on Linux, or a one-shot transient LaunchAgent plist bootstrapped via `launchctl bootstrap` on macOS), and returns a `self-update` system job id. Under `KANDEV_E2E_MOCK=true`, the helper path is fake so UI and backend tests can exercise the flow without mutating npm/Homebrew or restarting the service.

### Disk-usage cache

Cache is an in-memory `{ value *Breakdown, computedAt time.Time, computing bool }` guarded by a mutex. The walk is lazy — never runs at boot. `GET /api/v1/system/disk-usage` returns immediately:

- If `value == nil && !computing` → start the walk in a goroutine, return `{ data: null, computing: true }`.
- If `value == nil && computing` → return `{ data: null, computing: true }`.
- If `value != nil && time.Since(computedAt) < 2h` → return `{ data: value, computing: false }`.
- If `value != nil && time.Since(computedAt) >= 2h` → return `{ data: value, computing: true }` and start a background refresh.

`POST /api/v1/system/disk-usage/refresh` forces a refresh regardless of TTL. The job publishes a `system.job.update` event so the frontend can swap the cached value for the fresh one without polling.

### Licenses generation

Generation is **lockfile-driven and committed to the repo**. The file is read statically in dev, build, prod, and release with zero runtime cost and no network access.

- **Generator:** `apps/web/scripts/generate-licenses.ts` — pure function of two inputs.
  - npm side: walks `pnpm-lock.yaml` via `license-checker-rseidelsohn` (or equivalent), capturing `{ name, version, licenses, repository, licenseFile }` per package.
  - Go side: resolves modules from `apps/backend/go.sum` via `go-licenses report` (or by reading vendored LICENSE files), captured into the same shape.
  - Output: `apps/web/generated/licenses.json`, **committed to git**.
- **Local trigger:** `pnpm licenses:gen` — devs run it after a dep bump.
- **CI gate:** A workflow step in `.github/workflows/` runs the generator and fails the build if the result differs from the committed file. This guarantees the artifact is up to date whenever `pnpm-lock.yaml` or `go.sum` changes, without requiring it to run on every `pnpm build`.

The page reads the JSON statically; no backend endpoint is needed.

## Scenarios

- **GIVEN** a user opens `/settings/system/status` for the first time after backend boot, **WHEN** the page mounts, **THEN** the Disk Usage card shows a spinner and "Calculating…", the backend kicks off the walk, and the value populates within seconds without further interaction (via WS job update or 5s poll fallback).
- **GIVEN** the disk-usage cache is 30 minutes old, **WHEN** the user reopens the Status page, **THEN** the cached value renders instantly with "as of <30 min ago>" and **no** refresh kicks off.
- **GIVEN** the disk-usage cache is 3 hours old, **WHEN** the user reopens the Status page, **THEN** the stale value renders immediately, the page badge shows "Refreshing…", and the value updates when the background walk completes.
- **GIVEN** the user is on `1.2.3` and the GitHub latest release is `1.2.4`, **WHEN** they open `/settings/system/updates`, **THEN** an "Update available" badge renders next to the version, the changelog list shows `1.2.4` highlighted as the new entry, and the System sidebar group shows a `1` badge.
- **GIVEN** the user is running a kandev-managed user service and a newer release exists, **WHEN** they open `/settings/system/updates`, **THEN** the page shows **Apply update** and the confirmation queues a `self-update` job.
- **GIVEN** the user is not running as a kandev-managed service or is running a `--system` service, **WHEN** they open `/settings/system/updates`, **THEN** the page does not render an update-apply control and instead shows the manual update commands.
- **GIVEN** the user clicks **VACUUM** on the Database page, **WHEN** the operation completes, **THEN** the DB size delta is shown ("Reclaimed 12.3 MB"), the page Database stats refresh, and a transient info issue appears on the Status page.
- **GIVEN** the user clicks **Factory Reset**, types `RESET`, and confirms, **WHEN** the backend executes, **THEN** a fresh snapshot is created first, all tables are dropped and migrations re-run, the backend restarts, and the frontend redirects to the empty onboarding state once it reconnects.
- **GIVEN** the backend cannot reach GitHub, **WHEN** the poller fires, **THEN** the failure is logged but the previous `latest_version` and `latest_version_checked_at` remain in `kandev_meta`; the Updates page surfaces the stale value with a "Last checked <time>" subtitle.
- **GIVEN** the user clicks **Check now** twice within 30 seconds, **WHEN** the second click fires, **THEN** the endpoint returns `429 Too Many Requests` and the UI shows "Already checked, try again in <N>s".
- **GIVEN** lumberjack has rotated the current log into `kandev.log.1`, **WHEN** the user opens `/settings/system/logs`, **THEN** the viewer shows the tail of the current `kandev.log` and the rotated files appear in the list with download buttons.
- **GIVEN** the user opens `/settings/system/licenses` while offline, **WHEN** the page renders, **THEN** every dependency's license text is available locally (no network calls).

## Data model

Two new columns on the existing `kandev_meta` table (key/value or pivoted to columns — match the existing schema). One new in-memory cache in the system package. No new SQLite tables.

```
ALTER TABLE kandev_meta ADD COLUMN latest_version TEXT;
ALTER TABLE kandev_meta ADD COLUMN latest_version_url TEXT;
ALTER TABLE kandev_meta ADD COLUMN latest_version_checked_at INTEGER;
```

(If `kandev_meta` is key/value-shaped, write three rows with these keys instead.)

## State machine — long-running jobs

Jobs (vacuum, optimize, reset, restore, snapshot create, disk walk) progress through:

```
queued → running → succeeded
                 ↘ failed
```

Each transition publishes `system.job.update` with `{ jobId, kind, state, message, result? }` on the event bus. Reset and restore additionally emit a `system.restart.pending` event that the frontend uses to switch into a "waiting for backend" state.

## Permissions

All System endpoints require the same "logged-in install user" check as the existing `/api/v1/settings/*` endpoints — there is no admin tier in v1. Factory reset and restore endpoints additionally validate the `confirm` body field server-side as a defence-in-depth check against accidental fetches.

## Failure modes

- **GitHub poll failure** — log + keep the previous `kandev_meta` row; `/api/v1/system/updates` returns the stale value.
- **Disk walk failure** (permission error on a subdir) — return the partial result with a `warnings: [...]` array per subdir; the page renders a per-row warning icon.
- **VACUUM failure** — DB is unaffected (VACUUM is atomic); the job ends `failed` with the SQLite error string. Status page shows a recoverable error issue.
- **Factory reset failure mid-run** — the pre-reset snapshot remains in `<data-dir>/backups/`; the user can restore it from the Backups page on next boot. Recovery is documented inline in the failure UI.
- **Restore failure** — original DB file is left untouched; restore writes to a temp file and atomic-renames only on success.
- **Log file missing / unreadable** — viewer renders an empty state with the file path so the user can investigate manually.

## Persistence guarantees

- `kandev_meta` writes are SQLite-atomic.
- Snapshot files in `<data-dir>/backups/` are created via `VACUUM INTO <tmp>` then atomic-renamed; partial files cannot appear.
- Restore writes the snapshot to `<data-dir>/kandev.db.new` and atomic-renames over `kandev.db` after closing all pool connections; on failure the original file is intact.
- The existing boot-time backup retention (newest 2) is preserved, but **only auto-snapshots are subject to it**. Snapshots are distinguished by filename prefix:
  - `auto-<version>-<ts>.db` — created automatically on a version-change boot or as the pre-reset snapshot before factory reset. Pruned by the existing "keep newest 2" rule (operating only on files with the `auto-` prefix).
  - `manual-<ts>.db` — created by the user clicking **Create snapshot** in the Backups page. Never auto-pruned; lives until the user deletes it explicitly from the same page.
- The existing pre-migration backup path is updated to use the `auto-` prefix (a one-shot rename of existing snapshots happens on first boot after this change, treating any extension-less / unprefixed `*.db` file under `<data-dir>/backups/` as `auto-<existing-name>` for retention purposes).

## Out of scope (v1)

- Live log tail (WS streaming of new lines as they're written).
- Channel-aware upgrade hints (npm vs homebrew vs binary install path).
- Admin/role gating of destructive actions; everyone who can sign in can VACUUM/Reset.
- Importing or analyzing older log files beyond filename listing + download.
- Inline editing of `kandev_meta` or other system tables.
- A separate top-level "System" nav entry alongside "Settings" (chose nested-in-Settings instead).
- Tasks/Cron page (Radarr-equivalent) — kandev's office routines surface already covers this.
- Events page — covered by the office activity feed.

## Future scope

- Live log tail via WS subscription.
- Channel-aware update flow (detect install method, surface the right upgrade command, optionally invoke `kandev upgrade` from the UI for binary installs).
- Selective reset (drop only tasks, only sessions, etc.) — only if users actually need finer-grained recovery than Restore-from-snapshot.
- Backup scheduling (e.g., daily snapshot) beyond the existing pre-migration trigger.
- Health-issue → action wiring (e.g., a "VACUUM" CTA inline on the "Database is XYZ MB" health issue).

## Open questions

- Should the Updates page show only the latest release entry, or the full changelog history with the running version highlighted? Default: **full history, running version highlighted**, matches the existing `/settings/changelog` page.
