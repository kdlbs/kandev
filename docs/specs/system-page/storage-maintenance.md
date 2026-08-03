---
status: building
created: 2026-07-14
updated: 2026-08-02
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
- On first load, maintenance policy, maintenance history, quarantine, and storage analysis load as
  independent sections. A cold filesystem or Docker scan keeps only Storage analysis in its loading
  state; policy controls, persisted run history, and the database-backed quarantine list render as
  soon as their own requests finish.
- Each independently loaded section surfaces its own loading and failure state. A failed or slow
  analysis never replaces already available policy, history, or quarantine content with a page-wide
  loading or error state.
- User-facing storage totals and editable size limits are shown in GB. The frontend converts those
  values to and from the byte-based API without changing the persisted data model.
- Maintenance settings use separate cards grouped by scope: schedule, workspaces and containers,
  Go build cache, Docker cleanup, and quarantine safety. Every option includes focusable,
  pointer-accessible help that explains what it can change, when it runs, and which safety checks
  apply. Threshold and path fields are disabled while their parent cleanup option is disabled;
  quarantine retention remains independently editable because it governs entries created by future
  cleanup even when the other resource rules are disabled.
- Read-only analysis is available even when scheduled maintenance is disabled. It reports total
  task workspace bytes alongside active and orphan-candidate bytes, active quarantined count and
  bytes, the managed Go cache, the service user's default Go cache when it is a distinct path,
  Kandev-managed container count and writable-layer bytes, Docker image-layer bytes, Docker build
  cache, and unused Docker images.
- Storage analysis shows a total counted size derived from the available non-overlapping top-level
  measurements: total task workspaces, quarantine, managed and distinct user Go caches,
  Kandev-managed container writable layers, Docker image layers, and Docker build cache. Active and
  candidate workspace bytes and unused-image bytes remain visible subset measurements and are not
  added again. If any top-level measurement is unavailable, the total is visibly identified as
  partial rather than presented as complete host disk usage.
- A successful storage analysis is reused for 15 minutes. Opening or refreshing the Storage page,
  saving policy settings, and adopting an external Go cache consume that cached snapshot instead of
  starting another filesystem or Docker scan. Manual **Analyze** always bypasses the cache and
  replaces it with a fresh successful snapshot.
- The Storage analysis card shows when its snapshot was measured using a relative timestamp.
  Policy editing remains available while read-only analysis or cleanup jobs run. A settings
  mutation that can conflict with another settings mutation may block saving briefly, but the UI
  names that operation instead of reporting an unspecified storage action.
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
  the configured quiet period. When current Kandev activity blocks it, the page names each activity
  type it found (for example, a running agent session or test command), warns that cleanup can
  disrupt that work, and offers a distinct **Run anyway** action directly in the busy state.
- **Run anyway** starts the requested manual cleanup alongside the activity that originally blocked
  it. It skips only the current-activity admission check: it neither stops the activity nor allows
  two storage-maintenance runs to overlap. New task work may still preempt the cleanup through the
  normal maintenance cancellation path.
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
- The Quarantine section shows each entry's exact deletion-eligibility timestamp and whether it is
  protected or eligible now. It explains that the timestamp is the earliest deletion time, while
  the actual automatic deletion occurs on the first successful scheduled maintenance run after
  that time. When scheduling is disabled, it says that automatic deletion is off and names full
  manual **Run now** or an explicit quarantine action as the available cleanup paths.
- The Quarantine section shows the sum of `size_bytes` for every currently listed restorable entry.
  Its total is derived from the independently loaded quarantine list and does not wait for or depend
  on the Storage analysis snapshot.
- Scheduled and full manual maintenance runs permanently delete eligible quarantine entries.
  Resource-specific manual runs do not delete unrelated quarantine entries.
- **Clear eligible** permanently deletes every active quarantine entry whose retention deadline has
  elapsed and leaves protected entries intact. Its result reports deleted, protected, and failed
  entry counts and bytes.
- A separate **Force clear all** action may delete protected and eligible entries together after the
  user types `DELETE ALL NOW`. The force action bypasses only retention; it never bypasses resource
  ownership, path containment, symlink, state-transition, or Git worktree-pruning validation.

