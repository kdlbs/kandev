---
status: shipped
created: 2026-08-08
owner: kandev
---

# Message favorite star mobile sizing

## Why

On viewports below the `sm` breakpoint (640px), the star control that marks a
chat message as favorite renders roughly twice the size of every other icon in
the same message action row (copy, navigation, metadata). The oversized star
breaks the visual rhythm of the transcript actions and reads as a layout bug
on phone.

## What

- The favorite star control in a chat message action row is the same size as
  the sibling action controls (copy, message navigation, metadata) on every
  viewport, including mobile (< 640px).
- The star control's glyph matches the glyph size of the sibling icons.
- Tapping the star on a phone viewport still toggles the message favorite
  state and its accessible labels and `aria-pressed` state are unchanged.
- Desktop (≥ 640px) rendering is unchanged: the star control already matches
  the sibling sizing there.

## Scenarios

- **GIVEN** a chat message with its action row visible on a phone viewport,
  **WHEN** the user compares the favorite star control with the copy action in
  the same row, **THEN** the two controls render at the same size (within 2px).
- **GIVEN** a chat message with its action row visible on a phone viewport,
  **WHEN** the user taps the star, **THEN** the message is marked as favorite
  (the star is filled and the accessible label switches to "Remove message from
  favorites"), and a second tap removes the favorite.
- **GIVEN** a chat message on a desktop viewport, **WHEN** the user inspects
  the action row, **THEN** the star control size is unchanged from the current
  desktop rendering.

## Out of scope

- Resizing any of the sibling action controls (copy, navigation, metadata) on
  any viewport.
- Introducing a larger mobile touch target for the entire action row (all
  sibling controls keep their current 20px sizing on mobile; the star joins
  them).
- Changing the favorite feature's state model, sessionStorage persistence,
  accessible labels, or toggle behavior.
