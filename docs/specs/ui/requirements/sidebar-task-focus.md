---
status: active
system: ui
created: 2026-08-26
owners:
  - kandev
---

# Sidebar Task Focus Requirements

## Overview

The task switcher needs a persistent visual treatment for the task that is
currently open. The UI system owns this contract because it defines reusable
presentation for task rows while the task and session systems continue to own
which task is active.

## Terminology

- **Active task row:** A task row that represents the current task selected by
  the existing task/session state.
- **Task color marker:** The user-selected color bar already shown at the
  leading edge of a task row.

## Requirements

### REQ-UI-SIDEBAR-TASK-FOCUS-001: Persistent active-task prominence

**Intent:** Make the currently open task easy to find without taking control
away from the user's task colors.

**User story:** As a Kandev user, I want the open task to stand out in the
sidebar, so that I can identify it quickly after scanning or switching tasks.

#### Acceptance criteria

- **AC-UI-SIDEBAR-TASK-FOCUS-001.1:** When a row represents the active task on
  desktop, the row shall show a persistent background treatment that is
  visually stronger than the inactive-row hover treatment and may add a
  horizontal border on the top and bottom edges only. The active treatment
  shall not add left or right borders.
- **AC-UI-SIDEBAR-TASK-FOCUS-001.2:** When the active task has any supported task
  color, the active treatment shall remain readable and the task color marker
  shall remain visible; the active treatment shall not use the task color as
  its only source of contrast.
- **AC-UI-SIDEBAR-TASK-FOCUS-001.3:** When the mobile task switcher shows the
  active task, it shall use the same active treatment as the desktop row. The
  row shall remain the primary tap target and the treatment shall not cause
  horizontal overflow.
- **AC-UI-SIDEBAR-TASK-FOCUS-001.4:** The treatment shall preserve the existing
  task activation, keyboard focus, multi-selection, task-color, metadata, and
  task-action-menu behavior.
- **AC-UI-SIDEBAR-TASK-FOCUS-001.5:** When the active task has no supported task
  color assigned, the active treatment shall not render a leading color or
  status marker. The leading marker shall appear only when the user assigns a
  supported task color.

## Out of scope

- Changing task/session state or the source of the active task.
- Changing task-color choices, persistence, labels, or the color picker.
- Adding a new global focus preference or a second active-task state.
- Changing task-row metadata, status icons, task actions, or mobile navigation.
