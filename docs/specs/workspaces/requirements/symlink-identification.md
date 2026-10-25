---
status: active
system: workspaces
created: 2026-09-14
owners:
  - kandev
---

# Symlink identification

## Overview

Users shall recognize symbolic links in workspace Changes and file editor views.
Workspaces owns this capability because link identity belongs to repository file
state; the UI consumes that identity across desktop and phone surfaces.

## Requirements

### REQ-WORKSPACES-SYMLINK-001: Visible link identity

A symlink is the selected directory entry itself, not a regular file reached
through a symlinked ancestor. A Changes row describes its selected change layer.

#### Acceptance criteria

- **AC-WORKSPACES-SYMLINK-001.1:** Staged, unstaged and untracked symlink rows
  shall carry a persistent link marker in both list and tree modes. A deletion
  shall identify the removed entry. For a replacement, the resulting entry's
  type shall determine the marker independently for each change layer.
- **AC-WORKSPACES-SYMLINK-001.2:** Opening a readable symlink shall show a visible
  localized Symlink label in the file editor, for either editor provider and on
  phones. Opening its regular target shall not show that label. Repository and
  session identity shall isolate same-named files.
- **AC-WORKSPACES-SYMLINK-001.3:** A broken or inaccessible symlink shall remain
  identifiable in Changes when entry metadata is available, without requiring
  target contents. Existing file-open errors and permission boundaries shall
  remain in force. Missing metadata shall not produce a guessed link marker.
- **AC-WORKSPACES-SYMLINK-001.4:** Markers shall be perceivable without color or
  hover, have localized accessible names, remain visible beside truncated long
  paths, and preserve existing actions and touch targets. Metadata refresh shall
  remove stale labels when an entry becomes a regular file. The Files tree shall
  use a link glyph in the existing icon slot for symlink entries, including
  directories, on desktop and phones. Regular entries retain their usual icons.

## Scope and exclusions

The requested outcome is identification, not target navigation or a new editing
mode. Preserve save, stage, discard, diff, file-tree traversal, and access behavior.
The scope is workspace Changes rows, Files tree icons, and opened files, including existing
preview variants. Remote PR and historical commit rows without authoritative mode
metadata are excluded; never classify them from today's checkout. No new setting,
feature toggle, target-path disclosure, or persisted metadata is required.

## Implementation plans

- [Symlink identification](../../../plans/symlink-identification/plan.md)