### Task cleanup and orphan workspaces

Decision: [ADR-2026-07-19-workspace-symlink-entries](../../decisions/2026-07-19-workspace-symlink-entries.md)

Retention override:
[ADR-2026-07-29-quarantine-retention-override](../../decisions/2026-07-29-quarantine-retention-override.md)

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
  it, and distinguishes retention-protected deletion from the explicitly forced bulk override.
- The default orphan grace period is seven days and the default quarantine retention is seven
  additional days. Both are configurable in whole hours and apply to scheduled and manual runs.
- Quarantine never deletes a Git branch. Permanent deletion removes the quarantined files and
  prunes stale Git worktree registration only after the retention deadline.
- Scheduled and full manual maintenance include a quarantine provider that permanently deletes
  entries whose deadlines have elapsed. A resource-specific manual run, such as **Clean Go cache**,
  does not purge unrelated quarantine entries.
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
- After adoption succeeds, the external Go-cache field shows the persisted
  `go_cache.adopted_path` immediately and after page reload. Unrelated overview refreshes do not
  erase a path the user is currently editing.
- Analysis reports the managed cache's current bytes and read-only usage for the service user's
  distinct default Go cache (`$GOCACHE` when absolute, otherwise the platform user-cache path).
  Reporting the default cache does not adopt it or grant cleanup ownership. Cleanup rotates the
  owned cache into Kandev trash and recreates an empty cache when its size is greater than the
  configured maximum. The limit is a cleanup trigger, not a hard quota; the cache can temporarily
  grow beyond it while tasks are active.
- Disabling managed Go cache stops injecting `GOCACHE` into new executions. It does not delete the
  previously managed cache. Scheduled cleanup and a global manual run with no resource selection
  leave it untouched; only a manual run whose non-empty selection includes `go_cache` may rotate it.

### Agent session temporary data

- Host-local agent instances inherit `TMPDIR`, `TMP`, and `TEMP` from the Kandev service unchanged.
  Kandev does not create or inject a per-instance temporary root. When the service leaves those
  variables unset, agents and their child tools use the operating system default temporary
  location; when an operator configures them for the service, every host-local agent shares that
  configured location.
- Tool-managed caches may therefore be shared when the tool's own default uses the temporary
  location. Persistent caches remain governed by their own variables and policies: in particular,
  Go's default `GOCACHE` is separate from `TMPDIR`, and Kandev only injects its managed Go-cache path
  when the existing Storage setting is explicitly enabled.
- Kandev-specific files that require collision-free identity must use an explicit unique path or
  filename. A future collision in one tool is fixed at that tool boundary; it does not justify
  replacing the complete temporary environment for every agent child process.
- Archive/delete teardown still closes process admission and reaps each owned process tree. It does
  not recursively delete arbitrary files from the inherited system temporary directory because
  those files are shared and cannot be attributed safely to one task.
- Existing `/tmp/kandev-agent/*` directories created by older versions are legacy host data. The
  Storage scheduler does not delete them by name or age, and new agent runs do not add to that root.
  Operators may remove confirmed-inactive legacy data through their normal host temporary-file
  policy or a deliberate one-time maintenance procedure. See
  [ADR 0045](../../decisions/0045-install-wide-storage-maintenance.md).

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
state             enum       pending | running | retry_wait | succeeded | failed | cancelled
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
       -> { settings, capabilities, summary, analyzed_at, last_run }

GET    /api/v1/system/storage/settings
       -> { settings, capabilities }

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
       body: { resources?: string[], force?: boolean }
       -> 202 { job_id }
       -> 409 {
            error: string,
            busy_resources: [{ kind: string, label: string }],
            force_available: boolean
          } when current activity or another maintenance run holds the gate

GET    /api/v1/system/storage/runs?limit=20
       -> { runs: StorageMaintenanceRun[] }

GET    /api/v1/system/storage/quarantine
       -> { entries: StorageQuarantineEntry[] }

POST   /api/v1/system/storage/quarantine/:id/restore
       -> { entry }

