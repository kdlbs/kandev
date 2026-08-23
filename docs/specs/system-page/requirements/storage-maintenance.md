---
status: active
system: system-page
created: 2026-07-14
updated: 2026-08-12
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

## System design

The migrated technical source is split into [part 1](../system-design/storage-maintenance-01.md), [part 2](../system-design/storage-maintenance-02.md), [part 3](../system-design/storage-maintenance-03.md).
