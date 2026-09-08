---
status: active
system: ui
created: 2026-07-31
owners:
  - product
---
# Responsive Changes Walkthrough Action Requirements

## Overview

The Changes toolbar can be narrower than the browser viewport inside the task workbench. At narrow panel widths, the **Walkthrough** action label is clipped instead of adapting to the space actually available in that panel.

## Requirements

### REQ-UI-CHANGES-WALKTHROUGH-TOOLBAR-WIDTH-001: Responsive Changes Walkthrough Action

**Intent:** The Changes toolbar can be narrower than the browser viewport inside the task workbench. At narrow panel widths, the **Walkthrough** action label is clipped instead of adapting to the space actually available in that panel.

#### Acceptance criteria

- **AC-UI-CHANGES-WALKTHROUGH-TOOLBAR-WIDTH-001.1:** The Changes toolbar adapts the **Walkthrough** action to the inline width of the Changes panel, independent of the browser viewport width.
- **AC-UI-CHANGES-WALKTHROUGH-TOOLBAR-WIDTH-001.2:** At a Changes-panel width of 349px or less, the action shows its route icon without the visible **Walkthrough** label.
- **AC-UI-CHANGES-WALKTHROUGH-TOOLBAR-WIDTH-001.3:** At a Changes-panel width of 350px or more, the action shows both its route icon and the **Walkthrough** label.
- **AC-UI-CHANGES-WALKTHROUGH-TOOLBAR-WIDTH-001.4:** Both presentations retain the same accessible name, tooltip, enabled and disabled behavior, and click outcome.
- **AC-UI-CHANGES-WALKTHROUGH-TOOLBAR-WIDTH-001.5:** The action and the toolbar's branch and Pull controls remain contained within the Changes panel without introducing document-level horizontal overflow.
- **AC-UI-CHANGES-WALKTHROUGH-TOOLBAR-WIDTH-001.6:** The desktop and phone Changes surfaces share the same width-driven behavior and preserve the ability to request a walkthrough.
- **AC-UI-CHANGES-WALKTHROUGH-TOOLBAR-WIDTH-001.7:** **GIVEN** a desktop task whose Changes panel is 349px wide, **WHEN** the Changes toolbar renders, **THEN** the Walkthrough label is hidden while its icon button remains visible, accessible, and fully contained in the panel.
- **AC-UI-CHANGES-WALKTHROUGH-TOOLBAR-WIDTH-001.8:** **GIVEN** a desktop task whose Changes panel is 350px wide, **WHEN** the Changes toolbar renders, **THEN** the Walkthrough label is visible and the complete action remains contained in the panel.

## Migrated source detail

## Why

The Changes toolbar can be narrower than the browser viewport inside the task
workbench. At narrow panel widths, the **Walkthrough** action label is clipped
instead of adapting to the space actually available in that panel.

## What

- The Changes toolbar adapts the **Walkthrough** action to the inline width of
  the Changes panel, independent of the browser viewport width.
- At a Changes-panel width of 349px or less, the action shows its route icon
  without the visible **Walkthrough** label.
- At a Changes-panel width of 350px or more, the action shows both its route
  icon and the **Walkthrough** label.
- Both presentations retain the same accessible name, tooltip, enabled and
  disabled behavior, and click outcome.
- The action and the toolbar's branch and Pull controls remain contained within
  the Changes panel without introducing document-level horizontal overflow.
- The desktop and phone Changes surfaces share the same width-driven behavior
  and preserve the ability to request a walkthrough.

## Scenarios

- **GIVEN** a desktop task whose Changes panel is 349px wide, **WHEN** the
  Changes toolbar renders, **THEN** the Walkthrough label is hidden while its
  icon button remains visible, accessible, and fully contained in the panel.
- **GIVEN** a desktop task whose Changes panel is 350px wide, **WHEN** the
  Changes toolbar renders, **THEN** the Walkthrough label is visible and the
  complete action remains contained in the panel.
- **GIVEN** a wide laptop viewport with a Changes panel narrowed to 349px,
  **WHEN** the toolbar renders, **THEN** the panel width rather than the
  viewport width selects the icon-only presentation.
- **GIVEN** the phone Changes surface is wide enough to show the label, **WHEN**
  the user requests a walkthrough, **THEN** the action remains within the
  viewport and produces the same walkthrough request as desktop.

## Out of scope

- Redesigning the Changes toolbar or moving actions into an overflow menu.
- Changing the responsive behavior of Diff, Review, branch, or Pull controls.
- Changing walkthrough generation, display, navigation, or persistence.
