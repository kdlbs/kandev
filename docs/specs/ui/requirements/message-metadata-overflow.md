---
status: active
system: ui
created: 2026-08-14
owners:
  - kandev
---
# Message metadata dialog scroll containment Requirements

## Overview

The chat message "Message Metadata" dialog (`MessageDebugDialog` in `apps/web/components/task/chat/messages/message-actions.tsx`) renders up to ten debug fields, the last of which is `turn_metadata`. Turns persist a large `runtime_config_snapshot` (baseline plus a `config_options` array), so the combined entries can easily exceed the dialog's `max-h-[85vh]` cap.

## Requirements

### REQ-UI-MESSAGE-METADATA-OVERFLOW-001: Message metadata dialog scroll containment

**Intent:** The chat message "Message Metadata" dialog (`MessageDebugDialog` in `apps/web/components/task/chat/messages/message-actions.tsx`) renders up to ten debug fields, the last of which is `turn_metadata`. Turns persist a large `runtime_config_snapshot` (baseline plus a `config_options` array), so the combined entries can easily exceed the dialog's `max-h-[85vh]` cap.

#### Acceptance criteria

- **AC-UI-MESSAGE-METADATA-OVERFLOW-001.1:** The message metadata dialog SHALL keep every entry reachable: when the entries exceed the dialog's available height, the entries area scrolls.
- **AC-UI-MESSAGE-METADATA-OVERFLOW-001.2:** The dialog SHALL use the repo's established scrollable-dialog pattern (`github-app-policy-dialog`, `jira-ticket-dialog`): `DialogContent` becomes a flex column (`flex max-h-[85vh] flex-col overflow-hidden`), the `DialogHeader` is `shrink-0`, and the entries container becomes a height-constrained flex child (`min-h-0 flex-1`) so its existing `overflow-auto` engages.
- **AC-UI-MESSAGE-METADATA-OVERFLOW-001.3:** The title and the dialog's close button SHALL remain visible while the entries area scrolls.
- **AC-UI-MESSAGE-METADATA-OVERFLOW-001.4:** Individual JSON value boxes keep their existing `max-h-[48vh] overflow-auto` internal scrolling; that per-field cap is unchanged.
- **AC-UI-MESSAGE-METADATA-OVERFLOW-001.5:** No change to entry content, ordering, keys, or data.
- **AC-UI-MESSAGE-METADATA-OVERFLOW-001.6:** **GIVEN** a message whose debug entries (including a large `turn_metadata`) exceed the dialog's `max-h-[85vh]`, **WHEN** the user opens the Message Metadata dialog, **THEN** the entries area scrolls and `turn_metadata` is reachable.
- **AC-UI-MESSAGE-METADATA-OVERFLOW-001.7:** **GIVEN** the entries area is scrolled to the bottom, **WHEN** the user reads the last field, **THEN** `turn_metadata` is fully visible inside the dialog and the dialog's title and close button remain on screen.
- **AC-UI-MESSAGE-METADATA-OVERFLOW-001.8:** **GIVEN** a short metadata set that fits inside the dialog, **WHEN** the user opens the dialog, **THEN** no scrollbar appears and the layout is unchanged from today.

## Migrated source detail

## Why

The chat message "Message Metadata" dialog (`MessageDebugDialog` in
`apps/web/components/task/chat/messages/message-actions.tsx`) renders up to
ten debug fields, the last of which is `turn_metadata`. Turns persist a large
`runtime_config_snapshot` (baseline plus a `config_options` array), so the
combined entries can easily exceed the dialog's `max-h-[85vh]` cap.

The dialog clips instead of scrolling. `DialogContent` gets
`max-h-[85vh] overflow-hidden`, and its entries container
(`div.grid.gap-3.overflow-auto.pr-1`) is a grid item of the dialog's base
`grid` layout with no height constraint: CSS grid auto-rows grow to content
height, so the container's own height equals its content height and its
`overflow-auto` never engages. `turn_metadata`, being the last field, is
pushed entirely below the visible area. The result is JSON clipped mid-content
with no scrollbar anywhere, exactly as reported.

## What

- The message metadata dialog SHALL keep every entry reachable: when the
  entries exceed the dialog's available height, the entries area scrolls.
- The dialog SHALL use the repo's established scrollable-dialog pattern
  (`github-app-policy-dialog`, `jira-ticket-dialog`): `DialogContent` becomes
  a flex column (`flex max-h-[85vh] flex-col overflow-hidden`), the
  `DialogHeader` is `shrink-0`, and the entries container becomes a
  height-constrained flex child (`min-h-0 flex-1`) so its existing
  `overflow-auto` engages.
- The title and the dialog's close button SHALL remain visible while the
  entries area scrolls.
- Individual JSON value boxes keep their existing
  `max-h-[48vh] overflow-auto` internal scrolling; that per-field cap is
  unchanged.
- No change to entry content, ordering, keys, or data.

## Scenarios

- **GIVEN** a message whose debug entries (including a large `turn_metadata`)
  exceed the dialog's `max-h-[85vh]`, **WHEN** the user opens the Message
  Metadata dialog, **THEN** the entries area scrolls and `turn_metadata` is
  reachable.
- **GIVEN** the entries area is scrolled to the bottom, **WHEN** the user
  reads the last field, **THEN** `turn_metadata` is fully visible inside the
  dialog and the dialog's title and close button remain on screen.
- **GIVEN** a short metadata set that fits inside the dialog, **WHEN** the
  user opens the dialog, **THEN** no scrollbar appears and the layout is
  unchanged from today.
- **GIVEN** the dialog is open, **WHEN** the user presses Escape or clicks the
  close button, **THEN** the dialog closes as before.

## Out of scope

- Changing what metadata is captured, how it is ordered, or the per-field
  `max-h-[48vh]` cap.
- Restyling the scrollbar or changing other dialogs' scroll behavior.
- Altering the dialog's trigger, i18n strings, or accessible names.

## Failure modes

- If the dialog container keeps `display: grid` semantics (e.g. the merged
  class list regresses to the base `grid`), the entries container cannot be
  height-constrained and the clipping returns; the E2E regression asserts the
  entries area actually scrolls, so it fails on that regression.
- If `min-h-0` is dropped from the entries container, the flex child's
  `min-height: auto` prevents it from shrinking below content and clipping
  returns; the E2E scroll assertion catches this too.
- A viewport shorter than the smallest single field set must still show the
  header and at least the top entries; the flex-column layout preserves the
  header regardless of remaining space.
