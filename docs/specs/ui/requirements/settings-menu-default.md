---
status: active
system: ui
created: 2026-09-04
owners:
  - kandev
---

# Settings Menu Default Requirements

## Overview

The Settings Menu Shape preference controls how Settings navigation is composed
on each device. The default should expose the current navigation path through
workspaces, agents, and executors while preserving any choice that a user has
already made on that device.

## Terminology

- **Default mode:** The mode used when the device has no valid stored choice.
- **Stored mode:** A valid mode saved for the current device in local storage.

## Requirements

### REQ-UI-SETTINGS-MENU-DEFAULT-001: Accordion tree as the default

**Intent:** New or reset devices should start with the focused hierarchical
Settings navigation shown by Accordion tree.

**User story:** As a Kandev user, I want Settings to open with the current
navigation path expanded, so that I can reach related workspace, agent, and
executor settings without first changing a preference.

#### Acceptance criteria

- **AC-UI-SETTINGS-MENU-DEFAULT-001.1:** When the device has no stored Settings Menu Shape, the Settings menu shall use Accordion tree on desktop and phone.
- **AC-UI-SETTINGS-MENU-DEFAULT-001.2:** When the device has a valid stored Flat list or Persistent tree choice, the Settings menu shall continue to use that stored choice after the default changes.
- **AC-UI-SETTINGS-MENU-DEFAULT-001.3:** When the device has a missing, malformed, or unsupported stored mode, the Settings menu shall fall back to Accordion tree.
- **AC-UI-SETTINGS-MENU-DEFAULT-001.4:** The Settings Menu Shape descriptions shall identify Accordion tree as the default and shall not identify Flat list as the default.

## Mobile contract

- The phone Settings index keeps its existing full-page tree interaction and
  uses the same default mode as the desktop Settings takeover.
- This change does not add a drawer, alter the Settings route, or change the
  touch target, scrolling, or branch interaction rules.
- An explicit stored mode remains device-local and is not overwritten by the
  default change.

## Failure modes

- Invalid local storage values are treated like an absent value and resolve to
  Accordion tree.
- A storage read failure uses the same safe default without blocking Settings
  navigation.

## Out of scope

- Migrating devices that already store Flat list or Persistent tree.
- Changing the behavior or presentation of any supported menu mode.
- Changing the account-level settings API or adding server-side preference
  storage.
