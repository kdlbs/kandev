---
status: current
system: tasks
requirements:
  - REQ-TASKS-SIDEBAR-FLAGS-001
---

# Sidebar task flag design

## Task ownership and persistence

For criteria .1, .3 and .4, store one shared plain-text note per task. Add a task column with an empty-string default in both supported database dialects. Existing rows start unflagged. Expose the value as `flag_note` on the task DTO and sidebar task summaries. A missing field on an older partial payload preserves the cached note. A task with no known note starts unflagged.

Extend the existing authorized task PATCH and [field update path](task-field-updates.md). An omitted field preserves the note. A supplied empty string clears it. Trim outer whitespace on save. Reject an update whose trimmed note exceeds 1,024 UTF-16 code units before writing any task fields. Apply only supplied fields under the existing row lock. Concurrent flag edits follow commit order. Return and publish the persisted task through the existing task update flow. Include the note in boot, list, and live snapshot projections so reloading and switching workflows agree.

## Sidebar presentation

For criteria .1 through .5, pass the note through `TaskSwitcherItem` and its row comparison. Render a fixed-size red flag glyph beside the flexible title in `task-item.tsx`. Keep the title truncation and status indicators usable. Follow the fine-pointer interaction in `workflow-move-proceed-button.tsx`. Use `useHoverPopover` and an anchored Popover. Hover opens without stealing focus. Keep the surface open across pointer travel and while the form has focus or a pending save. Make the field reachable by keyboard. Escape cancels. A failed save keeps the draft available for retry. Read-only rows expose only the note. Wrap long text, including unbroken words. Give the control a localized accessible name and associate the note with it.

Use the single-task action group in `task-switcher-context-menu.tsx`. Capture the invoked task ID, including when another task is active. Follow existing single-task edit eligibility for archived and multi-task rows. Open a labeled note field with Save and Cancel. For a saved flag, render Change flag as a submenu using the same primitives as Priority. Edit flag opens the editor for the invoked task. Remove flag submits an empty note immediately. Surface a removal error and preserve the confirmed flag on failure. Limit the note field to 1,024 UTF-16 code units and disable Save for blank input. Disable repeat submission while saving. Close on success. On failure show inline feedback and keep the editor open. Return focus to the invoking control or the task-list trigger if the row has unmounted.

Reuse the visible actions button and phone surface handoff in `mobile/session-task-switcher-sheet.tsx`. A short bottom drawer holds the single field and actions. Tapping a flag opens the same editor without activating its row. Use one internal scroll region, safe-area padding, and at least 44px phone targets. Desktop Edit flag opens the anchored editor and focuses the field after the menu closes. Add flag uses the compact dialog for an unflagged row. Share submit and draft logic across layouts. Translate interface labels in every supported locale. For criterion .6 use Tabler IconListNumbers on the Priority submenu trigger. Preserve the individual priority indicators and choices.
