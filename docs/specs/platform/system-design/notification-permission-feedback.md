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
Its actions remain in the `SettingsGroup` header.
Resolve one warning value inside the component with the existing translation keys.
Use `SettingsGroup` with `frame="card"` only when that warning exists.
Otherwise use its existing `frame="none"` path, with no warning paragraph, and hide
that path's empty content element so it contributes no reserved inter-section space.
This path has no border, background, vertical content padding, or reserved warning space.
The page retains its normal spacing between sections.

Keep the shared `SettingsGroup` implementation unchanged.
Its other consumers can intentionally render empty or loading content.
React child counts cannot reliably identify visible content because conditional children can evaluate to false.

## Permission states

| State | Warning | Frame |
| --- | --- | --- |
| `default` | None | None |
| `granted` | None | None |
| `denied`, native available | Existing operating-system warning | Card |
| `denied`, native unavailable | Existing browser warning | Card |
| `unsupported` | Existing unsupported warning | Card |
| `error` | Existing permission-check warning | Card |

Keep `useNotificationPermission`, action handlers, permission requests, and native detection behavior intact.
Refresh changes the same permission state and therefore the warning value and frame together.
Retain every existing button label, handler, and disabled condition.
No new copy, persistence, API, telemetry, or permission prompt is required.

## Responsive composition

The entry point is Settings > Notifications on both desktop and phone.
The nearest shipped settings exemplar is `NotificationSoundSection`, with the same group header and inline body.
The curated mobile precedent is direct navigation to one focused page, rather than an additional overlay.
Desktop keeps the heading and actions in the existing header row.
Phone keeps actions below the heading using `SettingsGroupHeader`'s existing stack below `md`.
The warning follows the header and wraps inside the existing card.
The page retains one scroll owner and its current safe-area handling.
No new fixed controls or touch targets are introduced.
Permission state and warning selection remain shared across viewports.

## Verification

Extend `notification-permission-section.test.tsx` through the real `SettingsGroup`.
Cover every table row and transitions from warning to normal state and back.
Assert the warning text, frame presence, and existing action behavior.
Do not use a class-string assertion as the only evidence.

Add focused permission-layout cases to the existing desktop and phone notification type-scale Playwright specs.
Set browser permission deterministically before navigation without opening an operating-system permission prompt.
Cover a normal state, a denied warning, and refresh back to granted.
Assert computed zero vertical padding for the normal content, a hidden empty body,
a normal inter-section gap, and no warning card.
Assert warning wrapping, action reachability, and no document horizontal overflow.
Capture desktop and phone results through the existing `prCapture` fixture.
Component tests cover native denial and check-error cases that need transport stubs.

## Documentation and delivery

Public instructions, terminology, and notification policy remain accurate.
This package changes internal layout intent only, so public documentation needs no edit.
The [implementation plan](../../../plans/notification-permission-layout/plan.md) owns delivery evidence.
