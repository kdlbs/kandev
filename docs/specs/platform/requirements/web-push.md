---
status: draft
system: platform
created: 2026-10-30
owners:
  - kandev
---

# Browser Web Push Requirements

## Overview

Browser Web Push delivers selected Kandev notifications when a browser page is
not open. Platform owns this contract because it covers notification delivery,
startup configuration, and localization. It uses the shared event and provider
rules in [Semantic Notifications](notifications.md).

## Requirements

### REQ-PLATFORM-WEB-PUSH-001: Browser push delivery

**Intent:** Users can receive selected notifications on their browser devices
without keeping a Kandev page open.

#### Acceptance criteria

- **AC-PLATFORM-WEB-PUSH-001.1:** When a user enables Web Push, the browser
  shall request permission only from that action and register the subscription
  to that user.
- **AC-PLATFORM-WEB-PUSH-001.2:** When an event is selected for Web Push,
  Kandev shall deliver it to that user's active subscriptions under the
  existing recipient and occurrence-deduplication rules.
- **AC-PLATFORM-WEB-PUSH-001.3:** When a user disables Web Push or logs out,
  the browser shall unsubscribe its current device and attempt server deletion.
  Server deletion failure shall not block logout.
- **AC-PLATFORM-WEB-PUSH-001.4:** Before Kandev sends a push request, its host
  shall match a built-in push-service suffix or an operator-configured suffix.
  Kandev shall not follow redirects.
- **AC-PLATFORM-WEB-PUSH-001.5:** When a push endpoint returns 404 or 410,
  Kandev shall remove the endpoint and shall not count that send as delivered.
- **AC-PLATFORM-WEB-PUSH-001.6:** When push data contains a destination, the
  worker shall accept only a sanitized same-origin path and use it after a
  notification tap.
- **AC-PLATFORM-WEB-PUSH-001.7:** When the app is offline, the worker shall
  show a localized offline page. It shall not cache API, WebSocket, or
  authenticated app HTML as an asset.

## Out of scope

- Offline task editing or cached task data.
- Changes to provider-independent notification event selection.
- A separate native desktop notification provider.
