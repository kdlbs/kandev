---
status: active
system: ui
created: 2026-08-12
owners:
  - kandev
---
# Pin the Message Queue Panel Requirements

## Overview

The expanded message queue panel collapses whenever the user navigates away from the task and back, because its open state is component-local. Users monitoring a long-running agent must click the queue chip again after every navigation, even when they deliberately left the queue open.

## Requirements

### REQ-UI-MESSAGE-QUEUE-PIN-001: Pin the Message Queue Panel

**Intent:** The expanded message queue panel collapses whenever the user navigates away from the task and back, because its open state is component-local. Users monitoring a long-running agent must click the queue chip again after every navigation, even when they deliberately left the queue open.

#### Acceptance criteria

- **AC-UI-MESSAGE-QUEUE-PIN-001.1:** The expanded queue panel header gains a pin toggle placed between **Clear all** and the collapse (**X**) button.
- **AC-UI-MESSAGE-QUEUE-PIN-001.2:** The pin is a per-session on/off preference, persisted in `localStorage` so it survives navigation within the SPA and full page reloads.
- **AC-UI-MESSAGE-QUEUE-PIN-001.3:** When pinned, the queue panel opens automatically on mount whenever the session has queued entries; the user does not need to click the chip.
- **AC-UI-MESSAGE-QUEUE-PIN-001.4:** When unpinned, behavior is unchanged: the panel starts collapsed and opens only when the user clicks the chip.
- **AC-UI-MESSAGE-QUEUE-PIN-001.5:** Collapsing via the **X** button closes the panel for the current view but keeps the pin state; a later mount (navigation away and back, or reload) reopens the panel while the session still has queued entries. Unpinning is the way to stop the panel from reopening.
- **AC-UI-MESSAGE-QUEUE-PIN-001.6:** The pin button exposes its state through `aria-pressed` and a localized label ("Pin message queue" / "Unpin message queue") used as its accessible name and tooltip.
- **AC-UI-MESSAGE-QUEUE-PIN-001.7:** On coarse-pointer viewports the pin button keeps the existing queue header touch sizing (at least 44 by 44 CSS pixels), matching Clear all and X.
- **AC-UI-MESSAGE-QUEUE-PIN-001.8:** The pin is **desktop-only**: on phone viewports (the app's `mobile` breakpoint, width below 768px) the queue panel renders without the pin; Auto-run, Clear all, every row's Send Now, and the collapse button remain available. On phone the queue panel keeps the existing mount behavior (starts collapsed).

## Migrated source detail

## Why

The expanded message queue panel collapses whenever the user navigates away
from the task and back, because its open state is component-local. Users
monitoring a long-running agent must click the queue chip again after every
navigation, even when they deliberately left the queue open.

## What

- The expanded queue panel header gains a pin toggle placed between **Clear
  all** and the collapse (**X**) button.
- The pin is a per-session on/off preference, persisted in `localStorage` so
  it survives navigation within the SPA and full page reloads.
- When pinned, the queue panel opens automatically on mount whenever the
  session has queued entries; the user does not need to click the chip.
- When unpinned, behavior is unchanged: the panel starts collapsed and opens
  only when the user clicks the chip.
- Collapsing via the **X** button closes the panel for the current view but
  keeps the pin state; a later mount (navigation away and back, or reload)
  reopens the panel while the session still has queued entries. Unpinning is
  the way to stop the panel from reopening.
- The pin button exposes its state through `aria-pressed` and a localized
  label ("Pin message queue" / "Unpin message queue") used as its accessible
  name and tooltip.
- On coarse-pointer viewports the pin button keeps the existing queue header
  touch sizing (at least 44 by 44 CSS pixels), matching Clear all and X.
- The pin is **desktop-only**: on phone viewports (the app's `mobile`
  breakpoint, width below 768px) the queue panel renders without the pin;
  Auto-run, Clear all, every row's Send Now, and the collapse button remain
  available.
  On phone the queue panel keeps the existing mount behavior (starts
  collapsed).

## Data model

No backend change. One `localStorage` key per session:

```
kandev:queue:pinned:<session_id>:v1   boolean, "true" only counts as on
```

Absent or unreadable entries default to `false` (unpinned).

## Failure modes

- **`localStorage` unavailable (private mode / quota):** the pin defaults to
  unpinned; reading and writing degrade silently and the UI stays functional.
- **Entry count drops to zero:** the panel closes as today; there is nothing
  to display, regardless of pin state. The pin itself is retained.
- **Session switch while mounted:** the panel follows the target session's pin
  state (open when pinned with entries, closed otherwise).

## Scenarios

- **GIVEN** a session queue with at least one entry and the panel expanded,
  **WHEN** the user clicks the pin between Clear all and X, **THEN** the
  button shows the pinned state (`aria-pressed=true`) and the preference is
  persisted for that session.
- **GIVEN** the panel is pinned with queued entries, **WHEN** the user
  navigates away from the task and back (or reloads the page), **THEN** the
  queue panel is already expanded and the chip is not shown.
- **GIVEN** the panel is unpinned with queued entries, **WHEN** the user
  navigates away and back, **THEN** the panel starts collapsed with the chip
  visible, matching current behavior.
- **GIVEN** the panel is pinned and expanded, **WHEN** the user clicks **X**,
  **THEN** the panel collapses for the current view, and navigating away and
  back reopens it while the session still has queued entries.
- **GIVEN** the panel is pinned, **WHEN** the user clicks the pin again,
  **THEN** the button returns to the unpinned state and the panel no longer
  reopens on later mounts.
- **GIVEN** the panel is pinned and the queue drains to zero entries, **WHEN**
  the queue refetches, **THEN** the panel closes and the chip is hidden;
  pinning a later queue for the same session still reopens the panel when a
  new entry arrives, and on the next mount (navigation away and back, or
  reload).
- **GIVEN** the panel is pinned but the session's queue is empty, **WHEN**
  entries arrive asynchronously (initial load after a session switch), **THEN**
  the expanded panel appears without a chip click.
- **GIVEN** the panel is pinned and the queue entries load asynchronously
  after the affordance mounts (e.g. returning to the task route), **WHEN** the
  entries arrive, **THEN** the expanded panel appears without a chip click.
- **GIVEN** the user pins the queue in one task session, **WHEN** they open
  another task's queue, **THEN** that queue starts collapsed unless it has its
  own pin.
- **GIVEN** a phone viewport with the queue panel expanded, **WHEN** the user
  inspects the header, **THEN** no pin control is rendered and Clear all and
  the collapse button remain touch-sized.
- **GIVEN** the session was pinned at desktop width, **WHEN** the same
  session's queue is viewed on a phone viewport, **THEN** the stored pin does
  not auto-open the panel (it starts collapsed with the chip, and there is no
  pin control to unpin); returning to a desktop viewport re-applies the
  stored pin.

## Out of scope

- Backend persistence or synchronization of the pin across devices/installs.
- Rendering the pin on phone viewports (desktop-only control).
- Pinning the queue chip or collapsing behavior beyond the expanded panel.
- Changing Auto-run, Clear all, Send Now, or merge/reorder controls.
- Any change to queued-message admission, delivery, or capacity rules.
