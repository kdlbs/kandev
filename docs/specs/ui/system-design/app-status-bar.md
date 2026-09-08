---
status: draft
system: ui
requirements:
  - REQ-UI-APP-STATUS-BAR-001
created: 2026-07-21
updated: 2026-08-23
owners:
  - kandev
---
# App Status Bar System Design

## Purpose and boundaries

This design preserves the technical source detail for `REQ-UI-APP-STATUS-BAR-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-APP-STATUS-BAR-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Why

Kandev has useful app-wide state, but it is scattered through route headers. A small global surface makes connection and opted-in resource state consistently available without inventing new operational data or changing chat-local controls. Users can arrange that dense surface around what they scan most often and keep the layout across Kandev clients.

## What

- Non-phone viewports render one persistent **24 px**, in-flow bottom status bar in the route-content column. Where the global sidebar is visible, the bar begins at the sidebar's right edge and tracks its expanded, collapsed, and resized width; the sidebar itself continues to the bottom of the viewport. Where the sidebar is hidden, the bar fills the available app width. Desktop uses `full` density; tablet uses `compact` density.
- Phone renders no persistent second bottom bar. Native route controls open one global **Status** inset bottom drawer, so it does not collide with task bottom navigation. The drawer mirrors the saved bar order vertically (saved left sequence, then saved right sequence), has a fixed header, one internal scroll body, safe-area clearance, 44 px action rows, and returns focus to its trigger.
- The portable **Show status bar** preference in Settings > Preferences > Appearance controls both general-purpose presentations and defaults off. Turning it on adds the desktop/tablet bar and ordinary phone Status entry points as soon as the shared Appearance save succeeds; no Kandev restart is required. An active [WebSocket connectivity warning](../requirements/ws-connectivity-warning.md) is the sole visibility exception: it uses problem-only fallback chrome and a connection-only phone drawer without mounting the bar, metrics, or plugin contributions. The preference changes visibility only; it does not stop connections, metrics collection requested by other clients, or plugin execution.
- Built-ins are limited to Kandev-owned state:
  - Canonical connection state and error from `state.connection.status` / `state.connection.error`, with a restrained semantic dot, the connected detail **Connected to Kandev**, accessible text, and readable failure detail.
  - Existing Kandev-host CPU/memory metrics, preserving enabled-metric order, formatting, thresholds, and tooltips. Desktop and tablet retain their density limits. The phone Status drawer renders every enabled host metric because its vertical surface does not need the bar's horizontal limit. The built-in surface does not render active-task, active-session, or executor metrics.
  - Resource metrics offer two persisted presentation styles. **Detailed** is the default and shows the host marker plus percentage meter bars. **Simplified** shows only each metric icon and formatted value, with no host marker or meter bars. The selected style applies consistently to the desktop/tablet bar, the pre-status-bar topbar fallback, and the phone Status drawer.
  - Phone metrics use a width-aware grid instead of one horizontal strip. The detailed Host marker occupies its own leading line, then the grid chooses the number of columns that fit the available row width, uses two columns in the configured Pixel 5 viewport, and adds vertical rows as indicators increase (six indicators form two columns by three rows there). Every indicator remains inside the metrics item without clipping or horizontal scrolling.
- `userSettings.systemMetricsDisplay.showInTopbar` remains the persisted/wire compatibility key. User-visible copy calls this the Status bar setting; no migration or API break occurs.
- Existing composer-local `ChatStatusBar` remains separate. Queue, PR, share, and next-step affordances stay with chat.
- The connection indicator, complete metrics cluster, and each plugin component registration are individual status items. Holding Cmd on macOS or Ctrl on other platforms while dragging with the mouse reorders an item horizontally across the full bar, including across its flexible spacer.
- A modified pointer press that does not become a horizontal drag preserves the contribution's normal click behavior. Plain mouse/touch interaction never starts reordering.
- Modified dragging does not start browser text selection. The surface uses a clear grabbing state after the horizontal drag threshold is crossed.
- Status item contents share one optical vertical center. The 1 px top separator does not reduce or offset the 24 px content alignment box.
- The bar is a quiet technical strip rather than a collection of chips: its background, foreground, and separator use the same Kandev app-surface tokens as the sidebar, with deliberate space between opaque status items, Geist Sans for labels, Geist Mono plus tabular numerals for changing metrics, consistent icon weight, and no decorative elevation.

## Responsive and layout contract

- `AppShell` owns viewport geometry: an `h-dvh` row with the sidebar beside a `min-w-0 flex-1` route/status column. That column contains a `min-h-0 flex-1` route region followed by the status surface. Shell-owned route roots use parent height and explicit local overflow rather than adding a second viewport height.
- The status bar's left edge equals the sidebar layout edge after expand/collapse settles and throughout direct resize updates. Its top separator begins at that same edge; no status-bar strip or empty footer is rendered beneath the sidebar. When responsive CSS hides the sidebar, the route/status column occupies the full viewport width.
- `--app-status-bar-height` is `1.5rem` exactly while the inline tablet/desktop bar is mounted and `0` otherwise, including when the preference is off or the phone drawer is used. It offsets only audited desktop `position: fixed` overlays; it is not global content padding. Phone bottom navigation and phone drop targets retain `bottom: 0`.
- Exactly one general-purpose presentation mounts at once: bar on tablet/desktop, drawer contents only while the phone drawer is open. This prevents duplicate plugin effects and metrics subscriptions at breakpoints. Turning off **Show status bar** mounts neither general-purpose presentation; a connection-only warning drawer may mount under the exception above.
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

The surface reads existing Zustand connection, active-context, user-settings, and system-metrics state. The backend-owned user settings JSON is authoritative for the portable top-level `app_status_bar_enabled` field. Settings > Preferences > Appearance exposes it as **Show status bar**. An absent stored value or an initial compatibility payload that omits the field means `false`; omission in a PATCH or partial live update leaves the current value unchanged, and an explicit boolean is preserved. Every user-settings mutation atomically increments a per-user numeric revision. Boot hydration, PATCH responses, and live events carry that revision so every client ingestion path rejects older snapshots without treating wall-clock timestamps or store object identity as ordering signals. Appearance saves with only local theme or menu changes do not send an empty user-settings PATCH. The phone drawer's open state is presentation-local and is not persisted. Existing `show_in_topbar` user-setting persistence remains authoritative for metrics visibility.

The existing backend-owned user settings JSON stores visibility, the metrics display preference, and the portable layout:

```ts
type SystemMetricsDisplay = {
  show_in_topbar: boolean;
  simplified: boolean;
};

