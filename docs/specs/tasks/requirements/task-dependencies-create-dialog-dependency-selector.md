---
status: active
system: tasks
created: 2026-08-13
owners:
  - kandev
---
# Task-create dependency selector refinement Requirements

## Overview

This document is the migrated task-system source for the capability. The source detail below remains authoritative while the system is migrated into separate requirement and design records.

## Requirements

### REQ-TASKS-TASK-DEPENDENCIES-CREATE-DIALOG-DEPENDENCY-SELECTOR-001: Task-create dependency selector refinement

**Intent:** Preserve the observable task or workflow behavior recorded by the legacy specification.

#### Acceptance criteria

- **AC-TASKS-TASK-DEPENDENCIES-CREATE-DIALOG-DEPENDENCY-SELECTOR-001.1:** When a consumer uses this capability, the system shall provide the observable behavior and exclusions documented below.

## Migrated source detail

## Why

The task-create dialog currently presents dependencies as a small action beside
a separate label and then renders selected tasks as chips below it. This makes
the control look different from the agent, executor, and workflow selectors,
and the dependency behavior is easy to miss when a user is planning an ordered
set of tasks.

## What

- In create mode, the dependency control appears in the row below the agent and
  executor selectors, on the right of the workflow selector when that selector
  is visible.
- The dependency control is one searchable selector. It keeps the existing
  ability to choose multiple predecessor tasks by toggling task entries in the
  same selector.
- With no predecessor selected, the trigger shows `No dependency` and the
  dependency icon. With one predecessor selected, it shows that task's title.
  With multiple predecessors selected, it shows a localized dependency count.
- The selector includes a `No dependency` entry that clears every selected
  predecessor. A selected task can be removed by toggling its entry off.
- Every task entry in the selector displays a task icon. Archived tasks are not
  offered as candidates, and the existing candidate sources from both Kanban
  board slices remain available.
- The selector's search row has an info control on its right side. Hovering or
  focusing the control explains that dependencies wait for all selected tasks
  to finish successfully and that starting an agent starts this task
  automatically after they resolve. The control has an accessible name and is
  usable with touch and keyboard input.
- The existing `blockedBy` form state and `blocked_by` create payload remain
  unchanged. Selecting several predecessors keeps the existing AND semantics.
- The dependency selector is disabled while the create flow is creating a
  session, matching the current dependency control behavior.
- The selector is available only for an unstarted task in create mode. It does
  not add an edge editor to task detail or edit mode.
- All new trigger labels, search copy, empty states, count labels, and help
  copy use the task translation namespace. No new user-facing literal is
  hardcoded in a component.

## Mobile design contract

- Desktop entry point: the dependency selector in the task-create dialog row
  beside the workflow selector. Mobile entry point: the same full-width
  selector in the dialog's vertically scrolling form.
- The nearest shipped mobile exemplar is the task-create repository picker and
  its `Pill` command surface. Reuse its contained searchable picker geometry,
  touch-safe actions, and no-horizontal-overflow expectations while keeping the
  dependency selector's full-width selector trigger consistent with the agent
  and executor controls.
- Mobile hierarchy is workflow first, dependency second, followed by the rest
  of the form. The dependency trigger and each task row are the primary touch
  targets for this surface.
- The picker remains a viewport-contained searchable popover with an internal
  command-list scroll owner. The dialog form remains the outer scroll owner
  when the picker is closed. No document-level horizontal scrolling is added.
- The selector shares task derivation, filtering, selection, and `blockedBy`
  mutation logic across viewports. Only layout and touch presentation vary.

## Scenarios

- **GIVEN** a new task dialog with no selected predecessors, **WHEN** the user
  views the workflow/dependency row, **THEN** the dependency trigger is on the
  workflow row's right side and shows `No dependency` with its icon.
- **GIVEN** the workflow selector is hidden because a single workflow is
  already enforced, **WHEN** the user opens the create dialog, **THEN** the
  dependency selector remains available in the same row position instead of
  disappearing with the workflow control.
- **GIVEN** the dependency trigger, **WHEN** the user opens it, **THEN** the
  search field, the info control, the `No dependency` entry, and all non-archived
  board tasks are available.
- **GIVEN** two non-archived candidate tasks, **WHEN** the user selects both
  entries, **THEN** both entries are marked selected, the trigger shows a
  localized count, and the form retains both task IDs in `blockedBy`.
- **GIVEN** selected predecessors, **WHEN** the user selects `No dependency`,
  **THEN** the trigger returns to its default state and the form's `blockedBy`
  value is empty.
- **GIVEN** the selector's info control, **WHEN** the user hovers, focuses, or
  taps it, **THEN** the dependency explanation is readable without changing
  the current task selection, and Escape or outside interaction dismisses it.
- **GIVEN** an archived board task, **WHEN** the user searches the dependency
  selector, **THEN** the archived task is not offered.
- **GIVEN** a Pixel 5-sized viewport, **WHEN** the user opens the create dialog
  and dependency selector, **THEN** the trigger and task rows remain reachable,
  the picker stays within the viewport, and the document has no horizontal
  overflow.
- **GIVEN** a task with selected predecessors, **WHEN** the user submits the
  create dialog, **THEN** the request still contains the selected IDs in
  `blocked_by`; no dependency API or backend contract changes.

## Out of scope

- Changing the dependency graph, auto-start semantics, or backend/API payload.
- Limiting a task to one predecessor. Multiple predecessors and AND semantics
  remain supported.
- Adding dependency editing to task detail, Kanban card menus, or MCP.
- Adding a new mobile navigation surface or replacing the task-create dialog.
- Changing workflow, agent, executor, repository, or branch selector behavior.