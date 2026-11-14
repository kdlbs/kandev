---
status: active
system: ui
created: 2026-11-11
owners:
  - kandev
---

# Touch Selection Actions Requirements

## Overview

Several Kandev surfaces let users select read-only or editable text and attach
a comment to the selection: settled agent replies in Task Chat and Quick Chat,
Office thread comments, the Markdown file preview, and the code file editors.
On desktop these surfaces show a small comment button next to the selection.

On Android and iOS the operating system places its own Copy, Select all, Share,
and related selection menu around the same selection rectangle, outside
Kandev's DOM stacking order. On a phone the native menu appears over Kandev's
selection-anchored comment button almost immediately and blocks it. The Plan
editor already avoids this conflict by docking its actions at the bottom edge
of the visible surface (see
[`REQ-UI-RESPONSIVE-PLAN-FORMATTING-001`](responsive-plan-formatting.md)).
This requirement makes that docked pattern the shared contract for every
touch text-selection comment action.

The UI system owns this presentation contract. Each feature keeps ownership of
its comment data, composer, and delivery behavior.

## Terminology

- **Selection action surface:** A Kandev surface where selecting text offers a
  comment action. In scope: agent-message comments, Office thread comment
  extracts, Markdown preview comments, and code file editor comments.
- **Floating selection trigger:** A Kandev control positioned next to the
  current text selection.
- **Selection action dock:** Compact Kandev icon actions positioned at the
  bottom-right of the hosting surface's visible bottom edge, or immediately
  above the software keyboard when one is open.
- **Touch presentation:** The presentation used when the primary pointer is
  coarse, the same condition that already selects the touch comment Drawer.

## Requirements

### REQ-UI-TOUCH-SELECTION-ACTIONS-001: Docked touch selection actions

**Intent:** Keep text-selection comment actions reachable with a thumb and never
placed where native selection chrome can cover them.

**User story:** As a phone or tablet user, I want the Comment action for my
text selection to appear in a toolbar at the bottom of the screen, so that the
device's selection menu cannot cover it and I can reach it with my thumb.

#### Acceptance criteria

- **AC-UI-TOUCH-SELECTION-ACTIONS-001.1:** When a fine-pointer user selects
  eligible text on a selection action surface, Kandev shall keep that surface's
  existing floating selection trigger, shortcut, and composer behavior.
- **AC-UI-TOUCH-SELECTION-ACTIONS-001.2:** When a touch-presentation user
  selects eligible non-whitespace text on a selection action surface, Kandev
  shall present the Comment action in a selection action dock and shall not
  render a floating selection trigger next to the selection.
- **AC-UI-TOUCH-SELECTION-ACTIONS-001.3:** While the software keyboard is
  closed, the dock shall sit at the visible bottom edge of the hosting surface,
  above mobile task navigation, the chat composer, and the device safe area.
  When the hosting surface cannot be measured, the dock shall sit at the bottom
  of the visual viewport above the safe area.
- **AC-UI-TOUCH-SELECTION-ACTIONS-001.4:** When a software keyboard resizes or
  scrolls the visual viewport, the dock shall stay fully visible immediately
  above the keyboard and follow later viewport changes without the user
  recreating the selection.
- **AC-UI-TOUCH-SELECTION-ACTIONS-001.5:** The dock shall follow the live
  selection. Adjusting native selection handles updates the target that the
  Comment action uses. Scrolling the surface does not dismiss the dock while
  the selection remains inside the surface. Collapsing the selection or moving
  it outside the surface hides the dock.
- **AC-UI-TOUCH-SELECTION-ACTIONS-001.6:** Activating the docked Comment action
  shall use the selection that was current when it was tapped and open the
  surface's existing touch composer. Agent-message and Office thread comment
  surfaces keep their bottom Drawer and Add/Run hierarchy. A dock inside a
  modal surface such as Quick Chat shall remain interactive.
- **AC-UI-TOUCH-SELECTION-ACTIONS-001.7:** Kandev shall leave the device's
  native selection actions available while the dock is shown.
- **AC-UI-TOUCH-SELECTION-ACTIONS-001.8:** The dock shall be a labelled
  toolbar of icon-only actions aligned to the trailing edge, like the Plan
  dock's Comment action: a 32 CSS pixel square visual surface inside a touch
  target of at least 44 CSS pixels, with a localized accessible name. It shall
  not occupy or block the rest of its row, shall be no taller than 48 CSS
  pixels, and shall not cause document-level horizontal overflow.
- **AC-UI-TOUCH-SELECTION-ACTIONS-001.9:** Selections that a surface does not
  accept today (streaming replies, read-only Office tasks, disabled editor
  comments, whitespace-only text) shall not show the dock.
- **AC-UI-TOUCH-SELECTION-ACTIONS-001.10:** When the surface already has a
  persistent toolbar row near the bottom of the screen (the Task Chat and Quick
  Chat composer status row), the Comment action shall appear inline at the
  trailing end of that row's utility icons instead of in a separate dock, so
  the selection adds no row. Surfaces without such a row use the dock.

## Out of scope

- Suppressing, replacing, or automating the native selection menu.
- Changes to comment storage, anchoring, delivery, or backend behavior.
- Redesigning desktop floating triggers or popovers.
- Image-region comments in clarification review and Browser preview feedback;
  neither uses a selection-anchored text trigger.
- The Plan editor dock, which already satisfies this pattern under
  `REQ-UI-RESPONSIVE-PLAN-FORMATTING-001`; it shares geometry code only.
