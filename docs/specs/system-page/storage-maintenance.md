---
status: shipped
created: 2026-07-14
owner: cfl
---

# Storage Maintenance

## Why

Self-hosted Kandev installations execute many short-lived tasks that create worktrees,
dependency directories, Go build artifacts, and Docker resources. Archive and delete
normally release task resources, but interrupted cleanup and shared tool caches can still
consume the disk until an operator edits the host or runs broad commands such as
`docker system prune -a`. Operators need an in-app, ownership-aware way to understand and
reclaim that space without maintaining cron or systemd configuration outside Kandev.

## What

- Settings includes a **System → Storage** page at `/settings/system/storage` for disk
  analysis, maintenance policy, manual cleanup, run history, and quarantined workspaces.
- The page presents storage analysis and maintenance policy as separate full-width sections.
  Analysis and cleanup state replaces the label and icon inside the action button that started it
  instead of appearing as detached page status.
- User-facing storage totals and editable size limits are shown in GB. The frontend converts those
  values to and from the byte-based API without changing the persisted data model.
- Maintenance settings use separate cards grouped by scope: schedule, workspaces and containers,
  Go build cache, Docker cleanup, and quarantine safety. Every option includes focusable,
  pointer-accessible help that explains what it can change, when it runs, and which safety checks
  apply. Threshold and path fields are disabled while their parent cleanup option is disabled;
  quarantine retention remains independently editable because it also governs existing entries.
- Read-only analysis is available even when scheduled maintenance is disabled. It reports total
  task workspace bytes alongside active and orphan-candidate bytes, active quarantined count and
  bytes, the managed Go cache, the service user's default Go cache when it is a distinct path,
  Kandev-managed container count and writable-layer bytes, Docker image-layer bytes, Docker build
  cache, and unused Docker images.
- Scheduled maintenance is install-wide, persists in Kandev's database, and is disabled by
  default. Enabling it does not require editing the VM, a systemd unit, or environment
  variables.
- Scheduled destructive work runs only after Kandev has been resource-idle for the configured
  quiet period. Resource-idle means there is no task execution starting, preparing, running,
  stopping, or executing a shell command, test, setup script, cleanup script, or Docker image
  build.
- A task launch that arrives after maintenance acquired the idle gate cancels the maintenance
  context, waits for the active provider operation to stop, and then proceeds. Maintenance
  never races a newly admitted task.
- Manual **Run now** uses the same mutual-exclusion/current-activity gate, but does not wait out
  the configured quiet period. It returns a visible busy result when task activity is current or
  another maintenance run holds the gate, and has no force-while-busy mode.
- The Go-cache analysis row exposes a resource-specific **Clean Go cache** action only when the
  cache is Kandev-owned and above its configured maximum. That action submits an explicit
  `go_cache` selection through the same manual-run gate.
- Manual **Analyze** is read-only, does not require the idle gate, and never changes files,
  containers, images, caches, or database rows.
- Each cleanup resource has its own enablement and threshold. The initial defaults are:
  - orphan task workspaces: enabled after scheduled maintenance is enabled;
  - stopped/orphaned Kandev-managed containers: enabled after scheduled maintenance is enabled;
  - Kandev-managed Go build cache: disabled, 15 GiB maximum when enabled;
  - Docker build cache: disabled;
  - unused Docker images: disabled;
  - Docker volumes: never globally pruned.
- Host-global Docker cleanup requires a persisted **This is a dedicated Docker daemon**
  acknowledgment. Clearing the acknowledgment disables Docker build-cache and unused-image
  cleanup immediately.
- Kandev invokes typed, built-in maintenance providers. Users cannot configure arbitrary shell
  commands to run as the Kandev service account.

### Task cleanup and orphan workspaces

Decision: [ADR-2026-07-19-workspace-symlink-entries](../../decisions/2026-07-19-workspace-symlink-entries.md)

