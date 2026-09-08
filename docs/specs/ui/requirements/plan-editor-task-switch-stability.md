---
status: active
system: ui
created: 2026-09-08
owners:
  - kandev
---

# Plan Editor Task-Switch Stability Requirements

## Overview

The Plan panel can remain open while a user selects another task. The UI system
owns the editor and search-control lifecycle during that task change. The tasks
system continues to own plan data and task selection.

## Terminology

- **Available plan editor:** The mounted editor for the selected task that can
  accept plan-search actions.
- **Plan-search action:** A query update, query clear, next-match action, or
  previous-match action from the Plan panel.

## Requirements

### REQ-UI-PLAN-EDITOR-TASK-SWITCH-001: Stable editor replacement

**Intent:** Keep task navigation usable while Kandev replaces the plan editor.

**User story:** As a Kandev user, I want to switch tasks while the Plan panel is
open, so that an outgoing editor cannot break the selected task page.

#### Acceptance criteria

- **AC-UI-PLAN-EDITOR-TASK-SWITCH-001.1:** When a user selects another task
  while the Plan panel is mounted, the selected task page shall remain usable.
- **AC-UI-PLAN-EDITOR-TASK-SWITCH-001.2:** During editor replacement, a
  plan-search action shall run only against the available plan editor.
- **AC-UI-PLAN-EDITOR-TASK-SWITCH-001.3:** If no plan editor is available, a
  plan-search action shall not cause the task page to show its recovery screen.
- **AC-UI-PLAN-EDITOR-TASK-SWITCH-001.4:** After the selected task editor is
  available, plan search shall update queries and navigate matches in that
  editor.
- **AC-UI-PLAN-EDITOR-TASK-SWITCH-001.5:** A plan-comment command shall run
  only against the available plan editor.

## Out of scope

- Changing task selection, plan persistence, or saved Dockview layouts.
- Changing Plan panel layout, touch controls, or responsive composition.
- Changing the route error boundary or its recovery screen.
- Changing optional voice services or analytics scripts.
