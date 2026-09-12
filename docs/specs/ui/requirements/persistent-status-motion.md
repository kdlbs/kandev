---
status: active
system: ui
created: 2026-08-23
updated: 2026-09-10
owners:
  - kandev
---

# Persistent Status Motion Requirements

## Overview

Kandev uses rotating icons to show that tasks, sessions, runs, and agents are active.
These indicators can stay visible for a long time. Their motion must remain
clear without causing continuous main-thread rendering work.

## Terminology

- **Persistent status indicator:** A status icon whose lifetime follows a
  task, session, agent, or run state instead of a short UI request.
- **Compositor-prepared motion:** A transform-only animation on an HTML element
  that the browser can move to its compositor.
- **Persistent activity animation:** Motion whose lifetime follows ongoing
  task, session, agent, or run activity instead of a bounded UI request.

## Requirements

### REQ-UI-PERSISTENT-STATUS-MOTION-001: Efficient persistent rotation

**Intent:** Keep visible working-state motion while avoiding unnecessary idle
CPU use.

**User story:** As a Kandev user, I want active status icons to keep rotating,
so that I can identify ongoing work without high idle CPU use.

#### Acceptance criteria

- **AC-UI-PERSISTENT-STATUS-MOTION-001.1:** When a task, session, agent, or run
  has a long-lived active status, the matching indicator shall remain visibly
  rotating on desktop and mobile.
- **AC-UI-PERSISTENT-STATUS-MOTION-001.2:** When the active status ends, the
  indicator shall stop rotating and the existing next-state icon shall appear.
- **AC-UI-PERSISTENT-STATUS-MOTION-001.3:** While an indicator rotates, the UI
  shall animate an HTML transform target and shall not animate the SVG element.
- **AC-UI-PERSISTENT-STATUS-MOTION-001.4:** The change shall preserve each
  indicator's size, color, speed, status precedence, accessible meaning, and
  test selector.
- **AC-UI-PERSISTENT-STATUS-MOTION-001.5:** The desktop and mobile surfaces
  shall use the same status state and motion primitive without horizontal
  overflow or a new interaction.

### REQ-UI-PERSISTENT-STATUS-MOTION-002: Efficient grid activity motion

**Intent:** Keep the recognizable grid activity animation while removing its
recurring main-thread rendering cost.

**User story:** As a Kandev user, I want the grid indicator to keep moving while
work is active, so that progress remains recognizable without unnecessary CPU
use.

#### Acceptance criteria

- **AC-UI-PERSISTENT-STATUS-MOTION-002.1:** When a long-lived active state uses
  the grid activity indicator, all nine cells shall remain visibly animated in
  their existing staggered pattern on desktop and mobile.
- **AC-UI-PERSISTENT-STATUS-MOTION-002.2:** When the active state ends or the
  indicator unmounts, its animation shall stop and the existing next state shall
  appear.
- **AC-UI-PERSISTENT-STATUS-MOTION-002.3:** In Chromium with Web Animations API
  support, a settled grid indicator shall cause no recurring main-thread
  `UpdateLayoutTree` or `Layerize` work attributable to its animation targets.
- **AC-UI-PERSISTENT-STATUS-MOTION-002.4:** The optimized indicator shall
  preserve its nine-cell geometry, spacing, color, scale range, duration,
  stagger, accessible status label, and existing selectors.
- **AC-UI-PERSISTENT-STATUS-MOTION-002.5:** When Web Animations API support is
  unavailable, the grid indicator shall retain its current animated CSS
  fallback.

### REQ-UI-PERSISTENT-STATUS-MOTION-003: Efficient persistent opacity motion

**Intent:** Keep long-lived glow and pulse feedback visible without sampling
their opacity animations on the main thread.

**User story:** As a Kandev user, I want busy composers and live status dots to
keep pulsing, so that active work remains obvious without high steady CPU use.

#### Acceptance criteria

- **AC-UI-PERSISTENT-STATUS-MOTION-003.1:** While their owning active states
  remain true, the task composer glow and persistent status dots shall remain
  visibly animated on desktop and mobile.
