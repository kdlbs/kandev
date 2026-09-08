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
# Storage Maintenance System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Why

Self-hosted Kandev installations execute many short-lived tasks that create worktrees,
dependency directories, Go build artifacts, and Docker resources. Archive and delete
normally release task resources, but interrupted cleanup and shared tool caches can still
consume the disk until an operator edits the host or runs broad commands such as
`docker system prune -a`. Operators need an in-app, ownership-aware way to understand and
reclaim that space without maintaining cron or systemd configuration outside Kandev.

The service also creates some longer-lived temporary roots for diagnostics and host
utilities. Operators need a way to reclaim abandoned roots created by this installation, but shared
temporary directories also contain unrelated caches, preview/CI data, developer harnesses, and other
Kandev installations. Cleanup must therefore prove ownership instead of treating a `/tmp` name or
mtime as sufficient evidence.

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
  registered temporary artifacts, Kandev-managed container writable layers, Docker image layers, and
  Docker build cache. Active and candidate workspace/temporary-artifact bytes and unused-image bytes
  remain visible subset measurements and are not added again. If any top-level measurement is
  unavailable, the total is visibly identified as partial rather than presented as complete host disk
  usage.
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
- Read-only analysis includes a separate **Kandev temporary artifacts** resource for exact paths
  registered by the current Kandev service. It reports total, active, stale-candidate, skipped, and
  unavailable bytes/counts without recursively scanning the shared operating-system temp directory.
- The resource row exposes a manual **Clean stale artifacts** action only for registered roots that
  are closed or abandoned, have no recent owner lease/heartbeat, and are older than a fixed 24-hour
  stale interval. The action uses an explicit `temporary_artifacts` selection through the existing
  activity gate and a reversible confirmation.
- Temporary-artifact cleanup is manual-only in the first release. Scheduled maintenance and an
  unscoped full **Run now** never delete this resource, and `storage_maintenance` gains no temp
  toggle. A future scheduled opt-in requires a separate operational decision.
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

Decision: [ADR-2026-07-19-workspace-symlink-entries](../../../decisions/2026-07-19-workspace-symlink-entries.md)

Retention override:
[ADR-2026-07-29-quarantine-retention-override](../../../decisions/2026-07-29-quarantine-retention-override.md)

- Archive, delete, cascade, workspace-delete, and quick-chat expiration persist a task resource
  cleanup intent before mutating or removing the task row. Cleanup inventory needed after a
  task deletion is captured in that intent and is not dependent on foreign-keyed rows surviving.
- A durable worker replaces detached fire-and-forget task cleanup. Failed and interrupted jobs
  remain retryable across backend restarts with their last error and next attempt time.
- Archive-triggered cleanup re-checks that the task is still archived before every destructive
  step. If it has been unarchived, remaining cleanup is cancelled without deleting the newly
  active task's resources.
- The storage reconciler treats `~/.kandev/tasks/` as Kandev-owned but follows the fail-closed
  inventory rules in [ADR 0009](../../../decisions/0009-fail-closed-gc-semantics.md). A directory is
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

- Storage cleanup preserves historical task-environment repository rows and branch metadata for
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
- Only one restorable Go-cache generation can remain active for an original path. If the replacement
  cache exceeds its maximum before that generation becomes terminal, cleanup reports a successful
  no-op with `skipped: true` and `reason: "active_quarantine"`. It does not create another intent,
  change either cache path, or fail the complete maintenance run.
- Permanent deletion treats an absent Go-cache quarantine payload as already removed after all
  confirmation, retention, ownership, containment, and state checks succeed. It marks the durable
  entry `deleted`, does not change a populated replacement cache, and reports zero deleted bytes for
  the absent payload. Restore continues to fail closed when the payload is absent.
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
  [ADR 0045](../../../decisions/0045-install-wide-storage-maintenance.md).

### Kandev-owned temporary artifacts

Decision: [ADR-2026-08-08-owned-temp-artifact-cleanup](../../../decisions/2026-08-08-owned-temp-artifact-cleanup.md)

- A backend service component creates a temporary artifact through the storage registry. The
  registry persists an exact absolute path, an allowlisted artifact kind, an opaque artifact ID,
  lifecycle timestamps, owner lease/heartbeat state, a per-service-run identity, and metadata in
  `storage_temp_artifacts`.
- The registry writes an owner-only `.kandev-temp-artifact.json` marker atomically inside the root.
  Analysis and cleanup require the marker's artifact ID and kind to match the durable record,
  validate the root with `Lstat`, reject symlink/path replacement, and never follow nested symlink
  targets while measuring or moving the root.
- The first registered producers are long-lived backend service roots such as Improve Kandev bundle
  directories and the host-utility parent directory. Standalone `acpdbg`/preview commands, E2E and
  `dev-isolated` harness roots, CI/PR checkouts, package-manager and compiler caches, generic `tmp.*`
  paths, and artifacts from another Kandev installation remain unregistered.
- Normal producer shutdown closes and removes its artifact. After restart, a missing owner lease or
  heartbeat, including a same-PID row from an earlier service run, can classify a registered root
  as abandoned, but it remains protected until the fixed 24-hour stale interval. Uncertain
  liveness, unreadable rows/markers, and missing paths are skipped and reported rather than treated
  as ownership.
- Explicit cleanup atomically renames an eligible root on the same filesystem into
  `<KANDEV_HOME_DIR>/trash/temporary-artifacts/` and records a `temporary_artifact` quarantine entry.
  A cross-device rename has no copy/delete fallback. Restore is allowed while the original path is
  free; permanent deletion follows the existing quarantine retention and path-safety rules.
- Existing unmarked `/tmp/kandev-agent/*` directories and arbitrary shared temp files remain host
  policy data even when their names resemble a registered prefix.

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
