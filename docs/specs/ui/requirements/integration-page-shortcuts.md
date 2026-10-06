---
status: active
system: ui
created: 2026-10-06
owners:
  - kandev
---

# Integration Page Shortcuts Requirements

## Overview

Users can assign a personal hotkey to open each integration from Settings >
Preferences > Keyboard Shortcuts (the Keybindings settings area). UI owns this
navigation preference; provider credentials, permissions, and plugin-authored
actions remain with their existing systems.

An integration page means its existing Kandev dashboard when one exists, or its
connection settings page otherwise. Shortcuts open Kandev pages in the current
tab, rather than the external service's website.

## Requirements

### REQ-UI-INTEGRATION-PAGE-SHORTCUTS-001: Configurable integration navigation

**Intent:** Open integration pages directly with a user-selected key combination.

#### Acceptance criteria

- **AC-UI-INTEGRATION-PAGE-SHORTCUTS-001.1:** The Keybindings settings area
  shall show a labelled Integrations group with one initially unbound action
  for Azure DevOps, GitHub, GitLab, Jira, Linear, and Sentry, including when
  a connection is absent or disabled.
- **AC-UI-INTEGRATION-PAGE-SHORTCUTS-001.2:** Each active plugin navigation
  destination offered in the Integrations group shall have a distinct host-owned
  page-opening shortcut in the same settings group. Plugin-authored action
  shortcuts shall remain on the plugin detail page. Repeated registrations with
  the same plugin-owned navigation ID shall produce one shortcut, using the first
  eligible integration registration.
- **AC-UI-INTEGRATION-PAGE-SHORTCUTS-001.3:** Recording or resetting a shortcut
  shall modify only a draft until Save changes succeeds. A saved binding shall
  survive reload and portable user-settings synchronization. Reset shall restore
  the unbound state; saving shall preserve unrelated shortcut overrides. A failed
  save shall retain a retryable draft.
- **AC-UI-INTEGRATION-PAGE-SHORTCUTS-001.4:** From any authenticated application
  route, invoking a saved binding shall open that integration's existing page
  in the current tab. Azure DevOps, GitHub, GitLab, Jira, and Linear shall open
  their dashboards. Sentry shall open its connection settings page. Navigation
  shall use the current workspace context and preserve existing route permission
  and connection handling.
- **AC-UI-INTEGRATION-PAGE-SHORTCUTS-001.5:** An integration navigation shortcut
  shall not run during text entry, shortcut recording, an already handled event,
  or a held-key repeat. Unbound or no-longer-registered destinations shall not
  navigate or consume keyboard input. Removing or disabling a plugin shall leave
  its saved navigation binding inert without deleting it. Integration recording
  and dispatch shall leave Tab and Shift+Tab without Control, Command, or Alt
  available for existing focus and keyboard behavior, including saved legacy bindings.
- **AC-UI-INTEGRATION-PAGE-SHORTCUTS-001.6:** Conflict warnings shall compare
  integration navigation bindings with other integration bindings, Kandev's
  configurable shortcuts, and installed plugin action shortcuts. Existing
  Kandev shortcuts shall take precedence over integration navigation; integration
  navigation shall take precedence over plugin actions. Two integration bindings
  sharing a combination shall navigate once to the first displayed destination.
  The fixed Ctrl/Cmd+Shift+P command-panel combination shall remain reserved.
- **AC-UI-INTEGRATION-PAGE-SHORTCUTS-001.7:** On phones, the same group shall
  remain reachable from the settings index, with labels above wrapping controls,
  visible reset actions, touch targets of at least 44px, one vertical settings
  scroller, and no document horizontal overflow. An attached keyboard shall
  support recording and invoking bindings. Touch-only users shall retain ordinary
  integration navigation.
- **AC-UI-INTEGRATION-PAGE-SHORTCUTS-001.8:** Action labels, grouping, recorder
  feedback, and conflict information shall be localized and accessible. Plugin
  navigation labels shall identify the owning plugin where needed to disambiguate
  destinations; product names shall remain unchanged. A recorder shall expose its
  current binding or unbound state as an accessible description and announce
  recording and binding changes through a localized live status.

## Out of scope

- Default assigned integration hotkeys, external website shortcuts, or new dashboards.
- Changing provider connection state, sidebar visibility preferences, plugin
  manifests, plugin action settings ownership, or navigation permissions.
- A shortcut picker for the phone software keyboard or new command-palette entries.
- Cleaning up orphaned overrides or changing existing non-navigation shortcuts.

## Implementation Plans

- [Integration page shortcuts](../../../plans/integration-page-shortcuts/plan.md)
