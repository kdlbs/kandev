---
status: active
system: ui
created: 2026-08-04
owners:
  - kandev
---
# Quick Chat elevation Requirements

## Overview

Quick Chat opens over the current page with a transparent backdrop, so the floating panel can blend into the page and its opened state is not immediately clear. Users need a restrained visual separation that makes the panel feel elevated while preserving the surrounding page as context.

## Requirements

### REQ-UI-QUICK-CHAT-ELEVATION-001: Quick Chat elevation

**Intent:** Quick Chat opens over the current page with a transparent backdrop, so the floating panel can blend into the page and its opened state is not immediately clear. Users need a restrained visual separation that makes the panel feel elevated while preserving the surrounding page as context.

#### Acceptance criteria

- **AC-UI-QUICK-CHAT-ELEVATION-001.1:** When Quick Chat opens at tablet or desktop widths, the page behind it is visually de-emphasized by a subtle semitransparent backdrop.
- **AC-UI-QUICK-CHAT-ELEVATION-001.2:** The Quick Chat panel remains above the backdrop with its existing elevation shadow, dimensions, position, content, and interaction behavior unchanged.
- **AC-UI-QUICK-CHAT-ELEVATION-001.3:** Closing Quick Chat removes the backdrop and restores the page's normal appearance and interaction.
- **AC-UI-QUICK-CHAT-ELEVATION-001.4:** On phone widths, Quick Chat keeps its existing full-screen composition, explicit close control, and viewport containment. The backdrop does not need to be visible because the panel covers the viewport.
- **AC-UI-QUICK-CHAT-ELEVATION-001.5:** **GIVEN** Quick Chat is closed on a tablet or desktop viewport, **WHEN** the user opens Quick Chat, **THEN** the Quick Chat dialog is visible above a non-transparent backdrop and its computed panel shadow is not `none`.
- **AC-UI-QUICK-CHAT-ELEVATION-001.6:** **GIVEN** Quick Chat is open on a tablet or desktop viewport, **WHEN** the user closes it, **THEN** the dialog and backdrop are removed so the page is fully visible again.
- **AC-UI-QUICK-CHAT-ELEVATION-001.7:** **GIVEN** a phone viewport, **WHEN** the user opens Quick Chat from an existing mobile entry point, **THEN** the full-screen dialog remains usable, the explicit close control remains reachable, and the document has no horizontal overflow.

## Migrated source detail

## Why

Quick Chat opens over the current page with a transparent backdrop, so the floating panel can
blend into the page and its opened state is not immediately clear. Users need a restrained visual
separation that makes the panel feel elevated while preserving the surrounding page as context.

## What

- When Quick Chat opens at tablet or desktop widths, the page behind it is visually de-emphasized by
  a subtle semitransparent backdrop.
- The Quick Chat panel remains above the backdrop with its existing elevation shadow, dimensions,
  position, content, and interaction behavior unchanged.
- Closing Quick Chat removes the backdrop and restores the page's normal appearance and interaction.
- On phone widths, Quick Chat keeps its existing full-screen composition, explicit close control,
  and viewport containment. The backdrop does not need to be visible because the panel covers the
  viewport.

## Scenarios

- **GIVEN** Quick Chat is closed on a tablet or desktop viewport, **WHEN** the user opens Quick Chat,
  **THEN** the Quick Chat dialog is visible above a non-transparent backdrop and its computed panel
  shadow is not `none`.
- **GIVEN** Quick Chat is open on a tablet or desktop viewport, **WHEN** the user closes it,
  **THEN** the dialog and backdrop are removed so the page is fully visible again.
- **GIVEN** a phone viewport, **WHEN** the user opens Quick Chat from an existing mobile entry
  point, **THEN** the full-screen dialog remains usable, the explicit close control remains
  reachable, and the document has no horizontal overflow.

## Out of scope

- Changing Quick Chat size, position, responsive composition, content, sessions, or persistence.
- Adding a new backdrop component, user setting, animation, or navigation behavior.
- Changing the separate new-Quick-Chat picker dialog or configuration-chat popover.

## Implementation plan

[Quick Chat elevation implementation](../../../plans/quick-chat-elevation/plan.md)
