---
created: 2026-09-19
status: done
requirements:
  - REQ-PLUGINS-ISOLATED-WEB-APPS-001
  - REQ-PLUGINS-ISOLATED-WEB-APPS-003
  - REQ-PLUGINS-ISOLATED-WEB-APPS-007
  - REQ-PLUGINS-ISOLATED-WEB-APPS-012
  - REQ-PLUGINS-ISOLATED-WEB-APPS-013
  - REQ-UI-PANEL-TOOLBARS-001
  - REQ-CANVASES-AGENT-WEB-APPS-006
  - REQ-CANVASES-AGENT-WEB-APPS-007
system_design:
  - ../../specs/plugins/system-design/isolated-web-app-contributions.md
  - ../../specs/ui/system-design/panel-toolbars.md
  - ../../specs/canvases/system-design/canvas-host-chrome.md
legacy_specs: []
---

# Implementation Plan: Canvas authentication and consistent panel toolbars

## Overview

Allow a trusted canvas to use the viewing browser's reverse-proxy login cookie.
Change both sandbox declarations and startup credentials in one vertical slice.
Then prove browser behavior through a cookie-gated HTTPS proxy and update public guidance.
Finally consolidate canvas chrome and migrate inconsistent primary panel toolbars.

The user selected this trust model. Backend validation remains authoritative,
but canvas grants do not constrain direct user-session API calls or parent DOM access.
See the [decision](../../decisions/2026-09-19-trusted-same-origin-canvases.md).

## Scope

### In scope

- Shared `ui.web_apps` runtime, including task, workspace, imported, and installed-plugin canvases.
- Same-origin iframe and response CSP, plus cookie-capable startup.
- Existing capability, grant, input-validation, and exact-origin checks.
- Retained release compatibility and unchanged persisted artifact bytes.
- Desktop and phone tests through a cookie-authenticated proxy.
- Authoring and operator documentation for the changed trust boundary.
- Shared fixed-height panel toolbars, caller audit, and a single canvas host header.

### Out of scope

- A parent-page bridge, separate hostname, new runtime mode, or feature toggle.
- Changes to manifest grants, backend domain validation, or token lifetime.
- Native plugin bundles, arbitrary preview iframes, and unrelated CORS changes.
- Automatic Cloudflare account changes or enabling remote analytics scripts.
- Rewriting explicit `credentials: "omit"` in retained application source.
- A promise that same-origin code remains limited to its canvas grants.

## Technical approach

`WebAppFrame` and `BuildContentSecurityPolicy` must both include
`allow-same-origin`. The injected `hostRuntimeBootstrap` must fetch context with
`credentials: "same-origin"`. Ordinary relative fetches, assets, XHR, and
EventSource requests then use same-origin browser cookie behavior.

`Runtime.Serve`, its binding validator, and the protocol handlers remain the
capability gate. Preserve the restricted opaque-origin CORS fallback for older
or distinct-origin callers. Same-origin requests need no wildcard CORS change.
Do not add `Access-Control-Allow-Credentials: true` for `Origin: null`.

The proxy test must require a Secure, HttpOnly, SameSite=Lax cookie for runtime
HTML, assets, context, data, writes, and events. Forward the public Host and
browser-supplied cookies. Do not fabricate missing cookies or replace backend
responses with successful stubs. A live browser must observe Ready and data.

Use the existing HTTPS alias fixture as the pattern. It currently buffers SSE
and substitutes an empty stream; the new coverage needs real event forwarding.
Its current virtual wrapper does not mount `WebAppFrame`. New tests must open
the real canvas host, not only that wrapper, to test both sandbox declarations.

Cookie expiry must produce a recoverable startup failure. Restoring the cookie
and choosing Retry must reach Ready with a fresh runtime. A valid cookie with
an invalid capability must still fail at Kandev. A denied capability write must
not mutate data. Verify a direct ordinary API denial under Kandev authentication
using the existing auth test patterns; do not claim canvas grants govern it.

Keep Cloudflare RUM disabled on runtime responses for the final operator smoke
test. An injected remote script can still fail the CSP and startup monitor.
Preserve that failure rather than suppressing it.

## ASCII UI preview

UI-01: Final canvas host and neighboring panel. Maps to
`AC-PLUGINS-ISOLATED-WEB-APPS-013.2/.6`, `AC-UI-PANEL-TOOLBARS-001.1-.6`,
and `AC-CANVASES-AGENT-WEB-APPS-006.10/.11`.

```text
Desktop: adjacent task panels, below unchanged Dockview tabs
+---------------------------------------+--------------------------+
| Coordinator   [Releases] [Share] [...] | [Diff] [Review] [...]     | 30px
+---------------------------------------+--------------------------+
| Canvas content OR                     | Changes content          |
| Canvas unavailable                    |                          |
| [Try again]                           |                          |
+---------------------------------------+--------------------------+

Phone: focused standalone canvas, one page header
+----------------------------------+
| Back   Coordinator          [...]| navigation + >=44px targets
+----------------------------------+
| Canvas content OR                |
| Canvas unavailable               |
| [Try again]                      |
+----------------------------------+
[...] opens existing inset actions/picker drawer.
```