type AppStatusBarOrder = {
  left_item_ids: string[];
  right_item_ids: string[];
};

type UserSettingsStatusSurface = {
  app_status_bar_enabled: boolean;
  system_metrics_display: SystemMetricsDisplay;
  app_status_bar_order: AppStatusBarOrder;
};
```

The visibility field name is `app_status_bar_enabled` and defaults to `false` when absent, preserving the former production and development default while making the shipped surface a portable opt-in for existing and new users. The metrics field remains `system_metrics_display`; an absent `simplified` value means `false` so existing users retain the detailed presentation. The layout field name is `app_status_bar_order`. Omission keeps each existing value, and an absent order uses the default order: connection plus left-slot registrations, flexible spacer, metrics plus right-slot registrations. Built-ins have reserved stable identities. A plugin identity is derived from its plugin ID, original slot, and zero-based registration ordinal within that plugin and slot; the serialized ID is host-owned and opaque to plugins. Temporarily unavailable identities remain stored but are not rendered, so a disabled plugin or hidden metrics cluster returns to its saved position. A newly seen identity appends to its default side. Successful PATCHes survive frontend/backend restarts and propagate through `user.settings.updated`; last successful write wins across clients. See [ADR-2026-07-21-portable-status-bar-order](../../../decisions/2026-07-21-portable-status-bar-order.md) and [ADR-2026-08-11-user-owned-status-bar-visibility](../../../decisions/2026-08-11-user-owned-status-bar-visibility.md).

The former runtime identities `features.appStatusBar` and `KANDEV_FEATURES_APP_STATUS_BAR` are retired and never reused. Existing unknown override rows remain inert; no install-wide value is migrated into a per-user preference, and the retired environment variable has no effect. The `/api/v1/features` response no longer includes `appStatusBar`.

The `users.settings_revision` relational column is added as an atomic per-user counter so clients can reject stale boot, HTTP, and WebSocket settings snapshots. No new table, endpoint, WebSocket action, plugin manifest field, or plugin protocol is added. The existing user-settings PATCH, boot payload, and `user.settings.updated` event carry visibility, order, and revision.

The only public API addition is `registerComponent("app-status-bar-left" | "app-status-bar-right", Component)` with the exact slot props above. Plugin registration ownership, enable/disable lifecycle, and error isolation reuse the existing registry.

## Failure modes

- Before the first metrics snapshot arrives, the metrics item remains hidden while its normal WebSocket subscription stays active; loading is not presented as an availability failure and does not create a fallback fetch or provider.
- A received snapshot without the Kandev host source renders the recognizable unavailable state. Unavailable metric samples keep their existing per-metric degraded presentation and inspectable error detail.
- Connection errors remain inspectable through accessible detail; reconnecting is not misrepresented as connected. The bottom connection item follows the problem-only timing and severity contract in [WebSocket connectivity warning](../requirements/ws-connectivity-warning.md).
- A failed plugin contribution is contained by its own boundary; remaining contributions and first-party state remain usable.
- If Status drawer closes during a metrics update or breakpoint changes, the inactive presentation unmounts and releases only its own ref-counted subscription.
- Invalid, duplicate, or temporarily unavailable saved item identities never mount duplicate UI. The host normalizes active items once and retains unavailable identities for later plugin re-enable.
- If an order write fails after the standard user-settings retries, the UI restores the last confirmed order and reports the save failure; it does not create a browser-storage fallback.
- If a visibility save fails, the last confirmed surface remains active and Appearance stays dirty and retryable; the frontend does not invent a browser-storage fallback.

## Accessibility

- Connection state is programmatically named and changes are announced without relying on color or hover.
- Bar details remain keyboard reachable with visible focus and accessible labels; reorder wrappers do not introduce nested interactive controls or extra tab stops.
- Reordering is an optional pointer customization activated only by Cmd/Ctrl plus mouse drag. Keyboard-arrow and touch reordering are not part of this interaction; content remains usable in its current order without reordering.
- Phone Status entry points and drawer rows meet the 44 px touch target expectation. Drawer dismissal supports Escape/back, outside dismissal, and focus return.
- The bar and drawer avoid document horizontal overflow; plugin content truncates or scrolls within its owning surface rather than widening it.

## Attribution

Visual interaction is a clean Kandev adaptation of Orca's public status-bar ideas, not a source transplant. The implementation carries one focused source comment and ships a third-party notice naming Orca, pinned revision `d9d939a33b5858495ffb33489a952f1ac9293610`, repository URL, and full MIT notice through Kandev's generated licenses manifest. A licenses-page test proves that notice is visible.

## Scenarios

- **GIVEN** **Show status bar** is on where the global sidebar is visible, **WHEN** a route opens or the sidebar finishes expanding or collapsing, **THEN** one 24 px app status bar begins at the sidebar's right edge, ends at the viewport's right edge, and the sidebar continues to the viewport bottom without a new page scrollbar.
- **GIVEN** **Show status bar** has never been stored, **WHEN** user settings hydrate, **THEN** ordinary desktop/tablet status chrome and phone Status entry points remain off while active connectivity warnings retain their problem-only fallback.
- **GIVEN** an expanded sidebar and visible status bar, **WHEN** the user resizes the sidebar, **THEN** the bar's left edge follows the sidebar layout edge while its right edge remains at the viewport edge.
- **GIVEN** a non-phone viewport where responsive layout hides the global sidebar, **WHEN** the route opens, **THEN** the 24 px status bar spans the available viewport width.
- **GIVEN** metrics preference enabled, **WHEN** a desktop/tablet status bar mounts, **THEN** existing Kandev-host metrics appear there, task/session/executor metrics do not, and no route header still renders metrics.
- **GIVEN** metrics preference enabled and no metrics snapshot has arrived yet, **WHEN** a desktop/tablet status bar mounts or a phone user opens Status, **THEN** the metrics item remains hidden without showing an unavailable message until the first snapshot arrives, while the normal metrics subscription remains active.
- **GIVEN** the detailed metrics style or no stored style preference, **WHEN** host metrics render in a status surface, **THEN** the host marker and percentage meter bars remain visible.
- **GIVEN** the user selects **Simplified metrics** and saves Appearance settings, **WHEN** host metrics render in the desktop/tablet status bar or pre-status-bar topbar fallback, **THEN** each enabled metric shows its icon and formatted value without a host marker or percentage meter bar, and the choice survives reload.
- **GIVEN** the simplified metrics preference, **WHEN** a phone user opens Status, **THEN** the metrics row shows the same icon-and-value presentation without a host marker or percentage meter bar.
- **GIVEN** all host metrics are enabled on a phone, **WHEN** the user opens Status, **THEN** every host metric is visible in enabled order inside a width-aware grid; the configured Pixel 5 viewport uses two columns and additional metrics continue on later rows without widening the metrics item, drawer, or document.
- **GIVEN** metrics preference disabled, or a phone Status drawer closed, **WHEN** the app runs, **THEN** no system-metrics WebSocket subscription is held by this feature.
- **GIVEN** **Show status bar** is on for a phone user, **WHEN** they choose Status from a native entry point, **THEN** the drawer shows the same built-ins and plugin regions; dismissing it restores focus and leaves no persistent status bar.
- **GIVEN** a user turns off **Show status bar** and saves Appearance, **WHEN** the save succeeds, **THEN** the bar/drawer and ordinary native Status triggers disappear immediately, remain absent after reload and on another client, and no restart is required.
- **GIVEN** **Show status bar** is off, **WHEN** the user turns it on and saves, **THEN** the correct responsive presentation becomes available immediately without changing metrics or saved item order.
- **GIVEN** **Show status bar** is off, **WHEN** a WebSocket warning becomes active, **THEN** only the problem-only fallback defined by [WebSocket connectivity warning](../requirements/ws-connectivity-warning.md) appears; metrics and plugin contributions remain unmounted.
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
- Changing the sidebar's existing snap-and-overlay animation behavior or adding a second sidebar-width state source.
- An install-wide status-bar kill switch, runtime feature toggle, environment override, or migration from the former install-wide value into individual user preferences.

## Implementation plans

- [Original App status bar plan](../../../plans/app-status-bar/plan.md)
- [Status bar Appearance setting promotion plan](../../../plans/app-status-bar-appearance-setting/plan.md)
- [Mobile Status metrics grid repair plan](../../../plans/mobile-status-metrics-grid/plan.md)
