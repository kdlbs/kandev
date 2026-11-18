---
status: current
system: tasks
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-002
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-004
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-005
---

# Web Preview Feedback System Design, Part 3: Annotation Mode

## Purpose and boundaries

This part replaces the one-shot Annotate menu with a persistent annotation
mode, adds comment-on-selection outside that mode, and lets screenshot regions
be drawn in any number and resized after capture. It extends the
[main design](web-preview-feedback.md) and
[part 2](web-preview-feedback-02.md).

Unchanged: the pending collection, composer delivery, atomic acceptance, the
shared [selection comment surface](../../ui/system-design/selection-comment-surface.md),
and canvas readability. Changed: the inspector bridge protocol (version 3),
the preview capture hook, the preview controls, and one backend mutation that
can replace a saved screenshot.

The image-review annotation in question review (PR #5239) is the reference
interaction. Its region outlines, mode hint, and region geometry become shared
components, so both features draw, number, and edit regions the same way.

## Requirement mapping

| Criterion | Section |
| --- | --- |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-005.1` | [Comment on selection](#comment-on-selection) |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-005.2`, `.3` | [Mode and tools](#mode-and-tools), [Layout](#layout) |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-005.4` | [Drafts](#drafts) |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-005.5` | [Escape and exit](#escape-and-exit) |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-005.6` | [Regions](#regions), [Replacing a saved screenshot](#replacing-a-saved-screenshot) |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-005.7` | [Tracking regions through scrolling](#tracking-regions-through-scrolling) |

## Shared region annotation

Move these out of `components/task/chat/question-review/` and
`lib/question-review/regions.ts` into `components/shared/region-annotation/`
and `lib/regions/geometry.ts`. Question review imports them from the new
location and keeps its behavior.

- `RegionOverlay`: numbered outlines plus an optional dashed draft outline. It
  gains two optional props. `pendingIndexes` draws listed regions dashed, for
  unsaved drafts. `onHandlePointerDown(index, handle, event)` renders a move
  area and four corner handles, with a 44-pixel hit area on coarse pointers.
  Question review passes neither, so it looks and behaves as before.
- `AnnotationModeHint`, from `AnnotateHint`: the pill with hint text and a
  Done button. The preview toolbar reuses its Exit button styling and touch
  sizing.
- Geometry: `rectFromPoints`, `moveRegion`, `resizeRegion`, and a new
  `resizeRegionFromCorner(rect, corner, delta, bounds, minSize)`. The bounds are the
  image size in question review and the iframe viewport in the preview.

Region coordinates stay in the host's own space. Question review uses image
pixels inside its transformed layer. The preview uses iframe-viewport CSS
pixels inside an overlay aligned with the iframe, at scale 1.

## Mode and tools

`usePreviewAnnotationMode` replaces the single-shot `startCapture` flow in
`use-preview-capture`. Its state is `{ active, tool }`, where `tool` is
`interact | text | element | region`. The header Annotate button toggles
`active`. Entering starts with the last tool used in this preview, or Region
the first time.

The parent sends the bridge `set-capture-mode` with the bridge mode for the
tool: `null` for Interact, `text`, `element`, or `screenshot`. In protocol 3,
completing a capture no longer resets the bridge mode. The bridge stays in its
mode until the parent changes it, which keeps the tool selected
(`AC-TASKS-WEB-PREVIEW-FEEDBACK-005.3`).

A full page load re-runs the injected bridge. On `inspector-ready` the parent
re-sends the current mode and region anchors, so a navigation keeps the mode
and tool. A same-document route change no longer cancels the mode.

Tool behavior:

- **Interact:** no capture listeners. Page input passes through. Region
  outlines are drawn without handles and ignore the pointer. Comment on
  selection works as it does outside the mode.
- **Text:** a completed selection creates a text draft at once, with no
  button press, as today.
- **Element:** hover outline and label, and a click captures, as today.
- **Region:** see [Regions](#regions).

## Layout

> [Part 4](web-preview-feedback-04.md) changes this layout: the count button becomes a comment panel toggle, and the panel docks beside the preview.

`PreviewAnnotationLayer` is a new component mounted in the iframe's positioned
container in `BrowserPanel` and `HtmlPreviewContent`. It is `absolute inset-0`
and `pointer-events: none`. Only the toolbar, region handles, and selection
button take pointer events. It renders:

- a 2-pixel primary ring around the preview while the mode is active, like the
  image-review stage;
- `PreviewAnnotationToolbar`;
- `RegionOverlay` for the current route's region captures;
- the comment-on-selection button.

`PreviewAnnotationToolbar` is a segmented tool group (one pressed tool, each
with an icon, a label, and a tooltip on fine pointers), the pending count
button that opens the existing collection, and Exit. On fine pointers it
floats centered at the top of the preview. On coarse pointers it is a bottom
bar with 44-pixel controls and safe-area padding, laid out like
`review-phone-controls.tsx` (`AC-TASKS-WEB-PREVIEW-FEEDBACK-005.2`).

The header keeps a single Annotate toggle with the pending count. The capture
choices and Cancel item leave the Annotate menu, and the menu and Drawer are
removed. The pending list opens from the toolbar count button and from the
composer's pending-context item.

## Comment on selection

The bridge listens for `selectionchange` whenever its mode is `null`, which
covers both outside the mode and Interact. Once per animation frame it sends
`selection-changed` with `{ rect }`: the last client rectangle of a
non-collapsed selection inside the page, or `null`. Selections inside
inspector-owned nodes and inputs are ignored.

The parent offsets the rectangle by the iframe origin and renders
`EditorSelectionCommentTrigger`. On fine pointers that is the floating button
beside the selection. On coarse pointers it is `SelectionActionDock` hosted by
the preview container, the same pattern used by chat, plan, and editor
comments (`AC-TASKS-WEB-PREVIEW-FEEDBACK-005.1`). Activating it sends the new
`capture-selection` command. The bridge runs its existing
`captureTextSelection` and replies with `capture-completed`. The result is an
ordinary text draft, so payload, persistence, and delivery do not change.

The button only appears when feedback controls are enabled, meaning a task and
session exist and the page is proxied. Other pages never load the bridge.

## Drafts

> [Part 4](web-preview-feedback-04.md) changes dismissal: a capture closed with no text is removed, and drafts are numbered and listed in the comment panel.

`use-preview-capture` replaces its single `draft` with an ordered list of
drafts, each with a client ID, capture payload, typed comment, optional PNG
and upload ID, and an `open` flag. One draft at most is open. Opening a draft
closes the other surfaces without discarding them.

- A new capture appends a draft and opens it. Earlier drafts stay
  (`AC-TASKS-WEB-PREVIEW-FEEDBACK-005.4`).
- Dismissing a surface by outside click or Escape keeps the draft. Region
  drafts stay as dashed numbered outlines while the mode is active, and every
  draft is projected as a dashed in-page marker through the existing marker
  projection, which gains a `pending: true` flag. Clicking a region outline, or
  **Resume** in the pending panel's Unsaved group, reopens it. In-page markers
  stay pointer-transparent, so text and element drafts resume from the panel.
- Save follows the current upload-then-create path and removes the draft.
  Discard releases its PNG and staged upload.
- Unsaved and saved screenshots together are limited to the backend's pending
  screenshot limit of 10. At the limit the Region tool shows a disabled state
  and a visible message, and the bridge is put in `null` mode for that tool.
- Drafts are local state for this preview. They survive mode exit, tool
  changes, and same-preview navigation, but not unmounting the preview or a
  page reload, as before.

The Resume and Discard row and the capture gating from part 2 are removed.

## Escape and exit

> [Part 4](web-preview-feedback-04.md) changes step 1 to use its close reducer, which removes empty captures.

The order is fixed (`AC-TASKS-WEB-PREVIEW-FEEDBACK-005.5`):

1. If a comment surface is open, close it and keep the draft.
2. Else, if the tool is not Interact, switch to Interact.
3. Else, exit annotation mode.

Keys pressed while the iframe has focus never reach the parent document. In
protocol 3 the bridge handles Escape in every mode, including `null` while
annotation mode is active. It calls `preventDefault` and sends `escape`. The
parent applies the next step. The parent sends `set-annotation-active` so the
bridge knows whether Escape belongs to Kandev while the mode is `null`; outside
the mode, Escape stays with the page. Kandev-side keydown on the layer and the
surface's own Escape handler drive the same reducer, so both paths agree.

Exit sets the bridge mode to `null`, clears region anchors, and restores the
page cursor and `touch-action`. Saved text and element markers keep their
existing dots. Drafts remain, and the Annotate button shows an unsaved
indicator when any exist.

## Regions

In the Region tool the bridge keeps its current drag overlay and gesture
ownership. When the drag ends it sends `screenshot-region-selected` and stays
in region mode. The parent rasterizes at once, using `rasterizePreviewRegion`
and the canvas warning from part 2, appends a draft, and opens its surface
anchored below the rectangle.

Moving and resizing happen in the parent layer, on `RegionOverlay` handles,
for unsaved and saved regions of the current route while the Region tool is
selected. During a drag the outline follows the pointer, clamped to the iframe
viewport. On release:

1. The parent builds the new `capture_rect` from the released viewport
   rectangle and the same-origin preview's current document and container
   scroll offsets (`recaptureRect`), so the anchor matches what the bridge
   would record.
2. It rasterizes the new rectangle while the region is on screen, since the
   user is looking at it.
3. For a draft it replaces the PNG and drops the previous staged upload. For a
   saved item it follows
   [Replacing a saved screenshot](#replacing-a-saved-screenshot).
4. The changed draft or item is re-projected with `project-markers`, and the
   bridge tracks the new anchor from the next `region-layout`.

If rasterizing or replacing fails, the outline returns to its previous
rectangle, the previous image stays, and the localized
`task:previewRegionRecaptureFailed` error is shown. The comment is never
touched (`AC-TASKS-WEB-PREVIEW-FEEDBACK-005.6`).

Clicking a region without moving it (5-pixel slop, as `useRegionClick` does)
opens its comment surface. For a saved region the surface edits the comment.

## Tracking regions through scrolling

When a drag starts, the bridge finds the nearest scrollable ancestor of the
element under the start point: `overflow` of `auto`, `scroll`, or `overlay`,
with scrollable size larger than client size. It falls back to the document
scroller. `capture_rect` gains three optional, bounded fields:

- `scroll_container_selector`, from the existing `getSelector`, omitted for
  the document;
- `container_scroll_x` and `container_scroll_y` at capture time.

The backend already stores `capture_rect` as bounded JSON. The web validator
`isCaptureRect` accepts the new fields. Older items without them track the
document scroll, as markers do today.

The parent sends region anchors with `project-markers` while the mode is
active, and an empty list after Exit. The bridge already
re-renders markers on scroll and resize. It also computes each region's
current viewport rectangle: the original position, minus the container's
scroll change since capture, minus the document scroll change. It sends
`region-layout` with `{ id, rect, visible }` for each, at most once per
animation frame and only when something changed. `visible` is false when the
container selector no longer resolves. The parent positions outlines from the
latest layout.

Scrolling inside a canvas, as in Flutter, changes no DOM scroll offset, so the
outline stays where it was drawn while its image remains correct. Outlines for
other routes are not projected, and their items stay in the collection
(`AC-TASKS-WEB-PREVIEW-FEEDBACK-005.7`). Send uses stored attachments and never
rasterizes.

## Bridge protocol 3

`INSPECTOR_PROTOCOL_VERSION` becomes `3` in both `inspector.js` and
`preview-inspect-bridge.ts`. They ship together, so no mixed-version handling
is needed beyond the existing legacy console messages.

| Direction | Message | Change |
| --- | --- | --- |
| parent -> page | `set-capture-mode` | Mode persists after a capture |
| parent -> page | `set-annotation-active` | New: `{ active }`, so Escape is routed |
| parent -> page | `capture-selection` | New: capture the current text selection |
| parent -> page | `project-markers` | Markers gain `pending`. New `regions` list of `{ id, page_route, capture_rect }`, at most 32 |
| page -> parent | `selection-changed` | New: `{ rect: CaptureRect or null }` |
| page -> parent | `region-layout` | New: `{ regions: [{ id, rect, visible }] }`, bounded to 32 entries |
| page -> parent | `escape` | New: Escape pressed while Kandev owns it |
| page -> parent | `screenshot-region-selected` | `capture_rect` gains the container fields |
| page -> parent | `capture-cancelled` | Removed: Escape sends `escape`, and route changes keep the mode |

The parent validates every new message with the existing source, version,
closed-discriminator, and bound checks. Unknown or oversized messages are
dropped.

## Replacing a saved screenshot

`task.preview_feedback.update` gains an optional `capture` object with
`capture_rect` and `screenshot_attachment_id`. It is accepted only for
`kind: "screenshot"` items, together with `expected_version`, and the comment
stays optional. The service runs one transaction:

1. Load the item, check the task, the version, and the kind.
2. Validate and claim the new staged attachment exactly as create does: owner,
   workspace, task, PNG signature, size, 16-megapixel bound, and uniqueness.
3. Replace `capture_rect` and `screenshot_attachment_id`, and increment the
   version.
4. Release the previous unsent attachment through the same path delete uses.

It then publishes the snapshot as other mutations do. A stale version, foreign
or already claimed attachment, or validation failure rolls back everything.
The old image stays, and the new staged upload expires through the existing
staged-attachment lifecycle. The screenshot count does not change, so the
limits hold. The fingerprint for create replay is unaffected, because update
does not use it.

## Verification

- Go: replacing a screenshot (success, stale version, non-screenshot kind,
  foreign or claimed attachment, the old attachment deleted only on commit)
  for SQLite, and Postgres where configured.
- Go: the embedded `inspector.js` declares protocol 3. Node-executed tests
  check that the mode persists after a capture, Escape is routed by annotation
  state, `selection-changed` is sent, and region layout follows a scrolled
  container.
- Vitest: protocol 3 validators; shared geometry including corner resize;
  `RegionOverlay` with and without handles (question-review tests unchanged);
  the draft list (append, dismiss keeps, resume, discard, screenshot limit);
  the Escape reducer; recapture failure rollback.
- Desktop Chromium E2E: select text outside the mode and comment through the
  floating button; enter the mode and draw two regions in a row without
  reopening anything; dismiss one and resume it; resize a saved region and see
  the image replaced; scroll an inner container and see the outline follow;
  Escape steps out layer by layer with focus inside the iframe; Exit restores
  page interaction.
- Mobile Chrome E2E: the bottom toolbar, the selection dock, drawing a region,
  and Exit.

## Implementation plans

- [Web preview annotation mode](../../../plans/web-preview-annotation-mode/plan.md)
- [Web preview annotation review](../../../plans/web-preview-annotation-review/plan.md) (with part 4)
