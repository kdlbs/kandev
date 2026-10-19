---
status: active
system: ui
created: 2026-10-09
owners:
  - Kandev
---

# Composer Action Wrapping Requirements

## Overview

Keep the next workflow step above the transcript utility icons in the phone
chat composer tray, aligned to its right edge. UI owns this presentation-only
contract; Tasks retains workflow movement, eligibility, and preview semantics.

## Requirements

### REQ-UI-COMPOSER-ACTION-WRAP-001: Right-aligned composer workflow action

**Intent:** A phone user can find and activate the next step at the right edge
of the composer toolbar, above its utility icons.

#### Acceptance criteria

- **AC-UI-COMPOSER-ACTION-WRAP-001.1:** At or above 768 CSS pixels, when transcript controls and the next-step action fit on one line, the action shall remain after those controls at the right edge of the toolbar action group. When they wrap at those widths, the action shall retain right alignment.
- **AC-UI-COMPOSER-ACTION-WRAP-001.2:** Below 768 CSS pixels, the visible next-step action shall occupy its own right-aligned row above every visible transcript utility control, including when the controls and action could fit on one line. Utility controls shall remain between that action and the composer input.
- **AC-UI-COMPOSER-ACTION-WRAP-001.3:** At phone widths of 360 and 393 CSS pixels, including with a longer workflow-step label that fits within the toolbar, the action shall remain fully inside the toolbar and viewport, retain an active target of at least 44 CSS pixels in each dimension, and cause no document horizontal overflow. The same phone placement shall apply with a fine pointer.
- **AC-UI-COMPOSER-ACTION-WRAP-001.4:** Activating the next-step action shall perform the existing workflow move. Busy and clarification gates, movement disabling, preview and options behavior, accessible naming, and the relative order of utility controls shall retain their current behavior. Keyboard traversal shall follow the rendered action/control order. A hidden action shall reserve no row, and a disabled moving action shall retain its position.
- **AC-UI-COMPOSER-ACTION-WRAP-001.5:** On desktop with enough width, status items, transcript controls, and the next-step action shall retain their existing single-line order and density.

## Out of scope

Toolbar overflow menus, hiding or regrouping icons, composer input layout,
new copy, and workflow routing or persistence changes. Oversized labels wider
than the entire toolbar are outside this focused regression.

## Placement revision

The 2026-10-14 user correction replaces the previous crowded-phone composition
(utilities followed by a wrapped workflow action) with a workflow row above
the utilities. Desktop composition is retained. The follow-up work order
implements and verifies the revised row order; earlier screenshots and Task 01
results describe the previous composition.

## References

- [Design](../system-design/composer-action-wrapping.md)
- [Control sizing](control-sizing.md)
- [Workflow move preview](../../tasks/requirements/workflow-move-preview.md)
- [Implementation plan](../../../plans/mobile-composer-action-wrapping/plan.md)
