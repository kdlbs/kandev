---
status: draft
system: platform
created: 2026-10-22
owners:
  - kandev
---

# Web asset delivery requirements

## Overview

Browsers should receive compressed application assets when they support them.
This reduces cold-load transfer without changing the application or requiring
an operator to configure a reverse proxy. Platform owns the native web server;
UI retains component loading and editor lifecycle ownership.

## Requirements

### REQ-PLATFORM-WEB-ASSET-DELIVERY-001: Equivalent compressed assets

**Intent:** Reduce transfer for immutable JavaScript and CSS assets while
preserving the bytes that the browser executes and renders.

#### Acceptance criteria

- **AC-PLATFORM-WEB-ASSET-DELIVERY-001.1:** When a supported compressed
  representation is available and acceptable to the client, the server shall
  serve it at the original asset URL with correct encoding, media type, and
  cache variation headers. Decoding shall produce the original asset bytes.
- **AC-PLATFORM-WEB-ASSET-DELIVERY-001.2:** Clients that request identity, or
  installations without compressed representations, shall retain working asset
  delivery when identity is acceptable. Explicitly excluded encodings shall
  never be selected. If no available representation is acceptable, the server
  shall return a negotiation failure.
- **AC-PLATFORM-WEB-ASSET-DELIVERY-001.3:** GET, HEAD, conditional requests,
  and byte-range responses shall describe the selected representation
  consistently. Cached variants shall not be confused with one another.
- **AC-PLATFORM-WEB-ASSET-DELIVERY-001.4:** Production asset bundles used by
  native launch and packaged runtimes shall include available compressed
  representations. Asset optimization shall preserve the private boot shell,
  API, WebSocket, development-server, and desktop navigation behavior.

## Out of scope

Dynamic API compression, private boot-payload caching, proxy configuration,
new user settings, content changes, and a new asset hosting service.

## Implementation plans

- [Asset delivery extraction](../../../plans/asset-delivery-extraction/plan.md)
