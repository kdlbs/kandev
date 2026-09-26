---
status: active
system: ui
created: 2026-09-25
owners:
  - kandev
---

# First-run dialog availability requirements

## Overview

The first-run tour introduces desktop workflows. A phone visit must not show the tour or consume the opportunity to see it later on a larger screen. UI owns this responsive presentation rule; the executor system owns the facts shown within its step.

## Requirements

### REQ-UI-FIRST-RUN-DIALOG-001: Keep the first-run tour off phones

**Intent:** Let a new user reach the phone experience directly while retaining the first-run tour for a later desktop or tablet visit.

#### Acceptance criteria

- **AC-UI-FIRST-RUN-DIALOG-001.1:** At the canonical phone breakpoint, below 768 CSS pixels, the first-run dialog shall not appear, regardless of its completion marker. The phone shall show the normal page instead.
- **AC-UI-FIRST-RUN-DIALOG-001.2:** A phone visit shall not set the first-run completion marker or save agent-profile changes merely because the tour is suppressed.
- **AC-UI-FIRST-RUN-DIALOG-001.3:** At 768 CSS pixels or wider, an unfinished user shall see the existing first-run dialog. A completed user shall not see it.
- **AC-UI-FIRST-RUN-DIALOG-001.4:** Resizing from a larger viewport to a phone shall close the dialog without completing it. Resizing back shall reopen it if the completion marker is absent. The dialog shall not flash before the phone breakpoint is known.
- **AC-UI-FIRST-RUN-DIALOG-001.5:** On larger viewports, Back, Next, Skip, Get Started, and dirty agent-profile saves shall retain their existing behavior. Skip and completion shall continue to write the current browser-local marker.

## Related requirements

The [executor discovery requirement](../../executors/requirements/first-run-discovery.md) defines content inside the tour. Future contextual feature guides may use the same phone exclusion, but their once-per-user persistence is a separate contract.

## Out of scope

- A new cross-install or cross-browser completion service for the existing tour.
- Changes to the content of non-executor tour steps or to other dialogs.
