---
status: draft
system: system-page
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-001
created: 2026-07-14
updated: 2026-08-12
owners:
  - cfl
---
# Storage Maintenance System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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
resource_type     enum       task_workspace | go_cache | temporary_artifact
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

### `storage_temp_artifacts`

```text
id                  string     primary key; opaque artifact ID
kind                enum       improve_bundle | host_utility
path                string     absolute, normalized, unique while active
marker_token        string     matches the owner-only marker in the root
state               enum       active | closed | abandoned | quarantined | deleted | failed
owner_pid           int        positive process ID
created_at          timestamp
last_heartbeat_at   timestamp  nullable
closed_at           timestamp  nullable
quarantined_at      timestamp  nullable
deleted_at          timestamp  nullable
last_error          string     default ""
metadata            json       producer details plus the registry run identity
```

The row is the current installation's ownership record; the marker is a second path-level check.
Unknown kinds, rows whose marker or path no longer matches, and rows whose root is missing are
reconciled without deletion. The fixed 24-hour stale interval is not a persisted setting in this
release.

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
availability, configured Docker host, whether host-global Docker cleanup is allowed, and whether the
registered temporary-artifact provider is available. API responses never expose secret environment
values.

The existing `POST /storage/run` resource selection accepts `temporary_artifacts` in addition to the
existing provider names. It is an explicit-only selection: an empty `resources` list and scheduled
maintenance invoke the provider's no-op path and never move temporary roots. The temporary-artifact
summary contains `total_bytes`, `active_bytes`, `protected_bytes`, `stale_bytes`, `total_count`,
`active_count`, `protected_count`, `stale_count`, `skipped_count`, `available`, and an optional
`warnings` array.

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

### Registered temporary artifact

```text
active    -> closed                 producer releases the lease normally
          -> abandoned              restart/reconciliation finds no live lease
closed    -> quarantined             explicit temporary_artifacts cleanup
abandoned -> quarantined             explicit cleanup after the stale interval
quarantined -> restored|deleted|failed
failed      -> quarantined|deleted
```

The `active`, `closed`, and `abandoned` states are never eligible before the fixed 24-hour stale interval.
State transitions require the matching durable row and marker; an uncertain transition leaves the
root in place.

## Permissions

Storage routes use the same install-user authorization as other System pages. Adopting an external
Go cache, acknowledging a dedicated Docker daemon, moving registered temporary artifacts to
quarantine, permanently deleting one or more quarantine entries, and enabling host-global Docker
cleanup require explicit UI confirmation and server-side validation. **Force clear all** uses the
distinct `DELETE ALL NOW` confirmation because it removes the configured restore window.

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
- An active Go-cache quarantine generation defers another rotation without failing maintenance.
  The active entry remains available for restore or permanent deletion.
- An absent Go-cache quarantine payload cannot be restored. Permanent deletion can close its durable
  entry without deleting or replacing data at the original cache path.
- A temporary-artifact registry read, marker read, lease/heartbeat check, or ownership/path
  validation failure keeps that root in place and records a skipped warning. There is no fallback to
  prefix, mtime, or process-name classification.
- A temporary-artifact rename failure, including a cross-device rename, leaves the original root
  untouched. The provider never copies and then deletes a root as a quarantine fallback.
- A backend crash after a temporary-artifact rename but before the lifecycle or quarantine state
  update is reconciled from the durable row, matching marker, and trash path. An unmatched path is
  retained rather than guessed into a quarantine entry.
- A recent active lease or heartbeat protects a temporary artifact even when its directory is large.
  A stale PID, absent lease, or unreadable liveness signal is not by itself permission to delete;
  the fixed stale interval and marker/row checks must also pass.
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
- A retained Go-cache generation keeps its unique active-path slot until restore or permanent
  deletion makes the entry terminal. A deferred cleanup does not change that entry or its deadline.
- Temporary-artifact ownership rows and matching markers survive backend restarts. A producer's
  normal close remains idempotent; an interrupted active row is reconciled and can become eligible
  only after the stale interval.
- Temporary-artifact analysis is included in the process-local 15-minute overview snapshot. A
  successful explicit cleanup invalidates that snapshot after its provider result is persisted.
- A temporary artifact moved to quarantine remains restorable until the existing retention deadline;
  the temporary-artifact action itself does not purge unrelated quarantine entries.
- Expired entries are automatically considered only by scheduled or full manual maintenance.
  Disabling scheduling prevents unattended quarantine deletion; it does not start an independent
  sweeper or remove existing entries.
- Run history retains the newest 20 completed entries plus all non-terminal entries.