The shared panel toolbar is 30px on fine-pointer desktop and 48px in touch
contexts. Standalone page chrome keeps its own geometry. Ready has no visible
status row; application content and a polite accessible announcement convey it.
A narrow panel uses overflow before actions wrap. Blocking status appears once
in the body; nonblocking connection state may appear inline in the header.
The body owns content scrolling. Preserve safe areas and focus return.

UI owns reusable toolbar geometry; Canvases owns its single-header state
presentation. Extend existing `panel-primitives.tsx`, with no competing toolbar.
The source audit and exclusions are in the
[toolbar design](../../specs/ui/system-design/panel-toolbars.md).
Task 03 must repeat the audit and test geometry, overflow, and retained actions.

## Tests

| Criteria | Evidence |
| --- | --- |
| 001.2/.3, 007.1/.7, 013.1 | `web-app-frame.test.tsx`, `policy_test.go`, `runtime_test.go`; assert exact sandbox tokens and startup credentials |
| 003.1/.2/.5, 007.2/.3, 013.3 | Runtime and protocol tests with valid cookies plus invalid capabilities, denied permissions, and invalid write bodies |
| 012.1-.4, 013.5/.6 | Existing startup/component tests, immutable artifact checks, cookie-expiry and Retry E2E |
| 013.4 | Bundled authoring tests and public-doc validation; remove obsolete isolation claims |
| 013.2/.7 | Cookie-gated browser data, permitted write, real event, and phone tests |

New test names are defined by the work orders. Existing suites remain regression
evidence; do not relabel old opaque-origin results as proof of the new model.

## E2E tests

- `tests/panel-toolbars.spec.ts`: adjacent panels, all migrated header families,
  narrow panels, long labels, multiple PRs, editor states, and single canvas header.
- `tests/mobile-panel-toolbars.spec.ts`: phone actions and Retry, responsive
  boundary styles, coarse-pointer targets, containment, and drawer focus return.

- `tests/canvas/canvas-authenticated-proxy.spec.ts`: real host startup, CSS/JS,
  task data, allowed write, event delivery, cookie expiry, and Retry.
- `tests/canvas/mobile-canvas-authenticated-proxy.spec.ts`: phone startup, data,
  and Retry through the same cookie gate.
- `tests/canvas/canvas-host-origins.spec.ts`: exact aliases and foreign ancestors.
- `tests/canvas/plugin-canvas.spec.ts`: existing direct/task-host lifecycle and Retry.

The fixture must expose scoped cookie-gate counters or request observations
without recording cookie values or capability URLs. No Cloudflare account is
needed for CI. Record a separate real-installation retest when credentials and
operator configuration are available; local proxy results do not prove it.

## Related implementation records

This package supersedes opaque-origin assertions in the completed
[initial runtime work](../plugin-backed-canvases/task-03-isolated-browser-runtime.md)
and the corresponding authoring/documentation work. The
[startup work](../canvas-runtime-permission-fixes/task-03-runtime-startup.md)
keeps its handshake and failure behavior; only the trust and credential
assumptions change. Historical verification results remain historical.

## Work orders

- [x] [Task 01: Enable trusted same-origin runtime requests](task-01-runtime.md)
- [x] [Task 02: Verify authenticated proxy behavior and document trust](task-02-proxy-verification.md)

- [x] [Task 03: Unify panel toolbars and canvas chrome](task-03-panel-toolbars.md)

Run 01, 02, then 03 sequentially. No subagents are authorized.

## Verification results

Design-package checks passed: specification catalog validation, specification
lint, all 36 specification-linter tests, and `git diff --check`.
Implementation checks passed: focused backend package tests, broad frontend
Vitest coverage, frontend typecheck and lint, the internationalization gate,
production web and backend builds, public-document validation, and the final
specification catalog and lint checks.
Browser checks passed: desktop authenticated proxy, exact-origin, canvas
lifecycle, and panel-toolbar coverage with 11 passed tests; mobile authenticated
proxy, canvas lifecycle, and panel-toolbar coverage with 7 passed tests.
The review-remediation rerun added width-aware canvas, Changes, Browser, and
editor action coverage. It passed 14 desktop browser cases and 7 mobile cases,
including the deterministic proxy disconnect/reconnect check, multiple-PR
review overflow, fine-pointer phone containment, and dirty-editor overflow.
The live Cloudflare Access configuration and operator browser retest remain
external deployment work.

## Risks

- Same-origin code can act as the viewing user and change host UI or storage.
- Previously published and imported packages gain that authority when opened.
- Auth-disabled installations retain their existing broad backend authority.
- Capability revocation cannot retract parent callbacks or storage mutations.
- A proxy that rewrites public Host can cause Origin checks to reject writes.
- Cloudflare HTML injection and expired login sessions remain separate failure causes.
- Explicit credential omission in authored packages still needs republishing.
- Later restoration of opaque isolation can break code relying on host access.

Toolbar risks: narrow action groups can overflow, touch targets can be clipped,
and removing the status row can break old selectors or live announcements.
Task 03 covers these regressions without changing runtime authority.
