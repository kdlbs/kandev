---
status: active
system: ui
created: 2026-08-05
owners:
  - kandev
---

# Command-panel Sidebar Task Reveal Requirements

## Overview

When the desktop task list overflows, a task opened from Cmd+K can be outside
the visible part of the sidebar. The UI owns the scroll and visual-feedback
contract that helps the user find the newly active task without changing task
state or sidebar preferences.

## Requirements

### REQ-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001: Command-panel Sidebar Task Reveal

**Intent:** Keep the command-selected task visible and easy to locate in the
desktop sidebar after navigation.

#### Acceptance criteria

- **AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.1:** Selecting a task from
  Cmd+K shall open the task's canonical route.
- **AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.2:** When the selected task has
  a rendered row outside the visible desktop task-list viewport, the sidebar
  shall scroll smoothly. It shall use the minimum distance that makes the row
  visible.
- **AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.3:** After navigation, the
  selected row shall keep the normal active-task state and show a transient
  reveal cue that distinguishes it from adjacent rows.
- **AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.4:** When the row is already
  fully visible, the sidebar shall not reposition it, but the transient reveal
  cue shall still identify it.
- **AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.5:** The reveal shall not move
  keyboard focus, scroll the document, reset the task list, or change a saved
  sidebar view or collapse preference.
- **AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.6:** The reveal shall wait
  through bounded asynchronous route and sidebar rendering. It shall start
  only after the canonical route and matching active-task state render.
- **AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.7:** When another task is
  selected before an earlier reveal finishes, the latest request shall cancel
  the earlier request. The stale row shall not scroll or receive the cue.
- **AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.8:** When the active sidebar
  view, a collapsed branch, or a hidden desktop sidebar omits the row, task
  navigation shall still succeed without changing sidebar preferences.
- **AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.9:** When the user prefers
  reduced motion, the sidebar shall use immediate minimum-distance scrolling
  and a non-animated transient cue.
- **AC-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001.10:** On a phone viewport,
  Cmd+K shall keep the existing direct task-route outcome and shall not target
  the CSS-hidden desktop sidebar.

## Out of scope

- Changing task search, task-route loading, or backend task APIs.
- Changing filters, saved views, task ordering, or collapsed branches so a row
  renders.
- Expanding the desktop sidebar rail when the user has collapsed it.
- Opening or redesigning the mobile task-switcher sheet.

## Implementation plan

- [Initial command-panel sidebar task reveal](../../../plans/command-panel-sidebar-task-reveal/plan.md)
- [Command-center task focus repair](../../../plans/command-center-task-focus/plan.md)
