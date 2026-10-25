---
status: active
system: ui
created: 2026-07-21
updated: 2026-08-11
owners:
  - kandev
---
# App Status Bar Requirements

## Overview

Kandev has useful app-wide state, but it is scattered through route headers. A small global surface makes connection and opted-in resource state consistently available without inventing new operational data or changing chat-local controls. Users can arrange that dense surface around what they scan most often and keep the layout across Kandev clients.

## Requirements

### REQ-UI-APP-STATUS-BAR-001: App Status Bar

**Intent:** Kandev has useful app-wide state, but it is scattered through route headers. A small global surface makes connection and opted-in resource state consistently available without inventing new operational data or changing chat-local controls. Users can arrange that dense surface around what they scan most often and keep the layout across Kandev clients.

#### Acceptance criteria

- **AC-UI-APP-STATUS-BAR-001.1:** Non-phone viewports render one persistent **24 px**, in-flow bottom status bar in the route-content column. Where the global sidebar is visible, the bar begins at the sidebar's right edge and tracks its expanded, collapsed, and resized width; the sidebar itself continues to the bottom of the viewport. Where the sidebar is hidden, the bar fills the available app width. Desktop uses `full` density; tablet uses `compact` density.
- **AC-UI-APP-STATUS-BAR-001.2:** Phone renders no persistent second bottom bar. Native route controls open one global **Status** inset bottom drawer, so it does not collide with task bottom navigation. The drawer mirrors the saved bar order vertically (saved left sequence, then saved right sequence), has a fixed header, one internal scroll body, safe-area clearance, 44 px action rows, and returns focus to its trigger.
- **AC-UI-APP-STATUS-BAR-001.3:** The portable **Show status bar** preference in Settings > Preferences > Appearance controls both general-purpose presentations and defaults off. Turning it on adds the desktop/tablet bar and ordinary phone Status entry points as soon as the shared Appearance save succeeds; no Kandev restart is required. An active [WebSocket connectivity warning](ws-connectivity-warning.md) is the sole visibility exception: it uses problem-only fallback chrome and a connection-only phone drawer without mounting the bar, metrics, or plugin contributions. The preference changes visibility only; it does not stop connections, metrics collection requested by other clients, or plugin execution.
- **AC-UI-APP-STATUS-BAR-001.4:** Built-ins are limited to Kandev-owned state:
- **AC-UI-APP-STATUS-BAR-001.5:** Canonical connection state and error from `state.connection.status` / `state.connection.error`, with a restrained semantic dot, the connected detail **Connected to Kandev**, accessible text, and readable failure detail.
- **AC-UI-APP-STATUS-BAR-001.6:** Existing Kandev-host CPU/memory metrics, preserving enabled-metric order, formatting, thresholds, limits, and tooltips. The built-in surface does not render active-task, active-session, or executor metrics.
- **AC-UI-APP-STATUS-BAR-001.7:** Resource metrics offer two persisted presentation styles. **Detailed** is the default and shows the host marker plus percentage meter bars. **Simplified** shows only each metric icon and formatted value, with no host marker or meter bars. The selected style applies consistently to the desktop/tablet bar, the pre-status-bar topbar fallback, and the phone Status drawer.
- **AC-UI-APP-STATUS-BAR-001.8:** `userSettings.systemMetricsDisplay.showInTopbar` remains the persisted/wire compatibility key. User-visible copy calls this the Status bar setting; no migration or API break occurs.

## System design

The migrated technical source is split into [part 1](../system-design/app-status-bar.md).
