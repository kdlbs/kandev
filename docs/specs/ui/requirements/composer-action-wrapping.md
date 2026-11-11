---
status: active
system: ui
created: 2026-10-09
owners:
  - Kandev
---

# Composer Action Wrapping Requirements

## Overview

Keep the next workflow step in the same phone composer row as the status icons
and transcript utilities, leading that row, so the tray spends as few rows as
possible above the composer. UI owns this presentation-only
contract; Tasks retains workflow movement, eligibility, and preview semantics.

## Requirements

### REQ-UI-COMPOSER-ACTION-WRAP-001: Right-aligned composer workflow action

**Intent:** A phone user can find and activate the next step in the composer
toolbar without it taking a row of its own.

#### Acceptance criteria

- **AC-UI-COMPOSER-ACTION-WRAP-001.1:** At or above 768 CSS pixels, when transcript controls and the next-step action fit on one line, the action shall remain after those controls at the right edge of the toolbar action group. When they wrap at those widths, the action shall retain right alignment.
- **AC-UI-COMPOSER-ACTION-WRAP-001.2:** Below 768 CSS pixels, the visible next-step action shall lead the shared status and utility row, with utilities aligned to the trailing edge, whenever their complete targets fit on one line. When they do not fit, controls shall wrap without overlapping or clipping the action.
- **AC-UI-COMPOSER-ACTION-WRAP-001.3:** At phone widths of 360 and 393 CSS pixels, including with a longer workflow-step label that fits within the toolbar, the action shall remain fully inside the toolbar and viewport, retain an active target of at least 44 CSS pixels in each dimension, and cause no document horizontal overflow. The same phone placement shall apply with a fine pointer.
- **AC-UI-COMPOSER-ACTION-WRAP-001.4:** Activating the next-step action shall perform the existing workflow move. Busy and clarification gates, movement disabling, preview and options behavior, accessible naming, and the relative order of utility controls shall retain their current behavior. Keyboard traversal shall follow the rendered action/control order. A hidden action shall reserve no row, and a disabled moving action shall retain its position.
- **AC-UI-COMPOSER-ACTION-WRAP-001.5:** On desktop with enough width, status items, transcript controls, and the next-step action shall retain their existing single-line order and density.
- **AC-UI-COMPOSER-ACTION-WRAP-001.6:** Below 768 CSS pixels, status icons,
  including todo and CI indicators, and transcript utilities shall share one row
  when their complete targets and existing gaps fit within the composer tray.
  Status icons shall lead and utilities shall align to the trailing edge.
  A visible workflow action shall precede the status icons in that row.
  A hidden workflow action shall leave no empty row.
- **AC-UI-COMPOSER-ACTION-WRAP-001.7:** When status and utility controls cannot
  fit on one row, they shall wrap without clipped or overlapping targets.
  Phone targets shall remain at least 44 CSS pixels in both dimensions.
  Empty groups shall reserve no space. Terminal notices shall retain their
  full-width region above actions. Changing width or control visibility shall
  preserve actions, disclosure state, utility order, and saved preferences.

## Out of scope

Toolbar overflow menus, hiding icons, composer input layout, Threads auto-hide
CI placement, passthrough composition, new copy, and workflow routing or
persistence changes. Oversized labels wider
than the entire toolbar are outside this focused regression.

## Placement revision

The 2026-10-14 user correction replaces the previous crowded-phone composition
(utilities followed by a wrapped workflow action) with a workflow row above
the utilities. Desktop composition is retained. The follow-up work order
implements and verifies the revised row order; earlier screenshots and Task 01
results describe the previous composition.

The 2026-10-20 revision extends that placement with a shared status/utility row.

The 2026-11-11 user correction (kdlbs/kandev#5270) replaces the dedicated phone
workflow row: the action now leads the shared status/utility row so the tray,
together with the touch selection Comment action, fits on one row.
The workflow action retains its separate upper row. Acceptance criteria .6
and .7 are implemented and verified through Task 03.

## References

- [Design](../system-design/composer-action-wrapping.md)
- [Control sizing](control-sizing.md)
- [Workflow move preview](../../tasks/requirements/workflow-move-preview.md)
- [Implementation plan](../../../plans/mobile-composer-action-wrapping/plan.md)
