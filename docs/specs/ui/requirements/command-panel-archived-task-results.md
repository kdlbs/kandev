---
status: active
system: ui
created: 2026-09-02
owners:
  - kandev
---

# Command Panel Archived Task Results Requirements

## Overview

Task search includes archived tasks. Users must identify an archived task before they select it and open its read-only detail.

The UI system owns this result-row presentation. The Tasks system continues to own archive state through `archived_at`.

## Requirements

### REQ-UI-COMMAND-PANEL-ARCHIVED-TASKS-001: Archived task result presentation

**Intent:** Make archived task results distinct before selection without changing their navigation behavior.

#### Acceptance criteria

- **AC-UI-COMMAND-PANEL-ARCHIVED-TASKS-001.1:** When `archived_at` is present, the task result shall show an **Archived** label instead of its workflow-step badge.
- **AC-UI-COMMAND-PANEL-ARCHIVED-TASKS-001.2:** When `archived_at` is present, the result shall show an archive icon instead of its task-activity icon.
- **AC-UI-COMMAND-PANEL-ARCHIVED-TASKS-001.3:** The archive icon shall have an accessible **Archived** description and shall remain part of the single result action.
- **AC-UI-COMMAND-PANEL-ARCHIVED-TASKS-001.4:** An archived result shall use muted semantic colors. Keyboard focus and the selected state shall remain clear.
- **AC-UI-COMMAND-PANEL-ARCHIVED-TASKS-001.5:** When `archived_at` is absent, a terminal workflow state shall not cause archived presentation.
- **AC-UI-COMMAND-PANEL-ARCHIVED-TASKS-001.6:** Task search shall rank archived matches after non-archived matches by using `archived_at` as the archive source.
- **AC-UI-COMMAND-PANEL-ARCHIVED-TASKS-001.7:** Selecting an archived result shall keep the current task-detail navigation behavior.
- **AC-UI-COMMAND-PANEL-ARCHIVED-TASKS-001.8:** Desktop and phone results shall show the same archive cues without reducing title readability or causing horizontal overflow.

## Out of scope

- Changes to task archive state or task-detail behavior.
- Changes to search matching, result limits, or ranking within each archive group.
- Changes to the command-panel layout, scrolling, or touch targets.
- New task or session API fields.
