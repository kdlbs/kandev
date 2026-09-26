---
status: active
system: system-page
created: 2026-07-14
updated: 2026-09-22
owners:
  - cfl
---
# Storage Maintenance Requirements

## Overview

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

## Requirements

### REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-001: Storage Maintenance

**Intent:** Self-hosted Kandev installations execute many short-lived tasks that create worktrees,
dependency directories, Go build artifacts, and Docker resources. Archive and delete normally
release task resources, but interrupted cleanup and shared tool caches can still consume the disk
until an operator edits the host or runs broad commands such as `docker system prune -a`. Operators
need an in-app, ownership-aware way to understand and reclaim that space without maintaining cron or
systemd configuration outside Kandev. The service also creates some longer-lived temporary roots for
diagnostics and host utilities. Operators need a way to reclaim abandoned roots created by this
installation, but shared temporary directories also contain unrelated caches, preview/CI data,
developer harnesses, and other Kandev installations. Cleanup must therefore prove ownership instead
of treating a `/tmp` name or mtime as sufficient evidence.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.1:** Settings includes a **System → Storage** page at `/settings/system/storage` for disk analysis, maintenance policy, manual cleanup, run history, and quarantined workspaces.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.2:** The page presents storage analysis and maintenance policy as separate full-width sections. Analysis and cleanup state replaces the label and icon inside the action button that started it instead of appearing as detached page status.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.3:** On first load, maintenance policy, maintenance history, quarantine, and storage analysis load as independent sections. A cold filesystem or Docker scan keeps only Storage analysis in its loading state; policy controls, persisted run history, and the database-backed quarantine list render as soon as their own requests finish.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.4:** Each independently loaded section surfaces its own loading and failure state. A failed or slow analysis never replaces already available policy, history, or quarantine content with a page-wide loading or error state.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.5:** User-facing storage totals and editable size limits are shown in GB. The frontend converts those values to and from the byte-based API without changing the persisted data model.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.6:** Maintenance settings use separate cards grouped by scope: schedule, workspaces and containers, Go build cache, Docker cleanup, and quarantine safety. Every option includes focusable, pointer-accessible help that explains what it can change, when it runs, and which safety checks apply. Threshold and path fields are disabled while their parent cleanup option is disabled; quarantine retention remains independently editable because it governs entries created by future cleanup even when the other resource rules are disabled.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.7:** Read-only analysis is available even when scheduled maintenance is disabled. It reports total task workspace bytes alongside active and orphan-candidate bytes, active quarantined count and bytes, the managed Go cache, the service user's default Go cache when it is a distinct path, Kandev-managed container count and writable-layer bytes, Docker image-layer bytes, Docker build cache, and unused Docker images.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.8:** Storage analysis shows a total counted size derived from the available non-overlapping top-level measurements: total task workspaces, quarantine, managed and distinct user Go caches, registered temporary artifacts, Kandev-managed container writable layers, Docker image layers, and Docker build cache. Active and candidate workspace/temporary-artifact bytes and unused-image bytes remain visible subset measurements and are not added again. If any top-level measurement is unavailable, the total is visibly identified as partial rather than presented as complete host disk usage.

### REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-002: Database footprint visibility

**Status:** Active.

**Intent:** Operators can see the space occupied by the local database and its backups.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.1:** Storage analysis shall show separate Database and Database backups sizes, with their resolved locations available in row details.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.2:** For SQLite, the Database measurement shall include the database file and its existing SQLite sidecars (`-wal`, `-shm`, `-journal`). Backups shall include automatic and manual files in the active backup directory.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.3:** Total counted shall include the independently attributed bytes from both measurements once. A fully overlapping measurement shall identify that overlap without increasing the total, and a partially overlapping measurement shall show its full footprint while adding only its non-overlapping bytes.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.4:** Missing backups shall show zero. Failed measurements shall show unavailable and make the total partial, while successful categories remain visible.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.5:** Both rows shall participate in initial scan progress, cached snapshots, and manual Analyze refresh. While a source is pending or scanning without a measurement, its row shall show that source progress. Missing terminal response fields shall remain unknown or unavailable and shall not appear as measured zero.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.6:** For a database driver without a local SQLite footprint, both rows shall explain that local measurement is not applicable. They shall not increase the total or alone make it partial.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.7:** Desktop and mobile users shall be able to inspect both rows and long paths without horizontal page scrolling. Labels and explanations shall use the selected language.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.8:** Analysis shall remain read-only and preserve existing access restrictions. It shall not create backups, compact databases, change retention, or initiate cleanup.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.9:** The analysis shall visibly explain that its counted categories do not represent all filesystem usage.

#### Exclusions

Remote database sizing, additional cleanup controls, backup retention changes, database compaction,
host-wide reconciliation, and additional categories such as logs are outside this extension.
The existing application size-unit convention remains unchanged.

### REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-003: System temporary storage visibility

**Status:** Active.

