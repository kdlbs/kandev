---
status: draft
system: ui
created: 2026-09-01
owners:
  - kandev
---

# File Tree Keyboard Scope Requirements

## Overview

The Files panel combines tree-level multi-selection shortcuts with nested text
controls for creating, renaming, and searching files. The UI system owns which
focused surface receives a keyboard shortcut; task and filesystem systems keep
ownership of file data and mutations.

## Terminology

- **Platform select-all shortcut:** Command+A on macOS and Control+A on other
  supported desktop platforms.
- **Editable Files control:** A text input, textarea, or contenteditable surface
  rendered inside the Files panel.

## Requirements

### REQ-UI-FILE-TREE-KEYBOARD-SCOPE-001: Focus-owned select-all behavior

**Intent:** Keep text-editing shortcuts inside the focused editor while
preserving file-tree multi-selection when the tree itself owns keyboard focus.

**User story:** As a Kandev user, I want the platform select-all shortcut to
select the filename I am editing, so that replacing it does not also select
files in the tree.

#### Acceptance criteria

- **AC-UI-FILE-TREE-KEYBOARD-SCOPE-001.1:** When the new-file name control has
  focus, invoking the platform select-all shortcut shall select the control's
  complete text value and shall not change file-tree row selection.
- **AC-UI-FILE-TREE-KEYBOARD-SCOPE-001.2:** When any other editable Files
  control has focus, including inline rename and file search, invoking the
  platform select-all shortcut shall remain owned by that control and shall not
  invoke file-tree multi-selection.
- **AC-UI-FILE-TREE-KEYBOARD-SCOPE-001.3:** When keyboard focus belongs to the
  non-editable file-tree surface, invoking the platform select-all shortcut
  shall continue to select every currently visible file-tree row.

## Out of scope

- Changing which file-tree rows are visible or eligible for multi-selection.
- Adding, removing, or making the file-tree shortcut configurable.
- Changing file creation, rename, search, or Escape-key behavior.
- Changing Files-panel layout, touch controls, navigation, or filesystem APIs.