- **AC-UI-PERSISTENT-STATUS-MOTION-003.2:** When an owning active state ends or
  its surface unmounts, the matching opacity animation shall stop and the
  existing settled presentation shall appear.
- **AC-UI-PERSISTENT-STATUS-MOTION-003.3:** In Chromium with Web Animations API
  support, a settled persistent opacity target shall cause no recurring
  main-thread `UpdateLayoutTree` or `Layerize` work attributable to that target.
- **AC-UI-PERSISTENT-STATUS-MOTION-003.4:** The optimized motion shall preserve
  the current opacity range, duration, easing, color, glow, geometry, status
  precedence, accessible meaning, and test selectors of each migrated surface.
- **AC-UI-PERSISTENT-STATUS-MOTION-003.5:** Desktop and mobile shall use the
  same active-state source and opacity-motion primitive without adding a touch
  interaction, changing scroll ownership, or causing horizontal overflow.
- **AC-UI-PERSISTENT-STATUS-MOTION-003.6:** When Web Animations API support is
  unavailable, each migrated surface shall retain its current animated CSS
  fallback and reduced-motion behavior.

## Proposed follow-up requirements

The following additions are draft for the
[rendering CPU follow-up](../../../plans/frontend-rendering-cpu/plan.md).
Requirements 001 through 003 remain the accepted contract. Their visible-motion
criteria apply while the target is visible; the proposed lifecycle below
governs hidden targets.

### REQ-UI-PERSISTENT-STATUS-MOTION-004: Hidden motion lifecycle

**Intent:** Avoid spending rendering resources on status motion the user cannot see.

#### Acceptance criteria

- **AC-UI-PERSISTENT-STATUS-MOTION-004.1:** When the document is hidden, its
  host-owned persistent rotation, grid, and pulse animations shall pause.
- **AC-UI-PERSISTENT-STATUS-MOTION-004.2:** When a status target is outside its
  scroll viewport or in an inactive hidden panel, its motion shall pause.
- **AC-UI-PERSISTENT-STATUS-MOTION-004.3:** When the target becomes visible,
  motion shall resume only if its current status still requires motion. A status
  that settled while hidden shall appear settled without replaying old motion.
- **AC-UI-PERSISTENT-STATUS-MOTION-004.4:** Pausing motion shall preserve task
  updates, transcript content, draft input, focus, scroll position, accessible
  status, and existing reduced-motion behavior on desktop and mobile.
- **AC-UI-PERSISTENT-STATUS-MOTION-004.5:** When visibility observation is
  unavailable, visible status feedback shall remain usable through the existing
  fallback. Repeated visibility changes shall not accumulate live animations.

### REQ-UI-PERSISTENT-STATUS-MOTION-005: Bounded context-ring motion

**Intent:** Animate changes in context usage without animating unrelated styles.

#### Acceptance criteria

- **AC-UI-PERSISTENT-STATUS-MOTION-005.1:** When context usage changes, the ring
  shall reach the new value with its existing 300 ms easing and geometry.
- **AC-UI-PERSISTENT-STATUS-MOTION-005.2:** The ring shall animate only its
  usage arc. Color and inherited scrollbar styles shall not start transitions.
- **AC-UI-PERSISTENT-STATUS-MOTION-005.3:** Desktop and phone shall retain the
  same usage value, threshold colors, accessible disclosure, and touch behavior.

## Implementation plans

- [Idle CPU (completed)](../../../plans/frontend-idle-cpu/plan.md)
- [Animation CPU (completed)](../../../plans/frontend-animation-cpu/plan.md)
- [Rendering CPU follow-up (draft)](../../../plans/frontend-rendering-cpu/plan.md)

## Out of scope

- Removing visible status motion.
- Changing task, session, or run state rules.
- Changing reduced-motion behavior.
- Migrating one-shot entrance, selection, search, or confirmation cues.
- Changing plugin-owned animations, including Kandy celebration effects.
