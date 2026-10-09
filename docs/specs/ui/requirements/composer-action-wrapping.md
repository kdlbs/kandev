---
status: active
system: ui
created: 2026-10-09
owners:
  - Kandev
---

# Composer Action Wrapping Requirements

## Overview

Keep the next workflow step in a predictable position above the chat composer
when transcript controls fill the phone toolbar. UI owns this presentation-only
contract; Tasks retains workflow movement, eligibility, and preview semantics.

## Requirements

### REQ-UI-COMPOSER-ACTION-WRAP-001: Right-aligned wrapped workflow action

**Intent:** A phone user can find and activate the next step at the right edge
of the composer toolbar, including when it occupies another line.

#### Acceptance criteria

- **AC-UI-COMPOSER-ACTION-WRAP-001.1:** When transcript controls and the next-step action fit on one line, the action shall remain after those controls at the right edge of the toolbar action group.
- **AC-UI-COMPOSER-ACTION-WRAP-001.2:** When a phone toolbar's transcript controls occupy the first action line and the next-step action wraps onto the following line, the next-step action shall align with the toolbar's right content edge rather than start at the left edge.
- **AC-UI-COMPOSER-ACTION-WRAP-001.3:** At phone widths of 360 and 393 CSS pixels, including with a longer workflow-step label that fits within the toolbar, the wrapped action shall remain fully inside the toolbar and viewport, retain an active target of at least 44 CSS pixels in each dimension, and cause no document horizontal overflow. The same phone placement shall apply with a fine pointer.
- **AC-UI-COMPOSER-ACTION-WRAP-001.4:** After wrapping, activating the next-step action shall perform the existing workflow move. Busy and clarification gates, movement disabling, preview and options behavior, accessible naming, and control order shall retain their current behavior.
- **AC-UI-COMPOSER-ACTION-WRAP-001.5:** On desktop with enough width, status items, transcript controls, and the next-step action shall retain their existing single-line order and density.

## Out of scope

Toolbar overflow menus, hiding or regrouping icons, composer input layout,
new copy, and workflow routing or persistence changes. Oversized labels wider
than the entire toolbar are outside this focused regression.

## References

- [Design](../system-design/composer-action-wrapping.md)
- [Control sizing](control-sizing.md)
- [Workflow move preview](../../tasks/requirements/workflow-move-preview.md)
- [Implementation plan](../../../plans/mobile-composer-action-wrapping/plan.md)
