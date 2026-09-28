---
status: active
system: ui
created: 2026-09-24
owners:
  - kandev
---

# Copy File Path Actions Requirements

## Overview

People can open a changed file's diff but must select its path manually to copy
it. The UI system owns the transient copy actions and their responsive
presentation. Task and workspace systems continue to own the file and
repository identities supplied to the UI.

## Requirements

### REQ-UI-COPY-FILE-PATH-ACTIONS-001: Copy a changed file path

**Intent:** Let people copy the repository-relative path of a changed file from
the Changes list or its Review diff header.

**User story:** As a person reviewing file changes, I want to copy a file path
directly so that I can reuse it without selecting the text.

#### Acceptance criteria

- **AC-UI-COPY-FILE-PATH-ACTIONS-001.1:** Given a working-tree file row in the
  Changes panel whose path has no C0 or DEL ASCII control character, when a
  user activates Copy path, the clipboard shall receive the exact
  repository-relative path represented by that row, and the action shall not
  open the file diff.
- **AC-UI-COPY-FILE-PATH-ACTIONS-001.2:** Given a file in the Review diff
  panel whose path has no C0 or DEL ASCII control character, when the user
  activates Copy path, the clipboard shall receive the current
  repository-relative file path. Copy path shall replace Copy diff in that
  panel's file toolbar.
- **AC-UI-COPY-FILE-PATH-ACTIONS-001.3:** For a renamed file, Copy path shall
  copy the current path shown by the file row or diff header when that path has
  no C0 or DEL ASCII control character. The previous path remains available
  only through the existing rename context.
- **AC-UI-COPY-FILE-PATH-ACTIONS-001.4:** Copy path shall be keyboard and
  pointer reachable on desktop and touch reachable on phones. It shall not
  depend on hover as the only access path, and its phone controls shall have
  hit areas of at least 44px in both dimensions.
- **AC-UI-COPY-FILE-PATH-ACTIONS-001.5:** The copied value shall not include a
  repository label or an absolute worktree path, including in a multi-repository
  task.
- **AC-UI-COPY-FILE-PATH-ACTIONS-001.6:** Given a file path containing a C0 or
  DEL ASCII control character, when a user activates Copy path in the Changes
  panel or Review diff panel, the clipboard shall remain unchanged and the UI
  shall report that the path cannot be copied.

## Out of scope

- Changing file-path data, repository routing, diff selection, or Git actions.
- Copying the entire diff from other editor or diff surfaces.
- Adding clipboard persistence or a new clipboard implementation.

## Implementation plans

- [Copy file path actions](../../../plans/copy-file-path-actions/plan.md)
