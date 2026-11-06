---
status: current
system: platform
requirements:
  - REQ-PLATFORM-NOTIFICATIONS-001
---

# Local Notification Attention

## Purpose and boundaries

Platform owns Local notification eligibility under the existing
[semantic notification requirement](../requirements/notifications.md).
This design covers the client attention gate for task-scoped events. The
[Desktop notification contract](../../desktop/system-design/desktop-tauri-app.md#notification-contract)
continues to own native transport, permission, origin checks, and deduplication.
The [semantic event decision](../../../decisions/2026-07-24-semantic-notification-events.md)
and [native integration boundary](../../../decisions/0039-native-desktop-integration-boundary.md)
remain authoritative. This correction adds no new architecture boundary or ADR.

The current client checks visibility and active task identity but omits document
focus. A document can remain visible while another window has focus, so current
task identity alone cannot establish attention. The
[Page Visibility API documentation](https://developer.mozilla.org/en-US/docs/Web/API/Page_Visibility_API#concepts_and_usage)
describes visibility and window focus as independent signals.

## Requirement mapping

| Requirement | Criteria | Design section |
| --- | --- | --- |
| `REQ-PLATFORM-NOTIFICATIONS-001` | `AC-PLATFORM-NOTIFICATIONS-001.11` | Attention gate; verification boundary |
| `REQ-PLATFORM-NOTIFICATIONS-001` | `AC-PLATFORM-NOTIFICATIONS-001.2`, `AC-PLATFORM-NOTIFICATIONS-001.3`, `AC-PLATFORM-NOTIFICATIONS-001.4` | Existing dispatch contract |

## Attention gate

In `apps/web/lib/ws/handlers/notifications.ts`,
`registerNotificationsHandlers` registers the existing semantic session handlers.
`registerNotificationHandler` reads `store.getState()` on each invocation and
calls `shouldSuppressNotification` before sound or notification dispatch.
Keep this flow and add a current `document.hasFocus()` check to the existing
conjunction in `shouldSuppressNotification`.

Suppression requires all of:

- `document.visibilityState === "visible"`;
- `document.hasFocus()` is true;
- a non-empty payload task ID equals `state.tasks.activeTaskId`.

Otherwise execution continues along the existing dispatch path. Read focus at
event time, alongside the current store snapshot. No listener, focus cache,
timer, persistent preference, or reusable session-domain focus helper is needed.
Changing focus does not replay an earlier suppressed occurrence.

The helper is also used by `office.inbox_item`. Retain that shared gate rather
than adding event-specific attention policies. Office events without task
identity remain eligible for their current browser fallback. The separate
`system.update_available` handler continues to update the in-app bridge without
entering this task attention gate. Session-failure alerts are outside this path.

## Existing dispatch contract

After admission, `playWaitingForInputSound` retains its device preference and
best-effort audio behavior. Copy keeps backend values and existing translated
fallback keys, resolved per occurrence. Event selection remains upstream; the
gate neither selects a provider nor creates a semantic occurrence.

When `nativeNotifications.isAvailable()` and the required task/event identity
are present, dispatch through the current native client. Semantic native event
IDs retain event type plus envelope ID, falling back to payload occurrence ID.
The native command retains deduplication. Otherwise use the current Web
Notification API fallback when available and granted. Missing identity,
permission denial, unsupported APIs, and rejected native calls retain their
current behavior. No new background permission request, retry, navigation,
transport fallback, backend change, persistence, or telemetry is introduced.

## Verification boundary

Extend `apps/web/lib/ws/handlers/notifications.test.ts` using independent
messages and store fixtures from the existing suite, invoking the real
`registerNotificationsHandlers`. Explicitly control both document visibility and
focus and restore spies/property descriptors after each test; the test runner's
default focus is not evidence. Mock only external notification/audio transports.

Cover both `session.clarification_requested` and `session.turn_finished` through
browser and native branches: visible/unfocused/current task dispatches;
visible/focused/current task suppresses; hidden/current task and
visible/focused/other task dispatch. Reuse one registered handler for distinct
occurrences across focused, unfocused, and focused states to prove event-time
sampling, without replaying the same identity. Assert sound admission and exact
notification payload/identity, and assert the unused transport stays unused.

Keep bounded controls for permission-denied/unsupported browser delivery,
missing native identity fallback, native dedup delegation, translated fallback
copy, Office task/no-task handling, and update-bridge routing. These tests prove
client dispatch decisions, not actual OS toast display or audio playback.

## Surface and documentation assessment

There is no markup, copy, navigation, touch, scrolling, breakpoint, or settings
change. The same gate applies in mobile browsers. Under mobile-parity's narrow
state/logic exception, direct handler tests are sufficient; no new phone
composition, ASCII preview, Playwright scenario, or browser artifact gate is
needed. Existing localization remains unchanged.

Docs-maintainer assessment: this design package changes internal intent only.
Existing public desktop and WebSocket descriptions remain accurate; no public
command, API payload, setting, terminology, or screenshot changes are required.
Do not expand notification-settings atomicity or historical delivery plans.

## Delivery record

- [Notification window focus plan](../../../plans/notification-window-focus/plan.md)