- Archive, delete, cascade, workspace-delete, and quick-chat expiration persist a task resource
  cleanup intent before mutating or removing the task row. Cleanup inventory needed after a
  task deletion is captured in that intent and is not dependent on foreign-keyed rows surviving.
- A durable worker replaces detached fire-and-forget task cleanup. Failed and interrupted jobs
  remain retryable across backend restarts with their last error and next attempt time.
- Archive-triggered cleanup re-checks that the task is still archived before every destructive
  step. If it has been unarchived, remaining cleanup is cancelled without deleting the newly
  active task's resources.
- The storage reconciler treats `~/.kandev/tasks/` as Kandev-owned but follows the fail-closed
  inventory rules in [ADR 0009](../../decisions/0009-fail-closed-gc-semantics.md). A directory is
  only a candidate when an authoritative inventory query succeeds, no active task environment,
  execution, session worktree, or protected ancestor references it, and it is older than the
  configured orphan grace period.
- The authoritative inventory covers both task layouts:
  `tasks/<semantic-task-dir>/<repo>` and `tasks/<workspace-id>/<task-id>`.
- Ready environment rows and active worktree rows protect files while their owning task exists and
  is not archived. A ready environment owned by an archived or deleted task remains protected while
  a live session of an unarchived task borrows it. Other rows retained for archived-task branch
  recovery are historical metadata, not live workspace references.
- New task roots contain a Kandev ownership marker with the task ID, workspace ID, task directory
  name, layout version, and creation time. Legacy unmarked directories remain eligible only when
  the authoritative inventory and grace-period checks positively classify them as unreferenced.
- Candidate task directories are atomically moved, on the same filesystem, to
  `~/.kandev/trash/tasks/`; they are not immediately deleted. Quarantine entries record their
  original path, size, task/workspace identity when known, and permanent-deletion deadline.
- The Storage page describes quarantine as a recoverable holding area, explains when Kandev uses
  it, and distinguishes restore from immediate permanent deletion.
- The default orphan grace period is seven days and the default quarantine retention is seven
  additional days. Both are configurable in whole hours and apply to scheduled and manual runs.
- Quarantine never deletes a Git branch. Permanent deletion removes the quarantined files and
  prunes stale Git worktree registration only after the retention deadline.
- Users can restore a quarantined task workspace to its original path while that path is free.
  A path conflict fails closed and leaves the quarantine entry intact.

### Unarchive compatibility

- Storage cleanup preserves historical `task_session_worktrees` rows and branch metadata for
  archived tasks. A historical row is recovery metadata, not proof that its old on-disk path is
  active.
- When an archived task is unarchived while its workspace is quarantined and the quarantine entry
  carries that task ID, Kandev restores the directory to its original path before probing branch
  recovery. If restoration fails, unarchive still succeeds and reports the quarantine failure
  alongside the existing branch-recovery status.
