---
status: active
system: ui
created: 2026-08-04
updated: 2026-08-28
owners:
  - kandev
---
# Quick Chat and terminal elevation Requirements

## Overview

The shared Quick Chat surface opens over the current page and can show either a
conversation or a Quick Terminal tab. Its current backdrop is too weak on
dark pages, so the open state is not immediately clear. Users need a stronger
but restrained visual separation that preserves the surrounding page as
context.

The UI system owns this presentation contract because the backdrop, panel
elevation, responsive geometry, and dismissal behavior are independent of the
conversation and terminal state owned by other systems.

## Terminology

- **Quick Chat surface:** The shared responsive dialog rendered by
  `QuickChatModal` for conversations and Quick Terminal tabs.
- **Backdrop treatment:** The semitransparent darkening and light blur rendered
  behind the Quick Chat surface on tablet and desktop widths.

## Requirements

### REQ-UI-QUICK-CHAT-ELEVATION-001: Quick Chat elevation

**Intent:** The shared Quick Chat surface must read as an elevated temporary
surface without removing the current page from view.

#### Acceptance criteria

- **AC-UI-QUICK-CHAT-ELEVATION-001.1:** When the Quick Chat surface opens at
  tablet or desktop widths, the page behind it shall be visibly de-emphasized
  by a dark semitransparent backdrop with a light background blur.
- **AC-UI-QUICK-CHAT-ELEVATION-001.2:** The Quick Chat surface shall remain
  above the backdrop with its existing elevation shadow, dimensions, position,
  content, and interaction behavior unchanged.
- **AC-UI-QUICK-CHAT-ELEVATION-001.3:** When Quick Chat closes, the system shall
  remove the backdrop and restore the page's normal appearance and interaction.
- **AC-UI-QUICK-CHAT-ELEVATION-001.4:** On phone widths, Quick Chat shall keep
  its existing full-screen composition, explicit close control, and viewport
  containment. The backdrop does not need to be visible because the panel
  covers the viewport.
- **AC-UI-QUICK-CHAT-ELEVATION-001.5:** **GIVEN** Quick Chat is closed on a
  tablet or desktop viewport, **WHEN** the user opens Quick Chat, **THEN** the
  dialog shall be visible above a non-transparent backdrop with a non-default
  background filter, and its computed panel shadow shall not be `none`.
- **AC-UI-QUICK-CHAT-ELEVATION-001.6:** **GIVEN** Quick Chat is open on a
  tablet or desktop viewport, **WHEN** the user closes it, **THEN** the dialog
  and backdrop shall be removed so the page is fully visible again.
- **AC-UI-QUICK-CHAT-ELEVATION-001.7:** **GIVEN** a phone viewport, **WHEN** the
  user opens Quick Chat from an existing mobile entry point, **THEN** the
  full-screen dialog shall remain usable, the explicit close control shall
  remain reachable, and the document shall have no horizontal overflow.

## Out of scope

- Changing Quick Chat size, position, responsive composition, content, tabs,
  sessions, or persistence.
- Adding a new backdrop component, user setting, animation, or navigation
  behavior.
- Changing the separate new-Quick-Chat picker dialog or configuration-chat
  popover.
- Changing the shared `Dialog` or `DialogOverlay` defaults for other dialogs.
