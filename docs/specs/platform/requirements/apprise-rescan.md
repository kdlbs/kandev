---
status: draft
system: platform
created: 2026-09-09
owners:
  - kandev
---

# Apprise Rescan Requirements

## Overview

Users can detect Apprise after installing it while Kandev is running.
Platform owns this capability because it owns notification provider availability.
This extends provider discovery; [semantic notifications](notifications.md)
continues to own event selection and delivery.

## Requirements

### REQ-PLATFORM-APPRISE-RESCAN-001: Manual Apprise detection

**Intent:** Refresh Apprise availability from Notifications settings without
reloading the page or losing settings drafts.

#### Acceptance criteria

- **AC-PLATFORM-APPRISE-RESCAN-001.1:** External Providers shall offer a
  labeled rescan action whether Apprise is detected or missing.
- **AC-PLATFORM-APPRISE-RESCAN-001.2:** When a rescan completes, the page shall
  show the current detection result and update access to Add Apprise Provider.
  Detection shall use the environment where the Kandev backend runs.
- **AC-PLATFORM-APPRISE-RESCAN-001.3:** While checking, the action shall show
  progress and prevent duplicate requests. After a request error, it shall show
  retry feedback and retain the last known availability.
- **AC-PLATFORM-APPRISE-RESCAN-001.4:** Rescanning shall preserve unsaved names,
  URLs, event selections, removals, and open forms. It shall neither save settings
  nor send notifications. A rescan alone shall not mark settings dirty.
- **AC-PLATFORM-APPRISE-RESCAN-001.5:** Desktop and mobile shall expose the same
  action and feedback. It shall support keyboard activation, translated copy,
  a touch target at least 44px high, and no document horizontal overflow.

## Out of scope

- Installing Apprise, importing its configuration, or discovering service URLs.
- Searching another machine or changing the running backend's environment.
- Automatic polling, new provider types, or changes to notification delivery.
- General notification loading-error redesign or unrelated settings layout.
