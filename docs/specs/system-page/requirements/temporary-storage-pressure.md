---
status: draft
system: system-page
created: 2026-09-28
owners:
  - kandev
---

# Temporary storage pressure

## Overview

Operators need to see when the filesystem used for temporary files approaches its capacity.
They also need to identify large entries without assuming that Kandev owns those entries.
System-page owns this operational view and its connection to existing maintenance controls.

This proposal extends the existing temporary-folder footprint with capacity warnings and a directory breakdown.
[Storage maintenance](storage-maintenance.md) remains authoritative for measurement scope, cleanup eligibility, and quarantine.
The proposal preserves tool defaults and inherited temporary-directory settings.

## Terminology

- **Capacity:** Space available on the filesystem containing a selected path.
- **Footprint:** File sizes observed beneath a selected directory.
- **Tracked entry:** An exact directory whose Kandev registration and ownership marker match.
- **Untracked entry:** An entry without verified ownership by this installation.

## Requirements

### REQ-SYSTEM-PAGE-TEMP-PRESSURE-001: Temporary filesystem pressure

**Intent:** A healthy Kandev home filesystem must not conceal a full temporary filesystem.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-TEMP-PRESSURE-001.1:** Storage shall show capacity, available space, and pressure for the effective service temporary folder and distinct Unix `/tmp`.
  The Kandev home capacity shall remain visible. Paths sharing a known filesystem shall identify that relationship without adding their capacities together.
- **AC-SYSTEM-PAGE-TEMP-PRESSURE-001.2:** A temporary filesystem shall show a warning at 80% used space and a critical warning at 90% used space.
  Zero available bytes shall be critical. Text shall explain that temporary-file operations can fail even when the task workspace has free space.
- **AC-SYSTEM-PAGE-TEMP-PRESSURE-001.3:** Capacity shall load independently of recursive analysis and maintenance policy.
  While the Host storage tab is visible, capacity shall refresh every 30 seconds without starting directory scans.
  Page entry, return to visibility, manual Analyze, and completed cleanup shall refresh capacity.
- **AC-SYSTEM-PAGE-TEMP-PRESSURE-001.4:** Each measurement shall show its observation time and unavailable or stale state.
  A failed measurement shall not become zero usage or discard successful sibling measurements.
  An older response without temporary capacity shall show unavailable, not infer capacity from folder size.
- **AC-SYSTEM-PAGE-TEMP-PRESSURE-001.5:** Reads shall preserve existing access restrictions and use only server-selected roots.
  Analysis shall not change tool cache locations, environment variables, cleanup settings, or operating-system policy.
- **AC-SYSTEM-PAGE-TEMP-PRESSURE-001.6:** Desktop and phone users shall see warnings before expanding folder details.
  All copy shall use the selected language. Text and controls shall remain usable without horizontal page scrolling or reliance on color.

### REQ-SYSTEM-PAGE-TEMP-PRESSURE-002: Temporary space explanation and cleanup scope

**Intent:** Operators can identify large temporary entries and understand which cleanup actions apply.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-TEMP-PRESSURE-002.1:** Each expanded temporary root shall show its twenty largest observed direct entries, sorted by file size.
  Directory values shall include their observed descendants. The view shall report the remaining observed bytes and entry count separately.
- **AC-SYSTEM-PAGE-TEMP-PRESSURE-002.2:** The breakdown shall retain scan time, completeness, and omitted-entry information.
  A partial scan shall say that the largest entries are sampled. Unvisited data shall not appear as zero or as a known remainder.
  The view shall explain that file sizes can differ from filesystem usage and do not establish reclaimable space.
- **AC-SYSTEM-PAGE-TEMP-PRESSURE-002.3:** Ownership labels shall distinguish verified Kandev registrations, untracked entries, and unavailable ownership information.
  Names, ages, and sizes shall not establish task ownership or deletion eligibility.
  The view shall explain that a task workspace does not contain all temporary or cache writes.
- **AC-SYSTEM-PAGE-TEMP-PRESSURE-002.4:** Review cleanup shall open the existing registered-artifact section without starting a mutation.
  The section shall identify installation-wide scope and retain its existing confirmation, permissions, eligibility checks, and quarantine behavior.
  The temporary-root view shall offer no Empty folder action and no arbitrary path deletion.
- **AC-SYSTEM-PAGE-TEMP-PRESSURE-002.5:** The feature shall preserve tool defaults and existing operator configuration.
  It shall create no task-specific temporary root, redirect no cache, and adopt no discovered directory.
- **AC-SYSTEM-PAGE-TEMP-PRESSURE-002.6:** Breakdown analysis shall retain the existing read-only traversal, deadlines, cancellation, and permission boundary.
  File contents, process environments, and task transcripts shall not be read to infer ownership.
  Analysis shall not delay completed storage sections or alter Total counted.
- **AC-SYSTEM-PAGE-TEMP-PRESSURE-002.7:** Desktop and phone users shall inspect full entry names, statuses, and cleanup limitations.
  Phone rows shall show details vertically, with touch targets of at least 44 pixels and no nested page scroller.

## Out of scope

- A universal Empty `/tmp` operation, operating-system cleanup controls, or deletion based on age alone.
- New npm, Go, Node, or Playwright cache cleanup providers. Those need tool-specific contracts for their standard locations.
- Changes to existing opt-in Go-cache settings, maintenance busy admission, or workspace symlink handling.
- Remote executor storage, historical growth attribution, task attribution without ownership records, or inode-pressure alerts.

## Implementation package

- [System design](../system-design/temporary-storage-pressure.md).
- [Plan and work orders](../../../plans/temporary-storage-pressure/plan.md).
