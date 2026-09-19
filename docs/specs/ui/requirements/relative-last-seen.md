---
status: draft
system: ui
created: 2026-08-16
owners:
  - kandev
---
# Relative Last Seen in Account Security Requirements

## Overview

The **Active sessions** table on `/settings/account/security` renders the "Last seen" column as an
absolute timestamp (`formatDateTime`, e.g. "Aug 15, 2026, 3:42 PM"). Absolute stamps are precise but
hard to scan: a user reviewing devices signed in to their account must mentally subtract to know
whether a session was active moments ago or a month ago. Relative human-readable time ("5 minutes
ago", "yesterday") makes session freshness legible at a glance, which is the primary security
question a user is asking.

Relative time is inherently stale the moment it renders, so the label must live-update, and users
who need the exact moment still need access to it. This feature adds a per-user display option on
the security page that switches the column to relative time with an absolute-time tooltip on fine
pointer devices and a touch drawer on coarse-pointer devices.

## Requirements

### REQ-UI-RELATIVE-LAST-SEEN-001: Relative Last Seen in Account Security

**Intent:** The **Active sessions** table on `/settings/account/security` renders the "Last seen"
column as an absolute timestamp (`formatDateTime`, e.g. "Aug 15, 2026, 3:42 PM"). Absolute stamps
are precise but hard to scan: a user reviewing devices signed in to their account must mentally
subtract to know whether a session was active moments ago or a month ago. Relative human-readable
time ("5 minutes ago", "yesterday") makes session freshness legible at a glance, which is the
primary security question a user is asking. Relative time is inherently stale the moment it renders,
so the label must live-update, and users who need the exact moment still need access to it. This
feature adds a per-user display option on the security page that switches the column to relative
time with an absolute-time tooltip on fine pointer devices and a touch drawer on coarse-pointer
devices.

#### Acceptance criteria

- **AC-UI-RELATIVE-LAST-SEEN-001.1:** The **Active sessions** card on `/settings/account/security` gains a **Last seen** display option (a select) offering **Absolute time** and **Relative time**.
- **AC-UI-RELATIVE-LAST-SEEN-001.2:** The default remains **Absolute time**; existing behavior is unchanged for users who never touch the option.
- **AC-UI-RELATIVE-LAST-SEEN-001.3:** With **Relative time** selected, the "Last seen" column renders a locale-aware relative label ("now", "5 minutes ago", "yesterday") that re-renders as time passes while the page is open.
- **AC-UI-RELATIVE-LAST-SEEN-001.4:** Hovering (or keyboard-focusing) a relative label shows a tooltip with the absolute timestamp on fine-pointer devices. Tapping the label opens a touch drawer with the absolute timestamp on coarse-pointer devices.
- **AC-UI-RELATIVE-LAST-SEEN-001.5:** The choice is a per-user persisted setting (`last_seen_display`), shared across the account and synced between tabs via the existing user-settings WebSocket push.
- **AC-UI-RELATIVE-LAST-SEEN-001.6:** Changing the select updates a local draft and the table preview. The shared **Save changes** action persists the draft. The shared **Reset** action discards the draft.
- **AC-UI-RELATIVE-LAST-SEEN-001.7:** **GIVEN** a user on `/settings/account/security` who has never changed the option, **WHEN** the Active sessions table renders, **THEN** the Last seen column shows absolute timestamps exactly as before.
- **AC-UI-RELATIVE-LAST-SEEN-001.8:** **GIVEN** the Last seen select, **WHEN** the user picks **Relative time**, **THEN** the column switches to relative labels immediately and the local draft becomes dirty.

## Migrated source detail

## Why

The **Active sessions** table on `/settings/account/security` renders the "Last seen" column as an
absolute timestamp (`formatDateTime`, e.g. "Aug 15, 2026, 3:42 PM"). Absolute stamps are precise but
hard to scan: a user reviewing devices signed in to their account must mentally subtract to know
whether a session was active moments ago or a month ago. Relative human-readable time ("5 minutes
ago", "yesterday") makes session freshness legible at a glance, which is the primary security
question a user is asking.

Relative time is inherently stale the moment it renders, so the label must live-update, and users
who need the exact moment still need access to it. This feature adds a per-user display option on
the security page that switches the column to relative time with an absolute-time tooltip on fine
pointer devices and a touch drawer on coarse-pointer devices.

## What

- The **Active sessions** card on `/settings/account/security` gains a **Last seen** display option
  (a select) offering **Absolute time** and **Relative time**.
- The default remains **Absolute time**; existing behavior is unchanged for users who never touch
  the option.
- With **Relative time** selected, the "Last seen" column renders a locale-aware relative label
  ("now", "5 minutes ago", "yesterday") that re-renders as time passes while the page is open.
- Hovering (or keyboard-focusing) a relative label shows a tooltip with the absolute timestamp on
  fine-pointer devices. Tapping the label opens a touch drawer with the absolute timestamp on
  coarse-pointer devices.
- The choice is a per-user persisted setting (`last_seen_display`), shared across the account and
  synced between tabs via the existing user-settings WebSocket push.
- Changing the select updates a local draft and the table preview. The shared **Save changes**
  action persists the draft. The shared **Reset** action discards the draft.

## Permissions

The option is a per-user preference on an account page; it requires only an authenticated session
(the same requirement as the security page itself). No admin or workspace authorization applies.

## Failure modes

- **Settings write fails:** the select keeps the draft and shows the error copy. The page remains
  usable, and the user can retry the shared **Save changes** action. The relative/absolute rendering
  never depends on a successful write to display.
- **Own-write echo vs unrelated snapshots:** the backend publishes a `user.settings.updated`
  event for this tab's own write before the PATCH response returns, and the gateway broadcasts it
  to every subscriber, including the initiating tab; the event is a full snapshot that any
  user-settings write re-publishes. A pending local selection is therefore never cleared while its
  write is in flight (own echoes and unrelated snapshots re-assert the pre-write value or the
  write's own confirmation); the selection settles when the write settles, and the page then
  always renders server truth, including any change another tab made meanwhile.
- **Unknown persisted value:** the backend coerces any value other than `"relative"` to
  `"absolute"` on read (normalization), so a stale or hand-edited blob cannot produce a broken
  column.
- **Concurrent unrelated PATCH (lost update):** user-settings updates are read-modify-write over a
  full settings blob; a concurrent PATCH that omits `last_seen_display` (for example an
  app-status-bar or review-top-bar save) could otherwise serialize a stale snapshot over this
  setting, and the frontend would accept it because its revision is newer. The PATCH write must be
  expected-revision CAS with bounded retry so an omitted-field write merges against the latest
  row and can never revert `last_seen_display`.
- **Unparseable `last_seen_at`:** the relative formatter returns an empty string for invalid input,
  but `formatDateTime` (used for the tooltip) throws `RangeError` on an invalid date. The relative
  cell must validate the timestamp once and render neither the tooltip nor the absolute label when
  it is unparseable, so the cell degrades to an empty cell instead of crashing the render.
- **Tooltip unavailable (touch/coarse pointer):** the absolute time remains reachable through the
  relative label's explicit tap action. The tap opens a drawer with the exact timestamp. The
  trigger also carries the absolute timestamp as its accessible name, and the fine-pointer path
  keeps the native `title` fallback. Mobile E2E proves the tap path at the Pixel 5 viewport.

## Scenarios

- **GIVEN** a user on `/settings/account/security` who has never changed the option, **WHEN** the
  Active sessions table renders, **THEN** the Last seen column shows absolute timestamps exactly as
  before.
- **GIVEN** the Last seen select, **WHEN** the user picks **Relative time**, **THEN** the column
  switches to relative labels immediately and the local draft becomes dirty.
- **GIVEN** a dirty Last seen draft, **WHEN** the user activates **Save changes**, **THEN** the
  choice persists to user settings and the draft becomes clean.
- **GIVEN** a dirty Last seen draft, **WHEN** the user activates **Reset**, **THEN** the select and
  table return to the confirmed setting without a settings write.
- **GIVEN** Relative time selected and a session last seen 3 minutes ago, **WHEN** the page stays
  open for a minute, **THEN** the label advances (for example from "3 minutes ago" to "4 minutes
  ago") without a page reload.
- **GIVEN** a relative label on a fine-pointer device, **WHEN** the user hovers or keyboard-focuses
  it (the trigger is focusable), **THEN** a tooltip shows the absolute timestamp for that session,
  and the absolute timestamp is also exposed as the trigger's accessible name and native title.
- **GIVEN** a coarse-pointer device with no hover, **WHEN** the user taps a relative label, **THEN**
  a drawer shows the absolute timestamp, and the select and relative labels remain usable at the
  phone viewport.
- **GIVEN** the user selects **Absolute time** again, **WHEN** the table renders, **THEN** absolute
  timestamps return and the persisted value flips back.
- **GIVEN** the option changed in another tab, **WHEN** the user-settings push arrives, **THEN**
  this page reflects the new value without a reload.

## Out of scope

- Changing how "Last seen" renders anywhere else (task surfaces, agent lists).
- Relative time for the **Created** or **Last used** columns elsewhere in account settings.
- A custom relative formatter: the shared locale-aware `formatRelativeTime` is used, consistent with
  the platform i18n spec.
- Backend storage schema changes: `last_seen_display` rides the existing JSON user-settings blob.

## Implementation plan

See [the implementation plan](../../../plans/relative-last-seen/plan.md).
