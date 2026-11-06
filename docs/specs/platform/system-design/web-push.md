---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-WEB-PUSH-001
  - REQ-PLATFORM-NOTIFICATIONS-001
---

# Browser Web Push System Design

## Purpose and boundaries

Platform owns browser push because this feature adds a notification transport,
device records, startup settings, and localized offline behavior. It reuses the
event selection and recipient rules in [Semantic Notifications](../requirements/notifications.md).
Browser and push-service behavior remains outside Kandev's control.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-WEB-PUSH-001` | Enrollment, delivery, and browser lifecycle |
| `REQ-PLATFORM-NOTIFICATIONS-001` | Event delivery and provider state |

## Components and responsibilities

- `WebPushSection` and `useWebPush` own the user-triggered browser permission,
  device status, enable, disable, refresh, and diagnostic actions.
- `push-api.ts` sends subscription and diagnostic requests through the existing
  web API client.
- `notifications/webpush` owns device validation, persistence operations,
  payload preparation, VAPID signing, and vendor delivery.
- The notification service owns recipient selection, provider event choices,
  and occurrence claims. The Web Push provider adapts selected events to push
  payloads.
- An owned bounded delivery queue separates event-bus callbacks from network
  I/O. It drains accepted jobs during normal shutdown.
- `sw/service-worker.ts` adapts browser events. `sw/sw-core.ts` owns safe
  routing, cache admission, push parsing, and notification suppression.
- The Vite service-worker plugin emits `/sw.js` and provides localized offline
  strings from the locale catalogs.
- The Go webapp serves the fixed worker, manifest, and offline page. The
  desktop native notification command accepts the shared permission event.

## Data and contracts

The API exposes the VAPID public key, current-user subscriptions, subscription
creation and deletion, and a diagnostic send. Device records belong to the
authenticated user. The backend stores VAPID keys and device subscriptions in
`webpush_vapid_keys` and `webpush_subscriptions`.

Provider event choices and delivery claims remain in the existing notification
tables. A push payload contains its title, body, event tag, same-origin target,
task ID, and session ID. It does not contain the conversation text.

`notifications.webPushSubject` and `notifications.webPushAllowedHosts` use the
typed startup configuration catalog. The sender accepts HTTPS destinations on
port 443 whose host matches a built-in or configured suffix. It rejects IP
literals, URL credentials, and fragments. It does not follow redirects.

## Control flow

### Device enrollment

`Enable action -> browser permission -> service-worker registration -> public-key request -> PushManager subscription -> subscription API -> user-owned device record -> Web Push provider appears in settings`

The page does not request permission during status detection. After a
successful subscription, the settings hook refreshes providers so users can
select events without reloading.

### Event delivery

`Domain occurrence -> notification service recipient and provider selection -> delivery claim -> Web Push provider -> bounded queue -> HTTPSender -> push service -> browser worker -> notification`

The notification service keeps its current provider and event ownership. Each
queue job has a bounded timeout. The queue has four workers and a capacity of
16. If admission fails, provider dispatch reports an error so the existing
delivery claim can be released. A hard process stop can lose queued jobs; the
queue does not add durable delivery state.

The sender counts accepted messages as delivered. It removes subscriptions
whose push service returns 404 or 410 and does not count those responses as
successful delivery.

### Logout and notification tap

`Logout -> remove current browser subscription -> unsubscribe browser device -> continue logout`

Server deletion has a short timeout and does not block logout. The browser
subscription is also removed locally. On a push tap, the worker rejects
external or malformed targets, prefers a window on the target, and otherwise
uses an open window or opens the same-origin target. It suppresses a push only
when a focused visible window already shows the same task.

## Failure and recovery

Denied permission and unsupported browsers produce localized settings status.
A failed provider-list refresh does not undo successful device enrollment.
Expired endpoints are deleted so later sends do not retry them.

The worker caches static resources only. It rejects redirects, opaque responses,
and HTML for normal assets. The offline page is the only precached HTML. A
failed network navigation uses that localized page; the worker does not store
the authenticated app shell.

## Persistence

VAPID keys and user-owned subscriptions survive backend restarts. The browser
owns its local `PushSubscription`; logout removes that local state and attempts
to remove the corresponding server record. Provider event choices and delivery
claims remain in their existing stores.

The delivery queue is in memory. Normal shutdown drains accepted jobs. Existing
notification occurrence claims remain the authority for provider
deduplication.

## Security

Subscription validation and the sender both enforce the destination allowlist.
The HTTP client refuses redirects, so a valid push host cannot redirect a
request to another destination. Operator-configured suffixes are trusted
deployment configuration.

The worker accepts only origin-relative paths without control characters,
backslashes, or protocol-relative prefixes. It does not intercept API,
WebSocket, debug, or MCP traffic. The backend stores encryption and VAPID
secrets; API responses do not expose them.
