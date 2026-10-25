---
status: active
system: ui
created: 2026-09-04
owners:
  - kandev
---

# Message Queue Automation Controls Requirements

## Overview

The expanded queue panel must present queue progress and both session automation
policies without spending a second row on one control. Auto-run retains its
backend-owned behavior while Auto-merge gains a session-scoped control.

This requirement supersedes `REQ-UI-MESSAGE-QUEUE-RUN-001` when activated. It
preserves that requirement's queue execution behavior while replacing its
header composition and visible-help contract.

## Requirements

### REQ-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001: Message Queue Automation Controls

**Intent:** Keep queue automation visible, compact, accessible, and equivalent
on desktop and mobile.

#### Acceptance criteria

- **AC-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001.1:** When the queue panel is
  expanded, one responsive header composition shall contain the queue summary,
  labeled Auto-run and Auto-merge switch pills, and the existing queue actions.
  Layouts with enough width shall keep the groups on the first visual line;
  narrower layouts may wrap groups but shall not render a dedicated second
  Auto-run row.
- **AC-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001.2:** The Auto-run switch shall
  expose its checked state, disabled state, keyboard focus, and localized ON or
  OFF behavior description to assistive technology. Its state-specific
  description need not remain visually present after the compact header
  replaces the prior helper row.
- **AC-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001.3:** Auto-run ON shall allow the
  backend to start the FIFO head whenever the session is eligible and continue
  with one distinct turn per entry after each preceding turn completes.
- **AC-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001.4:** Auto-run OFF shall prevent
  every later automatic FIFO take without cancelling, truncating, or altering
  an active turn or a queued handoff accepted before OFF won the race.
- **AC-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001.5:** Turning Auto-run ON while
  the session is promptable shall attempt the FIFO head immediately. When the
  session is busy or another lifecycle guard applies, ON shall persist and
  delivery shall resume at the next eligible trigger.
- **AC-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001.6:** Auto-run shall remain
  backend-owned per session, default to ON, survive an empty queue, navigation,
  reload, and backend restart, and remain OFF until an explicit resume changes
  it.
- **AC-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001.7:** Every visible queue row
  shall retain Send Now. A successful Send Now shall enable Auto-run, dispatch
  the selected entry first, and preserve the relative FIFO order of the
  remainder. The first-party header shall not add a bulk Send Now action.
- **AC-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001.8:** A switch shall be disabled
  while its mutation or another conflicting queue or cancellation operation is
  pending. A failed mutation shall restore authoritative state and show a
  localized error.
- **AC-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001.9:** Desktop and mobile shall
  expose the same controls and outcomes. Header groups shall wrap without
  document horizontal overflow; coarse-pointer effective targets shall be at
  least 44 CSS pixels in each dimension.
- **AC-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001.10:** Reorder, edit, manual
  merge, Remove, Clear all, pin, collapse, and the queue panel's single internal
  scroll owner shall retain their existing behavior.

## Exclusions

- No mobile-only drawer or alternate queue route.
- No aggregate first-party Send Now control.
- No inherited-versus-explicit badge or reset-to-global control.

## Related requirements

- [Control Pending Message Auto-run](message-queue-run.md)
- [Per-session Automatic Message Merge Overrides](message-queue-auto-merge-session-overrides.md)
- [Send Queued Messages Now](message-queue-send-now.md)
