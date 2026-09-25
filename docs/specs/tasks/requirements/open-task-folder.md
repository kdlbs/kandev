---
status: active
system: tasks
created: 2026-09-14
owners:
  - kandev
---

# Open task folder requirements

## Overview

Expose the existing native folder-opening capability inside the task's Open in editor dropdown.
Tasks owns this shortcut because its target is the selected task session's workspace.
The existing workspace-actions contract remains in
[Attach workspace sources](attach-workspace-sources.md).

## Requirements

### REQ-TASKS-OPEN-FOLDER-001: Task folder shortcut

**Intent:** Open task files in the host file manager without finding the file-panel menu.

#### Acceptance criteria

- **AC-TASKS-OPEN-FOLDER-001.1:** For an unarchived task with a selected session, desktop task tools shall expose an accessible Open folder action inside the Open in editor dropdown, independent of editor configuration. There shall be no separate task-toolbar folder button. The dropdown shall remain reachable with no configured editors; its primary editor action shall retain its existing enablement. With one worktree, activation shall open that worktree; with no worktrees, existing repository-root fallback shall apply.
- **AC-TASKS-OPEN-FOLDER-001.2:** With multiple session worktrees, the action shall offer repository/branch choices and open only the selected worktree. An invalid selection shall fail without opening a different directory.
- **AC-TASKS-OPEN-FOLDER-001.3:** Without a selected session or an installed host folder-opening executable, the shortcut and Files-menu action shall be disabled and shall not open a picker. Unknown or failed capability discovery shall also disable opening. While opening, it shall prevent repeat activation and show progress. Failed requests shall show a localized error and permit retry. Opening shall not launch an agent or modify task/editor preferences.
- **AC-TASKS-OPEN-FOLDER-001.4:** Phone users shall retain a visible Files-menu folder action with the same selection and error behavior, reachable through touch targets of at least 44px, without horizontal page overflow. Desktop controls shall retain 28px sizing. Labels and errors shall be localized.
- **AC-TASKS-OPEN-FOLDER-001.5:** Opening shall use the existing file-manager integration on the Kandev host: Finder on macOS, the default file manager on Linux, and Explorer on Windows. The action shall not imply it opens a remote host path on the browser device. Missing workspace paths shall produce a visible failure.

## Out of scope

Remote filesystem mounting, opening on a different browser device, new native integrations,
new editor settings, arbitrary file/path selection, and changing archived-task tool visibility.

## Implementation plans

- [Editor dropdown folder action](../../../plans/editor-dropdown-folder/plan.md) (dropdown relocation)
- [Open task folder](../../../plans/open-task-folder/plan.md) (original delivery)
