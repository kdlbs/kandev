---
status: shipped
created: 2026-07-21
owner: kandev
---

# App Status Bar

## Why

Kandev has useful app-wide state, but it is scattered through route headers. A small global surface makes connection and opted-in resource state consistently available without inventing new operational data or changing chat-local controls. Users can arrange that dense surface around what they scan most often and keep the layout across Kandev clients.

## What

- Desktop and tablet render one persistent **24 px**, in-flow bottom status bar across the app shell, including sidebar and route content. Desktop uses `full` density; tablet uses `compact` density.
- Phone renders no persistent second bottom bar. Native route controls open one global **Status** inset bottom drawer, so it does not collide with task bottom navigation. The drawer mirrors the saved bar order vertically (saved left sequence, then saved right sequence), has a fixed header, one internal scroll body, safe-area clearance, 44 px action rows, and returns focus to its trigger.
- The production-default-off `features.appStatusBar` runtime toggle controls both presentations. When disabled, neither the desktop/tablet bar nor phone Status entry points render. Enabling or disabling it requires a Kandev restart. It changes visibility only; it does not stop connections, metrics collection requested by other clients, or plugin execution.
- Built-ins are limited to Kandev-owned state:
  - Canonical connection state and error from `state.connection.status` / `state.connection.error`, with a restrained semantic dot, the connected detail **Connected to Kandev**, accessible text, and readable failure detail.
  - Existing Kandev-host CPU/memory metrics, preserving enabled-metric order, formatting, thresholds, limits, and tooltips. The built-in surface does not render active-task, active-session, or executor metrics.
  - Resource metrics offer two persisted presentation styles. **Detailed** is the default and shows the host marker plus percentage meter bars. **Simplified** shows only each metric icon and formatted value, with no host marker or meter bars. The selected style applies consistently to the desktop/tablet bar, the pre-status-bar topbar fallback, and the phone Status drawer.
- `userSettings.systemMetricsDisplay.showInTopbar` remains the persisted/wire compatibility key. User-visible copy calls this the Status bar setting; no migration or API break occurs.
- Existing composer-local `ChatStatusBar` remains separate. Queue, PR, share, and next-step affordances stay with chat.
- The connection indicator, complete metrics cluster, and each plugin component registration are individual status items. Holding Cmd on macOS or Ctrl on other platforms while dragging with the mouse reorders an item horizontally across the full bar, including across its flexible spacer.
- A modified pointer press that does not become a horizontal drag preserves the contribution's normal click behavior. Plain mouse/touch interaction never starts reordering.
- Modified dragging does not start browser text selection. The surface uses a clear grabbing state after the horizontal drag threshold is crossed.
- Status item contents share one optical vertical center. The 1 px top separator does not reduce or offset the 24 px content alignment box.
- The bar is a quiet technical strip rather than a collection of chips: its background, foreground, and separator use the same Kandev app-surface tokens as the sidebar, with deliberate space between opaque status items, Geist Sans for labels, Geist Mono plus tabular numerals for changing metrics, consistent icon weight, and no decorative elevation.

## Responsive and layout contract

- `AppShell` owns viewport geometry: an `h-dvh` column with a `min-h-0 flex-1` sidebar/route row, followed by the status surface. Shell-owned route roots use parent height and explicit local overflow rather than adding a second viewport height.
- `--app-status-bar-height` is `1.5rem` on tablet/desktop and `0` on phone. It offsets only audited desktop `position: fixed` overlays; it is not global content padding. Phone bottom navigation and phone drop targets retain `bottom: 0`.
- Exactly one presentation mounts at once: bar on tablet/desktop, drawer contents only while the phone drawer is open. This prevents duplicate plugin effects and metrics subscriptions at breakpoints. Disabling `features.appStatusBar` mounts neither.
- Standard mobile headers, Home utility menu, task bottom navigation, Settings mobile menu, and Office topbar expose Status. A full-bleed plugin route (`topbar: false`) owns its chrome and must mount `AppStatusDrawerTrigger` if it wants status access.

## Plugin slots

