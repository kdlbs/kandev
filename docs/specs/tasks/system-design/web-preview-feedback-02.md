---
status: current
system: tasks
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-002
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-004
---

# Web Preview Feedback System Design, Part 2: Canvas Capture and Comment Surface

## Purpose and boundaries

This part extends the [main design](web-preview-feedback.md) with two
contracts. Screenshot captures can read WebGL canvas pixels, and capture
comments and pending-item edits use the shared
[selection comment surface](../../ui/system-design/selection-comment-surface.md).
Persistence, delivery, the bridge protocol, and the backend feedback API do
not change.

## Requirement mapping

| Criterion | Section |
| --- | --- |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-002.8`, `.9` | [Canvas pixel readability](#canvas-pixel-readability) |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-004.6` | [Comment surface](#comment-surface) |

## Canvas pixel readability

`html2canvas` copies a `<canvas>` element with `drawImage`. A WebGL context
created with `preserveDrawingBuffer: false`, which is the default, clears its
drawing buffer after compositing. A later read therefore returns a transparent
buffer and the PNG comes out blank (issue #3992). Flutter web's default
CanvasKit renderer, and most WebGL pages, create their contexts this way.

The gateway's port-proxy runtime shim (`runtimeShimTemplate` in
`internal/gateway/websocket/proxy_rewriter.go`) is injected at the top of
`<head>` before any page script, for every proxied HTML response. The Browser
panel and the native HTML preview both use it. The shim wraps
`HTMLCanvasElement.prototype.getContext` and, when present,
`OffscreenCanvas.prototype.getContext` on the page's main thread. Each is
wrapped once, guarded by a non-enumerable marker. For `webgl`, `webgl2`, and `experimental-webgl` it calls the original
with a copy of the caller's attributes that has `preserveDrawingBuffer: true`.
All other context types, arguments, and return values pass through unchanged.
The page renders the same frames, at a small compositing cost, so the
DOM-based capture can read them (`AC-TASKS-WEB-PREVIEW-FEEDBACK-002.8`). The
shim does not depend on whether feedback controls are active, because the
attribute must be set when the context is created.

Flutter web renderers in a Kandev preview:

| Build | Renderer loaded in a Kandev preview | Captured |
| --- | --- | --- |
| `flutter build web` | CanvasKit (`canvaskit.wasm`) | Yes, verified with Flutter 3.50 |
| `flutter build web --wasm` | `skwasm.wasm`, page not cross-origin isolated | Yes, verified with Flutter 3.50 |
| Any build on a cross-origin-isolated page that renders in a worker `OffscreenCanvas` | Worker renderer | No |

A worker renderer needs `crossOriginIsolated`. A framed document is isolated
only if every ancestor is too, and Kandev serves no COOP or COEP headers, so
previews never get one, even after Flutter makes Wasm the default. If Kandev ever enables cross-origin isolation, that decision must
first add a capture path that does not rely on reading the DOM.

The shim cannot reach contexts created inside a worker. Those regions, and
cross-origin media, can still rasterize blank. A uniform color alone does not
prove failure, because the user may have selected a genuinely solid area. So
the check produces a warning, not an error. After rasterization,
`rasterizePreviewRegion` checks whether the capture rectangle intersects any
`<canvas>` bounding box in the iframe document, including canvases inside open
shadow roots, where Flutter web renders. When it does, it samples the
output on a canvas of at most 64 × 64 pixels and returns
`uniformOverCanvas: true` when every sampled pixel has the same RGBA value.
The flag travels on the screenshot draft (`draft.screenshot.uniformOverCanvas`).
`PreviewFeedbackEvidence` renders `task:previewScreenshotCanvasUniformWarning`
under the thumbnail on the comment surface, and Add stays enabled. Discard
returns to capture mode
(`AC-TASKS-WEB-PREVIEW-FEEDBACK-002.9`). A uniform region with no canvas shows
no warning.

The 16-megapixel bound shared by `previewScreenshotScale` and the
question-review `fitAnnotationToPixelLimit` moves to one helper,
`pixelLimitScale(width, height, maxPixels)` in `lib/images/pixel-limit.ts`.
Both callers keep their own limit constants.

## Comment surface

Capture comments and pending-item edits use `SelectionCommentSurface` from
the [selection comment surface design](../../ui/system-design/selection-comment-surface.md).
This replaces the inline `DraftEditor` in `preview-feedback-controls.tsx` and
the inline textarea editor in `preview-feedback-collection.tsx`
(`AC-TASKS-WEB-PREVIEW-FEEDBACK-004.6`).

- **Anchor:** the capture `rect` is in iframe-viewport CSS pixels. The parent
  adds the iframe element's `getBoundingClientRect()` origin and anchors just
  below the rectangle's bottom-left corner, clamped to the iframe's visible
  bounds. The helper `anchorBelowRect(rect, origin, bounds)` in
  `components/task/floating-selection-position.ts` replaces the local
  arithmetic in the question-review `regionAnchor`. A pending-item edit
  anchors below that row's Edit button.
- **Content:** `contextLabel` is the sanitized `page_route`. `evidence` is one
  `PreviewFeedbackEvidence` component, shared with the pending list, that
  renders selected text, the element label, or the screenshot thumbnail with
  its dimensions. `selectedText` carries the plain-text form for the Drawer
  description. No `onAddAndRun` is passed, so no Run action appears.
- **Draft:** the surface is controlled through `draftComment` and
  `onDraftChange` from `use-preview-capture`. `onAdd` calls `saveDraft` and
  returns its boolean, so a failed upload or create keeps the surface open with
  `errorMessage`. Dismissal calls only `closeDraftSurface`, so the logical
  draft, PNG, and upload ID survive (`AC-TASKS-WEB-PREVIEW-FEEDBACK-005.4`).
  [Part 3](web-preview-feedback-03.md#drafts) replaces the single resumable
  draft and its capture gating with a list of drafts.
- **Editing saved items:** the Edit action opens the surface with
  `editingComment` and `onDelete`. `onAdd` calls the collection update with the
  rendered version and returns `false` on conflict or failure. On coarse
  pointers the edit Drawer opens above the collection Drawer.
- **Host containers:** the surface root carries `data-selection-comment-surface`.
  The collection's Radix Popover (`onInteractOutside`, `onEscapeKeyDown`) and
  the Annotate menu's outside-pointer and Escape handlers ignore events inside
  that marker. Working in a floating surface therefore does not close the
  surface's own host.

## Verification

- Go: the runtime shim contains the WebGL `getContext` wrapper exactly once,
  before the first page script.
- Vitest: uniform-pixel detection over a canvas (warning, still saveable)
  versus a non-canvas region (no warning),
  `pixelLimitScale`, the anchor helper, the controlled draft surviving
  dismissal, and Resume/Discard gating.
- Desktop Chromium E2E covers a WebGL page that does not preserve its drawing
  buffer. A screenshot capture over it has non-uniform pixels, and the comment
  surface opens next to the capture. Saved-item editing and draft
  resume/discard are also covered.
- Mobile Chrome E2E covers the capture and edit Drawers.

## Implementation plans

- [Web preview feedback and comment surface](../../../plans/web-preview-feedback-comment-surface/plan.md)
