---
status: draft
system: plugins
requirements:
  - REQ-PLUGINS-PLUGIN-NAV-SIDEBAR-FOOTER-001
created: 2026-08-12
owners:
  - nova28
---
# Plugin nav items in the sidebar footer icon row System Design Part 4

## Purpose and boundaries

This design preserves the technical source detail for `REQ-PLUGINS-PLUGIN-NAV-SIDEBAR-FOOTER-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLUGINS-PLUGIN-NAV-SIDEBAR-FOOTER-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Out of scope

- **Renaming the three shipped `NavItem.section` values** (`"main"`, `"integrations"`,
  `"settings"`). They are already independent of the host's internal `NavSection` names
  and are in use by shipped plugins; renaming them would be a breaking change for no
  gain. Only the new member is named after placement intent.
- **Accepting `"insights"` as an alias for `"sidebar-footer"`, or warning when a plugin
  passes an unrecognised `section`.** Both are decided against in **Plugin-facing
  section vocabulary**; the degrade rule is the whole error behaviour, and no runtime
  validation, console warning, or developer-mode diagnostic is added.
- **Backwards compatibility for `section: "insights"`.** Nothing has shipped that uses
  it: PR #2562 is unmerged and `kandev-plugin-rill` has not switched.
- **A capacity policy on the phone Utilities group.** It is a scrolling sheet of
  labelled rows; the clipping failure the desktop budget prevents does not occur there.
- **Capping, truncating, or overflowing first-party `insights` destinations.** The
  budget counts plugin entries only. If `APP_DESTINATIONS` ever grows a second
  `insights` entry, that is a first-party layout decision, not this contract.
- **Making the inline budget configurable** — by user setting, plugin manifest field,
  or runtime feature flag. It is a layout constant in the footer component.
- **Making the sidebar footer scroll, or changing the `overflow-hidden` sidebar
  container.** The budget removes the need; changing the container is a sidebar-layout
  decision, not part of opening a section to plugins.
- **Rolling back nav registrations made before a plugin's `initialize()` throws or
  times out.** Pre-existing loader behaviour across every section; see **Failure
  modes**.
- **A per-plugin or per-surface ordering control** (`order`, `priority`, or
  alphabetical sorting) for nav items, and any stickiness that would keep a plugin
  inline across a re-enable. Registration order stands for every section, including
  this one.
- **Changing the footer's or the phone group's other non-manifest controls.** The footer
  component gains exactly one new control, the overflow trigger, at the position stated
  in **Capacity and overflow**. No change to its bespoke buttons (gear, doctor, What's
  new, Office, theme, connection, user chip), to their styling or order, or to the phone
  Utilities group's bespoke rows (Status, theme, Improve Kandev, Health issues).
- **Changing the first-party catalog** (`core-destinations.ts`) or the resolver
  (`resolve-destinations.ts`). Neither needs to know a plugin item can be
  `sidebar-footer`; the budget is applied in the footer component, not in the manifest.
- **Adding a plugin `data-testid` prefix to the phone Utilities rows.** Those rows are
  test-id-less for first-party and plugin entries alike today; adding one is a
  separate surface-contract decision, and label-based selection covers this feature's
  conformance needs.
- **Palette entries for plugin nav items** of any section, and more broadly **any
  plugin route into the command palette**. There is none today: the palette's
  Navigation group resolves `useStaticDestinations("palette")` and plugin destinations
  never declare the `palette` surface, while `registerKeybinding` binds a global
  keydown handler rather than a palette row. Building one is its own feature and is not
  a prerequisite for this change — the exclusion here is the status quo, kept.
- **Backend manifest changes.** `ui.pages[].surface` is a different enum for a
  different purpose and gains no new value.
- **De-duplicating repeated `registerNavItem` calls** for the same `(pluginId, id)`.
  Pre-existing behaviour across every section.
- **The `utilities` nav section.** Only `insights` is opened to plugins; `utilities`
  holds the first-party Settings entry and stays closed.
- **Real-locale translations** of `sidebar:morePluginItems` (`zh-cn`, `pt-pt`). Those
  catalogs are translated out of band and only warn in CI; `en` and `pseudo` gate and
  are in scope.
- **Any change to `kandev-plugin-rill`**, the private plugin that motivates this. It
  switches its own `section` from `"main"` to `"sidebar-footer"` after this ships, in
  its own repository.
