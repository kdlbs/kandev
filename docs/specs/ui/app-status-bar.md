---
status: draft
created: 2026-07-21
owner: kandev
---

# App Status Bar

## Why

Kandev has useful app-wide state, but it is scattered through route headers. A small global surface makes connection and opted-in resource state consistently available without inventing new operational data or changing chat-local controls.

## What

- Desktop and tablet render one persistent **24 px**, in-flow bottom status bar across the app shell, including sidebar and route content. Desktop uses `full` density; tablet uses `compact` density.
- Phone renders no persistent second bottom bar. Native route controls open one global **Status** inset bottom drawer, so it does not collide with task bottom navigation. The drawer has a fixed header, one internal scroll body, safe-area clearance, 44 px action rows, and returns focus to its trigger.
- Built-ins are limited to Kandev-owned state:
  - Canonical connection state and error from `state.connection.status` / `state.connection.error`, with semantic visual state, accessible text, and readable detail.
  - Existing host and active-executor CPU/memory metrics, preserving current source selection, formatting, thresholds, and tooltips.
- `userSettings.systemMetricsDisplay.showInTopbar` remains the persisted/wire compatibility key. User-visible copy calls this the Status bar setting; no migration or API break occurs.
- Existing composer-local `ChatStatusBar` remains separate. Queue, PR, share, and next-step affordances stay with chat.

## Responsive and layout contract

- `AppShell` owns viewport geometry: an `h-dvh` column with a `min-h-0 flex-1` sidebar/route row, followed by the status surface. Shell-owned route roots use parent height and explicit local overflow rather than adding a second viewport height.
- `--app-status-bar-height` is `1.5rem` on tablet/desktop and `0` on phone. It offsets only audited desktop `position: fixed` overlays; it is not global content padding. Phone bottom navigation and phone drop targets retain `bottom: 0`.
- Exactly one presentation mounts at once: bar on tablet/desktop, drawer contents only while the phone drawer is open. This prevents duplicate plugin effects and metrics subscriptions at breakpoints.
- Standard mobile headers, Home utility menu, task bottom navigation, Settings mobile menu, and Office topbar expose Status. A full-bleed plugin route (`topbar: false`) owns its chrome and must mount `AppStatusDrawerTrigger` if it wants status access.

## Plugin slots

Plugins may register components in two live slots: `app-status-bar-left` and `app-status-bar-right`. They render in matching desktop clusters and as ordered vertical sections in the phone drawer. Registry enable/disable changes render without reload; every contribution stays behind its own plugin error boundary.

```ts
export interface AppStatusBarSlotProps {
  placement: "left" | "right";
  presentation: "bar" | "mobile-drawer";
  density: "full" | "compact";
  pathname: string;
  activeWorkspaceId: string | null;
  activeTaskId: string | null;
  activeSessionId: string | null;
}
```

IDs are current-context hints, not entity records; plugins read complete records from `host.store`. Registration order is preserved. No cross-plugin priority, persistence, backend protocol, manifest field, or sandbox change is introduced. Plugin UI must fit the supplied presentation and must not rely on one presentation remaining mounted.

## Metrics subscription

The existing setting is the first gate. If disabled, no status-surface metrics subscription exists. If enabled, tablet/desktop subscribe while their bar is mounted; phone subscribes only while Status drawer is open. The existing ref-counted WebSocket client owns reconnect behavior. The change must not leave header metrics mounted or create duplicate subscriptions.

## Data, API, and persistence

No backend schema, endpoint, WebSocket action, plugin manifest field, or plugin protocol changes. The surface reads existing Zustand connection, active-context, user-settings, and system-metrics state. The phone drawer's open state is presentation-local and is not persisted. Existing `show_in_topbar` user-setting persistence remains authoritative.

The only public API addition is `registerComponent("app-status-bar-left" | "app-status-bar-right", Component)` with the exact slot props above. Plugin registration ownership, enable/disable lifecycle, and error isolation reuse the existing registry.

## Failure modes

- Missing metrics snapshot renders a recognizable unavailable/loading state; it does not create a fallback fetch or provider.
- Connection errors remain inspectable through accessible detail; reconnecting is not misrepresented as connected.
- A failed plugin contribution is contained by its own boundary; remaining contributions and first-party state remain usable.
- If Status drawer closes during a metrics update or breakpoint changes, the inactive presentation unmounts and releases only its own ref-counted subscription.

## Accessibility

- Connection state is programmatically named and changes are announced without relying on color or hover.
- Bar details remain keyboard reachable with visible focus and accessible labels; plugin containers do not introduce nested interactive controls.
- Phone Status entry points and drawer rows meet the 44 px touch target expectation. Drawer dismissal supports Escape/back, outside dismissal, and focus return.
- The bar and drawer avoid document horizontal overflow; plugin content truncates or scrolls within its owning surface rather than widening it.

## Attribution

Visual interaction is a clean Kandev adaptation of Orca's public status-bar ideas, not a source transplant. The implementation carries one focused source comment and ships a third-party notice naming Orca, pinned revision `d9d939a33b5858495ffb33489a952f1ac9293610`, repository URL, and full MIT notice through Kandev's generated licenses manifest. A licenses-page test proves that notice is visible.

## Scenarios

- **GIVEN** a desktop or tablet route, **WHEN** it opens, **THEN** one 24 px app status bar remains at its bottom and route/sidebar content use the remaining height without a new page scrollbar.
- **GIVEN** metrics preference enabled, **WHEN** a desktop/tablet status bar mounts, **THEN** existing host and active-executor metrics appear there and no route header still renders them.
- **GIVEN** metrics preference disabled, or a phone Status drawer closed, **WHEN** the app runs, **THEN** no system-metrics WebSocket subscription is held by this feature.
- **GIVEN** a phone user, **WHEN** they choose Status from a native entry point, **THEN** the drawer shows the same built-ins and plugin regions; dismissing it restores focus and leaves no persistent status bar.
- **GIVEN** a plugin registered for either status slot, **WHEN** it enables or disables, **THEN** its contribution appears or disappears without reload in the active presentation. A failed contribution does not suppress a following healthy one after registrations change.

## Out of scope

- New provider-usage, account, ports, SSH, process-management, update-check, billing, or metrics backend built to fill the bar.
- Changing `ChatStatusBar` or moving its chat-local controls.
- A phone persistent bar, plugin slot priority system, plugin manifest/protocol change, or plugin JavaScript sandbox.
- Broad global fixed-position padding; only audited desktop overlays receive the height offset.

## Implementation plan

[App status bar plan](../../plans/app-status-bar/plan.md)
