---
status: active
system: ui
created: 2026-07-30
updated: 2026-08-11
owners:
  - kandev
---
# WebSocket Connectivity Warning Requirements

## Overview

Kandev can look current while its live WebSocket updates are delayed or unavailable. Users need a quiet but unavoidable indication that task, session, and workspace state may be stale, without showing permanent healthy-state chrome or flashing for harmless reconnects.

## Requirements

### REQ-UI-WS-CONNECTIVITY-WARNING-001: WebSocket Connectivity Warning

**Intent:** Kandev can look current while its live WebSocket updates are delayed or unavailable. Users need a quiet but unavoidable indication that task, session, and workspace state may be stale, without showing permanent healthy-state chrome or flashing for harmless reconnects.

#### Acceptance criteria

- **AC-UI-WS-CONNECTIVITY-WARNING-001.1:** Kandev derives one transient connectivity severity from the continuous time since its canonical application WebSocket last entered `connected`:
- **AC-UI-WS-CONNECTIVITY-WARNING-001.2:** **Healthy/grace:** no warning before 3 seconds offline.
- **AC-UI-WS-CONNECTIVITY-WARNING-001.3:** **Unstable:** yellow from 3 seconds offline until 10 seconds offline.
- **AC-UI-WS-CONNECTIVITY-WARNING-001.4:** **Lost:** red from 10 seconds offline until the connection recovers.
- **AC-UI-WS-CONNECTIVITY-WARNING-001.5:** Transitions among `connecting`, `reconnecting`, `disconnected`, and `error` belong to the same continuous offline interval. They do not restart either threshold.
- **AC-UI-WS-CONNECTIVITY-WARNING-001.6:** Any `connected` transition clears the warning immediately. A later disconnect starts a new grace interval.
- **AC-UI-WS-CONNECTIVITY-WARNING-001.7:** The yellow detail reads **Connection unstable. Reconnecting to Kandev.**
- **AC-UI-WS-CONNECTIVITY-WARNING-001.8:** The red detail reads **Connection lost for at least 10 seconds. Live updates may be stale.**

## Migrated source detail

## Why

Kandev can look current while its live WebSocket updates are delayed or unavailable. Users need a
quiet but unavoidable indication that task, session, and workspace state may be stale, without
showing permanent healthy-state chrome or flashing for harmless reconnects.

## What

- Kandev derives one transient connectivity severity from the continuous time since its canonical
  application WebSocket last entered `connected`:
  - **Healthy/grace:** no warning before 3 seconds offline.
  - **Unstable:** yellow from 3 seconds offline until 10 seconds offline.
  - **Lost:** red from 10 seconds offline until the connection recovers.
- Transitions among `connecting`, `reconnecting`, `disconnected`, and `error` belong to the same
  continuous offline interval. They do not restart either threshold.
- Any `connected` transition clears the warning immediately. A later disconnect starts a new grace
  interval.
- The yellow detail reads **Connection unstable. Reconnecting to Kandev.**
- The red detail reads **Connection lost for at least 10 seconds. Live updates may be stale.**
- Color is never the only signal. The active surface exposes the matching detail as an accessible
  status name and through hover/focus on fine-pointer layouts or tap on phone.
- Desktop and tablet show exactly one warning:
  - When **Show status bar** is on, its built-in connection item is problem-only: absent while
    healthy or in grace, yellow while unstable, and red while lost.
  - When **Show status bar** is off, a problem-only connection indicator appears in the persistent
    app-sidebar footer beside the theme control. The fallback is absent while healthy or in grace.
- Phone uses its existing native Status paths rather than adding a persistent bar or overlay:
  - Existing Status entry points receive yellow/red warning treatment.
  - Route chrome that leads to a nested Status action, such as the Home or Settings menu, carries
    the same warning treatment so the issue is visible before opening the menu.
  - When **Show status bar** is off, these paths appear only during an active warning and open a
    connection-only Status drawer. Plugin contributions and metrics stay disabled.
- The warning does not alter reconnect attempts, WebSocket subscriptions, queued requests, or
  last-known application data.

## State machine

