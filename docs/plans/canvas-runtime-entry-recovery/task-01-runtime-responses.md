---
id: "01-runtime-responses"
title: "Preserve runtime responses through proxies"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-ISOLATED-WEB-APPS-012
  - REQ-PLUGINS-ISOLATED-WEB-APPS-013
acceptance_criteria:
  - AC-PLUGINS-ISOLATED-WEB-APPS-012.1
  - AC-PLUGINS-ISOLATED-WEB-APPS-013.2
  - AC-PLUGINS-ISOLATED-WEB-APPS-013.8
system_design:
  - ../../specs/plugins/system-design/isolated-web-app-contributions.md
  - ../../specs/plugins/system-design/runtime-response-preservation.md
---

# Task 01: Preserve runtime responses through proxies

## Summary

Add `no-transform` to the existing no-store runtime policy. Prove that a
directive-aware proxy preserves working retained canvases on desktop and phone.

## In scope

- Write `TestRuntimeNoTransformHeaders` first. It must fail because current
  entry, asset, and bootstrap responses lack the directive.
- Change `setRuntimeHeaders` to send `no-store, no-transform`.
- Extend `canvas-authenticated-proxy-fixture.ts` with optional runtime-HTML
  injection. Respect the parsed directive, buffer only HTML, and update length.
  Forward cookies and preserve real SSE streaming and cancellation.
- Add a negative control that removes the directive and injects a blocked
  external script before startup settles. It must produce the unavailable state.
- Add desktop and phone positive cases that reach Ready and read real scoped
  task data with injection enabled but prevented by the directive.
- Document runtime-path analytics exclusion and Retry in `docs/public/canvases.md`.
  Describe proxy overrides as an operational limitation, not a missing release.

## Out of scope

CSP changes, error suppression, Cloudflare account mutations, release republishing,
and new browser UI are excluded.

## Acceptance

1. All successful runtime documents, assets, and bootstrap responses contain
   both directives. Retained package bytes and digests remain unchanged.
2. Positive proxy cases reach Ready on desktop and phone. The negative control
   still fails through the existing recoverable state.
3. Existing authentication, expired-token, denied-write, and event-stream
   scenarios in the affected suites continue to pass.

## Verification

Run from the repository root. Install workspace dependencies before pnpm commands
if this worktree has no install. Managed E2E commands rebuild required artifacts.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/plugins/webapp -count=1)
(cd apps/web && pnpm exec vitest run components/plugins/web-app-frame.test.tsx)
(cd apps/web && pnpm e2e:run --project chromium tests/canvas/canvas-authenticated-proxy.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/canvas/mobile-canvas-authenticated-proxy.spec.ts)
git diff --check
```

## Files likely touched

- `apps/backend/internal/plugins/webapp/runtime.go`
- `apps/backend/internal/plugins/webapp/runtime_test.go`
- `apps/web/e2e/tests/canvas/canvas-authenticated-proxy-fixture.ts`
- `apps/web/e2e/tests/canvas/canvas-authenticated-proxy.spec.ts`
- `apps/web/e2e/tests/canvas/mobile-canvas-authenticated-proxy.spec.ts`
- `docs/public/canvases.md`

## Dependencies

None.

## Risks

The fixture demonstrates a conforming proxy, not every Cloudflare account setting.
Do not add `public` from the vendor example to capability-bound runtime responses.

## Parallelism

`sequential`

## Inputs

- Plugin runtime design: Runtime startup protocol and Same-origin transport and trust.
- Existing runtime header tests and authenticated-proxy fixture.
- [Plan evidence and scope](plan.md).

## Results

Implemented the shared runtime response policy and the directive-aware
authenticated proxy fixture. Runtime documents, packaged assets, host
bootstrap responses, and protocol responses now retain `no-store` and add
`no-transform`. The positive proxy cases exercise injection attempts that are
blocked by the origin response, while the negative control removes the
directive and preserves the existing recoverable startup failure.

Recorded verification:

- `cd apps/backend && go test ./internal/plugins/webapp -count=1` passed.
- `pnpm e2e:run --project chromium tests/canvas/canvas-authenticated-proxy.spec.ts` passed, 5 tests.
- `pnpm e2e:run --project mobile-chrome tests/canvas/mobile-canvas-authenticated-proxy.spec.ts` passed, 2 tests.
- `make -C apps/backend build` passed.
