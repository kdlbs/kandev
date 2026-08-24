---
spec: docs/specs/ui/requirements/ws-connectivity-warning.md
created: 2026-07-30
status: complete
---

# Implementation Plan: WebSocket Connectivity Warning

## Overview

Derive one time-based issue severity beside the existing WebSocket lifecycle, then project it into
the existing App status bar, the desktop/sidebar fallback, and native phone Status paths. The
timing owner lands first so every surface shares the same continuous outage clock. Desktop and
mobile presentation tasks follow, then focused production-build E2E and public documentation.

No backend, schema, HTTP, or WebSocket protocol change is required.

---

## Backend

None. The feature consumes the existing frontend `ConnectionStatus` transitions and leaves the
backend gateway, retry policy, and subscription protocol unchanged.

---

## Frontend

### Canonical issue timing

- Add `ConnectionIssueSeverity = "none" | "unstable" | "lost"` in
  `apps/web/lib/types/connection.ts`.
- Extend the transient `ConnectionState` in
  `apps/web/lib/state/slices/ui/types.ts`,
  `apps/web/lib/state/slices/ui/ui-slice.ts`, and `apps/web/lib/state/store.ts` with the current
  issue severity and an action that updates it. The default is `none`; hydration remains
  non-persistent.
- Add a small timer owner in `apps/web/lib/ws/connection-issue-monitor.ts`. It treats every
  non-`connected` status as one continuous interval, emits `unstable` at 3,000 ms and `lost` at
  10,000 ms, clears immediately on `connected`, and cancels both timers on disposal.
- Wire that owner into the single `apps/web/lib/ws/use-websocket.tsx` lifecycle. Raw connection
  status/error updates, retry options, and subscription behavior remain unchanged.

### Desktop and tablet presentation

- Refine `apps/web/components/app-status-bar/connection-status-item.tsx` so the bar presentation
  returns no content for `none`, renders the exact yellow/red copy for an active issue, and keeps
  complete healthy/raw connection detail in the manually opened phone drawer.
- Reuse the same semantic detail mapping in a focusable sidebar warning indicator rather than
  duplicating color/copy decisions.
- Update `apps/web/components/app-sidebar/app-sidebar-footer.tsx` to render that indicator
  immediately after `ThemeToggle` only when `features.appStatusBar` is off and issue severity is
  active. The sidebar remains the persistent tablet/desktop fallback in both expanded and collapsed
  layouts.
- Preserve `builtin:connection` ordering. Its desktop wrapper remains present in the saved order but
  collapses through the existing `empty:hidden` behavior while no warning content renders.

### Phone presentation

- Extend `AppStatusDrawerContextValue` in
  `apps/web/components/app-status-bar/app-status-surface-provider.tsx` with issue severity and
  whether the drawer is an issue-only fallback.
- On phone, enable Status access when either `features.appStatusBar` is on or an issue is active.
  When only the issue activates it, mount `apps/web/components/app-status-bar/app-status-drawer.tsx`
  in connection-only mode; do not mount metrics or plugin contributions.
- Apply shared yellow/red styling and accessible warning text to
  `AppStatusDrawerTrigger`. Existing Page and Office top-bar triggers inherit it without route-local
  state logic.
- Propagate the same severity to custom native paths:
  `apps/web/components/kanban/kanban-header-mobile.tsx`,
  `apps/web/components/kanban/mobile-menu-sheet.tsx`,
  `apps/web/components/settings/settings-layout-client.tsx`,
  `apps/web/components/task/mobile/session-mobile-layout.tsx`, and
  `apps/web/components/task/mobile/session-mobile-bottom-nav.tsx`.
- Home/Settings menu triggers show the warning before their nested Status row is opened. Task
  bottom navigation and direct Page/Office top-bar Status controls show it directly.

### Mobile design contract

- **Desktop outcome / phone entry:** desktop uses the enabled bottom status item or sidebar-footer
  fallback; phone uses the route's existing top-bar, bottom-nav, or menu Status path.
- **Nearest exemplar:** the shipped `AppStatusDrawer` supplies the inset bottom drawer, fixed
  summary, safe-area clearance, 44 px rows, single internal scroll owner, dismissal, and focus
  return.
- **Hierarchy:** persistent chrome communicates severity; opening Status reveals the exact
  connection explanation. No healthy-state phone chrome is added when the general status feature is
  off.
- **Presentation:** an inset drawer fits one short, temporary diagnostic better than a new route or
  full-height surface.
- **Shared logic:** the Zustand issue severity and detail mapper are common; phone code only selects
  and styles touch presentation.
- **Geometry:** existing `h-dvh` shell, drawer `100dvh` bounds, safe-area padding, and 44 px native
  controls remain authoritative. No additional document scroll owner is introduced.

---

## Tests

- **Continuous interval thresholds and reset**
  - **File:** `apps/web/lib/ws/connection-issue-monitor.test.ts`
  - **How:** Vitest fake timers prove no signal before 3 seconds, `unstable` at 3 seconds, `lost`
    at 10 seconds, raw non-connected transitions do not reset time, `connected` clears immediately,
    a later outage starts fresh, and dispose suppresses pending emissions.
