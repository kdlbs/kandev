---
status: active
system: plugins
created: 2026-09-04
owners:
  - kandev
---
# Plugin Shortcut Settings Requirements

## Overview

Plugin shortcut declarations belong to an installed plugin, but their controls
currently appear among Kandev's core shortcuts. This separates a plugin's
shortcut from the rest of that plugin's settings and makes a per-user
preference look like install-wide operator configuration. The plugin system
owns the declaration, identity, and configuration location; the UI system's
shared settings-save behavior remains an independent dependency.

## Requirements

### REQ-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001: Plugin-owned shortcut configuration

**Intent:** Keep each plugin's configurable shortcuts with that plugin while
preserving personal overrides, conflict visibility, and core-shortcut
precedence.

**User story:** As a Kandev user, I want to configure a plugin's shortcuts from
that plugin's settings page, so that I can find the controls where I manage the
plugin without mixing them into Kandev's core shortcuts.

#### Acceptance criteria

- **AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.1:** When an installed plugin
  declares one or more `ui.keybindings`, its detail page shall show an inline
  shortcut editor containing only that plugin's declared bindings, and the
  Preferences > Keyboard Shortcuts page shall show only Kandev-owned bindings.
  The host-owned editor shall remain present when an active plugin registers a
  replacement component at its exact detail route.
- **AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.2:** Every user who can open the
  installed plugin's detail page shall be able to edit their own shortcut
  overrides without administrator permission; plugin configuration, enable,
  disable, update, and uninstall controls shall retain their existing
  administrator-only authorization.
- **AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.3:** Recording, clearing, or
  resetting a plugin shortcut shall change a route-local draft and use the
  shared Settings save action. Saving only a shortcut shall not restart,
  enable, disable, or otherwise mutate the plugin installation.
- **AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.4:** The relocated editor shall
  read and write the existing namespaced per-user override identity
  `plugin:{pluginId}:{keybindingId}`. Existing overrides shall remain effective
  and visible after the relocation, a reload, and normal portable-settings
  synchronization, without a data migration.
- **AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.5:** Conflict warnings on both the
  core shortcut page and a plugin shortcut editor shall compare the effective
  bindings of all Kandev-owned shortcuts and all installed plugin declarations,
  name the other conflicting actions, and preserve the rule that a Kandev-owned
  shortcut wins during dispatch.
- **AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.6:** A disabled or errored plugin's
  declared shortcuts shall remain visible and configurable on its detail page,
  while the page makes clear that they run only while the plugin is active. A
  plugin with no declared keybindings shall not render an empty shortcut card.
- **AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.7:** On a phone, the shortcut editor
  shall be reachable through Settings > Plugins > the installed plugin, retain
  the detail page as the single vertical scroll owner, keep recording and
  reset controls touch-usable without horizontal overflow, and complete the
  same save-and-reload outcome as the desktop surface.

## Out of scope

- Changing the `ui.keybindings` manifest grammar, `registerKeybinding`, editor
  opt-in behavior, dispatch order, or core-shortcut precedence.
- Moving plugin-authored settings slots or install-wide plugin configuration
  across the existing user and administrator authorization boundary.
- Adding plugin-authored fields or individual plugin shortcuts to global
  Settings search results.
- Deleting orphaned per-user override keys when a plugin is uninstalled.
