# ADR-2026-09-19-trusted-same-origin-canvases: Trust same-origin canvas code

**Status:** accepted
**Date:** 2026-09-19
**Area:** frontend, backend, protocol, security

## Context

Cloudflare Access protects a Kandev installation with a browser cookie.
The existing canvas document has an opaque origin, and its startup request
explicitly omits credentials. Cloudflare redirects that request to login.
The canvas CSP blocks the redirect, and the host reports an unavailable canvas.

The user selected same-origin canvases after reviewing the isolation tradeoffs.
They prefer a smaller initial implementation over a parent-page request bridge.
This choice changes the browser trust boundary, including retained and imported
packages. It is not only a transport correction.

## Decision

Treat packages executed by the shared web-application runtime as trusted code
with the viewing user's same-origin browser authority. Add `allow-same-origin`
to both the iframe sandbox and the runtime response CSP sandbox. Use
`credentials: "same-origin"` for the host-owned startup request.

Keep ordinary relative browser requests and the existing capability protocol.
Do not add a bridge, runtime mode, manifest switch, or credential-fetch shim.
Do not rewrite stored releases or override explicit authored credential omission.

The backend remains authoritative for authorization, domain validation, and
capability-protocol grants. A browser cookie does not replace a runtime token.
However, same-origin code can call ordinary APIs as the viewing user. Canvas
grants do not constrain those calls or protect the parent DOM and browser storage.

Keep the remaining declared CSP and sandbox restrictions, exact frame ancestors,
and existing backend checks. Do not grant credentialed CORS to arbitrary origins.
These policies do not contain trusted code that can access the parent document.

This decision amends the browser-isolation and credential portions of
[the original canvas decision](2026-08-26-plugin-backed-web-app-canvases.md).
Package validation, immutable releases, scope, grants, and distribution remain.
The [implementation plan](../plans/canvas-same-origin-auth/plan.md) is pending.

## Consequences

- Same-origin startup and default API requests can carry reverse-proxy cookies.
- Backend validation still rejects unauthorized or invalid requests.
- Canvas code can modify host DOM, access shared storage, and use user-session APIs.
- Capability revocation does not undo parent changes or stop all code already installed there.
- Imported and retained canvases must be trusted before they execute.
- Browser storage is available under normal browser rules, but is not a supported shared-state contract.
- Existing code that explicitly omits credentials needs an author change when a proxy requires cookies.
- Proxy analytics injection remains a separate deployment concern. Exclude incompatible injection on runtime responses.
- Distinct-origin desktop hosts keep their existing capability transport; this change does not forward cookies across origins.
- A later isolation design needs an explicit migration for packages that start depending on host access.

## Alternatives Considered

- **Narrow Cloudflare Access bypass:** Smallest deployment fix, but requires a special operator rule.
- **Credentialed opaque-origin requests:** Requires cookie and CORS changes, with browser-dependent cookie restrictions.
- **Parent-page request bridge:** Preserves host isolation but needs a new transport, asset loading, and compatibility design.
- **Separate canvas origin:** Preserves separation from the SPA, but adds deployment and authentication complexity.
- **Same-origin only in the iframe attribute:** Insufficient. The response CSP would retain the opaque sandbox, and startup would still omit credentials.

## Related contracts

- [Runtime requirements](../specs/plugins/requirements/isolated-web-app-contributions.md)
- [Runtime design](../specs/plugins/system-design/isolated-web-app-contributions.md#same-origin-transport-and-trust)