Plugins may register components in two live slots: `app-status-bar-left` and `app-status-bar-right`. The slot chooses a contribution's default side; after user customization it is not a permanent placement guarantee. Desktop items may move across the spacer, and the phone drawer renders the resulting saved order vertically. Registry enable/disable changes render without reload; every contribution stays behind its own plugin error boundary.

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

IDs are current-context hints, not entity records; plugins read complete records from `host.store`. Default registration order is preserved until the user moves items. Each component registration is one opaque status item: the host does not inspect or separately order elements rendered inside it. No cross-plugin priority, manifest field, or sandbox change is introduced. Plugin UI must fit the supplied presentation and must not rely on one presentation remaining mounted.

## Metrics subscription

The existing setting is the first gate. If disabled, no status-surface metrics subscription exists. If enabled, tablet/desktop subscribe while their bar is mounted; phone subscribes only while Status drawer is open. The existing ref-counted WebSocket client owns reconnect behavior. The change must not leave header metrics mounted or create duplicate subscriptions.

## Data, API, and persistence

The surface reads existing Zustand connection, active-context, user-settings, system-metrics, and feature state. `features.appStatusBar` is a default-off, restart-required runtime flag exposed through the existing Feature Toggles page; `KANDEV_FEATURES_APP_STATUS_BAR` takes precedence and locks the control when set explicitly. The E2E profile enables it only for browser coverage. The phone drawer's open state is presentation-local and is not persisted. Existing `show_in_topbar` user-setting persistence remains authoritative.

The existing backend-owned user settings JSON stores the metrics display preference and portable layout:

```ts
type SystemMetricsDisplay = {
  show_in_topbar: boolean;
  simplified: boolean;
};

type AppStatusBarOrder = {
  left_item_ids: string[];
  right_item_ids: string[];
};
```

The metrics field remains `system_metrics_display`; an absent `simplified` value means `false` so existing users retain the detailed presentation. The layout field name is `app_status_bar_order`. Omission keeps the existing value and an absent value uses the default order: connection plus left-slot registrations, flexible spacer, metrics plus right-slot registrations. Built-ins have reserved stable identities. A plugin identity is derived from its plugin ID, original slot, and zero-based registration ordinal within that plugin and slot; the serialized ID is host-owned and opaque to plugins. Temporarily unavailable identities remain stored but are not rendered, so a disabled plugin or hidden metrics cluster returns to its saved position. A newly seen identity appends to its default side. Successful PATCHes survive frontend/backend restarts and propagate through `user.settings.updated`; last successful write wins across clients. See [ADR-2026-07-21-portable-status-bar-order](../../decisions/2026-07-21-portable-status-bar-order.md).

No new relational schema, endpoint, WebSocket action, plugin manifest field, or plugin protocol is added. The existing user-settings PATCH/event contracts carry the order, and the existing runtime-flags persistence and `/api/v1/features` response carry the visibility setting.

The only public API addition is `registerComponent("app-status-bar-left" | "app-status-bar-right", Component)` with the exact slot props above. Plugin registration ownership, enable/disable lifecycle, and error isolation reuse the existing registry.

## Failure modes

- Missing metrics snapshot renders a recognizable unavailable/loading state; it does not create a fallback fetch or provider.
- Connection errors remain inspectable through accessible detail; reconnecting is not misrepresented as connected.
- A failed plugin contribution is contained by its own boundary; remaining contributions and first-party state remain usable.
- If Status drawer closes during a metrics update or breakpoint changes, the inactive presentation unmounts and releases only its own ref-counted subscription.
- Invalid, duplicate, or temporarily unavailable saved item identities never mount duplicate UI. The host normalizes active items once and retains unavailable identities for later plugin re-enable.
- If an order write fails after the standard user-settings retries, the UI restores the last confirmed order and reports the save failure; it does not create a browser-storage fallback.

## Accessibility

- Connection state is programmatically named and changes are announced without relying on color or hover.
- Bar details remain keyboard reachable with visible focus and accessible labels; reorder wrappers do not introduce nested interactive controls or extra tab stops.
- Reordering is an optional pointer customization activated only by Cmd/Ctrl plus mouse drag. Keyboard-arrow and touch reordering are not part of this interaction; content remains usable in its current order without reordering.
- Phone Status entry points and drawer rows meet the 44 px touch target expectation. Drawer dismissal supports Escape/back, outside dismissal, and focus return.
- The bar and drawer avoid document horizontal overflow; plugin content truncates or scrolls within its owning surface rather than widening it.

