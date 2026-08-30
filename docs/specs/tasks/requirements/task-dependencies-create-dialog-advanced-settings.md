---
status: active
system: tasks
created: 2026-08-13
owners:
  - kandev
---
# Task-create advanced settings disclosure Requirements

## Overview

This document is the migrated task-system source for the capability. The source detail below remains authoritative while the system is migrated into separate requirement and design records.

## Requirements

### REQ-TASKS-TASK-DEPENDENCIES-CREATE-DIALOG-ADVANCED-SETTINGS-001: Task-create advanced settings disclosure

**Intent:** Preserve the observable task or workflow behavior recorded by the legacy specification.

#### Acceptance criteria

- **AC-TASKS-TASK-DEPENDENCIES-CREATE-DIALOG-ADVANCED-SETTINGS-001.1:** When a consumer uses this capability, the system shall provide the observable behavior and exclusions documented below.

## Migrated source detail

## Why

The searchable dependency selector is useful, but it is not needed for most
new tasks. Keeping it visible in the main form adds a full selector row to a
dialog whose primary job is to collect the task request and workspace context.
The create dialog should keep the common path compact while preserving a clear
place for dependency selection and future less-common options.

## What

- Add a compact, collapsed `Advanced settings` disclosure to the bottom of the
  create-task form, below the model, executor, and workflow selector controls.
- Render the disclosure label in muted, very small text with a subtle chevron.
  The whole row remains a semantic button so the small visual label does not
  reduce the interaction target.
- Start the disclosure collapsed for a new create-dialog instance. Expanding
  it reveals the existing dependency selector. The selector keeps its current
  `No dependency` default, task icon treatment, search, help action, and
  multiple-predecessor behavior.
- Show the dependency selector with a muted `Depends on` label and a contextual
  help control that explains the dependency wait and automatic-start behavior.
- Keep the label and selector in the same setting column. On desktop, render
  advanced settings as a two-column option grid so each row can hold two
  settings; on narrow screens, collapse the grid to one column.
- Keep the dependency selector's `blockedBy` state and disabled behavior
  unchanged. Collapsing the section must not clear a selected dependency, and
  reopening the section must show the same selector value.
- Keep the existing workflow selector and its visibility rules unchanged. The
  new disclosure replaces only the dependency's always-visible presentation.
- Make the expanded content a single extensible advanced-options region. Future
  advanced controls can be appended inside that region without adding another
  top-level row to the dialog. Do not add placeholder future controls in this
  change.
- Show the disclosure only for an unstarted task in create mode, matching the
  dependency selector's existing availability. Session mode, edit mode, and
  started-task forms do not gain a new advanced-settings row.
- Keep all new trigger copy localized through the task translation namespace.

## Interaction contract

### Collapsed state

- The row is visible at the bottom of the create-task settings, below the model,
  executor, and workflow selector controls.
- The row shows the localized `Advanced settings` label in a muted, subtle
  size and a direction indicator.
- The dependency trigger and its `No dependency` text are not visible while
  the section is collapsed.
- The row exposes `aria-expanded="false"` and a stable control/test identity.

### Expanded state

- Activating the row with pointer, touch, Enter, or Space expands the content.
- The dependency selector is visible inside the content with its existing
  searchable picker behavior. Its trigger uses the available row width on
  narrow screens and a compact, constrained width on desktop.
- The selector is introduced by a localized `Depends on` label with an
  accessible help control. Hovering or focusing the control explains the
  dependency wait and automatic-start behavior.
- The row exposes `aria-expanded="true"`, and the expanded content is connected
  to it through the normal collapsible relationship.
- Closing the section preserves every selected predecessor. Reopening it
  reveals the existing selected title or localized count.
- The selector's search-row info control continues to explain dependency wait
  behavior without changing the disclosure state or selection.

## Mobile design contract

- Desktop outcome: the create dialog keeps the workspace or worktree context,
  then provides a compact advanced-settings entry that reveals dependencies.
- Mobile entry point: the same inline disclosure in the full-screen task-create
  form. A new drawer or separate mobile navigation surface is not needed for
  one short advanced field.
- The nearest shipped mobile exemplars are the task-create selector controls,
  `task-create-dialog-pill.tsx`, and the existing dependency picker. Reuse the
  project's contained popover and command-list behavior.
- The disclosure trigger has a mobile hitbox of at least 44 CSS pixels even
  though its label uses a subtle text size. The expanded dependency trigger and
  dependency help control and task rows retain their existing touch-safe sizing.
- The task-create form remains the outer scroll owner when the collapsible is
  closed or expanded. The dependency command list remains the inner scroll
  owner while its picker is open.
- The expanded content uses a responsive option grid. On desktop, each row can
  contain two settings, and each setting keeps its label/help to the left of its
  selector in the same column. On narrow screens, the grid collapses to one
  column and must not introduce document-level horizontal overflow.
- State, selection logic, candidate filtering, and payload behavior are shared
  across desktop and mobile. Only the disclosure geometry and touch
  presentation are responsive.

## Scenarios

- **GIVEN** a new task create dialog, **WHEN** the form first renders, **THEN**
  the advanced-settings row is visible and collapsed, and the dependency
  trigger is not visible.
- **GIVEN** the collapsed advanced-settings row, **WHEN** the user activates it,
  **THEN** the row reports expanded and the dependency selector appears below
  the model, executor, and workflow selector controls.
- **GIVEN** the expanded selector with no predecessors, **THEN** its trigger
  shows the existing `No dependency` label and dependency icon.
- **GIVEN** a selected predecessor, **WHEN** the user collapses and reopens the
  advanced section, **THEN** the selected task title or localized dependency
  count remains visible in the selector and the form state is unchanged.
- **GIVEN** a workflow selector that is visible, hidden for a single workflow,
  or locked by a caller, **WHEN** the create dialog renders, **THEN** the
  workflow's existing behavior is preserved and the advanced dependency
  control remains available under the same create-mode rules.
- **GIVEN** session mode, edit mode, or a started task, **WHEN** the form
  renders, **THEN** no advanced-settings disclosure is added.
- **GIVEN** a mobile create dialog, **WHEN** the user taps the disclosure and
  opens the dependency picker, **THEN** the controls remain reachable, the
  picker is contained in the viewport, and the document has no horizontal
  overflow.
- **GIVEN** selected predecessors, **WHEN** the user submits the task after
  using the disclosure, **THEN** the existing `blocked_by` payload contains the
  same IDs and no dependency API contract changes.

## Out of scope

- Changing dependency persistence, AND semantics, candidate derivation,
  archived-task filtering, or automatic task-start behavior.
- Changing the dependency selector's search, task icons, info help, or picker
  interaction except where needed to mount it inside the disclosure.
- Adding dependency editing to task detail, Kanban card menus, or MCP.
- Adding or designing future advanced controls beyond the extensible content
  region.
- Changing workflow, agent, executor, repository, or branch selector behavior.