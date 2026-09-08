---
status: active
system: ui
created: 2026-08-02
owners:
  - kandev
---
# Sidebar Diff Stat Priority Requirements

## Overview

An active task in the desktop sidebar currently retains row focus after it is selected. That focus exposes the overflow control and obscures its addition/deletion totals, making the most useful progress signal disappear exactly when the task is active.

## Requirements

### REQ-UI-SIDEBAR-DIFF-STAT-PRIORITY-001: Sidebar Diff Stat Priority

**Intent:** An active task in the desktop sidebar currently retains row focus after it is selected. That focus exposes the overflow control and obscures its addition/deletion totals, making the most useful progress signal disappear exactly when the task is active.

#### Acceptance criteria

- **AC-UI-SIDEBAR-DIFF-STAT-PRIORITY-001.1:** A desktop/fine-pointer sidebar task row with non-zero diff stats shows its `+additions` and `-deletions` totals while idle, including when the row is the active task or has row focus.
- **AC-UI-SIDEBAR-DIFF-STAT-PRIORITY-001.2:** Hovering that row temporarily replaces the diff totals with the Task actions ellipsis control.
- **AC-UI-SIDEBAR-DIFF-STAT-PRIORITY-001.3:** Opening the task context menu keeps its trigger visible while the menu is open; closing the menu restores the idle diff totals unless the pointer remains over the row.
- **AC-UI-SIDEBAR-DIFF-STAT-PRIORITY-001.4:** Keyboard users can still discover and operate the Task actions control when the control itself receives keyboard focus. Row focus alone does not replace the diff totals.
- **AC-UI-SIDEBAR-DIFF-STAT-PRIORITY-001.5:** Rows without diff stats retain their existing action-control behavior.
- **AC-UI-SIDEBAR-DIFF-STAT-PRIORITY-001.6:** The phone task-switcher drawer retains its existing explicit controls: non-zero diff totals and the 44px Task actions button remain visible together, with no hover-only requirement.
- **AC-UI-SIDEBAR-DIFF-STAT-PRIORITY-001.7:** **GIVEN** an active desktop sidebar task with non-zero additions or deletions, **WHEN** the pointer is not over its row, **THEN** the row displays its diff totals and does not display the Task actions ellipsis.
- **AC-UI-SIDEBAR-DIFF-STAT-PRIORITY-001.8:** **GIVEN** a desktop sidebar task with non-zero diff totals, **WHEN** the user hovers the row, **THEN** the Task actions ellipsis replaces the totals and can open the existing context menu.

## Migrated source detail

## Why

An active task in the desktop sidebar currently retains row focus after it is selected. That
focus exposes the overflow control and obscures its addition/deletion totals, making the most
useful progress signal disappear exactly when the task is active.

## What

- A desktop/fine-pointer sidebar task row with non-zero diff stats shows its `+additions` and
  `-deletions` totals while idle, including when the row is the active task or has row focus.
- Hovering that row temporarily replaces the diff totals with the Task actions ellipsis control.
- Opening the task context menu keeps its trigger visible while the menu is open; closing the
  menu restores the idle diff totals unless the pointer remains over the row.
- Keyboard users can still discover and operate the Task actions control when the control itself
  receives keyboard focus. Row focus alone does not replace the diff totals.
- Rows without diff stats retain their existing action-control behavior.
- The phone task-switcher drawer retains its existing explicit controls: non-zero diff totals and
  the 44px Task actions button remain visible together, with no hover-only requirement.

## Scenarios

- **GIVEN** an active desktop sidebar task with non-zero additions or deletions, **WHEN** the
  pointer is not over its row, **THEN** the row displays its diff totals and does not display the
  Task actions ellipsis.
- **GIVEN** a desktop sidebar task with non-zero diff totals, **WHEN** the user hovers the row,
  **THEN** the Task actions ellipsis replaces the totals and can open the existing context menu.
- **GIVEN** a desktop sidebar task with non-zero diff totals and a closed context menu,
  **WHEN** keyboard focus reaches the Task actions button, **THEN** the button becomes visible and
  usable without treating focus on the row itself as hover.
- **GIVEN** a phone task-switcher row with non-zero diff totals, **WHEN** the task switcher opens,
  **THEN** the totals and the touch-sized Task actions button remain visible without overlap.

## Out of scope

- Changing task diff calculation, task-status summary delivery, or the displayed numbers.
- Changing context-menu actions, task selection, focus ownership, or sidebar navigation.
- Redesigning the mobile task-switcher drawer, its action sheet, or its touch-target sizing.

## Implementation plan

- [Sidebar diff stat priority plan](../../../plans/sidebar-diff-stat-priority/plan.md)
