---
status: draft
system: ui
created: 2026-08-26
owners:
  - kandev
---

# Responsive Plan Formatting Requirements

## Overview

The task Plan editor exposes inline formatting and commenting actions. The UI
system owns how those actions are presented across pointer modes and viewports;
the task system continues to own plan content, revisions, and persistence.

Desktop selection-anchored controls remain useful with a fine pointer. On a
phone or touch-oriented tablet, the browser or operating system can display its
own Cut, Copy, and Paste surface next to the selection. Kandev must keep its
formatting controls reachable without competing for that same position.

## Terminology

- **Selection bubble:** Kandev's formatting controls positioned next to the
  current editor selection.
- **Docked formatting strip:** Kandev's formatting controls positioned at the
  visible bottom edge of the Plan editor or immediately above the software
  keyboard.
- **Visual viewport:** The portion of the page visible after browser chrome,
  zoom, and a software keyboard are accounted for.

## Requirements

### REQ-UI-RESPONSIVE-PLAN-FORMATTING-001: Pointer-appropriate plan formatting

**Intent:** Keep Plan formatting and commenting actions predictable and
reachable without obstructing native text-selection actions.

**User story:** As a user editing a task plan, I want Kandev's formatting
controls to stay separate from my device's text-selection menu, so that I can
use both without racing overlapping controls.

#### Acceptance criteria

- **AC-UI-RESPONSIVE-PLAN-FORMATTING-001.1:** When a fine-pointer desktop user
  selects non-code-block Plan text, Kandev shall show the existing formatting
  controls next to the selection and hide them when the selection is empty.
- **AC-UI-RESPONSIVE-PLAN-FORMATTING-001.2:** When a user selects
  non-whitespace Plan text outside a code block in Kandev's phone or
  touch-tablet layout, Kandev shall present the formatting controls in a docked
  strip instead of positioning them next to the text selection. When the Plan
  editor has only a caret or a whitespace-only selection, the docked strip
  shall remain hidden.
- **AC-UI-RESPONSIVE-PLAN-FORMATTING-001.3:** When an eligible text selection is
  present, the docked strip shall expose the desktop formatting capabilities.
  Selection-only actions shall not be presented without an eligible text
  selection.
- **AC-UI-RESPONSIVE-PLAN-FORMATTING-001.4:** Kandev shall leave the device's
  native Cut, Copy, Paste, and related selection actions available while the
  docked strip is shown.
- **AC-UI-RESPONSIVE-PLAN-FORMATTING-001.5:** When a software keyboard resizes
  or scrolls the visual viewport, the docked strip shall remain fully visible
  immediately above the keyboard and shall follow subsequent visual-viewport
  changes without requiring the user to recreate the selection.
- **AC-UI-RESPONSIVE-PLAN-FORMATTING-001.6:** When the software keyboard is
  closed, the docked strip shall remain inside the Plan surface above mobile
  task navigation and the device safe area. Plan content shall remain
  scrollable without its final editable lines being covered by the strip.
- **AC-UI-RESPONSIVE-PLAN-FORMATTING-001.7:** Every docked action shall have an
  accessible name, expose its pressed or disabled state when applicable, and
  provide a touch target of at least 44 CSS pixels in the active dimension.
  The compact docked strip shall be no taller than 48 CSS pixels, and its
  visual action surfaces shall be no larger than 32 CSS pixels. The strip can
  scroll horizontally, but it shall not cause document-level horizontal
  overflow.
- **AC-UI-RESPONSIVE-PLAN-FORMATTING-001.8:** When the user taps a docked
  formatting action, Kandev shall preserve the editor selection and software
  keyboard, apply the action once, and keep the editor ready for continued
  editing. The inline link flow shall remain usable within the docked surface.

## Out of scope

- Hiding, restyling, reordering, or otherwise controlling browser or operating
  system text-selection menus.
- Changing task-plan content, autosave, revisions, comments, or persistence.
- Adding formatting commands beyond the actions already exposed by the Plan
  editor.
- Redesigning the Plan comment composer that opens after the Comment action.
- Changing the public `host.ui.RichTextEditor` prop contract or promising the
  Plan-specific comment action to plugin editors.
