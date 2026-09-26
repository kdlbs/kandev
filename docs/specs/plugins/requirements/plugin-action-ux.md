---
status: draft
system: plugins
created: 2026-09-25
owners:
  - kandev
---

# Plugin action UX requirements

## Overview

Plugin actions must match native controls in their host location. Existing
plugins must continue to work without a coordinated plugin update.
The plugin system owns this optional extension contract. The UI system owns
[control sizing](../../ui/requirements/control-sizing.md) and
[status surfaces](../../ui/requirements/app-status-bar.md).

A standard action is an interactive control that adopts host styling.
A legacy contribution is an existing component that retains its own styling.
Adoption is optional. Compatibility does not imply automatic visual migration.

## Requirements

### REQ-PLUGINS-ACTION-UX-001: Additive adoption

**Intent:** Authors can adopt native styling without disrupting installed plugins.

#### Acceptance criteria

- **AC-PLUGINS-ACTION-UX-001.1:** Existing contributions shall retain their
  registrations, context, appearance, interactions, and settings after a host update.
- **AC-PLUGINS-ACTION-UX-001.2:** Authors shall be able to adopt a standard
  action inside an existing slot without replacing the registration mechanism.
- **AC-PLUGINS-ACTION-UX-001.3:** Standard and legacy actions shall coexist
  without duplicate contributions, changed ordering identities, or shared state.
- **AC-PLUGINS-ACTION-UX-001.4:** An adopting plugin shall be able to select
  its legacy control when the host lacks standard-action support.
- **AC-PLUGINS-ACTION-UX-001.5:** Plugin disable, enable, and replacement shall
  retain existing cleanup and error-isolation behavior for both action forms.

### REQ-PLUGINS-ACTION-UX-002: Native appearance by location

**Intent:** The host location determines action appearance, including future theme changes.

#### Acceptance criteria

- **AC-PLUGINS-ACTION-UX-002.1:** Standard actions shall match adjacent native
  actions with the same role in height, border, radius, padding, and visual states.
- **AC-PLUGINS-ACTION-UX-002.2:** Standalone icon actions shall have square
  targets. Ordinary toolbar glyphs shall use a 16px box at the default root font.
  Compact locations shall match the native glyph role.
- **AC-PLUGINS-ACTION-UX-002.3:** Each location shall own spacing between
  standard actions. Disabled, busy, and pressed states shall retain control dimensions.
- **AC-PLUGINS-ACTION-UX-002.4:** The contract shall cover main and task topbars,
  sidebar workspace actions, status items, and all four native composer surfaces.
- **AC-PLUGINS-ACTION-UX-002.5:** Phone standard actions shall provide targets
  at least 44px high. Standalone touch icons shall also be at least 44px wide.
  Non-status ordinary controls shall provide the same targets on coarse-pointer devices.
- **AC-PLUGINS-ACTION-UX-002.6:** Status actions shall fit the existing 24px
  non-phone bar and use native rows in the phone Status drawer.
  Existing visibility preferences, saved ordering, and tablet density shall remain unchanged.
- **AC-PLUGINS-ACTION-UX-002.7:** Long labels and values shall not overlap
  adjacent controls or cause document horizontal overflow. The full action name shall remain accessible.

### REQ-PLUGINS-ACTION-UX-003: Interaction and content continuity

**Intent:** Authors can retain specialized behavior while adopting the shared control.

#### Acceptance criteria

- **AC-PLUGINS-ACTION-UX-003.1:** Standard actions shall support activation,
  controlled pressed state, disabled state, busy indication, and bounded decorative content.
  Busy indication alone shall not prevent a stop action.
- **AC-PLUGINS-ACTION-UX-003.2:** Authors shall retain focus, pointer-capture,
  press-and-hold, and existing overlay-trigger interactions without nested interactive controls.
- **AC-PLUGINS-ACTION-UX-003.3:** Actions shall have localized accessible names,
  visible focus, and keyboard activation. Informational tooltips shall not be the only access to essential content.
- **AC-PLUGINS-ACTION-UX-003.4:** Phone topbar actions shall retain the existing
  Plugins section and task-over-workspace selection. Composer actions shall remain near their composer.
- **AC-PLUGINS-ACTION-UX-003.5:** Presentation changes shall preserve mounted
  action state when the host keeps the contribution mounted. Actions shall not gain access to another composer.

## Compatibility and exclusions

The compact non-phone status bar retains its existing specialized geometry,
including on tablets. This package does not claim a 44px target inside that bar.
Changing the bar or its tablet routing requires separate UI-contract work.

This package excludes forced legacy restyling, removal of slots, plugin package
publication, new permissions, a new registry, and a generalized overflow system.
It does not redesign plugin dialogs or standardize arbitrary plugin pages.
Read-only status widgets can retain their existing slots. Native menu and
sidebar-footer navigation registrations keep their existing renderers.

## Related documents

- [System design](../system-design/plugin-action-ux.md)
- [Implementation plan](../../../plans/plugin-action-ux/plan.md)
