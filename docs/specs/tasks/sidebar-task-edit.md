---
status: approved
created: 2026-08-03
owner: Kandev
---

# Sidebar Task Editing

## Why

People working from a task detail page can rename, move, archive, or delete another task from the sidebar, but they must return to the Kanban board to open that task's full editor. The sidebar should offer the same edit capability as a Kanban card without changing task context or introducing a second editing experience.

## What

- The submenu for one non-archived sidebar task includes **Edit** in addition to the existing **Rename** action.
- **Edit** opens the existing task edit dialog for the task whose submenu was invoked; it does not navigate to or select that task.
- The dialog is initialized from that task's title, description, lifecycle state, primary repository, workflow, and workflow step, including when the target belongs to a different workflow than the task currently open.
- The sidebar entry uses the same task edit behavior as the Kanban card: pending tasks expose the existing editable fields, while started tasks keep the title editable and retain the existing locks on prompt and workspace-source fields.
- Saving uses the existing task update contract and the normal task-update state propagation so every visible sidebar instance reflects the saved values. Canceling leaves the task unchanged.
- Edit remains a single-task action. It is not offered for a multi-task selection or for the synthetic archived-task sidebar row.
- Desktop users reach Edit from the task row context menu. Phone and tablet users reach it from the visible task-actions control; the existing responsive menu treatment keeps action rows touch-sized.
- On phones, choosing Edit dismisses the task-switcher drawer before opening the task editor. On tablets, the task-switcher sheet remains available behind the editor so canceling returns to the same task list.
- The new menu label is localized and the menu item remains keyboard accessible through the existing menu primitives.

## API surface

No new API is introduced. Saving continues to use `PATCH /api/v1/tasks/:taskId` through the existing task edit dialog, with its current title, description, and repository update payload and validation behavior.

## Permissions

Edit follows the same access rules as editing from a Kanban card. The sidebar does not add a new permission or bypass backend authorization.

## Failure modes

- If validation fails, the existing editor displays its validation state and does not submit an update.
- If the task update request fails, the existing editor surfaces its normal failure feedback and the sidebar keeps the last confirmed task values; the sidebar entry point does not invent a different retry or rollback path.
- If a row no longer resolves to a live task or lacks workflow metadata, the full Edit action is unavailable; other valid sidebar actions continue to work.

## Scenarios

- **GIVEN** a non-archived task appears in the desktop sidebar, **WHEN** the user opens its submenu, **THEN** Edit appears alongside Rename and the other single-task actions.
- **GIVEN** the sidebar target is not the currently open task, **WHEN** the user chooses Edit, **THEN** the task edit dialog opens with the target task's values and the current route and active task do not change.
- **GIVEN** the target belongs to another workflow, **WHEN** the edit dialog opens, **THEN** it uses the target workflow and step options rather than the currently selected Kanban workflow.
- **GIVEN** a task has already started, **WHEN** Edit is opened from the sidebar, **THEN** the title remains editable while the prompt and workspace-source controls retain the existing started-task locks.
- **GIVEN** the user changes a valid field and saves, **WHEN** the update succeeds, **THEN** the editor closes and the saved value appears in the sidebar and in the persisted task.
- **GIVEN** the editor contains unsaved changes, **WHEN** the user cancels, **THEN** the task and sidebar row retain their prior values.
- **GIVEN** a phone user opens the task switcher and a task's visible actions menu, **WHEN** the user chooses Edit, **THEN** the task switcher closes and the same edit dialog opens with touch-accessible controls.
- **GIVEN** a tablet user opens Edit from the task-switcher sheet, **WHEN** the user cancels the editor, **THEN** the sheet is still open at the task list.
- **GIVEN** several tasks are selected or the row represents an archived task, **WHEN** its task menu opens, **THEN** the single-task Edit action is absent.

## Out of scope

- Replacing or removing the lightweight Rename action.
- Adding bulk editing or editing archived tasks.
- Changing which task fields are editable at each lifecycle state.
- Adding a new backend endpoint, persistence model, permission, or feature flag.
- Changing Office's separate inline task-property editing surface.

## Implementation plan

See [`../../plans/sidebar-task-edit/plan.md`](../../plans/sidebar-task-edit/plan.md).
