---
status: active
system: system-page
created: 2026-11-02
updated: 2026-11-03
owners:
  - kandev
---

# Storage action layout requirements

## Overview

Operators must be able to read and select Analyze and Run now when the Storage
section has limited horizontal space. The Settings sidebar can reduce that
space even when the browser uses a desktop layout.

The system-page system owns these maintenance actions. This document extends
the presentation contract in [Storage maintenance](storage-maintenance.md).
That document remains authoritative for operation semantics and permissions.
This narrow layout extension has its own file because the existing requirement
document is close to its size limit.

## Requirements

### REQ-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001: Contained maintenance actions

**Intent:** Operators can use both maintenance actions without overlapping
controls, clipped labels, or horizontal page scrolling.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.1:** At supported widths from 320
  CSS pixels, both actions shall remain inside their section with a visible
  gap. Neither action shall overlap the explanation or the other action.
- **AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.2:** When the section cannot fit
  its explanation and action group beside each other, the actions shall appear
  below the explanation. On phones below 768 CSS pixels, Analyze shall precede
  Run now in separate full-width rows.
- **AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.3:** Idle, queued, running,
  succeeded, failed, and permission-disabled actions shall retain readable
  localized labels and contained control boundaries. Longer labels shall wrap
  when needed. Disabled controls shall retain their allocated row width.
- **AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.4:** Phone and coarse-pointer
  actions shall have a rendered height of at least 44 CSS pixels. Single-line
  fine-pointer desktop actions shall retain the standard 28-pixel height at
  the standard root font. Wrapped labels may increase that height.
- **AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.5:** Run now shall retain the
  primary visual treatment. Analyze shall retain its outline treatment.
  Enabled actions shall support keyboard and pointer activation through their
  existing operation paths after a resize.

## Out of scope

- Changes to cleanup scope, permissions, requests, confirmation, or job state.
- New copy, icons, motion, menus, or mobile overlays.
- Changes to other Storage cards, Settings navigation, or shared button sizes.

## Related documents

- [System design](../system-design/storage-action-layout.md).
- [Implementation plan](../../../plans/storage-action-layout/plan.md).
