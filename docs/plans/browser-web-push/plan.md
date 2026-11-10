---
status: done
requirements:
  - REQ-PLATFORM-WEB-PUSH-001
  - REQ-PLATFORM-NOTIFICATIONS-001
system_design:
  - ../../specs/platform/system-design/web-push.md
---

# Browser Web Push

## Outcome

Add browser push as a transport for the existing notification provider model.
The browser owns permission and its local subscription. The backend owns the
user-linked device record, provider selection, and delivery claim. The worker
shows safe same-origin notifications and a localized offline page.

## Work order

- [Task 01: Browser push delivery](task-01-browser-push-delivery.md)

## Requirements and design

- [Browser Web Push requirements](../../specs/platform/requirements/web-push.md)
- [Semantic Notifications requirements](../../specs/platform/requirements/notifications.md)
- [Browser Web Push design](../../specs/platform/system-design/web-push.md)

## UI preview

`UI-01` shows the shared Notifications settings group. On phones, the settings
page scrolls vertically and the actions remain touch-operable.

```text
Notifications
┌ Web Push ─────────────────────────────┐
│ Device status                         │
│ [Disable] [Test] [Refresh]            │
└───────────────────────────────────────┘
┌ Notification events ─────────────────┐
│ Agent turn finished        Web Push □ │
│ Agent needs your answer    Web Push □ │
└───────────────────────────────────────┘
```

The preview shows structure only. Existing localized labels and provider
selection remain authoritative.

## Delivery record

The implementation and review fixes are recorded in Task 01. Local checks pass.
Hosted CI remains the merge gate.
