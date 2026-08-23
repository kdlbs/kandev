---
status: active
system: ui
created: 2026-08-23
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

## Out of scope

- Removing status motion.
- Changing task, session, or run state rules.
- Changing reduced-motion behavior.
- Migrating short request, save, refresh, upload, or download spinners that do
  not remain mounted as domain status.