- **Store and lifecycle wiring**
  - **Files:** `apps/web/lib/state/store.test.ts` and, if needed for the lifecycle boundary,
    `apps/web/lib/ws/use-websocket.test.tsx`
  - **How:** focused unit tests prove the derived severity reaches the canonical store without
    changing raw `ConnectionStatus` forwarding.
- **Connection details and desktop fallback**
  - **Files:** `apps/web/components/app-status-bar/connection-status-item.test.tsx`,
    `apps/web/components/app-sidebar/app-sidebar-footer.test.tsx`, and
    `apps/web/components/app-status-bar/app-status-bar.test.tsx`
  - **How:** component tests cover exact accessible copy/color, healthy bar collapse, enabled-bar
    exclusivity, feature-off sidebar fallback, and collapsed-sidebar focus/tooltip access.
- **Phone visibility gate and native controls**
  - **Files:** `apps/web/components/app-status-bar/app-status-surface-provider.test.tsx`,
    `apps/web/components/app-status-bar/app-status-drawer.test.tsx`,
    `apps/web/components/kanban/kanban-header-mobile.test.tsx`,
    `apps/web/components/settings/settings-layout-client.test.tsx`, and
    `apps/web/components/task/mobile/session-mobile-bottom-nav.test.tsx`
  - **How:** component tests cover feature-on ordinary access, feature-off healthy absence,
    feature-off warning access, connection-only drawer contents, severity styling, 44 px controls,
    and unchanged actions/focus behavior.

There is no handler/service/repository integration path to test because the feature is frontend-only.
The production-build Playwright scenarios below provide the cross-component integration proof.

---

## E2E Tests

- **Scenario:** App status bar enabled on desktop; setting issue severity shows exactly one bottom
  yellow/red connection item and clearing it removes the item content.
  - **File:** `apps/web/e2e/tests/layout/ws-connectivity-warning.spec.ts`
  - **What to verify:** semantic copy, yellow/red treatment, no sidebar duplicate, and recovery.
- **Scenario:** App status bar disabled on desktop/tablet; an issue shows the sidebar-footer
  fallback beside the theme control and no bottom bar.
  - **File:** `apps/web/e2e/tests/layout/ws-connectivity-warning.spec.ts`
  - **What to verify:** feature-off placement, hover/focus detail, severity transition, and recovery.
- **Scenario:** App status bar disabled on Pixel 5; an issue marks persistent route chrome, exposes a
  44 px Status path, and opens a connection-only drawer.
  - **File:** `apps/web/e2e/tests/layout/mobile-ws-connectivity-warning.spec.ts`
  - **What to verify:** warning is discoverable before nested navigation, only connection detail
    renders, viewport containment, focus return, safe-area/internal scrolling, no horizontal
    overflow, and recovery hides the issue-only path.

The tests may use the existing E2E store bridge to inject derived severity; unit tests own the
3/10-second clock so Playwright does not add ten-second sleeps or alter WebSocket accounting.

---

## Public documentation

- Amend `docs/public/operations.md` to explain the 3-second yellow / 10-second red warning,
  its placement with App status bar on or off, and that red means live data may be stale.
- Amend the `features.app_status_bar` row in `docs/public/configuration.md` so disabling the general
  status surface does not imply that urgent connectivity warnings disappear.
- No new page or navigation entry is needed.

---

## Implementation Waves And Parallel Candidates

Execution remains sequential in the primary conversation.

Wave 1:

- [x] [task-01-connection-issue-timing](task-01-connection-issue-timing.md)

Wave 2:

- [x] [task-02-desktop-warning-surfaces](task-02-desktop-warning-surfaces.md)

Wave 3:

- [x] [task-03-mobile-warning-surfaces](task-03-mobile-warning-surfaces.md)

Wave 4:

- [x] [task-04-connectivity-warning-e2e](task-04-connectivity-warning-e2e.md)
- [x] [task-05-public-documentation](task-05-public-documentation.md)

Tasks 02 and 03 both consume shared status components/provider contracts, so they are not marked
parallel-safe. Task 04 depends on both rendered paths. Task 05 is logically independent after the
spec but stays sequential to keep documented copy aligned with the final UI.

---

## Risks

- Raw `error → disconnected → reconnecting → connecting` churn can accidentally reset a
  component-local timer; the timer must live once beside `useWebSocket`.
- A feature-off issue-only phone drawer must filter before rendering contributions, or plugin
  effects and metrics subscriptions could run despite the toggle.
- Home and Settings hide Status inside a menu, so their persistent menu triggers must carry the
  warning; styling only the nested row would fail discoverability.
- Existing E2E status-bar ordering assumes a visible healthy connection dot. Its geometry/order
  assertions must be updated to inject an issue where connection-item visibility is material,
  without changing saved identity semantics.
