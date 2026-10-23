---
status: active
system: ui
created: 2026-10-22
owners:
  - kandev
---

# Button State Colors

## Overview

UI owns the reusable presentation of action availability. Feature owners retain
validation, permission, pending-operation, and activation rules.

## Requirements

### REQ-UI-BUTTON-STATE-COLORS-001: Readable action states

**Intent:** Make unavailable actions distinct from available primary actions
without making their label, icon, or shortcut difficult to read.

#### Acceptance criteria

- **AC-UI-BUTTON-STATE-COLORS-001.1:** An unavailable shared primary action
  shall use a neutral surface and readable foreground instead of a faded
  primary-colored surface. Its label and icon shall remain visible in light
  and dark themes. Disabled label contrast shall be at least 3:1 against its
  effective background; this is a product target, not a claim about accessibility
  conformance requirements for inactive controls.
- **AC-UI-BUTTON-STATE-COLORS-001.2:** An available primary action shall retain
  its primary or feature-specific color. Its inline shortcut and decorative
  icon shall inherit its foreground rather than use a page-level muted color.
- **AC-UI-BUTTON-STATE-COLORS-001.3:** A pending action shall retain its existing
  label or progress indicator, with a foreground consistent with the button's
  current availability. Styling shall not change whether the action is enabled.
- **AC-UI-BUTTON-STATE-COLORS-001.4:** A noninteractive control that communicates
  granted notification permission or a completed settings Save shall retain its distinct success state and
  readable label. This explicit status exception shall not apply to unavailable
  approval, merge, pending/invalid Save, Send, or task-start actions.
- **AC-UI-BUTTON-STATE-COLORS-001.5:** The sidebar Canvas setup action shall use
  neutral navigation styling, including its resting and hover foregrounds.
  The settings Canvas creation action shall retain primary-action styling.
- **AC-UI-BUTTON-STATE-COLORS-001.6:** These states shall follow the current theme
  on desktop and phone. Existing button dimensions, touch targets, focus,
  accessible names, activation guards, and dirty-draft behavior shall remain.

## Out of scope

Global palette changes, other Button variants, raw merge/walkthrough buttons,
action validation changes, new interactions, and editor behavior.

## Related documents

- [Design](../system-design/button-state-colors.md)
- [Control sizing](control-sizing.md)
- [Repair package](../../../plans/button-state-colors/plan.md)