**Intent:** Operators can inspect shared temporary storage without granting cleanup ownership.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.1:** Analysis shall measure the service temporary folder and, on Unix, `/tmp` when it resolves to a distinct folder.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.2:** Each folder shall show its resolved path, measured size, and completeness. Aliases and nested roots shall not increase the combined measurement twice.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.3:** The temporary-folder measurement shall appear as an informational footprint outside Total counted. Its explanation shall identify overlap with classified resources.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.4:** Registered Kandev artifacts shall retain their separate classification and existing contribution to Total counted. Shared and legacy files shall not appear as reclaimable Kandev artifacts.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.5:** Analysis shall join existing scan progress, caching, and Analyze refresh. Slow temporary-folder scans shall not delay policy, history, or completed sources.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.6:** Unreadable, disappearing, or unscanned entries shall produce visible partial results. An unavailable folder shall not appear as measured zero.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.7:** Analysis shall not read file contents, follow nested symlinks, cross nested mounts, change ownership, or remove files.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.8:** Desktop and phone users shall inspect sizes, long paths, and limitations without horizontal page scrolling. Copy shall use the selected language.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.9:** Existing storage access restrictions shall apply. Analysis requests shall not accept arbitrary paths from clients.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.10:** When the temporary-folder scan reaches its deadline, the expanded row shall show one localized timeout explanation and retain sampled bytes and partial status. Repeated deadline messages shall not appear. Other diagnostics shall remain bounded to ten distinct examples, with existing skipped-entry counts preserved.

### REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-004: Scheduled cleanup of owned temporary artifacts

**Status:** Active.

**Intent:** Operators can include verified Kandev artifacts in maintenance while shared files remain protected.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-004.1:** A persisted temporary-artifact cleanup option shall default to disabled, including after upgrade.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-004.2:** When the option is enabled, scheduled and full manual maintenance shall consider registered artifacts. Scheduling shall still require the global schedule option.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-004.3:** Explicit temporary-artifact cleanup shall remain available with the option disabled. Other resource-specific actions shall not clean temporary artifacts.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-004.4:** Cleanup shall require verified ownership, an inactive lifecycle, and at least 24 hours since closure or abandonment. Uncertain ownership or liveness shall protect the artifact.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-004.5:** Cleanup shall revalidate eligibility at mutation time. Busy overrides shall never bypass ownership, liveness, age, or path checks.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-004.6:** Eligible artifacts shall enter recoverable quarantine. A same-filesystem move shall use rename; a cross-filesystem move shall use a staged, verified copy under quarantine, publish it atomically, and remove the original only after source identity is revalidated. Copy, publication, or identity failures shall leave the original intact and report a failure.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-004.7:** Results shall distinguish quarantined bytes from freed space. Existing retention, restore, cancellation, and run-history behavior shall remain available.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-004.8:** Desktop and phone users shall inspect cleanup scope, save the option, run explicit cleanup, and inspect its result. Existing mutation restrictions shall apply.

#### Exclusions

Shared-file deletion, legacy-directory adoption, arbitrary cleanup paths, configurable stale age,
new artifact producers, remote temporary folders, and changes to inherited agent temporary variables
are outside this extension. Whole-host disk reconciliation remains outside storage analysis.

### REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-005: Relative storage usage bars

**Intent:** Operators can identify the largest measured categories without opening each storage row.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-005.1:** Analysis rows shall sort by measured bytes, largest first. Equal sizes shall retain their existing category order. Unmeasured rows shall appear last in that order.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-005.2:** Each measured row shall show its size and a proportional bar in the collapsible header. Every bar shall use zero as its origin and the largest displayed measurement as its maximum.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-005.3:** Measured zero shall show an empty track and its size. Unknown, pending, failed, and not-applicable rows shall retain their status without a measured bar. Partial measurements shall use sampled bytes and retain visible partial status.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-005.4:** Rows shall retain independent expansion, keyboard activation, and existing detail actions. Sorting and refreshed measurements shall preserve expansion and focus for each remaining row.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-005.5:** Bars shall compare category footprints without changing Total counted or the separate filesystem-capacity indicator. Visible copy shall explain relative sizing and possible category overlap. Bar color shall not imply a cleanup recommendation or capacity threshold.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-005.6:** Desktop headers shall show label, bar, size, and chevron. Phone headers shall place the bar beneath the label and size. Labels and details shall wrap without horizontal page scrolling. Phone triggers shall have a touch target of at least 44 pixels.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-005.7:** Text shall communicate size, measurement status, and expansion without reliance on color. All new copy shall use the selected language. Bars shall not add keyboard stops or announce scan completion.
- **AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-005.8:** First-scan progress shall retain distinct unmeasured states as measured rows enter size order. A refresh shall keep the previous snapshot until its atomic replacement. Existing refresh and failure behavior shall remain available.

#### Exclusions

New categories, sorting controls, saved order preferences, per-file drilldown, cleanup policy changes,
scan deadline changes, and host-wide disk attribution are outside this extension.

## System design

The implemented usage bars and timeout feedback are defined in
[Storage analysis presentation](../system-design/storage-analysis-presentation.md).
The [plan package](../../../plans/storage-analysis-presentation/plan.md) records implementation and verification evidence.

The implemented temporary-storage extension is defined in
[Temporary storage visibility and cleanup](../system-design/storage-temporary-folders.md).
Its [implementation package](../../../plans/storage-temporary-folders/plan.md) contains completed work orders.

The draft database extension is defined in [Database storage analysis](../system-design/storage-database-footprint.md).

The migrated technical source is split into [part 1](../system-design/storage-maintenance-01.md), [part 2](../system-design/storage-maintenance-02.md), [part 3](../system-design/storage-maintenance-03.md).
