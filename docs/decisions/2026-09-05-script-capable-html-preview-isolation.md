# ADR-2026-09-05-script-capable-html-preview-isolation: Capability-Free Runtime for Script-Capable HTML Preview

**Status:** proposed
**Date:** 2026-09-05
**Area:** frontend, desktop, security

## Context

HTML file preview was initially implemented with an opaque `srcDoc` iframe and
no script capability. The approved product contract requires inline JavaScript
for self-contained documents, while also requiring that untrusted preview code
cannot access Kandev authority, credentialed browser state, outbound network,
or preview-initiated navigation.

An iframe with `sandbox="allow-scripts"` does not satisfy that contract. The
script can navigate the iframe's own current navigable, and the browser does
not enforce the previously proposed `navigate-to` policy in the target
environment. Removing scripts satisfies isolation but changes the approved
feature. A separate origin alone does not solve browser navigation or network
egress either.

## Decision

Run preview source in a capability-free ECMAScript runtime inside a dedicated
Web Worker. The runtime owns a virtual DOM, bounded timers, event dispatch,
diagnostics, and preview-owned resource tokens. It receives only the current
HTML buffer and sanitized user events through a structured-clone message
protocol. It has no host imports and no access to browser globals, Kandev
stores, cookies, credentials, filesystem, network, storage, parent windows, or
navigation APIs.

Render the runtime's data-only virtual-DOM snapshots through a scriptless,
controlled preview surface. The renderer creates native nodes through an
allowlisted projection and filters static and dynamic resource URLs. It never
evaluates source code, inserts source-controlled executable markup, or grants
an iframe `allow-scripts` capability. Static link and meta-refresh
neutralization remains useful as input normalization, but the virtual DOM
mutation policy is authoritative for dynamically-created content.

The runtime enforces bounded execution, virtual heap, timer, event-queue, and
snapshot limits. It terminates on budget exhaustion or malformed messages and
reports a recoverable failure. Unsupported browser APIs fail closed. Only
`data:` resources and `blob:` values created by the preview runtime from
preview-owned bytes can reach the renderer.

## Consequences

- Inline JavaScript remains available without making the Kandev page a script
  host or trusting browser navigation/CSP behavior as the security boundary.
- The preview supports a deliberate virtual browser API surface, not every
  browser API. Unsupported APIs must fail closed and be documented.
- The feature requires a maintained and pinned ECMAScript engine, a virtual
  DOM implementation, a safe renderer, event bridging, resource filtering, and
  worker lifecycle controls.
- Bundle size, WebView worker support, script compatibility, rendering fidelity,
  and execution budgets become implementation risks that need targeted tests.
- The current static-only implementation and its inert-script assertions are
  an incomplete prototype. They are not evidence that the approved contract
  has shipped.

## Alternatives Considered

### Keep the static-only `srcDoc` preview

Rejected because it does not satisfy the approved inline-JavaScript contract.
It remains a useful fail-closed fallback during design work, but it cannot be
the final product behavior.

### Re-enable `allow-scripts` in the existing `srcDoc` iframe

Rejected because arbitrary scripts can self-navigate the frame and create
navigation or network requests. CSP `navigate-to` is not a reliable enforcement
mechanism in the target browsers.

### Use a separate origin with CSP or a service worker

Rejected as the sole boundary because a separate origin isolates credentials
but does not, by itself, prevent browser navigation or all client-side network
egress. The design needs an execution host with no ambient browser capabilities.

### Use a desktop-only native WebView or a separate browser process

Rejected as the primary contract because the web application and mobile web
surfaces need the same behavior. A native host may provide additional smoke
evidence or an optimized renderer later, but it cannot replace the
cross-surface capability-free runtime.
