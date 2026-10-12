---
id: "02-proxy-verification"
title: "Verify authenticated proxy behavior and document trust"
status: done
wave: 2
depends_on:
  - "01-runtime"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-ISOLATED-WEB-APPS-013
acceptance_criteria:
  - AC-PLUGINS-ISOLATED-WEB-APPS-013.2
  - AC-PLUGINS-ISOLATED-WEB-APPS-013.3
  - AC-PLUGINS-ISOLATED-WEB-APPS-013.4
  - AC-PLUGINS-ISOLATED-WEB-APPS-013.5
  - AC-PLUGINS-ISOLATED-WEB-APPS-013.6
  - AC-PLUGINS-ISOLATED-WEB-APPS-013.7
system_design:
  - ../../specs/plugins/system-design/isolated-web-app-contributions.md
---

# Task 02: Verify authenticated proxy behavior and document trust

## Summary

Prove that the real canvas host works through a cookie-authenticated HTTPS proxy.
Update authoring and operator guidance to describe the accepted trust boundary.

## In scope

- Reuse the TLS alias fixture pattern with a cookie gate and public Host forwarding.
- Mount the actual canvas host with the real backend; test retained default-fetch source.
- Observe cookie delivery for assets, bootstrap context, data, permitted writes, and SSE.
- Cover missing/expired cookie failure, restoration and Retry, denied writes, and invalid tokens.
- Add phone coverage using the shared proxy fixture and existing focused canvas route.
- Update embedded authoring guidance, public canvas/security/authoring docs, and relevant comments.
- Explain that Cloudflare RUM or other incompatible HTML injection must be excluded on runtime paths.

## Out of scope

- Changing user Cloudflare policies or collecting real cookie/token values in logs.
- Suppressing injected-script failures or allowing analytics origins in CSP.
- New dialogs, warning copy, mode controls, or mobile layout changes.

## Acceptance

1. Desktop and phone hosts reach Ready and display data through the cookie gate; desktop writes and events retain backend validation.
2. Cookie expiry fails recoverably; after restoring authentication, Retry succeeds without republishing or changing stored release bytes.
3. Authoring and public docs accurately describe same-origin trust, capability limits, proxy requirements, and retained-source compatibility.

## ASCII UI preview

UI-01, excerpt from [the plan](plan.md#ascii-ui-preview):

```text
Desktop: existing canvas tab        Phone: existing focused route
[Title] [Releases] [Share]           [Back] [Title] [...]
[Ready]                            [Ready]
[Canvas content]                   [Canvas content]

Expired authentication, either viewport:
[Canvas unavailable] [Try again]
```

Use the existing scroll owner, safe-area behavior, actions, and translations.
Maps to criteria 013.2 and 013.6. No new rendered composition is required.

## Verification

Create a regression test that fails against the previous opaque runtime.
Use a temporary isolated checkout/build if needed; do not revert shared files.
Run desktop and mobile sequentially. The managed runner builds fresh artifacts.

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/canvas/canvas-authenticated-proxy.spec.ts tests/canvas/canvas-host-origins.spec.ts tests/canvas/plugin-canvas.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/canvas/mobile-canvas-authenticated-proxy.spec.ts)
(cd apps/backend && go test ./internal/mcp/canvasskill ./internal/backendapp)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Name the new browser scenarios `loads a retained canvas through cookie authentication`,
`recovers after proxy authentication expires`, and `preserves capability checks with cookies`.
Do not stub event success: forward the SSE stream and observe a real authorized event.
Verify a rejected ordinary API operation using existing authentication fixtures if
backend validation coverage does not already establish that behavior.

## Files likely touched

- `apps/web/e2e/tests/canvas/canvas-origin-fixture.ts` or a narrow shared proxy fixture beside it.
- New `canvas-authenticated-proxy.spec.ts` and `mobile-canvas-authenticated-proxy.spec.ts` in that directory.
- `apps/backend/internal/mcp/canvasskill/files/SKILL.md` and relevant `references/` files.
- `apps/backend/internal/mcp/canvasskill/bundled_test.go`.
- `apps/backend/internal/backendapp/canvas_authoring_scaffold_test.go`.
- `docs/public/canvases.md`, `docs/public/plugins-authoring.md`, and `docs/public/security.md`.
- Existing plan result sections and the two affected system designs when implementation is complete.

## Dependencies

Task 01. Public docs must ship with the behavior change, not in a later release.
Search root README and screenshot documentation for the same trust claims;
change them only when they contain an affected claim.

## Risks

The current origin fixture drops real SSE coverage. Reusing it unchanged would
produce incomplete evidence. Browser cookie handling must not be bypassed by
Node-side requests that synthesize authentication.

## Parallelism

`sequential`

## Inputs

- [Runtime design](../../specs/plugins/system-design/isolated-web-app-contributions.md#same-origin-transport-and-trust).
- Existing `canvas-host-origins.spec.ts`, `plugin-canvas.spec.ts`, and phone canvas fixtures.
- `/e2e`, `/mobile-parity`, and `/docs-maintainer` skills.

## Results

Implemented the cookie-authenticated HTTPS proxy fixture and desktop/mobile
coverage against the real canvas host. The fixture forwards runtime HTML,
assets, context, task data, writes, and SSE through a Secure, HttpOnly,
SameSite cookie gate while recording only redacted request observations.
Coverage includes expiry and Retry, permitted writes and events, invalid
capabilities, and the mobile retained-canvas flow.

Updated bundled authoring and security guidance plus the public canvas,
security, plugin, and authoring documentation for trusted same-origin canvases.
The browser coverage exposed an action route parsing defect, so the protocol
handler now validates the `v1/actions/<key>` path before checking its
capability permission.

Checks passed:

- `go test ./internal/plugins ./internal/plugins/webapp ./internal/mcp/canvasskill ./internal/backendapp`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- Desktop authenticated proxy, origin, and lifecycle browser coverage.
- Mobile authenticated proxy and lifecycle browser coverage.

Review remediation now tracks every outstanding upstream request in the
authenticated proxy. Incoming aborts and downstream close/error events destroy
the upstream request, upstream response failures close the downstream stream,
and proxy shutdown closes all remaining upstream requests. The desktop proxy
suite includes a deterministic disconnect and reconnect scenario that observed
the active upstream count return to zero before opening the stream again.

The refreshed authenticated-proxy checks passed: four desktop cases and two
mobile cases, including the disconnect/reconnect and expiry/Retry scenarios.

Cloudflare Access exclusions and the live operator retest remain deployment
actions outside this repository.

## Following toolbar integration

Task 03 replaces the existing visible status strip with body states and an
accessible Ready announcement. Its final verification reruns the proxy tests.
Use startup and rendered-data outcomes rather than requiring a second toolbar.
See [UI-01](plan.md#ascii-ui-preview) for the final composition.