## Attribution

Visual interaction is a clean Kandev adaptation of Orca's public status-bar ideas, not a source transplant. The implementation carries one focused source comment and ships a third-party notice naming Orca, pinned revision `d9d939a33b5858495ffb33489a952f1ac9293610`, repository URL, and full MIT notice through Kandev's generated licenses manifest. A licenses-page test proves that notice is visible.

## Scenarios

- **GIVEN** the feature is enabled on a desktop or tablet route, **WHEN** it opens, **THEN** one 24 px app status bar remains at its bottom and route/sidebar content use the remaining height without a new page scrollbar.
- **GIVEN** metrics preference enabled, **WHEN** a desktop/tablet status bar mounts, **THEN** existing Kandev-host metrics appear there, task/session/executor metrics do not, and no route header still renders metrics.
- **GIVEN** the detailed metrics style or no stored style preference, **WHEN** host metrics render in a status surface, **THEN** the host marker and percentage meter bars remain visible.
- **GIVEN** the user selects **Simplified metrics** and saves Appearance settings, **WHEN** host metrics render in the desktop/tablet status bar or pre-status-bar topbar fallback, **THEN** each enabled metric shows its icon and formatted value without a host marker or percentage meter bar, and the choice survives reload.
- **GIVEN** the simplified metrics preference, **WHEN** a phone user opens Status, **THEN** the metrics row shows the same icon-and-value presentation without a host marker or percentage meter bar.
- **GIVEN** metrics preference disabled, or a phone Status drawer closed, **WHEN** the app runs, **THEN** no system-metrics WebSocket subscription is held by this feature.
- **GIVEN** the feature is enabled for a phone user, **WHEN** they choose Status from a native entry point, **THEN** the drawer shows the same built-ins and plugin regions; dismissing it restores focus and leaves no persistent status bar.
- **GIVEN** an administrator disables **App status bar** in Feature Toggles and restarts Kandev, **WHEN** a route renders on any breakpoint, **THEN** the bar/drawer and native Status triggers are absent while the rest of the shell remains available.
- **GIVEN** a plugin registered for either status slot, **WHEN** it enables or disables, **THEN** its contribution appears or disappears without reload in the active presentation. A failed contribution does not suppress a following healthy one after registrations change.
- **GIVEN** a desktop/tablet bar, **WHEN** the user holds Cmd/Ctrl and mouse-drags a built-in or plugin contribution across another item or the spacer, **THEN** the item moves horizontally, normal click activation is suppressed only after a drag begins, and the new side/order survives reload and backend restart.
- **GIVEN** a modified mouse press begins on status text, **WHEN** the pointer crosses the drag threshold, **THEN** the item enters its dragging state without selecting text in the bar.
- **GIVEN** a saved order containing a disabled plugin contribution, **WHEN** that plugin enables again, **THEN** its stable item returns to the saved position without remounting any other item twice.
- **GIVEN** a saved desktop order, **WHEN** Status opens on phone, **THEN** the drawer renders the saved left sequence followed by the saved right sequence as vertical 44 px rows and offers no drag interaction.
- **GIVEN** the bar at a 1x device scale, **WHEN** text, dots, and metric icons render beside one another, **THEN** they share the same vertical content center and the top separator does not create a half-pixel offset.

## Out of scope

- New provider-usage, account, ports, SSH, process-management, update-check, billing, or metrics backend built to fill the bar.
- Changing `ChatStatusBar` or moving its chat-local controls.
- A phone persistent bar, plugin slot priority system, plugin manifest/protocol change, plugin JavaScript sandbox, keyboard-arrow reordering, or touch reordering.
- Per-metric label controls, per-breakpoint metrics styles, or changes to which metrics are collected.
- Broad global fixed-position padding; only audited desktop overlays receive the height offset.

## Implementation plan

[App status bar plan](../../plans/app-status-bar/plan.md)
