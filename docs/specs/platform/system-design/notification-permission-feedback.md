---
status: current
system: platform
created: 2026-10-30
requirements:
  - REQ-PLATFORM-NOTIFICATIONS-001
---

# Notification Permission Feedback

## Ownership and scope

Platform owns notification permission feedback across browser and native clients.
This design extends the [notification requirement](../requirements/notifications.md).
It reuses the [settings composition](../../ui/system-design/settings-composition.md) primitives.
[ADR 0039](../../../decisions/0039-native-desktop-integration-boundary.md) retains authority over native transport and permission requests.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| AC-PLATFORM-NOTIFICATIONS-001.17 | Conditional warning frame |
| AC-PLATFORM-NOTIFICATIONS-001.18 | Permission states; responsive composition; verification |

## Conditional warning frame

`DesktopNotificationsSection` in `apps/web/components/settings/notification-permission-section.tsx` owns the warning decision.
Its permission, refresh, and test actions sit in the compact preference row defined by the
[notification settings layout](notification-settings-layout.md).
Resolve one warning value inside the component with the existing translation keys.
Render no warning element and no reserved footer in normal permission states.
When a warning exists, it follows the row controls inside the same bordered row.
Healthy states keep the preference row's compact height and the page's normal spacing.

Keep the shared `SettingsGroup` implementation unchanged.
Its other consumers can intentionally render empty or loading content.

## Permission states

| State | Warning |
| --- | --- |
| `default` | None |
| `granted` | None |
| `denied`, native available | Existing operating-system warning |
| `denied`, native unavailable | Existing browser warning |
| `unsupported` | Existing unsupported warning |
| `error` | Existing permission-check warning |

Keep `useNotificationPermission`, action handlers, permission requests, and native detection behavior intact.
Refresh changes the same permission state and therefore the warning value together.
Retain every existing button label, handler, and disabled condition.
No new copy, persistence, API, telemetry, or permission prompt is required.

## Responsive composition

The entry point is Settings > Notifications on both desktop and phone.
The compact row composition, touch targets, and warning placement are owned by the
[notification settings layout](notification-settings-layout.md).
Desktop places the label and its actions in one row.
Phone places the actions below the label and keeps the warning inside the row.
The page retains one scroll owner and its current safe-area handling.
Permission state and warning selection remain shared across viewports.

## Verification

Extend `notification-permission-section.test.tsx` through the compact preference row.
Cover every table row and transitions from warning to normal state and back.
Assert the warning text, absence of a warning footer in healthy states, and existing action behavior.
Do not use a class-string assertion as the only evidence.

Add focused permission-layout cases to the existing desktop and phone notification type-scale Playwright specs.
Set browser permission deterministically before navigation without opening an operating-system permission prompt.
Cover a normal state, a denied warning, and refresh back to granted.
Assert no warning footer or reserved row in normal states, a normal inter-section gap, and warning wrapping.
Assert action reachability and no document horizontal overflow.
Capture desktop and phone results through the existing `prCapture` fixture.
Component tests cover native denial and check-error cases that need transport stubs.

## Documentation and delivery

Public instructions, terminology, and notification policy remain accurate.
This package changes internal layout intent only, so public documentation needs no edit.
The [implementation plan](../../../plans/notification-permission-layout/plan.md) owns delivery evidence.
