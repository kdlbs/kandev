---
status: active
system: tasks
created: 2026-09-03
owners:
  - kandev
---

# Edit task dependencies Requirements

## Overview

The task system owns dependency edges and their effect on task execution. Users
need to change these edges when work plans change after task creation.

## Terminology

- **Predecessor:** A task that must complete successfully before the dependent task is unblocked.
- **Dependent task:** The open task that waits for one or more predecessors.

## Requirements

### REQ-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001: Manage dependencies in the Edit task dialog

**Intent:** Let users correct task order from the existing task editor without task recreation or direct database changes.

**User story:** As a developer, I want to change task dependencies after creation, so that the task graph matches the current work plan.

#### Acceptance criteria

- **AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.1:** The existing Edit task dialog shall show a dependency field for every non-archived task, including a task that has started.
- **AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.2:** The field shall show each current predecessor as selected and shall show `No dependency` when the set is empty.
- **AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.3:** The picker shall search non-archived tasks in the edited task's workspace and shall exclude the edited task.
- **AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.4:** Picker changes shall remain draft changes until the user activates Update.
- **AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.5:** When the user cancels the dialog, the system shall not change the dependency set.
- **AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.6:** Update shall replace the direct predecessor set as one atomic dependency operation.
- **AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.7:** After a successful update, the task editor and Kanban board shall show the new blocked state without a reload.
- **AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.8:** When the replacement creates a cycle, the dialog shall stay open, show the cycle error, preserve the confirmed dependency set, and keep the draft available for correction.
- **AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.9:** When another replacement error occurs, the dialog shall stay open, preserve the confirmed dependency set, and permit a retry.
- **AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.10:** Existing Edit actions on Kanban cards, desktop task menus, and touch task menus shall open the same dependency editor.
- **AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.11:** The task-detail dependency chip shall remain read-only. Its reverse `blocks` direction shall remain readable and navigable.
- **AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.12:** Desktop and touch layouts shall provide the same search, selection, save, cancel, and error outcomes.
- **AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.13:** Touch controls and task rows shall have a target of at least 44 CSS pixels in the active dimension.
- **AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.14:** The mobile dialog shall use one form scroll region and shall not add document-level horizontal overflow.
- **AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.15:** Dependency edits shall keep the existing workspace authorization, cycle prevention, and auto-start behavior.

## Out of scope

- Bulk dependency edits from the Kanban selection toolbar.
- Cross-workspace dependencies.
- Changes to `start_when_unblocked` after task creation.
- Editable attributes on an edge. An edge has only add and remove operations.
- Changes to the Office task-properties dependency picker.
- Inline mutation controls in the task-detail dependency chip.
