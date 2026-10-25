---
status: active
system: ui
created: 2026-08-07
owners:
  - carlosflorencio
---
# Browser inspect annotation submission Requirements

## Overview

Users can select a pin or area in the Browser panel's Inspect mode, but the comment popup currently loses the selection before the user can submit it. This makes the primary annotation workflow appear unresponsive and prevents users from giving the agent visual change requests.

## Requirements

### REQ-UI-BROWSER-INSPECT-ANNOTATIONS-SAVE-001: Browser inspect annotation submission

**Intent:** Users can select a pin or area in the Browser panel's Inspect mode, but the comment popup currently loses the selection before the user can submit it. This makes the primary annotation workflow appear unresponsive and prevents users from giving the agent visual change requests.

#### Acceptance criteria

- **AC-UI-BROWSER-INSPECT-ANNOTATIONS-SAVE-001.1:** Selecting a pin or area opens the existing comment popup with that selection pending until the user submits or cancels it.
- **AC-UI-BROWSER-INSPECT-ANNOTATIONS-SAVE-001.2:** Clicking **Save** submits the pending annotation, closes the popup, places its numbered marker in the preview, and adds it to the Browser panel's annotations list.
- **AC-UI-BROWSER-INSPECT-ANNOTATIONS-SAVE-001.3:** Pressing **Enter** without Shift has the same submission behavior as clicking **Save**. Shift+Enter continues to insert a newline in the comment.
- **AC-UI-BROWSER-INSPECT-ANNOTATIONS-SAVE-001.4:** The submitted annotation uses the existing `annotation-added` iframe message contract; the parent continues to assign the display number.
- **AC-UI-BROWSER-INSPECT-ANNOTATIONS-SAVE-001.5:** Clicking **Cancel** or pressing **Escape** closes the popup and discards the pending selection without adding a marker or annotation.
- **AC-UI-BROWSER-INSPECT-ANNOTATIONS-SAVE-001.6:** Empty comments remain valid submissions, matching the existing popup behavior.
- **AC-UI-BROWSER-INSPECT-ANNOTATIONS-SAVE-001.7:** **GIVEN** Inspect mode is active and the user clicks an element, **WHEN** the user enters a comment and clicks **Save**, **THEN** the popup closes, a numbered marker appears at the clicked point, and the parent receives one `annotation-added` message containing the pin and comment.
- **AC-UI-BROWSER-INSPECT-ANNOTATIONS-SAVE-001.8:** **GIVEN** Inspect mode is active and the user clicks an element, **WHEN** the user enters a comment and presses plain **Enter**, **THEN** the popup closes, a numbered marker appears at the clicked point, and the parent receives one `annotation-added` message containing the pin and comment.

## Migrated source detail

## Why

Users can select a pin or area in the Browser panel's Inspect mode, but the
comment popup currently loses the selection before the user can submit it.
This makes the primary annotation workflow appear unresponsive and prevents
users from giving the agent visual change requests.

## What

- Selecting a pin or area opens the existing comment popup with that selection
  pending until the user submits or cancels it.
- Clicking **Save** submits the pending annotation, closes the popup, places its
  numbered marker in the preview, and adds it to the Browser panel's
  annotations list.
- Pressing **Enter** without Shift has the same submission behavior as
  clicking **Save**. Shift+Enter continues to insert a newline in the comment.
- The submitted annotation uses the existing `annotation-added` iframe message
  contract; the parent continues to assign the display number.
- Clicking **Cancel** or pressing **Escape** closes the popup and discards the
  pending selection without adding a marker or annotation.
- Empty comments remain valid submissions, matching the existing popup
  behavior.

## Scenarios

- **GIVEN** Inspect mode is active and the user clicks an element, **WHEN** the
  user enters a comment and clicks **Save**, **THEN** the popup closes, a
  numbered marker appears at the clicked point, and the parent receives one
  `annotation-added` message containing the pin and comment.
- **GIVEN** Inspect mode is active and the user clicks an element, **WHEN** the
  user enters a comment and presses plain **Enter**, **THEN** the popup closes,
  a numbered marker appears at the clicked point, and the parent receives one
  `annotation-added` message containing the pin and comment.
- **GIVEN** a pin or area comment popup is open, **WHEN** the user presses
  **Shift+Enter**, **THEN** no annotation is submitted and the popup remains
  open with the comment newline inserted.
- **GIVEN** a pin or area comment popup is open, **WHEN** the user clicks
  **Cancel** or presses **Escape**, **THEN** the popup closes and no marker or
  `annotation-added` message is created.

## Out of scope

- Changing the parent React hook, annotation formatter, annotation panel, or
  iframe message shape.
- Persisting annotations across iframe reloads, page reloads, or Kandev
  restarts.
- Changing the Inspect button, preview layout, touch composition, or proxy
  injection behavior.
