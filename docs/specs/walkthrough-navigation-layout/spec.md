---
status: shipped
created: 2026-07-28
owner: product
---

# Stable Walkthrough Navigation

## Why

People progress through a code walkthrough one step at a time. When one step
has more explanation than another, the primary **Next** control moves, forcing
them to find it again instead of keeping a reliable interaction target.

## What

- The walkthrough card keeps **Previous** and **Next** in a fixed footer
  position while it remains open.
- Step titles, file metadata, and explanation text may vary in length without
  moving that footer between steps.
- The explanation region scrolls within the card when it needs more space; the
  footer and feedback controls remain available.
- Desktop preserves the draggable floating-card experience. On phones, the
  existing bottom-sheet walkthrough keeps the same capability with a
  thumb-reachable navigation footer, safe-area clearance, and no page-level
  horizontal overflow.

## Scenarios

- **GIVEN** an open desktop walkthrough whose current and following steps have
  different explanation lengths, **WHEN** the user selects **Next**, **THEN**
  the navigation footer retains its vertical position within the card and the
  next step content is shown.
- **GIVEN** a walkthrough explanation taller than its available content area,
  **WHEN** the user scrolls it, **THEN** the explanation scrolls without
  obscuring or moving **Previous** and **Next**.
- **GIVEN** a phone viewport with an open walkthrough bottom sheet, **WHEN**
  the user advances through differently sized steps, **THEN** the navigation
  footer remains visible, reachable, and within the viewport.
- **GIVEN** a walkthrough sheet covers its editor anchor, **WHEN** the active
  step changes, **THEN** the editor range indicator remains rendered while its
  occluded connector is suppressed.

## Out of scope

- Changing walkthrough step content, ordering, or persistence.
- Adding keyboard shortcuts, swipe navigation, or a progress scrubber.
- Changing the floating-card drag behavior or its anchoring connector.