- When no quarantine entry exists, unarchive behavior remains the contract introduced by
  [PR #1687](https://github.com/kdlbs/kandev/pull/1687): the next task execution reuses a local or
  remote branch when recoverable and warns when the branch is missing.
- Permanently deleting an archived workspace does not delete historical recovery rows. A later
  unarchive can still recover a pushed branch from its remote.

### Go build cache

- Enabling managed Go cache changes new host-local task executions to use
  `<KANDEV_HOME_DIR>/cache/go-build` through an injected absolute `GOCACHE` value. Kandev setup,
  cleanup, shell, agent, test, and build processes for that execution observe the same value.
- Containerized and remote executors keep an executor-local cache. Kandev does not inject a host
  cache path into them without an explicit mount or remote storage contract.
- Kandev creates an ownership marker beside the managed cache. It never deletes the default user
  cache such as `/root/.cache/go-build` unless that exact path was explicitly adopted through the
  Storage page with a destructive confirmation.
- Analysis reports the managed cache's current bytes and read-only usage for the service user's
  distinct default Go cache (`$GOCACHE` when absolute, otherwise the platform user-cache path).
  Reporting the default cache does not adopt it or grant cleanup ownership. Cleanup rotates the
  owned cache into Kandev trash and recreates an empty cache when its size is greater than the
  configured maximum. The limit is a cleanup trigger, not a hard quota; the cache can temporarily
  grow beyond it while tasks are active.
- Disabling managed Go cache stops injecting `GOCACHE` into new executions. It does not delete the
  previously managed cache. Scheduled cleanup and a global manual run with no resource selection
  leave it untouched; only a manual run whose non-empty selection includes `go_cache` may rotate it.

### Docker storage

- Kandev-owned container cleanup lists only containers labeled `kandev.managed=true`. It removes
  a stopped container only after the task/runtime inventory positively shows it is orphaned or no
  longer needed. Running containers are never removed by storage maintenance.
- Analysis reports the daemon's image-layer bytes and the count and writable-layer bytes of exactly
  labeled Kandev-managed containers; an unavailable usage API degrades Docker analysis without
  failing the other resource summaries.
- Docker build-cache and unused-image analysis may inspect the configured Docker daemon without
  changing it.
- Docker build-cache cleanup uses the Docker API's age/storage filters and does not invoke
  `docker system prune`.
- Unused-image cleanup removes only images unused by any container and older than the configured
  age. Because image/build-cache ownership cannot be reliably attributed to Kandev, both actions
  remain disabled unless the dedicated-daemon acknowledgment is set.
- Kandev never performs a daemon-wide volume prune. Volumes attached to a positively identified
  Kandev container may be removed through the existing container teardown path.
- An unavailable or unsupported Docker daemon degrades the Docker cards to **Unavailable** and
  does not fail workspace or Go-cache maintenance.

## Data model

### Install setting: `storage_maintenance`

The existing install-wide `settings` key/value table stores one JSON object under the
`storage_maintenance` key.

```text
enabled                              bool      default false
check_interval_hours                 int       default 24; range 1..168
idle_for_minutes                     int       default 10; range 1..1440
orphan_grace_hours                   int       default 168; range 24..2160
quarantine_retention_hours           int       default 168; range 24..2160
workspaces.enabled                   bool      default true
kandev_containers.enabled            bool      default true
go_cache.enabled                     bool      default false
go_cache.max_bytes                   int64     default 16106127360; minimum 1073741824
go_cache.adopted_path                string    default ""; absolute and explicitly confirmed
docker.dedicated_daemon_acknowledged bool      default false
docker.build_cache_enabled           bool      default false
docker.build_cache_keep_bytes        int64     default 10737418240; minimum 1073741824
docker.build_cache_unused_hours      int       default 168; minimum 24
docker.unused_images_enabled         bool      default false
docker.unused_images_hours           int       default 168; minimum 24
```

Unknown JSON fields are ignored on read. Missing fields receive current defaults. Invalid writes
return `400` and preserve the previously saved object. `PATCH settings` cannot set a previously
empty `go_cache.adopted_path` without the dedicated adoption endpoint, and a transition of
`docker.dedicated_daemon_acknowledged` from false to true requires the confirmation token described
below. An adopted Go-cache path must be on the same filesystem as Kandev trash so quarantine remains
an atomic rename.

### `task_resource_cleanup_jobs`

Durable intent for task lifecycle cleanup. It deliberately has no foreign key to `tasks`, because
delete cleanup must survive removal of the task row.

```text
id                string     primary key
operation_id      string     unique idempotency key for one lifecycle mutation
task_id           string     indexed, no foreign key
trigger           enum       archive | delete | cascade_archive | cascade_delete |
                             workspace_delete | quick_chat_expire | reconcile
state             enum       pending | running | retry_wait | succeeded | cancelled
resource_snapshot json       captured runtime/environment/worktree/path handles
attempts          int        non-negative
next_attempt_at   timestamp  nullable
last_error        string     default ""
created_at        timestamp
updated_at        timestamp
completed_at      timestamp  nullable
```

Each lifecycle mutation supplies one stable `operation_id` to its cleanup job. Repeated delivery of
the same mutation reuses that job; a later archive/delete cycle uses a new operation ID.

### `storage_maintenance_runs`

```text
id               string     primary key; also the System job ID
trigger          enum       scheduled | manual | analysis
state            enum       queued | running | succeeded | failed | cancelled | skipped_busy
settings_snapshot json      policy used by this run
result           json       per-provider counts, bytes before/after, warnings
message          string     default ""
started_at       timestamp
completed_at     timestamp  nullable
```

The UI lists the newest 20 runs. Run rows survive backend restarts.

### `storage_quarantine_entries`

```text
id                string     primary key
resource_type     enum       task_workspace | go_cache
task_id           string     nullable, no foreign key
workspace_id      string     nullable, no foreign key
original_path     string     absolute, normalized, unique while active
quarantine_path   string     absolute, beneath <KANDEV_HOME_DIR>/trash
size_bytes        int64
state             enum       quarantined | restored | deleted | failed
quarantined_at    timestamp
delete_after      timestamp
restored_at       timestamp  nullable
deleted_at        timestamp  nullable
last_error        string     default ""
metadata          json       ownership marker and Git worktree details
```

## API surface

All routes are under the existing authenticated System route group.

```text
GET    /api/v1/system/storage
       -> { settings, capabilities, summary, last_run }

PATCH  /api/v1/system/storage/settings
       body: {
         settings: complete StorageMaintenanceSettings object,
         confirmations?: { dedicated_docker?: "DEDICATED" }
       }
       -> { settings }

POST   /api/v1/system/storage/go-cache/adopt
       body: { path: string, confirm: "ADOPT" }
       -> { settings, capabilities }

POST   /api/v1/system/storage/analyze
       -> 202 { job_id }

POST   /api/v1/system/storage/run
       body: { resources?: string[] }
       -> 202 { job_id }
       -> 409 { error, busy_resources[] } when the idle gate is unavailable

GET    /api/v1/system/storage/runs?limit=20
       -> { runs: StorageMaintenanceRun[] }

GET    /api/v1/system/storage/quarantine
       -> { entries: StorageQuarantineEntry[] }

POST   /api/v1/system/storage/quarantine/:id/restore
       -> { entry }

DELETE /api/v1/system/storage/quarantine/:id
       body: { confirm: "DELETE" }
       -> 202 { job_id }
```

`capabilities` reports the managed Go path, whether Go-cache adoption is available, Docker
availability, configured Docker host, and whether host-global Docker cleanup is allowed. API
responses never expose secret environment values.

Storage operations use the existing `system.job.update` WebSocket event and polling fallback.
Job kinds are `storage-analysis`, `storage-cleanup`, and `storage-quarantine-delete`.

The task unarchive response may additionally include:

```json
{
  "workspace_recovery": [
    {
      "task_id": "...",
      "status": "restored|not_found|failed",
      "message": "..."
    }
  ]
}
```

## State machine

### Scheduled maintenance

```text
disabled
  -> eligible                 setting enabled or next interval reached
eligible
  -> skipped_busy             quiet period or idle gate unavailable
  -> running                  quiet period satisfied and idle gate acquired
running
  -> succeeded                selected providers finish
  -> failed                   provider or persistence failure
  -> cancelled                task launch preempts maintenance
```

A `skipped_busy` run does not advance destructive state. The scheduler evaluates eligibility again
at the next interval. Provider failure is isolated: a Docker failure does not roll back a workspace
quarantine or prevent a later Go-cache provider from running, but the overall run is `failed` and
records each provider result.

### Task cleanup intent

```text
pending -> running -> succeeded
                   -> cancelled       archived task became active again
                   -> retry_wait      bounded attempt failed
retry_wait -> running                 next attempt or manual maintenance run
```

### Quarantine entry

```text
quarantined -> restored
            -> deleted
            -> failed -> restored|deleted
```

## Permissions

Storage routes use the same install-user authorization as other System pages. Adopting an external
Go cache, acknowledging a dedicated Docker daemon, permanently deleting a quarantine entry, and
enabling host-global Docker cleanup require explicit UI confirmation and server-side validation.

## Failure modes

- Any authoritative workspace inventory query failure aborts workspace classification and performs
  no workspace move or deletion.
- Any uncertainty about path ownership, containment, active descendants, owned control-path
  symlinks, or task activity keeps the directory and records a warning. Nested workspace symlinks
  are opaque entries: analysis, quarantine, and deletion never follow their targets.
- A quarantine rename failure leaves the original directory untouched and records a failed entry
  only when the failure can be associated with a durable candidate ID.
- A backend crash after rename but before the database update is reconciled at startup by scanning
  ownership manifests beneath `<KANDEV_HOME_DIR>/trash`.
- A task unarchived while archive cleanup is pending cancels remaining destructive cleanup. A task
  launch cannot pass the activity gate until cancellation completes.
- An unarchive quarantine restore conflict leaves both the existing destination and quarantine
  entry untouched and reports `workspace_recovery.status=failed`.
- An invalid or unreadable settings object falls back to disabled scheduling, reports a health
  warning, and does not run destructive maintenance.
- A managed Go-cache cleanup failure leaves either the original cache or its quarantined rename
  intact; it never recursively deletes outside the configured owned path.
- Docker list/usage failure marks Docker analysis unavailable. Docker prune failure records the
  daemon error and does not affect other providers.
- Loss of the dedicated-daemon acknowledgment between analysis and cleanup cancels host-global
  Docker operations.
- Failure to persist a run or cleanup intent prevents its destructive operation from starting.

## Persistence guarantees

- Settings, cleanup intents, maintenance runs, and quarantine entries survive backend restarts.
- A scheduled loop starts only when `enabled=true`; startup does not immediately run destructive
  cleanup. The first scheduled run is eligible after one full configured interval.
- Pending/retryable task cleanup resumes after startup independent of scheduled-maintenance
  enablement. Task lifecycle cleanup is a correctness guarantee, not an optional disk policy.
- Kandev retains historical archived-task worktree rows required by branch recovery. Filesystem
  cleanup and permanent quarantine deletion do not cascade-delete that history.
- Quarantined data remains restorable until permanent deletion succeeds. A failed permanent delete
  remains visible and retryable.
- Run history retains the newest 20 completed entries plus all non-terminal entries.

## Scenarios

- **GIVEN** scheduled maintenance has never been configured, **WHEN** Kandev starts as a systemd
  daemon, **THEN** no destructive storage cleanup runs and the Storage page shows scheduling off.
- **GIVEN** scheduling is disabled, **WHEN** the user selects **Analyze**, **THEN** the page shows
  reclaimable bytes without changing any filesystem or Docker resource.
- **GIVEN** scheduling is enabled and a task is running a Go test, **WHEN** the maintenance interval
  arrives, **THEN** the run is recorded as `skipped_busy` and no provider changes resources.
- **GIVEN** maintenance holds the idle gate, **WHEN** a new task launch arrives, **THEN** maintenance
  is cancelled and the launch proceeds only after the active provider stops.
- **GIVEN** an unreferenced task directory older than the orphan grace period contains
  `node_modules`, **WHEN** workspace cleanup runs with a successful authoritative inventory,
  **THEN** the whole task root moves to quarantine and its measured bytes appear in the run result.
- **GIVEN** task roots include active, recent orphan, and grace-eligible orphan directories,
  **WHEN** storage analysis runs, **THEN** total workspace bytes include every classified task root
  while active and reclaimable bytes remain separate subsets.
- **GIVEN** archived or deleted tasks retain ready environment or active worktree rows for recovery,
  **WHEN** storage analysis or cleanup classifies their old directories, **THEN** those historical
  rows do not protect the directories from normal orphan grace and quarantine rules unless a live
  session of an unarchived task still borrows the environment.
- **GIVEN** the worktree inventory query fails, **WHEN** workspace cleanup runs, **THEN** no task
  directory moves and the run reports the inventory error.
- **GIVEN** a multi-repository task has one active descendant worktree, **WHEN** workspace cleanup
  scans the task root, **THEN** ancestor protection keeps the complete task root.
- **GIVEN** a repository-less task uses `tasks/<workspace-id>/<task-id>`, **WHEN** it is active,
  **THEN** inventory protection keeps that task directory without protecting unrelated orphan task
  siblings in the same workspace directory.
- **GIVEN** a quarantined task workspace has not reached its deletion deadline, **WHEN** the user
  selects **Restore**, **THEN** it returns to its original path and remains available to the task.
- **GIVEN** an archived task has a quarantined workspace, **WHEN** the user unarchives it, **THEN**
  Kandev restores the quarantined directory before reporting branch recovery.
- **GIVEN** an archive cleanup job is waiting to retry, **WHEN** the task is unarchived, **THEN** the
  cleanup job becomes `cancelled` and does not delete the active task's resources.
- **GIVEN** an archived task's workspace was permanently deleted but its branch exists on origin,
  **WHEN** the task is unarchived, **THEN** branch recovery remains `remote` and a new execution can
  recreate the worktree from origin.
- **GIVEN** managed Go cache is enabled and is 20 GiB with a 15 GiB threshold, **WHEN** an idle
  cleanup runs, **THEN** Kandev rotates the owned cache to trash, recreates an empty cache, and
  reports the reclaimed bytes.
- **GIVEN** `/root/.cache/go-build` was not explicitly adopted, **WHEN** storage cleanup runs,
  **THEN** Kandev does not modify it.
- **GIVEN** `/root/.cache/go-build` is the service user's default Go cache and is not adopted,
  **WHEN** storage analysis runs, **THEN** its path and bytes are reported read-only while cleanup
  remains unavailable for that path.
- **GIVEN** the Docker daemon reports image-layer usage, **WHEN** storage analysis runs, **THEN**
  image-layer bytes are shown separately from build-cache and managed-container writable bytes.
- **GIVEN** an exited container has `kandev.managed=true` and its task is positively absent,
  **WHEN** container cleanup runs, **THEN** the container and its attached Kandev volumes are removed.
- **GIVEN** an unrelated exited container exists, **WHEN** Kandev container cleanup runs, **THEN**
  the container remains unchanged.
- **GIVEN** Docker build-cache cleanup is selected without the dedicated-daemon acknowledgment,
  **WHEN** settings are saved or cleanup is requested, **THEN** the request is rejected without
  invoking Docker prune APIs.
- **GIVEN** the Storage page is opened on a mobile viewport, **WHEN** the user navigates through the
  settings sheet, analyzes storage, and expands a resource result, **THEN** every value and action is
  available without horizontal page scrolling or hover-only controls.
- **GIVEN** settings were saved, **WHEN** the backend restarts, **THEN** the Storage page shows the
  persisted policy and the next run uses it.

## Out of scope

- A hard filesystem quota for Go or Docker caches.
- Arbitrary user-defined maintenance commands or cron expressions.
- Global Docker volume or network pruning.
- Killing processes by executable name when no durable Kandev ownership handle exists.
- Cleaning remote SSH executor filesystems; remote maintenance requires a separate explicit design.
- Restoring uncommitted files after their quarantine retention has expired.
- Automatically cleaning a pre-existing user Go cache without explicit path adoption.

## Implementation plan

See [the implementation plan](../../plans/storage-maintenance/plan.md).
