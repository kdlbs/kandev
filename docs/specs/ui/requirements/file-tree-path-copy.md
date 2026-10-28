---
status: active
system: ui
created: 2026-10-28
owners:
  - kandev
---

# File Tree Path Copy Requirements

## Overview

A person browsing Files can copy the identity of a file or folder without opening it. UI owns this transient interaction and its responsive presentation; workspace systems retain ownership of the paths supplied to it. Filename search currently returns files, while the tree includes files and directories.

## Requirements

### REQ-UI-FILE-TREE-PATH-COPY-001: Copy a single Files node path

**Intent:** Make a node path available for reuse directly from Files.

#### Acceptance criteria

- **AC-UI-FILE-TREE-PATH-COPY-001.1:** A single file or directory in the Files tree offers Copy path from its desktop context menu and from the visible phone row-actions menu, without opening the node first. A filename search result offers the same action on both surfaces.
- **AC-UI-FILE-TREE-PATH-COPY-001.2:** For a path without C0 or DEL ASCII control characters, activating the action copies the exact path represented by the node, including any multi-root prefix, spaces and Unicode. It does not add an absolute root or shell quoting, remove a prefix, or substitute a display label.
- **AC-UI-FILE-TREE-PATH-COPY-001.3:** When more than one tree node is selected, Copy path is absent from the bulk menu on both desktop and touch; it never silently copies one selected node. Search results retain their existing single-result semantics.
- **AC-UI-FILE-TREE-PATH-COPY-001.4:** A path containing a C0 or DEL ASCII control character is refused before clipboard access and the existing localized refusal is shown. If clipboard writing and its existing fallback fail, no success is claimed; the existing silent failure behavior remains.
- **AC-UI-FILE-TREE-PATH-COPY-001.5:** Activating Copy path does not open a file, change the primary row selection, expand or collapse a folder, or write workspace files. Existing open, add-to-chat, and delete behavior remains available.
- **AC-UI-FILE-TREE-PATH-COPY-001.6:** The desktop action is pointer and keyboard reachable. The phone action uses the visible row trigger and responsive menu, without long press or hover, with at least 44px touch targets and localized labels.

## Out of scope

- Bulk copying, new selection modes, directory search, changing path identity or filesystem operations.
- Redesigning clipboard fallback, concurrency, feedback, or existing Changes and Review path actions.

## Implementation plans

- [File tree path copy](../../../plans/file-tree-path-copy/plan.md)
