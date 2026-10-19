---
status: active
system: ui
created: 2026-10-13
owners:
  - kandev
---

# File Move Selection Requirements

## Overview

Moving a selected directory together with selected descendants must preserve
the directory's hierarchy. UI owns this reusable selection-to-command contract:
selected rows describe effective move roots, not necessarily separate mutations.
Workspaces retains filesystem mutation eligibility and authority under
[file entry mutations](../../workspaces/requirements/file-entry-mutations.md).

## Terms

- **Selected directory:** A selected entry identified as a directory in the
  Files inventory used for the move.
- **Covered descendant:** A selected path strictly beneath any selected
  directory on a complete `/` segment boundary. `src/a.txt` is beneath `src`;
  `src-other/a.txt` is not.
- **Effective root:** A selected entry not covered by any selected directory.
  Retained root order is its relative order in the original selection.

## Requirements

### REQ-UI-FILE-MOVE-SELECTION-001: Move directory-covering selections as roots

**Intent:** A user can select a folder and rows within it and move the folder
without flattening its subtree or creating redundant child failures.

#### Acceptance criteria

- **AC-UI-FILE-MOVE-SELECTION-001.1:** For every selected directory, every
  selected descendant shall be covered by it. Moving the selection shall issue
  one move for each effective root and no separate move for a covered
  descendant. On successful directory move, selected and unselected descendants
  shall retain their relative hierarchy and bytes beneath the moved directory.
- **AC-UI-FILE-MOVE-SELECTION-001.2:** Directory coverage shall be independent
  of selection order and how the selection was formed, including Ctrl/Cmd
  toggles and Shift ranges. A selected outer directory shall cover selected
  nested directories and leaves at every depth. With no selected outer
  directory, a selected nested directory shall cover its own selected leaves.
- **AC-UI-FILE-MOVE-SELECTION-001.3:** Independent effective roots shall retain
  their relative selection order. Selecting `src` shall not suppress
  `src-other` or `src-other/a.txt`. Folder-only, child-only, and disjoint-root
  selections shall retain their individual move behavior.
- **AC-UI-FILE-MOVE-SELECTION-001.4:** Existing destination basename collision
  handling shall apply to effective roots, including directories. A covered
  descendant shall not reserve a destination basename or produce a standalone
  destination entry. Directory children shall follow the directory's chosen
  destination name, and existing destination entries shall remain intact.
- **AC-UI-FILE-MOVE-SELECTION-001.5:** Independent effective roots shall retain
  individual success/failure outcomes and existing failure feedback. Coverage
  shall not introduce atomic selection semantics, rollback, or a false child
  failure after its covering directory succeeds. Existing self/descendant drop
  rejection shall remain intact.
- **AC-UI-FILE-MOVE-SELECTION-001.6:** Desktop and phone shall retain their
  shipped Files compositions. Phone browsing, file opening, and visible row
  actions shall continue to use the shared inventory and operation state;
  this contract shall not introduce a phone touch drag interaction.

## Adjacent contracts and exclusions

[Task navigation responsiveness](task-navigation-responsiveness.md) criteria
`.15`/`.16` retain move settlement and authoritative-tree preservation ownership.
[Path scope](file-tree-path-scope.md), [keyboard scope](file-tree-keyboard-scope.md),
and [workspace file transfer](workspace-file-transfer.md) retain their contracts.

No transactional bulk-move API, backend policy change, copy/layout redesign,
same-folder mixed-selection repair, selection/path lifecycle hardening, or
delete/rename/upload companion work. No new filesystem authority or symlink
policy is established.

## Implementation plan

- [Directory-covering file moves](../../../plans/directory-covering-file-moves/plan.md)
