---
status: draft
system: plugins
created: 2026-08-12
owners:
  - nova28
---
# Plugin nav items in the sidebar footer icon row Requirements

## Overview

A plugin whose page is a compact, glanceable dashboard has nowhere good to put its
entry point. `registerNavItem` can place a row in the sidebar's labelled plugin rail
or in the Integrations section, but not in the sidebar footer's unlabelled icon strip
(the gear / Stats / doctor / Office row) where Kandev's own at-a-glance destination,
Stats, lives. Plugin authors either accept a full-width labelled row that visually
outranks what they are contributing, or they get no placement that matches the shape
of their page.

## Requirements

### REQ-PLUGINS-PLUGIN-NAV-SIDEBAR-FOOTER-001: Plugin nav items in the sidebar footer icon row

**Intent:** A plugin whose page is a compact, glanceable dashboard has nowhere good to put its entry
point. `registerNavItem` can place a row in the sidebar's labelled plugin rail or in the
Integrations section, but not in the sidebar footer's unlabelled icon strip (the gear / Stats /
doctor / Office row) where Kandev's own at-a-glance destination, Stats, lives. Plugin authors either
accept a full-width labelled row that visually outranks what they are contributing, or they get no
placement that matches the shape of their page.

#### Acceptance criteria

- **AC-PLUGINS-PLUGIN-NAV-SIDEBAR-FOOTER-001.1:** `NavItem.section` SHALL accept a fourth value, `"sidebar-footer"`, in addition to the existing `"main"`, `"integrations"`, and `"settings"`. The four values SHALL be declared as a named exported type, `PluginNavSection`. See **Plugin-facing section vocabulary** for why the value is `"sidebar-footer"` and not the host's internal section name.
- **AC-PLUGINS-PLUGIN-NAV-SIDEBAR-FOOTER-001.2:** A plugin nav item with `section: "sidebar-footer"` SHALL render as an icon button in the desktop sidebar footer's icon row, styled and behaving exactly like the first-party Stats button: icon only, tooltip and accessible name from `NavItem.label`, click navigates to `NavItem.path` — subject to the inline budget in **Capacity and overflow**, past which the item is reached through the footer's overflow menu instead of an inline button.
- **AC-PLUGINS-PLUGIN-NAV-SIDEBAR-FOOTER-001.3:** The same item SHALL also appear as a labelled row in the phone menu's Utilities group, which is where the first-party Stats destination already appears below the `md` breakpoint. The sidebar is hidden on phones, so a footer-only placement would make the item unreachable there. The phone group is **not** subject to the inline budget; see **Capacity and overflow**.
- **AC-PLUGINS-PLUGIN-NAV-SIDEBAR-FOOTER-001.4:** A `"sidebar-footer"` item SHALL NOT also appear in the sidebar's plugin rail or the phone menu's Plugins group. Choosing `sidebar-footer` moves the item; it does not add a second placement.
- **AC-PLUGINS-PLUGIN-NAV-SIDEBAR-FOOTER-001.5:** `"integrations"` and `"settings"` behaviour SHALL be unchanged: `"integrations"` items still render alongside the first-party integration links on both surfaces, and `"settings"` items are still skipped entirely by navigation, which means they render on no surface at all. A `section: "settings"` nav item is **not** what fills the settings tree's `PluginSlot`: that slot (`<PluginSlot name="settings-nav" />` in `settings-tree.tsx`) is fed only by `registerComponent("settings-nav", …)`, and a plugin that wants a settings page uses `registerSettingsRoute` / `registerComponent`. `section: "settings"` on a nav item is accepted and then dropped.
- **AC-PLUGINS-PLUGIN-NAV-SIDEBAR-FOOTER-001.6:** Any other `section` value, including omitted/`undefined` and a string outside the documented set, SHALL continue to render in the plugin rail / Plugins group exactly as `"main"` does. Plugin bundles are untyped JavaScript at runtime, so an unknown value must degrade to the default placement rather than being dropped. **The literal string `"insights"` is one such unknown value and SHALL degrade to the plugin rail** — it is deliberately not an accepted alias; see **Plugin-facing section vocabulary**.
- **AC-PLUGINS-PLUGIN-NAV-SIDEBAR-FOOTER-001.7:** Plugin footer items SHALL NOT appear in the command palette, unchanged from every other plugin section. This follows structurally, not from a rule this spec adds: `pluginDestinations()` stamps every plugin entry with `surfaces: SIDEBAR_AND_MENU`, and the palette's Navigation group is `useStaticDestinations("palette")` (`components/global-commands.tsx`), which filters on that surface. **Plugins have no route into the command palette at all today, by any API.** `registerKeybinding(id, handler)` is not one: it binds a handler to an id declared in the plugin's manifest `ui.keybindings[]`, which the host dispatches from a global capture-phase keydown listener (`hooks/use-plugin-shortcuts.ts`) — a keyboard binding, not a palette row. Adding a plugin→palette route is a separate feature (see **Out of scope**).
- **AC-PLUGINS-PLUGIN-NAV-SIDEBAR-FOOTER-001.8:** The desktop footer SHALL render at most `MAX_INLINE_PLUGIN_FOOTER_ITEMS` plugin buttons inline and reach any remainder through a single overflow menu. No item is dropped. See **Capacity and overflow** for the value, the derivation, and the exact rule.

## System design

The migrated technical source is split into [part 1](../system-design/plugin-nav-sidebar-footer-01.md), [part 2](../system-design/plugin-nav-sidebar-footer-02.md), [part 3](../system-design/plugin-nav-sidebar-footer-03.md), [part 4](../system-design/plugin-nav-sidebar-footer-04.md).
