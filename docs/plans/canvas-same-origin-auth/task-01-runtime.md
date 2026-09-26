---
id: "01-runtime"
title: "Enable trusted same-origin runtime requests"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-ISOLATED-WEB-APPS-001
  - REQ-PLUGINS-ISOLATED-WEB-APPS-003
  - REQ-PLUGINS-ISOLATED-WEB-APPS-007
  - REQ-PLUGINS-ISOLATED-WEB-APPS-012
  - REQ-PLUGINS-ISOLATED-WEB-APPS-013
acceptance_criteria:
  - AC-PLUGINS-ISOLATED-WEB-APPS-001.2
  - AC-PLUGINS-ISOLATED-WEB-APPS-001.3
  - AC-PLUGINS-ISOLATED-WEB-APPS-003.1
  - AC-PLUGINS-ISOLATED-WEB-APPS-003.2
  - AC-PLUGINS-ISOLATED-WEB-APPS-003.5
  - AC-PLUGINS-ISOLATED-WEB-APPS-007.1
  - AC-PLUGINS-ISOLATED-WEB-APPS-007.2
  - AC-PLUGINS-ISOLATED-WEB-APPS-007.3
  - AC-PLUGINS-ISOLATED-WEB-APPS-007.7
  - AC-PLUGINS-ISOLATED-WEB-APPS-012.1
  - AC-PLUGINS-ISOLATED-WEB-APPS-012.2
  - AC-PLUGINS-ISOLATED-WEB-APPS-012.3
  - AC-PLUGINS-ISOLATED-WEB-APPS-012.4
  - AC-PLUGINS-ISOLATED-WEB-APPS-013.1
  - AC-PLUGINS-ISOLATED-WEB-APPS-013.3
  - AC-PLUGINS-ISOLATED-WEB-APPS-013.5
system_design:
  - ../../specs/plugins/system-design/isolated-web-app-contributions.md
---

# Task 01: Enable trusted same-origin runtime requests

## Summary

Update both browser sandbox declarations and the host-owned startup request.
Keep runtime capability validation and all ordinary backend checks effective.

## In scope

- Use TDD for exact iframe/CSP sandbox token sets and startup credential behavior.
- Add `allow-same-origin` to `WebAppFrame` and `BuildContentSecurityPolicy` together.
- Use `credentials: "same-origin"` in `hostRuntimeBootstrap.checkContext`.
- Replace comments and assertions that describe an opaque browser boundary.
- Test cookies alongside invalid, expired, and stale tokens and denied writes.
- Preserve immutable artifacts, startup messages, CSP sources, and exact ancestors.

## Out of scope

- Broad CORS changes, cross-origin cookie forwarding, fetch interception, and a bridge.
- Editing retained canvas packages or adding a mode/feature toggle.
- New UI composition and Cloudflare account changes.

## Acceptance

1. Framed and top-level runtime documents retain their served origin; both policy layers agree.
2. Startup uses same-origin cookies; a cookie cannot replace a token or bypass capability grant/input validation.
3. Existing startup, retry, source validation, and artifact immutability tests pass with the revised trust assertions.

## Verification

Run focused RED tests before changing production code. Do not weaken unrelated assertions.
Run each command from the repository root:

```bash
(cd apps/backend && go test ./internal/plugins/webapp ./internal/plugins ./internal/backendapp)
(cd apps/web && pnpm exec vitest run components/plugins/web-app-frame.test.tsx components/plugins/web-app-startup.test.ts components/plugins/canvas-page.test.tsx components/settings/canvas-host-route.test.tsx components/settings/canvas-host-components.test.tsx)
(cd apps/web && pnpm run typecheck)
git diff --check
```

Add `TestRuntimeCookieDoesNotReplaceCapability` and a protocol test named
`TestWebAppProtocolCookieDoesNotExpandPermissions`. Extend the existing policy
test to compare the parsed sandbox token set, rather than a substring that
would also pass with unintended permissions.

## Files likely touched

- `apps/web/components/plugins/web-app-frame.tsx` and its component test.
- `apps/backend/internal/plugins/webapp/policy.go` and `policy_test.go`.
- `apps/backend/internal/plugins/webapp/runtime_bootstrap.go` and `runtime_test.go`.
- `apps/backend/internal/plugins/webapp_protocol_test.go`.
- `apps/backend/internal/backendapp/middleware_test.go`, if same-origin request coverage is missing.

## Dependencies

None. Read scoped backend and web `AGENTS.md` before implementation.

## Risks

Same-origin code gains ordinary user-session authority. Capability tests must
not falsely assert that the runtime limits every possible action of that code.

## Parallelism

`sequential`

## Inputs

- [Runtime design](../../specs/plugins/system-design/isolated-web-app-contributions.md#same-origin-transport-and-trust).
- [Decision](../../decisions/2026-09-19-trusted-same-origin-canvases.md).
- Existing `Runtime.Serve`, `corsMiddleware`, and `handleWebAppProtocol` tests.

## Results

Implemented trusted same-origin runtime transport. The iframe and runtime
response CSP both declare `allow-same-origin`, and the host startup bootstrap
uses `credentials: "same-origin"` for its relative context request. Runtime
token validation, scope checks, protocol grants, and input validation remain
unchanged. Added regressions for the exact CSP sandbox token set, startup
credentials, cookie-plus-capability behavior, and cookie-plus-protocol
permissions.

Checks passed:

- `go test ./internal/plugins/webapp ./internal/plugins ./internal/backendapp`
- `pnpm exec vitest run components/plugins/web-app-frame.test.tsx components/plugins/web-app-startup.test.ts components/plugins/canvas-page.test.tsx components/settings/canvas-host-route.test.tsx components/settings/canvas-host-components.test.tsx`
- `pnpm run typecheck`
- `git diff --check`