DELETE /api/v1/system/storage/quarantine/:id
       body: { confirm: "DELETE" }
       -> 202 { job_id }
       -> 409 before the entry's delete_after timestamp

DELETE /api/v1/system/storage/quarantine
       body: {
         scope: "eligible" | "all",
         confirm: "DELETE ELIGIBLE" | "DELETE ALL NOW"
       }
       -> 202 { job_id }
```

`capabilities` reports the managed Go path, whether Go-cache adoption is available, Docker
availability, configured Docker host, and whether host-global Docker cleanup is allowed. API
responses never expose secret environment values.

`GET /storage/settings` is the lightweight policy-read contract. It reads persisted settings and
capabilities without requesting an overview snapshot or invoking filesystem, Go-cache, quarantine,
or Docker analysis providers. `GET /storage` remains backward compatible and continues to return
the complete scan-backed overview contract.

`analyzed_at` is the RFC 3339 timestamp of the successful analysis that produced `summary`.
`GET /storage` reuses that snapshot for 15 minutes. `POST /storage/analyze` bypasses the freshness
window and replaces the cached snapshot only when the forced analysis succeeds.

Storage operations use the existing `system.job.update` WebSocket event and polling fallback.
Job kinds are `storage-analysis`, `storage-cleanup`, and `storage-quarantine-delete`.

Bulk quarantine jobs expose this result shape:

```json
{
  "scope": "eligible|all",
  "considered": 4,
  "deleted": 2,
  "deleted_bytes": 2048,
  "protected": 1,
  "protected_bytes": 1024,
  "failed": 1,
  "failures": [{ "id": "...", "error": "..." }]
}
```

`scope: "eligible"` requires `DELETE ELIGIBLE` and skips entries whose `delete_after` timestamp is
still in the future. `scope: "all"` requires `DELETE ALL NOW` and may bypass that timestamp. The
server rejects mismatched scope/confirmation pairs with `400`.

`busy_resources` uses stable machine-readable `kind` values and plain-language `label` values.
The response exposes activity categories, not task names, prompts, paths, or other session content.
`force_available` is true only when `force: true` can bypass the reported current task activity;
it is false when another storage-maintenance run already holds the install-wide maintenance lease.
When `force: true` is accepted, the response remains the ordinary `202 { job_id }` cleanup-job
contract.

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

The quarantine provider runs during scheduled and full manual maintenance. It evaluates the
persisted `delete_after` value at run time and attempts permanent deletion only for eligible
entries. If any entry deletion fails, successful deletions remain committed, the failed entry
remains visible and retryable, and the maintenance run records the quarantine provider failure.

### Task cleanup intent

```text
pending -> running -> succeeded
                   -> cancelled       archived task became active again
                   -> retry_wait      attempts 1-7 failed; retry uses the
                                      1m, 5m, 15m, 1h, 3h, 6h, 12h schedule
                   -> failed          attempt 8 failed; terminal diagnostic
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
Go cache, acknowledging a dedicated Docker daemon, permanently deleting one or more quarantine
entries, and enabling host-global Docker cleanup require explicit UI confirmation and server-side
validation. **Force clear all** uses the distinct `DELETE ALL NOW` confirmation because it removes
the configured restore window.

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
- Bulk quarantine deletion continues after one entry fails. Successful entries remain deleted,
  protected entries remain unchanged for eligible-only cleanup, failed entries remain visible with
  their last error, and the job finishes as failed with per-entry failure details.
- A force-clear request bypasses only the retention timestamp. Any ownership, containment, symlink,
  ambiguous-state, or Git worktree-pruning failure keeps the affected entry and reports the error.
- Docker list/usage failure marks Docker analysis unavailable. Docker prune failure records the
  daemon error and does not affect other providers.
- A policy, history, quarantine, or analysis read failure is isolated to that section. Other
  successful section responses remain visible and usable, and retrying or completing one section
  does not discard newer data already returned by another.
- Loss of the dedicated-daemon acknowledgment between analysis and cleanup cancels host-global
  Docker operations.
- Failure to persist a run or cleanup intent prevents its destructive operation from starting.

## Persistence guarantees

