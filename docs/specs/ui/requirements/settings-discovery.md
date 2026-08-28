---
status: active
system: ui
created: 2026-08-05
owners:
  - kandev
---
# Settings Discovery Requirements

## Overview

Kandev has enough settings that users cannot reliably infer which page owns a control. The settings tree and command palette should help users find a setting by the language they remember, without requiring them to browse every section or making the default command palette noisy.

## Requirements

### REQ-UI-SETTINGS-DISCOVERY-001: Settings Discovery

**Intent:** Kandev has enough settings that users cannot reliably infer which page owns a control. The settings tree and command palette should help users find a setting by the language they remember, without requiring them to browse every section or making the default command palette noisy.

#### Acceptance criteria

- **AC-UI-SETTINGS-DISCOVERY-001.1:** The Settings tree shows a search field above its existing navigation on desktop and phone.
- **AC-UI-SETTINGS-DISCOVERY-001.2:** On desktop, the search field follows the compact density of the surrounding navigation instead of using phone-sized vertical chrome.
- **AC-UI-SETTINGS-DISCOVERY-001.3:** An empty query preserves the existing tree, ordering, active route, accordion state, badges, and plugin navigation.
- **AC-UI-SETTINGS-DISCOVERY-001.4:** A non-empty query shows only matching first-party pages, sections, and controls, grouped by their settings area. Each result shows its own label and an owning-page breadcrumb.
- **AC-UI-SETTINGS-DISCOVERY-001.5:** As the query changes, matching rows use a brief layout transition instead of snapping between positions. Newly matching rows enter softly, while reduced-motion users get immediate updates.
- **AC-UI-SETTINGS-DISCOVERY-001.6:** Search matches translated labels and curated translated aliases. It never indexes saved values, secrets, paths, URLs, prompts, or other user-entered configuration.
- **AC-UI-SETTINGS-DISCOVERY-001.7:** At least one query token must match the result's own label or aliases. Other tokens may narrow by breadcrumb. A parent name by itself does not flood results with every descendant.
- **AC-UI-SETTINGS-DISCOVERY-001.8:** Exact labels rank above prefixes, aliases, and contextual matches; ties retain catalog order.

## Migrated source detail

## Why

Kandev has enough settings that users cannot reliably infer which page owns a control. The
settings tree and command palette should help users find a setting by the language they remember,
without requiring them to browse every section or making the default command palette noisy.

## What

- The Settings tree shows a search field above its existing navigation on desktop and phone.
- On desktop, the search field follows the compact density of the surrounding navigation instead
  of using phone-sized vertical chrome.
- An empty query preserves the existing tree, ordering, active route, accordion state, badges, and
  plugin navigation.
- A non-empty query shows only matching first-party pages, sections, and controls, grouped by their
  settings area. Each result shows its own label and an owning-page breadcrumb.
- As the query changes, matching rows use a brief layout transition instead of snapping between
  positions. Newly matching rows enter softly, while reduced-motion users get immediate updates.
- Search matches translated labels and curated translated aliases. It never indexes saved values,
  secrets, paths, URLs, prompts, or other user-entered configuration.
- At least one query token must match the result's own label or aliases. Other tokens may narrow by
  breadcrumb. A parent name by itself does not flood results with every descendant.
- Exact labels rank above prefixes, aliases, and contextual matches; ties retain catalog order.
- Every indexed control has a canonical settings URL and stable fragment. Selecting it opens the
  owning page, scrolls the control into view, focuses it without changing its value, and briefly
  highlights it.
- Same-page targeting does not trigger the unsaved-settings leave confirmation. Cross-page
  targeting uses the normal guarded router.
- Cmd+K exposes the same settings catalog only after the user types. The default palette retains
  its compact top-level Go to Settings destination.
- Cmd+K setting results show Settings plus the owning hierarchy as secondary context. Selecting a
  result navigates to the same exact target as Settings-tree search; it never edits the value.
- Typed Cmd+K results keep regular actions under Commands and place granular setting results in a
  separate localized Settings section. Empty sections are omitted.
- Discovery respects the same auth, admin, feature, capability, and resource availability rules as
  the rendered settings surface.
- Stable first-party pages and inline controls are indexed. Transient dialog fields, destructive
  confirmations, generated collection rows, and opaque plugin-authored controls are explicitly
  excluded.

