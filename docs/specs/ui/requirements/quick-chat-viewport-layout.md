---
status: active
system: ui
created: 2026-09-01
owners:
  - kandev
---

# Quick Chat viewport layout Requirements

## Overview

The Quick Chat dialog contains a tab strip, a transcript, and a chat composer.
The transcript can be empty or taller than the available viewport.

The UI system owns this presentation contract. The task system continues to own
the conversation data and session lifecycle.

## Terminology

- **Conversation slot:** The area between the Quick Chat tab strip and the
  bottom edge of the dialog.
- **Transcript scroll owner:** The element that scrolls the Quick Chat message
  history.
- **Chat composer:** The input area and its toolbar.

## Requirements

### REQ-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001: Quick Chat viewport layout

**Intent:** Keep the chat composer reachable while the transcript uses the
available dialog height.

**User story:** As a Quick Chat user, I want the transcript to scroll inside the
dialog, so that the chat composer remains visible.

#### Acceptance criteria

- **AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.1:** When a conversation opens on a
  tablet or desktop, the conversation slot shall fill the remaining dialog
  height.
- **AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.2:** When the transcript is short or
  empty, the chat composer shall stay at the bottom of the conversation slot.
- **AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.3:** When the transcript exceeds the
  available height, the transcript shall scroll inside its existing scroll
  owner. The tab strip and chat composer shall remain visible.
- **AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.4:** When the viewport height changes,
  the dialog shall keep the chat composer inside its lower edge.
- **AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.5:** On a phone viewport, the existing
  full-height surface shall preserve the same scroll ownership. The chat
  composer shall remain reachable above the bottom safe area.
- **AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.6:** Given a laptop-height viewport,
  both a new conversation and a long conversation shall keep the composer
  inside the dialog. The long transcript shall have scrollable overflow.

## Out of scope

- Changes to the size or internal resize behavior of the chat composer.
- Changes to Quick Chat tabs, conversation state, or session persistence.
- Changes to Quick Terminal or passthrough terminal layout.
- Changes to the shared `DialogContent` defaults for other dialogs.
