---
id: plugins-runtime-response-preservation-design
title: Runtime response preservation
status: draft
system: plugins
owners:
  - kandev
created: 2026-09-21
last_updated: 2026-09-21
requirements:
  - REQ-PLUGINS-ISOLATED-WEB-APPS-012
  - REQ-PLUGINS-ISOLATED-WEB-APPS-013
---

# Runtime response preservation

This design extends [same-origin runtime transport](isolated-web-app-contributions.md#same-origin-transport-and-trust).
Requirement 013 owns proxy-compatible response delivery. Requirement 012 retains
startup failure detection. This change adds no trust-boundary exception.

`setRuntimeHeaders` shall send `Cache-Control: no-store, no-transform` for
runtime documents, assets, and the host bootstrap. The directive applies to
served representations, including retained releases. Stored package bytes and
digests remain unchanged. Do not add `public` or weaken capability validation.
Cloudflare documents `no-transform` as preventing automatic beacon injection
([Web Analytics FAQ](https://developers.cloudflare.com/web-analytics/faq/)).
An explicit proxy rule can still require operator correction if the proxy
ignores origin directives. Kandev does not change external proxy settings.

Extend the cookie-authenticated HTTPS test proxy with optional HTML injection
that honors the response directive. Inject only into runtime HTML and preserve
streaming for events. A control that removes the directive must reproduce the
blocked-script startup failure. With the directive present, the real host must
reach Ready on desktop and phone. This fixture proves the HTTP contract, not
the configuration of a particular Cloudflare account.

See the [recovery work package](../../../plans/canvas-runtime-entry-recovery/plan.md).
