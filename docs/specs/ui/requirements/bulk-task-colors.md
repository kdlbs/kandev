---
status: active
system: ui
created: 2026-09-19
owners:
  - kandev
---

# Bulk task colors requirements

## Overview

Apply one personal manual color to multiple selected tasks. UI owns this
presentation preference; shared task records remain unchanged. This draft
extends [personal task colors](sidebar-automatic-task-colors.md), whose
palette, persistence, and automatic-rule precedence remain authoritative.

## Requirements

### REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006: Multi-selected manual colors

**Intent:** Apply one personal manual color to selected tasks without repeating the action for each task.

#### Acceptance criteria

- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006.1:** The sidebar multi-selection actions and board selection toolbar shall offer Color for the selected task set, including selections spanning workflows. Opening a sidebar menu on an unselected task shall retain its single-task targeting behavior.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006.2:** Choosing one of the existing seven manual colors shall apply it to exactly the selected tasks captured when the choice is made. Choosing None shall clear their manual colors. Unselected tasks and automatic rules shall remain unchanged.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006.3:** The picker shall identify a common manual value only when every selected task has that value. Mixed values shall show no checked color. None shall be enabled when any selected task has a manual color. Empty selection shall offer no actionable color control.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006.4:** Applying or cancelling a color choice shall preserve task selection and the active task. A successful change shall remain after reload. Automatic colors shall retain their existing precedence, explained in the picker.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006.5:** During saving, the initiating color control shall prevent duplicate submission and expose saving state. Failure shall show a localized error and restore failed changes to the latest confirmed values without reverting newer edits or unrelated settings. Selection shall remain available for retry.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006.6:** Phone users shall have a visible Select tasks control, available before any task is selected, to enter selection mode and select multiple board tasks and apply or clear a color through a visible touch control and a viewport-contained picker. Color options shall have text labels, keyboard support, dismissal with focus return, and touch targets of at least 44 CSS pixels. The document shall not scroll horizontally.
- **AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006.7:** Selections larger than one persistence request shall be processed without silent truncation. If saving stops partway, confirmed changes shall remain, unsaved changes shall be restored, and the user shall see how many tasks were saved and that retry is available.

## Out of scope

New task color fields, automatic-rule precedence changes, custom palettes,
board-card markers, and mobile task-switcher multi-selection.
