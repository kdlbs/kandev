---
id: "01-surface-runtime-startup-failure-cause"
title: "Surface the canvas runtime startup failure cause"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-007
acceptance_criteria:
  - AC-CANVASES-AGENT-WEB-APPS-007.5
  - AC-CANVASES-AGENT-WEB-APPS-007.9
  - AC-CANVASES-AGENT-WEB-APPS-007.10
system_design:
  - ../../specs/canvases/system-design/canvas-host-chrome.md
---

# Task 01: Surface The Canvas Runtime Startup Failure Cause

## Summary

Carry the startup-failure cause from the canvas frame to the canvas host, and render a
runtime-startup-failure state whose copy names the application or its runtime instead of the release.

## In scope

- Add a `WebAppStartupFailureReason` union to `apps/web/components/plugins/web-app-startup.ts`
  covering `document_error`, `context_unavailable`, and a startup timeout. An iframe load error was
  considered and dropped: React attaches no DOM `"error"` listener for `iframe`/`object`/`embed`
  (only `"load"`), so the existing `handleError` wiring at `web-app-frame.tsx:139` (pre-fix) could
  never fire; it is dead code, deleted rather than given a reason.
- Settle each attempt in `web-app-frame.tsx` with readiness or one reason, and give `onError` that
  reason as an argument. Pass the guest `code` from the parsed result at line 119; supply the
  timeout reason at line 105.
- Forward the reason through `canvas-page.tsx` and `canvas-host-components.tsx` to
  `canvas-host-route.tsx`.
- Add a `runtime_failed` member to `CanvasHostState`, record the reason in the host, and resolve the
  panel description from it. Leave the `unavailable` state reachable only from release status and
  host request failures.
- Add the new copy to `en` and to `pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`, and regenerate the pseudo
  catalog. Use `pnpm run i18n:zh-hant` for the Traditional pair.

## Out of scope

- CSP, sandbox tokens, or forwarding guest CSP-violation detail.
- `applyCanvasHostError` and the `offline` state.
- `plugins:webAppUnavailable`.
- Backend, persistence, permission, or observability changes.

## Acceptance

- A frame that reports `context_unavailable` or `document_error` while the active release is valid
  renders the runtime-startup-failure state, and its description names the application or runtime.
  The release-unavailable description does not appear.
- A canvas whose `active_release_status` is `unavailable` still renders `canvases:unavailable` and
  `canvases:unavailableDescription`.
- The 15-second timeout renders the runtime-startup-failure state with a description distinct from
  both guest causes, and Retry stays available.

## ASCII UI preview

`UI-01: Canvas host state panel`, unchanged from the
[plan preview](plan.md#ascii-ui-preview). Required structure: one title naming the canvas
application, one description naming the cause, and the existing Retry action. Wording is this work
order's to finalize within that structure.

## Verification

```bash
# From apps/web:
pnpm exec vitest run components/plugins/web-app-frame.test.tsx components/settings/canvas-host-route.test.tsx components/settings/canvas-host-components.test.tsx
pnpm run i18n:check
pnpm run typecheck
pnpm run lint
```

```bash
# From apps/web, for the two proxy specs that assert the current title:
pnpm e2e:run --grep "recovers after proxy authentication expires"
```

Baseline before the change: the three component test files pass with 15 tests, and
`node scripts/check-i18n-keys.mjs` exits 0.

Confirm which path each proxy spec settles on before editing its assertion. Leave
`plugin-canvas.spec.ts` alone unless it actually fails; it exercises the host capability request,
not the frame startup handshake. See
[the plan's E2E note](plan.md#existing-e2e-assertions-on-the-current-copy).

## Files likely touched

- `apps/web/components/plugins/web-app-startup.ts`
- `apps/web/components/plugins/web-app-frame.tsx`
- `apps/web/components/plugins/web-app-frame.test.tsx`
- `apps/web/components/plugins/canvas-page.tsx`
- `apps/web/components/settings/canvas-host-components.tsx`
- `apps/web/components/settings/canvas-host-components.test.tsx`
- `apps/web/components/settings/canvas-host-route.tsx`
- `apps/web/components/settings/canvas-host-route.test.tsx`
- `apps/web/components/settings/canvas-host-route-view.tsx`
- `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn,zh-hk,zh-tw}/canvases.json`
- `apps/web/e2e/tests/canvas/canvas-authenticated-proxy.spec.ts` (only if it moves)
- `apps/web/e2e/tests/canvas/mobile-canvas-authenticated-proxy.spec.ts` (only if it moves)

## Dependencies

None. The CSP fix this defect was found behind is already on `main` at 93a7cba93.

## Risks

- Reusing `unavailable` for the new state would re-create the defect. Assert the release path and the
  startup path render different `canvas-host-state` text.
- Omitting the new state from `STATE_COPY` is a type error, not a silent gap, because the map is a
  total `Record<CanvasHostState, ...>`.
- New copy containing U+2014 fails `check-no-em-dash-ui.mjs`.

## Parallelism

`sequential`

## Inputs

- `REQ-CANVASES-AGENT-WEB-APPS-007`, especially `.5`, `.9`, and `.10`.
- [Canvas host chrome design](../../specs/canvases/system-design/canvas-host-chrome.md),
  section "Startup-failure cause propagation".
- The guest bootstrap contract in
  `apps/backend/internal/plugins/webapp/runtime_bootstrap.go`.
- The measured root-cause table in [plan.md](plan.md#confirmed-root-cause).

## Results

Shipped in `fc4cd24ea`, reconciled to three causes (dropping the never-reachable iframe load-error
case) in `e962ff312` after Review round 1. The threading is `web-app-startup.ts`
(`WebAppStartupFailureReason` union) -> `web-app-frame.tsx` (`StartupAttempt`,
`finishAttempt(outcome)`, `onError?: (reason) => void`) -> `canvas-page.tsx` ->
`canvas-host-route.tsx` (`runtimeFailureReason` state, `markRuntimeUnavailable(reason)` ->
`state: "runtime_failed"`) -> `canvas-host-route-view.tsx` -> `canvas-host-components.tsx`
(`RUNTIME_FAILED_DESCRIPTIONS`). New `canvases:` keys shipped in `en`, `pseudo`, `pt-pt`, `zh-cn`,
`zh-hk`, `zh-tw`. All three verification commands and both proxy E2E specs pass; verbatim receipts
are in the task's Kandev task plan.