- Settings, cleanup intents, maintenance runs, and quarantine entries survive backend restarts.
- The 15-minute analysis snapshot is process-local and does not survive a backend restart. The first
  Storage overview request after startup measures a new snapshot; later requests reuse it until it
  expires or manual **Analyze** replaces it.
- A scheduled loop starts only when `enabled=true`; startup does not immediately run destructive
  cleanup. The first scheduled run is eligible after one full configured interval.
- Pending/retryable task cleanup resumes after startup independent of scheduled-maintenance
  enablement. Task lifecycle cleanup is a correctness guarantee, not an optional disk policy.
- Kandev retains historical archived-task worktree rows required by branch recovery. Filesystem
  cleanup and permanent quarantine deletion do not cascade-delete that history.
- Quarantined data remains restorable until permanent deletion succeeds. A failed permanent delete
  remains visible and retryable.
- Expired entries are automatically considered only by scheduled or full manual maintenance.
  Disabling scheduling prevents unattended quarantine deletion; it does not start an independent
  sweeper or remove existing entries.
- Run history retains the newest 20 completed entries plus all non-terminal entries.

## Scenarios

- **GIVEN** scheduled maintenance has never been configured, **WHEN** Kandev starts as a systemd
  daemon, **THEN** no destructive storage cleanup runs and the Storage page shows scheduling off.
- **GIVEN** scheduling is disabled, **WHEN** the user selects **Analyze**, **THEN** the page shows
  reclaimable bytes without changing any filesystem or Docker resource.
- **GIVEN** the first Storage analysis after backend startup is still scanning, **WHEN** the
  lightweight policy, history, and quarantine requests complete, **THEN** those three sections are
  visible and usable while only Storage analysis continues to show progress.
- **GIVEN** one Storage section request fails, **WHEN** another section request succeeds, **THEN**
  the successful section renders its current data and the failed section shows its own error state.
- **GIVEN** a successful storage snapshot is less than 15 minutes old, **WHEN** the user refreshes
  the page or saves policy settings, **THEN** the same summary and `analyzed_at` are returned without
  invoking the storage providers again.
- **GIVEN** a cached storage snapshot of any age, **WHEN** the user selects **Analyze**, **THEN** all
  analysis providers run and a successful result replaces the snapshot and `analyzed_at`.
- **GIVEN** a storage analysis or cleanup job is running, **WHEN** the user edits maintenance
  policy, **THEN** the policy controls and shared Save action remain available.
- **GIVEN** scheduling is enabled and a task is running a Go test, **WHEN** the maintenance interval
  arrives, **THEN** the run is recorded as `skipped_busy` and no provider changes resources.
- **GIVEN** maintenance holds the idle gate, **WHEN** a new task launch arrives, **THEN** maintenance
  is cancelled and the launch proceeds only after the active provider stops.
- **GIVEN** a running agent session or command blocks manual cleanup, **WHEN** the user selects
  **Run now**, **THEN** the page names the activity categories found, warns that cleanup can disrupt
  them, and shows **Run anyway** without opening a confirmation dialog.
- **GIVEN** a manual cleanup is blocked only by current task activity, **WHEN** the user selects
  **Run anyway**, **THEN** Kandev starts the requested cleanup with `force: true` while the existing
  activity continues and records the normal cleanup-job result.
- **GIVEN** a manual cleanup is blocked by another storage-maintenance run, **WHEN** the user views
  the busy feedback, **THEN** it identifies maintenance as the blocker and does not offer a bypass.
- **GIVEN** an unreferenced task directory older than the orphan grace period contains
  `node_modules`, **WHEN** workspace cleanup runs with a successful authoritative inventory,
  **THEN** the whole task root moves to quarantine and its measured bytes appear in the run result.
- **GIVEN** task roots include active, recent orphan, and grace-eligible orphan directories,
  **WHEN** storage analysis runs, **THEN** total workspace bytes include every classified task root
  while active and reclaimable bytes remain separate subsets.
- **GIVEN** all analysis providers return measurements, **WHEN** Storage analysis renders its total,
  **THEN** it sums the non-overlapping top-level measurements once and does not add active,
  candidate, or unused-image subset bytes again.
