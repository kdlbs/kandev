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
# Storage Maintenance System Design Part 3

## Purpose and boundaries

This design preserves the technical source detail for `REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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
- **GIVEN** an active Go-cache quarantine entry and a replacement cache above its threshold,
  **WHEN** cleanup runs, **THEN** the Go-cache result reports `skipped: true` with
  `reason: "active_quarantine"`, reclaims zero bytes, and the maintenance run does not fail.
- **GIVEN** an active Go-cache entry whose quarantine payload is absent and whose original path
  contains a replacement cache, **WHEN** eligible or forced permanent deletion runs, **THEN** the
  entry becomes `deleted`, the replacement cache remains unchanged, and deleted bytes are zero.
- **GIVEN** an active Go-cache entry whose quarantine payload is absent, **WHEN** restore runs,
  **THEN** restore fails closed and the entry remains active.
- **GIVEN** `/root/.cache/go-build` was not explicitly adopted, **WHEN** storage cleanup runs,
  **THEN** Kandev does not modify it.
- **GIVEN** an external Go cache was adopted successfully, **WHEN** the Storage page rerenders or is
  reopened, **THEN** the external-cache input contains the persisted adopted path.
- **GIVEN** `/root/.cache/go-build` is the service user's default Go cache and is not adopted,
  **WHEN** storage analysis runs, **THEN** its path and bytes are reported read-only while cleanup
  remains unavailable for that path.
- **GIVEN** a current-install temporary-artifact row has a matching marker and is closed or
  abandoned beyond 24 hours, **WHEN** Storage analysis runs, **THEN** its bytes/counts appear in the
  temporary-artifact resource as a stale candidate without changing the filesystem.
- **GIVEN** an active or recently heartbeating registered artifact, **WHEN** Storage analysis or
  cleanup runs, **THEN** it is reported as protected and remains untouched regardless of size.
- **GIVEN** an unregistered `kandev-*` directory, a package/compiler cache, a preview/CI/dev-
  isolated root, or legacy `/tmp/kandev-agent/*`, **WHEN** temporary-artifact cleanup runs, **THEN**
  it is neither counted as owned nor modified.
- **GIVEN** a stale registered artifact and no current activity blocker, **WHEN** the user confirms
  **Clean stale artifacts**, **THEN** the request uses `resources: ["temporary_artifacts"]`, the root
  moves by same-filesystem rename into quarantine, and the result appears in Quarantine.
- **GIVEN** the user selects unscoped **Run now** or scheduled maintenance, **WHEN** a stale
  registered temporary artifact exists, **THEN** the artifact remains in place because this provider
  is explicit-only.
- **GIVEN** a registered artifact has a missing/mismatched marker, a symlinked root, a path escape,
  or a cross-device quarantine destination, **WHEN** analysis or cleanup runs, **THEN** Kandev
  reports the safety warning and leaves the original path unchanged.
- **GIVEN** a quarantined temporary artifact's original path is free, **WHEN** the user selects
  **Restore**, **THEN** Kandev restores it through the existing quarantine flow before retention
  expiry; unrelated quarantine entries are not purged by the resource-specific run.
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
- Cleaning unregistered `kandev-*` roots from another installation or from standalone preview, CI,
  E2E, or `dev-isolated` harnesses.
- A persisted temporary-artifact threshold or scheduled cleanup toggle in the first release.
- Guaranteed compatibility with tools that require a fixed, globally unique name in shared temp;
  those tools need a scoped path override when a real collision is observed.

## Implementation plan

- [Original Storage maintenance implementation](../../../plans/storage-maintenance/plan.md)
- [Registered Kandev temporary-artifact cleanup](../../../plans/storage-temp-artifacts/plan.md)
- [Storage overview cache and settings follow-up](../../../plans/storage-overview-cache/plan.md)
- [Quarantine lifecycle follow-up](../../../plans/quarantine-lifecycle/plan.md)
- [Progressive Storage loading and totals](../../../plans/storage-progressive-loading/plan.md)
- [Go-cache quarantine lifecycle repair](../../../plans/go-cache-quarantine-lifecycle/plan.md)