## Permissions

Discovery does not grant access. Account, user-management, feature-gated, and capability-gated
entries appear only when the current user and runtime could render the destination. Installed
plugin pages may be discoverable from host-known metadata, but plugin-authored form internals are
not indexed without a plugin discovery contract.

## Failure modes

- Dynamic resource loading failures leave static settings searchable and omit unresolved dynamic
  profile or workspace entries; no error replaces the command palette or settings tree.
- A stale or manually entered target fragment that has no registered control leaves the page
  usable and does not move focus.
- A target rendered after asynchronous data arrives is revealed when it registers; discovery does
  not use fixed-delay retries.
- Canceling the existing unsaved-settings navigation confirmation leaves the current page and draft
  unchanged.

## Mobile contract

- The existing full-height Settings Sheet remains the phone entry point because the hierarchy is
  dense and frequently searched; this feature does not introduce another overlay.
- Desktop and phone share catalog, filtering, ranking, navigation, and target state.
- The Sheet keeps one navigation scroll owner. The settings page keeps its existing content scroll
  owner. Search does not add nested vertical or document horizontal scrolling.
- Search and clear controls have at least 44 px touch targets. Selecting a result closes the Sheet
  and leaves the destination control focused.

## Scenarios

- **GIVEN** an empty Settings search, **WHEN** the tree renders, **THEN** its current groups, active
  route, badges, and plugin slot remain unchanged below the search field.
- **GIVEN** the desktop Settings tree, **WHEN** the search field renders, **THEN** its visible height
  aligns with a neighboring navigation row rather than the 44 px phone control height.
- **GIVEN** the query `font size`, **WHEN** results render, **THEN** Terminal font size appears under
  General › Terminal and unrelated sections are absent.
- **GIVEN** visible Settings search results, **WHEN** the query changes and surviving results move,
  **THEN** rows settle into their new positions with restrained motion rather than snapping.
- **GIVEN** reduced motion, **WHEN** Settings search results change, **THEN** rows update immediately
  without layout or presence animation.
- **GIVEN** the query `General`, **WHEN** results render, **THEN** the General page can match without
  every General descendant appearing solely because of its breadcrumb.
- **GIVEN** the query `workflows`, **WHEN** a dynamic workspace result renders, **THEN** its label is
  the same translated `Workflows` navigation label, never a lowercase raw-key fallback.
- **GIVEN** a matching control on another settings page, **WHEN** the user selects it, **THEN** the
  guarded router opens its canonical URL and the control is scrolled, focused, and highlighted.
- **GIVEN** a matching control on the current page with unsaved changes, **WHEN** the user selects
  it, **THEN** the fragment changes and the control is revealed without a leave confirmation.
- **GIVEN** Cmd+K with no query, **WHEN** the palette opens, **THEN** granular settings are absent and
  Go to Settings remains available.
- **GIVEN** Cmd+K with `terminal font size`, **WHEN** results render, **THEN** the exact setting and
  Settings › General › Terminal context appear and Enter reveals the input.
- **GIVEN** a Cmd+K query matches both a regular action and a granular setting, **WHEN** results
  render, **THEN** the action appears under Commands and the granular result appears under Settings.
- **GIVEN** a delayed agent or executor profile list, **WHEN** matching commands register, **THEN**
  stable command IDs prevent the current matching selection from jumping.
- **GIVEN** reduced motion, **WHEN** an exact target opens, **THEN** scrolling is instant and the
  highlight does not pulse.
- **GIVEN** a 390 px phone viewport, **WHEN** the user searches and selects a setting, **THEN** the
  Sheet remains contained, closes after selection, and the destination is usable without horizontal
  overflow.

## Out of scope

- Editing or toggling settings directly from Cmd+K.
- Server-side, semantic, or saved-value search.
- A broader Settings Sheet/Drawer or settings-layout redesign.
- Direct targets inside transient dialogs, confirmations, or individual generated rows.
- A public discovery metadata contract for plugin-authored controls.
- Backend, database, or persistence changes.

## Implementation plan

See [the implementation plan](../../../plans/settings-discovery/plan.md) and
[ADR-2026-08-04-navigation-manifest-boundaries](../../../decisions/2026-08-04-navigation-manifest-boundaries.md).
