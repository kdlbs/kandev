---
id: "03-runtime-startup"
title: "Detect canvas startup"
status: done
wave: 3
depends_on:
  - "02-initial-owner-publication"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-ISOLATED-WEB-APPS-012
  - REQ-CANVASES-AGENT-WEB-APPS-007
acceptance_criteria:
  - AC-PLUGINS-ISOLATED-WEB-APPS-012.1
  - AC-PLUGINS-ISOLATED-WEB-APPS-012.2
  - AC-PLUGINS-ISOLATED-WEB-APPS-012.3
  - AC-PLUGINS-ISOLATED-WEB-APPS-012.4
  - AC-CANVASES-AGENT-WEB-APPS-007.5
  - AC-CANVASES-AGENT-WEB-APPS-007.6
system_design:
  - ../../specs/plugins/system-design/isolated-web-app-contributions.md
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
---

# Task 03: Detect canvas startup

## Summary

Replace URL-presence and iframe-load readiness with a bounded, current-frame
startup acknowledgement. Keep the frame sandboxed and support retained packages
without republishing them.

## In scope

- Serve-time entry bootstrap, before authored scripts, using the existing HTML
  tokenizer dependency. Preserve the doctype, encoding, nested entry paths,
  stored bytes, package digest, and non-entry assets. Recompute response length.
- Reserved capability-scoped bootstrap route, after existing token validation.
  Test malformed and oversized entry handling without an unbounded read.
- Capture initial script/asset failures and unhandled rejections; acknowledge
  document load plus successful context access, not application correctness.
- Versioned presentation-only probe/result messages with a fresh attempt nonce,
  exact current `contentWindow` matching, bounded shape, and no sensitive data.
- Mount while `loading_runtime`, then reveal after acknowledgement and existing
  appearance synchronization. Never wait for Ready before mounting the frame.
- A separate 15-second startup deadline, unavailable state, Retry, and Releases
  recovery. Cancel timers/listeners and invalidate stale attempts on retry,
  descriptor renewal, navigation, disposal, or revocation.
- Localized recovery copy, desktop/mobile E2E, and public runtime documentation.

## Out of scope

Privileged postMessage APIs, business-health monitoring, package rebuilds,
changing capability expiry, relaxing the sandbox, and new permission grants.

## Acceptance

1. `TestRuntimeStartupBootstrapPreservesArtifact` proves served entry injection,
   retained-package compatibility, unchanged stored artifact hashes, correct
   lengths, nested paths, token enforcement, and reserved-route precedence.
2. Unit tests with fake clocks reject wrong-frame, stale-nonce, invalid-version,
   malformed, and post-disposal messages. No acknowledgement means unavailable
   after 15 seconds, including iframe load events without executed bootstrap.
3. Real desktop/mobile frames become Ready only after successful document and
   context startup. Initial script/context failures and browser-blocked frames
   show recoverable errors. Retry succeeds with a new attempt; old callbacks
   cannot change its status. Existing theme, expiry, and revoke behavior works.

## ASCII UI preview

Excerpt of [UI-01 in the plan](plan.md#ui-01-canvas-startup-task-panel-or-focused-route):

```text
Desktop: [Live workflow]              [Releases and permissions]
Phone:   [< Back | Live workflow]     [Actions]
Both:    Loading canvas... -> acknowledged -> Application / Ready
                          -> deadline/error
         Canvas could not start
         [Try again] [Releases and permissions]
```

Keep recovery controls outside the failed frame. Use the same state machine in
the task panel and focused phone route; no smaller mobile error workflow.

## Verification

Run from the repository root, after adding the named regression files:

```bash
(cd apps/backend && go test ./internal/plugins/webapp/...)
(cd apps/web && pnpm exec vitest run components/plugins/web-app-startup.test.ts components/plugins/web-app-frame.test.tsx components/plugins/canvas-page.test.tsx components/settings/canvas-host-route.test.tsx components/settings/canvas-host-components.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/canvas/canvas-host-origins.spec.ts tests/canvas/plugin-canvas.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/canvas/mobile-plugin-canvas.spec.ts -- --retries=0)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use task 01's real TLS proxy fixtures, not header replacement or a production
instance. Create failure observers before navigation and avoid arbitrary waits.
Run scoped ESLint on changed frontend implementation files before completion.

## Files likely touched

- `apps/backend/internal/plugins/webapp/{runtime.go,runtime_test.go}` and new bootstrap implementation/tests
- `apps/web/components/plugins/{web-app-frame.tsx,web-app-frame.test.tsx,canvas-page.tsx,canvas-page.test.tsx}`
- New `apps/web/components/plugins/{web-app-startup.ts,web-app-startup.test.ts}`
- `apps/web/components/settings/{canvas-host-route.tsx,canvas-host-route.test.tsx,canvas-host-components.tsx,canvas-host-components.test.tsx}`
- `apps/web/e2e/tests/canvas/{canvas-host-origins.spec.ts,plugin-canvas.spec.ts,mobile-plugin-canvas.spec.ts}`
- `apps/web/src/locales/` in all supported languages; generate Traditional Chinese with `pnpm run i18n:zh-hant`
- `docs/public/{canvases.md,plugins-authoring.md,security.md}`

## Dependencies

Tasks 01 and 02 establish working embedding and first-publication activation.
Reconcile shared fixtures and runtime tests without weakening their assertions.

## Risks

HTML rewriting and startup correlation can introduce regressions. Keep the
protocol presentation-only and treat opaque `null` origins as insufficient
identity. No capability URL or context payload belongs in a startup message.

## Parallelism

`sequential`

## Inputs

- Plugin system design's Runtime startup protocol section.
- Existing runtime token validation, appearance synchronization, and frame tests.
- Task 01's alternate-host browser fixture and task 02's publication fixtures.

## Results

Implemented and verified.

- Added a bounded, capability-scoped host bootstrap that injects only into the
  served entry representation. It reports document failures and successful
  context access with a versioned, nonce-correlated presentation message while
  preserving stored artifact bytes, nested entry paths, token checks, and the
  opaque sandbox.
- Replaced URL/load readiness with current-frame startup acknowledgement.
  Loading mounts the frame, appearance is synchronized before reveal, and a
  15-second deadline tears down the frame into a recoverable unavailable state.
  Retry obtains a fresh capability and nonce, and stale messages/listeners are
  ignored.
- Added unit coverage for the startup wire shape, invalid messages, iframe
  failure/deadline handling, and retained runtime behavior. Added desktop
  startup-failure/retry coverage and the HTTPS-origin browser regression.
- Updated public and bundled canvas authoring/runtime guidance.
- `(cd apps/backend && go test ./internal/plugins/webapp/...)`: passed.
- `(cd apps/web && pnpm exec vitest run components/plugins/web-app-startup.test.ts components/plugins/web-app-frame.test.tsx components/plugins/canvas-page.test.tsx components/settings/canvas-host-route.test.tsx components/settings/canvas-host-components.test.tsx)`: passed.
- `(cd apps/web && pnpm run typecheck)`: passed.
- `(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)`: passed.
- Managed desktop and mobile Canvas E2E suites passed, including same-origin
  HTTPS aliases, startup recovery/retry, owner-created publication, and the
  mobile canvas flow.
