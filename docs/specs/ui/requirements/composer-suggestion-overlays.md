---
status: active
system: ui
created: 2026-08-23
updated: 2026-08-24
owners:
  - web
---

# Composer Suggestion Overlay Requirements

## Overview

Kandev composers expose contextual suggestion menus for saved prompts, tasks,
files, plans, agent commands, and external work items. On a phone, opening the
software keyboard can shrink the browser's visual viewport without moving the
composer's layout-viewport caret coordinates. The user must still be able to
see and select suggestions without dismissing the keyboard.

## Terminology

- **Layout viewport:** The coordinate space used by the editor caret and fixed
  layout before mobile keyboard occlusion is applied.
- **Visual viewport:** The currently visible browser region after zoom, browser
  chrome, and software keyboard occlusion.
- **Suggestion overlay:** The shared popup surface opened by a composer trigger,
  including `@`, `#`, and `/` menus.

## Requirements

### REQ-UI-COMPOSER-OVERLAY-001: Keep composer suggestions usable in the visible viewport

**Intent:** Preserve suggestion discovery and selection when a mobile software
keyboard changes the visible browser area.

**User story:** As a phone user composing a prompt, I want contextual
suggestions to remain visible above the keyboard, so that I can insert an item
without hiding the keyboard or abandoning my draft.

#### Acceptance criteria

- **AC-UI-COMPOSER-OVERLAY-001.1:** Given an open above-composer suggestion
  overlay, when the current visual viewport no longer contains the caret or
  direct anchor because a software keyboard or browser chrome changed the
  visible area, the complete rendered overlay surface shall be positioned and
  height-constrained inside the visual viewport whenever that viewport can fit
  the overlay header and one result row.
- **AC-UI-COMPOSER-OVERLAY-001.2:** Given an open suggestion overlay, when the
  visual viewport resizes or scrolls, the overlay shall reflow without requiring
  the user to close, retrigger, or retype the active query. When the Visual
  Viewport API is unavailable, the layout viewport shall provide the fallback
  bounds.
- **AC-UI-COMPOSER-OVERLAY-001.3:** Given a result list taller or wider than the
  available phone viewport, the overlay shall remain horizontally contained,
  keep result rows at least 44 CSS pixels tall, and scroll its results internally
  without introducing horizontal document overflow.
- **AC-UI-COMPOSER-OVERLAY-001.4:** Given a visible saved-prompt, task, file,
  plan, command, or entity-reference result, when the user selects it by touch
  or keyboard, the existing feature-specific insertion behavior shall complete,
  the draft shall not submit implicitly, and composer focus shall remain
  available for continued editing.
- **AC-UI-COMPOSER-OVERLAY-001.5:** Given the composer and its above-composer
  anchor remain inside the visual viewport, when a suggestion overlay opens or
  reflows, its rendered bottom edge shall remain directly adjacent to the
  composer rather than detach to another viewport edge.

## Out of scope

- Changing suggestion result sources, ranking, filtering, or empty states.
- Changing saved-prompt, task, file, plan, command, or entity-reference data
  contracts.
- Replacing the contextual popup with a drawer or full-screen picker.
- Adding automatic placement-side flipping to the plan editor's below-caret
  block menu.
- Changing message submission, task launch, or agent launch behavior.
