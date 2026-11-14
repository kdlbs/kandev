---
status: active
system: ui
created: 2026-11-11
owners:
  - kandev
---

# Selection comment actions

## Overview

Users can comment on selected text in several Kandev surfaces. Native selection
menus can cover an action that uses the same selection position on touch devices.
The UI system owns this reusable presentation contract across those surfaces.
Each feature retains ownership of selection eligibility, comment data, and delivery.

This contract covers Task Chat, Quick Chat, Office thread extracts, Markdown
file previews, and CodeMirror and Monaco file editors. The existing
[Plan formatting contract](responsive-plan-formatting.md) remains its separate,
shipped exemplar. Agent-message eligibility and delivery remain in
[agent-message comments](agent-message-comments.md).

## Terminology

- **Comment action:** The action that opens a comment editor for selected text.
- **Owner viewport:** The visible content region that contains the selected
  text. A message body and a document's full content height are not viewports.
- **Comment dock:** A toolbar at the visible bottom of the owner viewport.
- **Touch presentation:** The phone layout or a device with a coarse primary
  pointer, regardless of screen width.

## Requirements

### REQ-UI-SELECTION-COMMENT-ACTIONS-001: Reachable selection comments

**Intent:** Keep the Comment action reachable while native selection actions
remain available.

#### Acceptance criteria

- **AC-UI-SELECTION-COMMENT-ACTIONS-001.1:** For each covered surface, an
  eligible selection in touch presentation shall expose Comment in a dock.
  The dock's position shall not depend on the selection rectangle or the last
  mouse position. This applies to phones and coarse-pointer tablets or desktops.
- **AC-UI-SELECTION-COMMENT-ACTIONS-001.2:** On a fine-pointer viewport at
  least 768 CSS pixels wide, each surface shall retain its current selection
  action placement, editor, and keyboard shortcut.
- **AC-UI-SELECTION-COMMENT-ACTIONS-001.3:** The dock shall remain within its
  owner viewport and the visible page during resize, scroll, and keyboard
  movement. Chat docks shall clear the composer. Docks shall clear navigation
  and device safe areas. The final content shall remain scrollable above the dock.
- **AC-UI-SELECTION-COMMENT-ACTIONS-001.4:** An eligible native touch selection
  shall reveal the action without requiring mouse movement or a mouse release.
  Changes to the selected range shall update the eventual quote. An empty,
  whitespace-only, collapsed, outside, or ineligible selection shall expose no action.
- **AC-UI-SELECTION-COMMENT-ACTIONS-001.5:** Activating Comment shall open the
  surface's existing comment editor once with the exact selected text and
  existing message or line anchors. The action shall preserve that target until
  the editor receives it. Existing Add, Run, cancellation, and delivery rules
  shall remain unchanged.
- **AC-UI-SELECTION-COMMENT-ACTIONS-001.6:** Native selection, Copy, Share,
  Select all, and related device actions shall remain available. Kandev shall
  not suppress or replace the native menu to expose its own action.
- **AC-UI-SELECTION-COMMENT-ACTIONS-001.7:** The dock shall show a localized
  Comment label and accessible name. Its action shall support keyboard
  activation and have a touch target at least 44 CSS pixels high and wide.
  The dock shall not cause document horizontal overflow or add a vertical scroller.
- **AC-UI-SELECTION-COMMENT-ACTIONS-001.8:** A surface shall expose at most one
  selection Comment action. Dismissal, owner removal, or task, session, or file
  changes shall remove stale actions. An unrelated click shall not restore a
  dismissed action from a preserved selection. Opening a comment editor shall
  remove the dock until that editor closes and a new eligible selection occurs.

## Out of scope

- Changing comment persistence, formatting, delivery, permissions, or eligibility.
- Redesigning existing comment editors or the Plan formatting strip.
- Diff gutter actions, whole-file review comments, and Browser annotation tools.
- Controlling operating-system selection menus or clipboard behavior.
- Backend, API, WebSocket, database, plugin SDK, or public editor prop changes.

## Implementation plans

- [Touch selection comments](../../../plans/touch-selection-comments/plan.md).
