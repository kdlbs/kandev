---
status: active
system: platform
created: 2026-11-03
owners:
  - kandev
---

# Notification settings layout requirements

## Overview

Users need a concise page for device alerts and remote providers.
Platform owns this capability because it owns notification permissions and sound preferences.
This document supplements the [notification contract](notifications.md) with a focused settings layout contract.
The existing requirement is near its size limit and retains delivery and permission policy.

## Requirements

### REQ-PLATFORM-NOTIFICATION-SETTINGS-LAYOUT-001: Compact device preferences

**Intent:** Remove repeated introductions and keep related controls together.

#### Acceptance criteria

- **AC-PLATFORM-NOTIFICATION-SETTINGS-LAYOUT-001.1:** The page shall show one Notifications page heading and one shared introduction for device alerts.
  Desktop notifications and Notification sound shall use compact setting labels without separate repeated descriptions.
- **AC-PLATFORM-NOTIFICATION-SETTINGS-LAYOUT-001.2:** On desktop, the desktop-notification label and its permission, refresh, and test actions shall share one row.
  The sound label, switch, selected preset, and preview action shall share another row.
  Sound shall precede Push notifications.
- **AC-PLATFORM-NOTIFICATION-SETTINGS-LAYOUT-001.3:** When sound is off, the preset selector and preview action shall remain visible and disabled.
  Toggling sound shall preserve the selected preset and shall not add or remove a control row.
- **AC-PLATFORM-NOTIFICATION-SETTINGS-LAYOUT-001.4:** On phones, the sound label and switch shall share the first row.
  The preset selector and preview action shall share a second row.
  Desktop-notification actions shall appear below their label.
  Each phone action shall provide a touch target of at least 44px.
  Content shall remain contained without horizontal page scrolling.
- **AC-PLATFORM-NOTIFICATION-SETTINGS-LAYOUT-001.5:** Sound edits shall remain drafts until the existing route Save action succeeds.
  Reset shall restore the saved switch and preset values.
  Unsaved-change feedback shall remain visible, and resizing shall preserve drafts.
- **AC-PLATFORM-NOTIFICATION-SETTINGS-LAYOUT-001.6:** The page shall retain permission requests, refresh, test actions, and localized permission warnings.
  Healthy permission states shall reserve no warning space.
  Controls shall retain accessible names, keyboard access, and settings-search targets.
  All changed copy shall be localized in every supported catalog.

## Out of scope

- Provider persistence, delivery policy, event selection, and sound playback policy.
- Changes to push, external-provider, or webhook controls.
- Global settings primitives, navigation, or save coordination.

## Implementation plans

- [Notification settings layout](../../../plans/notification-settings-layout/plan.md)
