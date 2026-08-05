# ADR-2026-08-01-plugin-task-panel-contributions: Plugin Task Panel, Kanban Menu Action, and Card Indicator Contributions

**Status:** accepted (mobile navigation and lifecycle amended by ADR-2026-08-04-plugin-contribution-lifecycle-authority)
**Date:** 2026-08-01
**Area:** frontend

## Context

> **Amendment:**
> [ADR-2026-08-04-plugin-contribution-lifecycle-authority](2026-08-04-plugin-contribution-lifecycle-authority.md)
> replaces the elapsed-time revocation heuristic with authoritative lifecycle
> state and groups mobile-enabled task panels behind one bounded Panels picker.
> The registration API and generic `"plugin-panel"` identity remain unchanged.

The plugin contract's only UI surfaces are top-level routes, nav items,
settings routes, and a fixed `PluginSlot` name list — there is no way for a
plugin to contribute a panel to the task workspace (dockview desktop + mobile
bottom nav + saved layouts), an item to the kanban card context menu, or an
indicator on a kanban card. PR #2050's native task-notes feature needed all
three (a `notes` dockview panel, a mobile bottom-nav entry, a kanban
`Edit > Edit notes` shortcut, and a note glyph on the card) and, per this
repo's convention that feature-specific panels belong in a plugin repository
rather than the monorepo, porting it to a plugin requires these as host
extension points first.

PR #2117 (unmerged, `feat(plugins): add source-control extension contracts`)
independently modelled a provider-neutral `review-detail` dockview panel the
same way this ADR's panel primitive works, and defines a `registerTaskAction`
aimed at the Link submenu / task sidebar / task-switcher context menu. This
work targets `main` independently of #2117 (no dependency), but deliberately
mirrors its registration shape so the two primitives can be unified in a
follow-up once both have landed.

## Decision

**Task panels — one generic dockview component, not one per plugin.**
`registry.registerTaskPanel({ id, title, icon?, Component, mobileEnabled? })`
adds a row to the task workspace's "+" (add panel) menu. Every plugin panel
shares a single dockview component name, `"plugin-panel"`; panel identity
lives in `params: { pluginId, panelKey }`, and the panel id is
`plugin:{pluginId}:{panelKey}`. `PluginTaskPanel` resolves that pair against
the live registry on every render and supplies
`{ panelId, taskId, sessionId, presentation }` to the plugin's `Component`,
wrapped in a `PluginErrorBoundary`. Consequences of this shape:

- `renderPanel` (in both `dockview-shared.tsx` and the near-duplicate
  `dockview-panel-content.tsx`) gains exactly one `"plugin-panel"` case,
  regardless of how many plugins register panels — refactored from a `switch`
  to a lookup table so the addition doesn't trip the function-complexity lint
  ceiling.
- A saved layout round-trips a plugin panel reference even after that plugin
  is uninstalled: the component name (`"plugin-panel"`) is always known to
  `dockviewComponents`/`VALID_COMPONENTS`, so the layout restores; a
  registry-aware `resolvePluginPanelDefinition`/`isKnownPanelId`
  (`lib/state/layout-manager/plugin-panels.ts`) lets the layout manager treat
  an unresolvable reference as droppable rather than throwing, and
  `Settings > Layouts` renders a generic placeholder box for one it can't
  render live.
- `mobileEnabled: true` adds the panel to one grouped Panels bottom-nav action;
  the picker renders the same `Component` with `presentation: "mobile"` — no
  separate mobile registration or mobile-only contract.
- Disabling/uninstalling a plugin closes its currently open panels
  (`useCloseRevokedPluginPanels`, driven by authoritative lifecycle state) and
  removes its add-panel-menu row; a `Component` that throws during render shows
  a fallback scoped to just that panel. Slow or failed reloads preserve a panel
  until a ready generation omits it.

**Kanban menu action — data, not a rendered slot.**
`registry.registerTaskMenuAction({ id, label, icon?, group: "edit", visible?, run })`
returns menu *data* consumed by the existing `KanbanCardMenuEntry` structure
`buildKanbanCardMenuEntries` already builds for both the dropdown and context
menu. With no action registered, the card's `Edit` item renders exactly as
before (a flat item, not a submenu). Once any plugin registers one, the
existing item becomes `Edit > Edit task` and each visible plugin action
follows. A rendered React slot inside a Radix `DropdownMenu`/`ContextMenu`
was rejected because a plugin-authored component wouldn't carry
`DropdownMenuItem`/`ContextMenuItem` semantics (roving focus, typeahead,
close-on-select) unless it re-implemented them itself.

**Card indicator — the existing slot mechanism, not a new primitive.**
`registry.registerComponent("task-card-indicators", Component)` uses the
already-documented-as-open `PluginSlotName` union; `<PluginSlot
name="task-card-indicators" slotProps={{ taskId, workspaceId, workflowStepId }}/>`
renders beside `PRTaskIcon` in `kanban-card-content.tsx`. `PluginSlot`
already returns `null` for an empty slot, so the no-plugins-installed case is
byte-identical to today with zero extra code.

## Consequences

A plugin can now ship a full task-workspace panel (desktop dockview + mobile
bottom nav + layout-persistence-safe), a kanban card menu action, and a card
indicator with no bespoke host wiring per plugin — the enabling change for
porting PR #2050's native task-notes feature into a UI-only plugin. The
generic `"plugin-panel"` component means the host's dockview rendering code
does not grow linearly with the number of plugins that contribute panels.
`registerTaskMenuAction`'s `group: "edit"` is deliberately narrow (the kanban
card's `Edit` submenu only, not the Link submenu, task sidebar, or
task-switcher context menu #2117's `registerTaskAction` targets); unifying
the two primitives is left as a follow-up once #2117 lands, tracked as a risk
in the driving plan rather than solved here.

## Alternatives Considered

- **Register a dockview component per plugin panel.** Rejected: dockview
  reads its `components` map at construction time, so adding an entry after
  boot is fragile, and a saved layout referencing a since-removed
  per-plugin component name would throw on restore instead of degrading.
- **Reuse `PluginSlot` inside a single fixed "Plugins" panel** instead of a
  first-class panel primitive. Rejected: gives every plugin one shared panel
  with no per-plugin title/icon/layout identity and no mobile nav entry —
  does not satisfy independent add-panel-menu rows, saved-layout
  round-tripping, or the mobile bottom-nav requirement.
- **Wait for and depend on PR #2117's `registerTaskAction`** for the kanban
  menu action instead of a new `registerTaskMenuAction`. Rejected: #2117 is
  unmerged and targets a different menu surface (Link submenu / sidebar /
  task-switcher, not the card's `Edit` submenu); this work is scoped to land
  on `main` independently.
- **A dedicated `registerTaskCardIndicator` method** instead of reusing
  `registerComponent`. Rejected: a third near-identical registration list for
  no behavioral gain over the existing, already-open slot mechanism.