- **GIVEN** one top-level analysis measurement is unavailable, **WHEN** Storage analysis renders its
  total, **THEN** it sums the available measurements and identifies the result as partial.
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
- **GIVEN** protected and eligible quarantine entries, **WHEN** the Storage page renders, **THEN**
  each row shows its exact `delete_after` timestamp and protected-or-eligible status, and the page
  states whether automatic scheduled cleanup is enabled.
- **GIVEN** multiple restorable quarantine entries, **WHEN** the independently loaded Quarantine
  section renders, **THEN** its total equals the sum of every listed entry's `size_bytes` without
  waiting for Storage analysis.
- **GIVEN** protected and eligible quarantine entries, **WHEN** the user confirms **Clear eligible**
  with `DELETE ELIGIBLE`, **THEN** every eligible entry is permanently deleted, protected entries
  remain, and the completed job reports both groups.
- **GIVEN** protected and eligible quarantine entries, **WHEN** the user confirms **Force clear
  all** with `DELETE ALL NOW`, **THEN** Kandev attempts to permanently delete both groups while
  retaining every ownership and path-safety check.
- **GIVEN** an eligible quarantine entry and enabled scheduling, **WHEN** the next scheduled
  maintenance run acquires the idle gate, **THEN** its quarantine provider permanently deletes the
  entry and reports the reclaimed bytes.
- **GIVEN** an eligible quarantine entry and disabled scheduling, **WHEN** no manual maintenance or
  quarantine action occurs, **THEN** the entry remains restorable and the page states that
  automatic cleanup is off.
- **GIVEN** one invalid quarantine payload among otherwise deletable entries, **WHEN** bulk deletion
  runs, **THEN** valid entries are deleted, the invalid entry remains visible, and the job reports
  the partial failure.
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
- **GIVEN** an external Go cache was adopted successfully, **WHEN** the Storage page rerenders or is
  reopened, **THEN** the external-cache input contains the persisted adopted path.
- **GIVEN** `/root/.cache/go-build` is the service user's default Go cache and is not adopted,
  **WHEN** storage analysis runs, **THEN** its path and bytes are reported read-only while cleanup
  remains unavailable for that path.
- **GIVEN** the Kandev service has no temporary-directory variables configured, **WHEN** two
  host-local agents start, **THEN** neither instance receives an injected `TMPDIR`, `TMP`, or `TEMP`
  value and their tools use the operating system defaults.
- **GIVEN** an operator sets `TMPDIR`, `TMP`, or `TEMP` on the Kandev service, **WHEN** a host-local
  agent starts, **THEN** it inherits those values unchanged rather than receiving a per-instance
  replacement.
- **GIVEN** a task is archived or deleted, **WHEN** its local instance tears down, **THEN** Kandev
  reaps its owned processes but does not sweep the shared default temporary directory.
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
- **GIVEN** multiple protected and eligible quarantine entries on a mobile viewport, **WHEN** the
  user reviews deadlines and completes either bulk action, **THEN** the same counts, confirmation,
  result feedback, and safety behavior are available through at least 44-pixel touch targets
  without horizontal page scrolling.
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
- An independent quarantine sweeper that runs while scheduled maintenance is disabled.
- Promising an exact permanent-deletion instant when the maintenance idle gate may delay or preempt
  a scheduled run.
- Letting the force-clear retention override bypass ownership, containment, symlink, state, or Git
  worktree-pruning safety checks.
- Age-based or name-based deletion of unmarked `/tmp/kandev-agent/*` directories.
- A Kandev-owned general-purpose sweeper for the operating system's shared temporary directory.
- Guaranteed compatibility with tools that require a fixed, globally unique name in shared temp;
  those tools need a scoped path override when a real collision is observed.

## Implementation plan

- [Original Storage maintenance implementation](../../plans/storage-maintenance/plan.md)
- [Storage overview cache and settings follow-up](../../plans/storage-overview-cache/plan.md)
- [Quarantine lifecycle follow-up](../../plans/quarantine-lifecycle/plan.md)
- [Progressive Storage loading and totals](../../plans/storage-progressive-loading/plan.md)
