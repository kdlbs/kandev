---
status: active
system: ui
created: 2026-07-30
owners:
  - kandev
---
# Transcript Navigation Settings Requirements

## Overview

Transcript navigation aids should begin unobtrusively while remaining available to users who want them. The settings page must also keep its final controls reachable when the shared floating Save action is visible.

## Requirements

### REQ-UI-TRANSCRIPT-NAVIGATION-SETTINGS-001: Transcript Navigation Settings

**Intent:** Transcript navigation aids should begin unobtrusively while remaining available to users who want them. The settings page must also keep its final controls reachable when the shared floating Save action is visible.

#### Acceptance criteria

- **AC-UI-TRANSCRIPT-NAVIGATION-SETTINGS-001.1:** A user can independently show or hide the desktop-only anchored prompt bar, the scroll-to-last-prompt control, the scroll-to-start control, and the per-session transcript auto-scroll control.
- **AC-UI-TRANSCRIPT-NAVIGATION-SETTINGS-001.2:** Users with no saved value start with:
- **AC-UI-TRANSCRIPT-NAVIGATION-SETTINGS-001.3:** **Show anchored prompt bar** off.
- **AC-UI-TRANSCRIPT-NAVIGATION-SETTINGS-001.4:** **Show scroll to last prompt** on.
- **AC-UI-TRANSCRIPT-NAVIGATION-SETTINGS-001.5:** **Show scroll to start** off.
- **AC-UI-TRANSCRIPT-NAVIGATION-SETTINGS-001.6:** **Show transcript auto-scroll control** off.
- **AC-UI-TRANSCRIPT-NAVIGATION-SETTINGS-001.7:** Hiding the transcript auto-scroll control only removes the per-session header button. Transcript auto-scroll remains enabled by default for every session, and an existing per-session choice is not rewritten when the control is hidden or shown.
- **AC-UI-TRANSCRIPT-NAVIGATION-SETTINGS-001.8:** Each preference is a local settings draft until the shared **Save changes** action succeeds.

## Migrated source detail

## Why

Transcript navigation aids should begin unobtrusively while remaining available to users who want
them. The settings page must also keep its final controls reachable when the shared floating Save
action is visible.

## What

- A user can independently show or hide the desktop-only anchored prompt bar, the scroll-to-last-prompt
  control, the scroll-to-start control, and the per-session transcript auto-scroll control.
- Users with no saved value start with:
  - **Show anchored prompt bar** off.
  - **Show scroll to last prompt** on.
  - **Show scroll to start** off.
  - **Show transcript auto-scroll control** off.
- Hiding the transcript auto-scroll control only removes the per-session header button. Transcript
  auto-scroll remains enabled by default for every session, and an existing per-session choice is
  not rewritten when the control is hidden or shown.
- Each preference is a local settings draft until the shared **Save changes** action succeeds.
- Saved values apply after reload and across devices through backend-owned user settings.
- Missing portable-setting values are resolved at the backend boundary and mapped consistently
  across frontend delivery paths per [ADR 0041](../../../decisions/0041-backend-owned-portable-user-settings.md).
- When the floating Save action is visible, the settings content scrolls far enough for the final
  transcript-navigation control to sit fully above the action on desktop and phone viewports.

## Data Model

The existing per-user settings JSON object stores four independent booleans:

| Field | Default | Meaning |
|---|---:|---|
| `show_anchored_prompt_bar` | `false` | Show the desktop-only anchored copy of the latest prompt. |
| `show_scroll_to_last_prompt` | `true` | Show the control that jumps to the latest prompt when eligible. |
| `show_scroll_to_start` | `false` | Show the control that jumps to the beginning of the transcript when eligible. |
| `show_transcript_auto_scroll_control` | `false` | Show the per-session auto-scroll toggle in transcript chrome. |

An omitted field uses its documented default. An explicit saved `true` or `false` is preserved.
The per-session auto-scroll choice remains transient browser-session state and is not replaced by
`show_transcript_auto_scroll_control`.

## API Surface

The existing user-settings GET, PATCH, boot payload, and `user.settings.updated` websocket event
include `show_transcript_auto_scroll_control` alongside the three existing transcript-navigation
fields. PATCH omission leaves a saved value unchanged.

## Persistence Guarantees

- The four visibility preferences survive reloads, process restarts, and device changes after Save.
- Unsaved settings drafts do not survive reload or discard.
- Hiding or showing the auto-scroll control does not persist a new per-session auto-scroll state.

## Scenarios

- **GIVEN** a user has no saved transcript-navigation preferences, **WHEN** settings and a
  transcript load, **THEN** the anchored prompt bar and scroll-to-start control are disabled, the
  scroll-to-last-prompt is enabled, the auto-scroll control is hidden, and transcript auto-scroll is on.
- **GIVEN** a user changes any transcript-navigation switch, **WHEN** Save has not been pressed,
  **THEN** no user-settings request is sent and the changed control is marked dirty.
- **GIVEN** a user saves transcript-navigation changes, **WHEN** settings and a transcript reload,
  **THEN** every saved control visibility is restored independently.
- **GIVEN** the auto-scroll control is hidden, **WHEN** a transcript receives new output, **THEN**
  its default auto-scroll behavior is unchanged and no auto-scroll toggle is rendered.
- **GIVEN** a session has a per-session auto-scroll choice, **WHEN** the user hides and later shows
  the auto-scroll control, **THEN** that session choice is not overwritten by the visibility
  preference.
- **GIVEN** the final transcript-navigation switch is dirty on desktop or a 390px phone viewport,
  **WHEN** the user scrolls to the bottom of the settings page, **THEN** the switch is fully visible
  above the floating Save action, the Save action remains reachable, and no document-level
  horizontal overflow is introduced.

## Out of Scope

- Removing the per-session auto-scroll button or making auto-scroll disabled by default.
- Changing when transcript navigation controls become eligible.
- Adding durable per-session auto-scroll preferences.
- Changing the floating Save action's visual design or Configuration Chat placement.

## Implementation Plan

See [the implementation plan](../../../plans/transcript-navigation-settings/plan.md).
