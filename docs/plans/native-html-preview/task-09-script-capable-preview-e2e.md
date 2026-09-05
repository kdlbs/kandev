---
id: "09-script-capable-preview-e2e"
title: "Prove script execution and isolation in browsers"
status: cancelled
wave: 4
depends_on:
  - "07-responsive-preview-surfaces"
plan: "plan.md"
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-001
acceptance_criteria:
  - AC-UI-NATIVE-HTML-PREVIEW-001.1
  - AC-UI-NATIVE-HTML-PREVIEW-001.2
  - AC-UI-NATIVE-HTML-PREVIEW-001.3
  - AC-UI-NATIVE-HTML-PREVIEW-001.4
  - AC-UI-NATIVE-HTML-PREVIEW-001.5
  - AC-UI-NATIVE-HTML-PREVIEW-001.6
  - AC-UI-NATIVE-HTML-PREVIEW-001.7
  - AC-UI-NATIVE-HTML-PREVIEW-001.8
  - AC-UI-NATIVE-HTML-PREVIEW-001.9
system_design:
  - ../../specs/ui/system-design/native-html-preview.md
---
# Task 09: Prove script execution and isolation in browsers

## Summary

Replace the static-only browser assertions with causal evidence that inline
JavaScript executes in the preview runtime while network, credential, and
navigation capabilities remain unavailable. Repeat the user flow on desktop
and mobile, then add the required desktop WebView smoke evidence.

## In scope

- Chromium desktop coverage for inline script output, event-driven mutation,
  runtime failure, source recovery, and same-session restoration.
- Negative request and navigation assertions for fetch, XHR, WebSocket,
  dynamic links, meta refresh, forms, location, history, downloads, and
  window opening.
- Mobile Chrome coverage for runtime output, touch target, source recovery, and
  containment.
- Desktop WebView smoke coverage for the same request and navigation policy.

## Out of scope

- Broad E2E suite execution.
- Other browser engines and executor types.
- Review-diff, multi-file, remote-resource, or Browser-panel tests.

## Acceptance

- Desktop Chromium proves a script changes visible output and a user event
  reaches the virtual runtime.
- Causal request and frame-state assertions prove blocked network and
  navigation attempts do not leave the preview or contact an external origin.
- Mobile Chrome and desktop WebView prove the same safe entry, recovery, and
  responsive behavior.

## Verification

```bash
make build-web
cd apps/web
pnpm e2e:raw --project=chromium tests/chat/html-preview.spec.ts
pnpm e2e:raw --project=mobile-chrome tests/task/mobile-html-preview.spec.ts
cd ../desktop
pnpm e2e
```

## Files likely touched

- `apps/web/e2e/tests/chat/html-preview.spec.ts`
- `apps/web/e2e/tests/task/mobile-html-preview.spec.ts`
- `apps/web/e2e/pages/session-page.ts`
- `apps/desktop/e2e/desktop-launch-smoke.mjs` or a focused WebView test.

## Dependencies

Task 07 supplies the runtime-backed responsive surface and stable selectors.

## Risks

- A test that only observes the final DOM can miss a request made before the
  runtime snapshot arrives. Request listeners and frame-state assertions must
  be causal and installed before preview activation.
- Chromium and WebView may differ in worker, blob, and resource behavior.

## Parallelism

`parallel-safe` with Task 08 after Task 07.

## Inputs

- The revised acceptance criteria `.3` through `.9`.
- The browser verification strategy and security invariants in the system
  design.
- Existing desktop and mobile HTML preview fixtures from the superseded work.

## Results

Replaced the static iframe assertions with runtime-backed Chromium and mobile
coverage. The tests prove inline script output, event-driven mutation, inert
links and forms, blocked dynamic resources and navigation, source recovery,
same-session saved-source restoration, touch sizing, and page containment.

Verification completed:

```text
pnpm e2e:raw --project=chromium tests/chat/html-preview.spec.ts: 1 passed
pnpm e2e:raw --project=mobile-chrome tests/task/mobile-html-preview.spec.ts: 1 passed
pnpm e2e (desktop): build and WebView launch smoke passed
```