| State | Entry | Visible result | Exit |
|---|---|---|---|
| `healthy` | WebSocket enters `connected` | No warning | First non-`connected` status |
| `grace` | A continuous offline interval starts | No warning | `connected`, or 3 seconds elapse |
| `unstable` | The same interval reaches 3 seconds | Yellow warning | `connected`, or 10 seconds elapse |
| `lost` | The same interval reaches 10 seconds | Red warning | `connected` |

The state is client-local and transient. A page reload starts a new connection attempt and grace
interval; no outage timestamp or warning history persists.

## Failure modes

- If the reconnect client exhausts its attempts, the warning remains red until a later
  `connected` transition or page reload. It does not claim that retries are still running.
- If the App status surface is hidden by the Appearance preference, an active warning bypasses
  only its visibility gate. It does not mount metrics, plugin status contributions, or a
  desktop/tablet status bar.
- If a responsive presentation changes during an outage, the current severity moves to the new
  canonical surface without resetting its offline interval or rendering duplicate warnings.
- If the warning timer owner unmounts, it cancels pending thresholds and does not update stale
  application state.

## Responsive and mobile contract

- Desktop outcome: one problem-only warning in the enabled bottom status bar or, when that bar is
  disabled, beside the sidebar theme control.
- Phone entry points reuse the shipped Page/Office top-bar Status button, task bottom-nav Status
  action, and Home/Settings menu paths. The persistent route control signals severity before the
  nested Status action is opened.
- The existing App Status drawer is the nearest shipped mobile exemplar. It contributes the inset
  bottom surface, fixed summary, one safe-area-aware internal scroll owner, 44 px rows, dismissal,
  and focus return.
- A short connection explanation belongs in that temporary drawer; it does not justify a new route
  or full-height surface.
- Connection timing and severity are shared across breakpoints. Mobile-specific code only chooses
  presentation and touch entry points.

## Scenarios

- **GIVEN** a connected client, **WHEN** its WebSocket is offline for less than 3 seconds and
  reconnects, **THEN** no connectivity warning appears.
- **GIVEN** a connected client, **WHEN** its WebSocket remains offline for 3 seconds, **THEN** one
  yellow warning appears with **Connection unstable. Reconnecting to Kandev.**
- **GIVEN** the same outage, **WHEN** it reaches 10 seconds, **THEN** the existing warning becomes
  red with **Connection lost for at least 10 seconds. Live updates may be stale.**
- **GIVEN** an unstable or lost warning, **WHEN** any reconnect attempt enters `connected`, **THEN**
  the warning disappears immediately and a later outage receives a fresh grace interval.
- **GIVEN** reconnect attempts cycle through non-connected raw statuses, **WHEN** the continuous
  outage crosses a threshold, **THEN** the threshold is measured from the original disconnect.
- **GIVEN** **Show status bar** is on for desktop or tablet, **WHEN** connection severity changes,
  **THEN** only its built-in connection item reflects the warning and no sidebar fallback appears.
- **GIVEN** **Show status bar** is off for desktop or tablet, **WHEN** connection severity becomes
  unstable or lost, **THEN** one warning indicator appears beside the sidebar theme control and exposes
  the matching detail on hover and focus.
- **GIVEN** **Show status bar** is off on phone, **WHEN** connection severity becomes unstable or
  lost, **THEN** persistent route chrome signals the issue and its Status path opens a
  connection-only, touch-sized drawer without metrics or plugin contributions.
- **GIVEN** a phone warning, **WHEN** the user opens and dismisses Status, **THEN** the connection
  detail is readable, the drawer stays within the viewport, and focus returns to the trigger.
- **GIVEN** an active warning, **WHEN** the viewport crosses between phone and wider layouts,
  **THEN** the severity remains unchanged and exactly one canonical warning surface is visible.

## Out of scope

- Latency, packet-loss, heartbeat-quality, or REST-health sampling.
- Changing reconnect delays, retry limits, queueing, resubscription, or missed-event recovery.
- Manual reconnect actions, recovery toasts, healthy-state success messages, outage history, or
  persisted incident timestamps.
- A new phone bar, toast, route, full-screen alert, or desktop top-bar injection.

## Implementation plan

[WebSocket connectivity warning plan](../../../plans/ws-connectivity-warning/plan.md)
