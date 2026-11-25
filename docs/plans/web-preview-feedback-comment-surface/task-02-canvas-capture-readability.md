---
id: "02-canvas-capture-readability"
title: "Readable WebGL screenshot captures"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-002
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-002.8
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-002.9
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback.md
  - ../../specs/tasks/system-design/web-preview-feedback-02.md
---

# Task 02: Readable WebGL screenshot captures

## Summary

Fix issue #3992. Make the port-proxy runtime shim force
`preserveDrawingBuffer: true` on main-thread WebGL contexts, so the DOM-based
capture can read canvas pixels. Warn, without blocking, when a capture over a
canvas comes out one uniform color. Share the 16-megapixel scale helper with
question review.

## In scope

- `runtimeShimTemplate` gains one-time `getContext` wrappers on
  `HTMLCanvasElement.prototype` and, when defined,
  `OffscreenCanvas.prototype`, for `webgl`, `webgl2`, and
  `experimental-webgl`. They copy the attributes and leave every other type
  untouched.
- `rasterizePreviewRegion`: a canvas-intersection check, uniform sampling at
  no more than 64 × 64, and a `uniformOverCanvas` flag on the result.
- `use-preview-capture`: a `captureWarning: "canvas-uniform"` field on the
  draft. The warning key `task:previewScreenshotCanvasUniformWarning` appears
  under the thumbnail, in all seven locales plus pseudo, and Save stays
  enabled.
- Spike: build the issue's counter app with `flutter create` plus
  `flutter build web` (CanvasKit) and `flutter build web --wasm` (`skwasm_st`
  in a non-isolated frame). Serve each through a Browser-panel preview and
  confirm the screenshot has pixels. Record the result here. If `skwasm_st`
  stays blank, record the context-creation path it uses and update the design
  before closing the task.
- `lib/images/pixel-limit.ts` `pixelLimitScale`, used by
  `previewScreenshotScale` and `fitAnnotationToPixelLimit`.
- An E2E fixture page drawing WebGL with `preserveDrawingBuffer: false`.

## Out of scope

- `getDisplayMedia` or desktop-native capture, and worker or `OffscreenCanvas`
  renderers.
- Changes to `inspector.js` or the bridge protocol.

## ASCII UI preview

UI-04 from the [plan](plan.md#ui-04-uniform-canvas-capture-warning-state).
Until task 04 moves the draft onto the shared surface, the warning renders in
the existing draft editor under the thumbnail.

```text
| [ solid thumbnail 400x300 ]           |
| ! Canvas content may not have been    |
|   captured. Check the image first.    |
| [Discard]                     [+ Add] |
```

## Implementation acceptance conditions

1. In Chromium, a screenshot capture over the WebGL fixture produces a PNG
   whose pixels are not uniform. Without the shim it would be blank.
2. A region over a canvas that still rasterizes uniform (stub the renderer in
   Vitest) produces a draft with the warning, and saving it succeeds. A
   uniform region without a canvas produces no warning.
3. The shim appears exactly once, before the first page `<script>` in
   rewritten HTML, and the existing proxy rewriter tests stay green.

## Verification

```bash
cd apps/backend && go test -trimpath ./internal/gateway/websocket/ -run 'RewriteHTML|RuntimeShim' -count=1
cd ../web && pnpm exec vitest run lib/preview-screenshot.test.ts lib/images/pixel-limit.test.ts lib/question-review/regions.test.ts hooks/use-preview-capture.test.ts
pnpm run typecheck && pnpm run lint && pnpm run i18n:check
pnpm e2e:run --host --shards 1 --project chromium tests/preview/preview-feedback.spec.ts
```

## Likely files

- `apps/backend/internal/gateway/websocket/proxy_rewriter.go`, `proxy_rewriter_test.go` (or a new `proxy_rewriter_webgl_test.go`)
- `apps/web/lib/preview-screenshot.ts`, `.test.ts`; new `apps/web/lib/images/pixel-limit.ts`, `.test.ts`; `apps/web/lib/question-review/regions.ts`
- `apps/web/hooks/use-preview-capture.ts`, `.test.ts`; `apps/web/components/task/inspector/preview-feedback-controls.tsx` (error copy only)
- `apps/web/src/locales/*/task.json`
- `apps/web/e2e/tests/preview/preview-feedback-helpers.ts` (WebGL fixture route), `preview-feedback.spec.ts`

## Risks

- The spike needs a local Flutter SDK (install through `mise`). Do not add
  Flutter to CI. The Chromium E2E uses a plain WebGL fixture.
- jsdom has no WebGL or canvas pixels. Keep the uniformity check as a pure
  function over `Uint8ClampedArray` for Vitest, and prove real pixels in
  Playwright.
- The shim must not throw when `HTMLCanvasElement` is missing, for example in
  XML documents.

## Results

Done.

- `webglPreserveShimJS` in the gateway runtime shim wraps
  `HTMLCanvasElement` and `OffscreenCanvas` `getContext`; a node-executed Go
  test covers attribute handling and idempotency.
- `rasterizePreviewRegion` returns `uniformOverCanvas`; the warning renders
  under the thumbnail and Add stays enabled.
- `pixelLimitFactor` is shared with `fitAnnotationToPixelLimit`.
- Chromium E2E `captures WebGL pixels that the page does not preserve` passes
  (two-colour WebGL fixture drawn without `preserveDrawingBuffer`).
- Flutter spike (Flutter 3.50 via mise, counter app from the issue, served
  through the Browser panel in Chromium): `flutter build web` loads
  `canvaskit.wasm` and `--wasm` loads `skwasm.wasm` on a non-isolated page;
  both captures contained the UI (about 550 distinct colours, no warning).
  The spike showed that Flutter keeps its canvas in an open shadow root, so
  canvas detection now walks open shadow roots.
