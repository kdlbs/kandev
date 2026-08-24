---
status: draft
system: ui
created: 2026-08-05
owners:
  - kandev
---
# Command-panel Sidebar Task Reveal Requirements

## Overview

When the desktop sidebar contains more tasks than fit in its scroll viewport, opening a task from Cmd+K can leave the newly active row above or below the visible portion of the list. The workbench changes correctly, but the sidebar no longer provides visible context for where that task sits.

## Requirements

### REQ-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001: Command-panel Sidebar Task Reveal

**Intent:** When the desktop sidebar contains more tasks than fit in its scroll viewport, opening a task from Cmd+K can leave the newly active row above or below the visible portion of the list. The workbench changes correctly, but the sidebar no longer provides visible context for where that task sits.

#### Acceptance criteria

- **AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.1:** Selecting a task from Cmd+K continues to open the task's canonical route.
- **AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.2:** If that task already has a rendered row in the visible desktop task sidebar, the sidebar scrolls by the minimum amount needed to bring the row inside its own task-list viewport.
- **AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.3:** The revealed row carries the normal active-task highlight after navigation. The reveal does not move keyboard focus, scroll the document, or reset the task list to its top.
- **AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.4:** A row that is already fully visible is not unnecessarily repositioned.
- **AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.5:** Reveal waits through bounded asynchronous route and sidebar rendering instead of relying on a fixed delay. Command-panel selection queues the task ID while guarded navigation is pending and starts the reveal only after the canonical task route and matching active-task state render.
- **AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.6:** If another command-panel task is selected before the previous reveal finishes, the latest request immediately invalidates the earlier one; a late row from the earlier request must never be scrolled into view, even while the newer route is waiting behind a navigation guard.
- **AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.7:** If the active sidebar view filters the task out, a saved group or subtask branch hides it, or the desktop sidebar is not rendered, task navigation still succeeds and sidebar preferences remain unchanged.
- **AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.8:** **GIVEN** an overflowing desktop task sidebar and a Cmd+K result whose rendered row is below the task-list viewport, **WHEN** the user selects that result, **THEN** the task route opens and the active row is fully inside the task-list viewport.

## Migrated source detail

## Why

When the desktop sidebar contains more tasks than fit in its scroll viewport, opening a task from
Cmd+K can leave the newly active row above or below the visible portion of the list. The workbench
changes correctly, but the sidebar no longer provides visible context for where that task sits.

## What

- Selecting a task from Cmd+K continues to open the task's canonical route.
- If that task already has a rendered row in the visible desktop task sidebar, the sidebar scrolls
  by the minimum amount needed to bring the row inside its own task-list viewport.
- The revealed row carries the normal active-task highlight after navigation. The reveal does not
  move keyboard focus, scroll the document, or reset the task list to its top.
- A row that is already fully visible is not unnecessarily repositioned.
- Reveal waits through bounded asynchronous route and sidebar rendering instead of relying on a
  fixed delay. Command-panel selection queues the task ID while guarded navigation is pending and
  starts the reveal only after the canonical task route and matching active-task state render.
- If another command-panel task is selected before the previous reveal finishes, the latest request
  immediately invalidates the earlier one; a late row from the earlier request must never be
  scrolled into view, even while the newer route is waiting behind a navigation guard.
- If the active sidebar view filters the task out, a saved group or subtask branch hides it, or the
  desktop sidebar is not rendered, task navigation still succeeds and sidebar preferences remain
  unchanged.

## Failure modes

- Failure to find a rendered sidebar row is a no-op for the sidebar after the bounded reveal
  attempt; it never blocks or reverses task navigation.
- A hidden desktop sidebar must not become a scroll target on phone-sized viewports.

## Mobile contract

- Phone navigation keeps its existing direct task-route outcome. Cmd+K does not open the mobile
  task-switcher sheet or interact with the CSS-hidden desktop sidebar.
- The existing mobile task switcher remains the nearest shipped navigation exemplar: an inset
  bottom drawer with its own vertical scroll owner. This repair does not change that composition,
  its safe-area handling, or its touch targets.
- Desktop and phone continue to share task search and route navigation; only the desktop sidebar
  receives the additional reveal behavior.

## Scenarios

- **GIVEN** an overflowing desktop task sidebar and a Cmd+K result whose rendered row is below the
  task-list viewport, **WHEN** the user selects that result, **THEN** the task route opens and the
  active row is fully inside the task-list viewport.
- **GIVEN** an overflowing desktop task sidebar and a Cmd+K result whose rendered row is above the
  task-list viewport, **WHEN** the user selects that result, **THEN** the task route opens and the
  active row is fully inside the task-list viewport.
- **GIVEN** a dirty settings page that blocks navigation after a Cmd+K task selection, **WHEN** the
  user confirms leaving after the initial reveal retry budget expires, **THEN** the task route opens
  and the active row is still revealed after the guarded navigation completes.
- **GIVEN** two successive Cmd+K task selections where the first row is initially missing, **WHEN**
  the second task is revealed and the first row later renders, **THEN** only the second task is
  scrolled into view.
- **GIVEN** a Cmd+K task result whose sidebar row is already fully visible, **WHEN** the user selects
  it, **THEN** task navigation succeeds without an unnecessary sidebar jump.
- **GIVEN** a Cmd+K task result excluded by the active sidebar view or hidden by a persisted
  collapse, **WHEN** the user selects it, **THEN** task navigation succeeds without changing that
  view or collapse preference.
- **GIVEN** a phone viewport, **WHEN** the user selects a task through Cmd+K, **THEN** the task route
  opens and no hidden desktop sidebar becomes the reveal target.

## Out of scope

- Changing sidebar filters, the active saved view, collapsed groups, collapsed subtask branches, or
  task ordering to force a row to render.
- Expanding the desktop sidebar rail when the user has collapsed it.
- Opening or redesigning the mobile task-switcher sheet.
- Changing command-panel task search, task-route loading, or backend task APIs.

## Implementation plan

- [Command-panel sidebar task reveal](../../../plans/command-panel-sidebar-task-reveal/plan.md)
