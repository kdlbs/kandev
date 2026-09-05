---
id: "05-script-capable-preview-runtime"
title: "Build the capability-free preview runtime"
status: cancelled
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-001
acceptance_criteria:
  - AC-UI-NATIVE-HTML-PREVIEW-001.3
  - AC-UI-NATIVE-HTML-PREVIEW-001.4
  - AC-UI-NATIVE-HTML-PREVIEW-001.5
  - AC-UI-NATIVE-HTML-PREVIEW-001.9
  - AC-UI-NATIVE-HTML-PREVIEW-001.10
system_design:
  - ../../specs/ui/system-design/native-html-preview.md
---
# Task 05: Build the capability-free preview runtime

## Summary

Implement the isolated script execution boundary selected by the revised HTML
preview design. Inline source JavaScript runs in a dedicated worker-hosted
ECMAScript VM with a virtual DOM and no host imports. The worker emits only
data-only snapshots and diagnostics through a structured-clone protocol.

## In scope

- Select, pin, and license-check the ECMAScript engine and virtual-DOM support
  needed for the first supported capability set.
- Implement worker lifecycle, load, event-dispatch, snapshot, diagnostics, and
  disposal messages.
- Implement virtual DOM mutation, inline script order, virtual event handlers,
  bounded timers, console diagnostics, and runtime-owned blob tokens.
- Deny browser globals, Kandev data, credentials, storage, filesystem,
  network, navigation, downloads, and window-opening APIs.
- Enforce instruction, wall-clock, virtual-heap, timer, event-queue, and
  snapshot limits with fail-closed termination.
- Add unit tests for capability absence, script execution, event dispatch,
  dynamic resource filtering, navigation no-ops, and budget failure.

## Out of scope

- File-tab state or toolbar changes.
- Responsive surface composition and localization.
- Public documentation.
- Full browser API compatibility, remote resources, or multi-file sites.

## Acceptance

- A representative inline script mutates the virtual document and responds to
  a sanitized user event without executing in the native browser context.
- Attempts to use fetch, XHR, WebSocket, EventSource, location, history,
  parent/top, forms, downloads, or window APIs fail closed and cannot produce a
  browser request or navigation.
- Infinite loops, oversized output, unsupported APIs, malformed messages, and
  runtime exceptions terminate or reject the generation without falling back to
  native script execution.

## Verification

```bash
cd apps/web
pnpm exec vitest run lib/html-preview/preview-runtime.test.ts lib/html-preview/preview-resource-policy.test.ts
pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/html-preview/preview-runtime.ts`
- `apps/web/lib/html-preview/preview-runtime.worker.ts`
- `apps/web/lib/html-preview/preview-runtime-types.ts`
- `apps/web/lib/html-preview/preview-resource-policy.ts`
- `apps/web/lib/html-preview/preview-runtime.test.ts`
- `apps/web/lib/html-preview/preview-resource-policy.test.ts`
- `apps/web/package.json`
- `apps/pnpm-lock.yaml`

## Dependencies

None. This boundary must exist before UI integration can claim script support.

## Risks

- The engine may not support the required ECMAScript or virtual-DOM behavior in
  all supported browsers and WebViews.
- A host import, native function escape, or unbounded callback can invalidate
  the security boundary.
- Runtime limits that are too strict can break ordinary interactive documents.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-NATIVE-HTML-PREVIEW-001.3` through `.6` and `.9`.
- The runtime boundary, message contract, execution limits, and security
  invariants in the system design.
- The capability-free preview isolation design that this cancelled work order
  originally implemented.

## Results

Implemented the worker-hosted QuickJS runtime with a virtual document, bounded
event and timer execution, deny-by-default resource policy, runtime-owned blob
tokens, structured-clone snapshots, and fail-closed lifecycle errors.
Load and interaction execution each receive an independent wall-clock and
interrupt budget, so an idle preview cannot consume the next user interaction's
runtime allowance.

Verification completed:

```text
pnpm exec vitest run lib/html-preview/preview-runtime.test.ts lib/html-preview/preview-runtime-client.test.ts lib/html-preview/preview-resource-policy.test.ts
pnpm exec tsc --noEmit --pretty false
```
