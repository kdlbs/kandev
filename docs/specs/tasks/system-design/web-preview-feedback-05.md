---
status: current
system: tasks
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-002
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-005
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-006
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-007
---

# Web Preview Feedback System Design, Part 5: Floating Comments, Toolbar, and Region Alignment

## Purpose and boundaries

This part changes where comments are written, makes the toolbar movable and
easier to see, closes an open comment when the tool changes, keeps region
outlines on their content when the layout changes, and fixes markers that
disappeared after reopening an HTML preview. It follows the third test round
of [part 4](web-preview-feedback-04.md). Where the two differ, this part wins.

Unchanged: the pending collection, composer delivery, the backend, the
screenshot limit, numbering, closing rules, pausing, and capture feedback.

## Requirement mapping

| Criterion | Section |
| --- | --- |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-002.5` | [Page identity](#page-identity) |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-005.2` | [Toolbar](#toolbar) |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-005.3` | [Changing the tool](#changing-the-tool) |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-006.2`, `006.3`, `006.4` | [Floating comments only](#floating-comments-only) |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-007.1`, `007.2` | [Region anchors](#region-anchors) |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-007.3` | [Drift](#drift) |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-007.4` | [Showing the saved capture](#showing-the-saved-capture) |

## Defects found in testing

| Report | Cause |
| --- | --- |
| Reopening an HTML preview loses markers, outlines, and numbering, while the panel still lists the comments | `buildHtmlPreviewProxyUrl` adds `?v=<version>`, and each publish increments the version. `v` is not a sensitive parameter, so both route sanitizers keep it in `page_route`. After reopening, the current route no longer equals the saved routes, so the inspector draws none of their markers and `captureNumbers` starts the new route at 1. The panel lists every item of the source regardless of route. Editing a saved item opens its editor, but its outline stays hidden for the same reason. |
| A region does not follow its content when the panel opens or the window resizes | Regions are stored in document coordinates (`document_x`, `document_y`), plus a scroll container's offsets. Docking the panel narrows the iframe, which reflows the page, and centered or wrapped content moves while the document coordinates do not. Element and text markers already follow their elements by selector. |
| Choosing another tool while a comment is open changes the pressed tool, but the new tool does nothing until the comment closes | A text or element comment pauses the bridge (`set-capture-mode {mode: null}`, part 4). `setTool` updates the toolbar but the pause holds until the comment closes. |

## Page identity

Kandev-added query parameters are not part of a page's identity:

- Both route sanitizers, the inspector's `sanitizedSearch` and
  `sanitizePreviewPageRoute`, drop parameters whose names start with
  `kandev_`, alongside the existing sensitive-parameter rule.
- `buildHtmlPreviewProxyUrl` sends its cache-busting version as
  `kandev_preview_version` instead of `v`. The proxy ignores it either way;
  only the iframe reload depends on the URL changing.

Reopening an HTML preview then reports the same route as before, so its
markers, outlines, and numbers return and new captures continue the
numbering. Items saved before this change keep their `?v=` route: they stay in
the panel and are delivered normally, but their markers do not return. Pending
feedback is short-lived, so no migration is added.

## Floating comments only

Every comment is written and edited in the shared floating
`SelectionCommentSurface` next to its capture, with the panel open or hidden.
On coarse pointers the same surface renders its drawer, as today.

- The panel becomes a list. `PreviewCommentPanel` stops rendering
  composers: `PreviewInlineComments` keeps `numbers`, `selectedId`, `activeId`
  (to mark the entry whose comment is open), and `onOpen`, and loses
  `renderComposer`. The inline modes of `PendingFeedbackItem` and
  `DraftRow` are removed. `SelectionCommentComposer` is deleted if no other
  surface uses it after this change; its body stays shared through
  `CommentComposerBody`.
- `usePreviewCapture` drops `inlineComments`. `PreviewAnnotationLayer`
  always renders `PreviewDraftSurface` and `PreviewSavedRegionEditor`, and
  `set-annotation-active` sends `surface_open` whenever a comment is open, so
  a press in the page closes it in both panel states.
- A panel entry's Edit, or picking an entry, calls `selectCapture(id)` and
  `openCapture(id)`. `selectCapture` sets `reveal_id`, so the page scrolls the
  marker or outline into view and the floating box anchors next to it.
- **Anchoring a saved item's editor.** Regions anchor to their current
  outline, as today. Text and element items anchor to their marker: the
  inspector's `region-layout` message gains `highlighted: {id, rect} | null`,
  the viewport rectangle of the highlighted marker, and `editingAnchor` uses
  it. When the capture is on another route or its anchor does not resolve,
  the box opens at the frame's top-left fallback.
- **Markers outside the mode.** `useRegionProjection` sends no region
  anchors while annotation mode is off and drops screenshot markers, so only
  text and element markers stay. The inspector makes every marker clickable
  and reports `marker-activated` in either mode; `openCapture` opens the
  floating editor, anchored by the `highlighted` layout as in the mode.
  `set-annotation-active` carries `surface_open` whenever a comment is open,
  in or out of the mode, so a press in the page closes it (`005.5`).
- **Save announcement.** `usePreviewDraftSave` counts successful saves, and
  the annotation trigger renders a polite `role="status"` region that
  announces "Comment saved. N pending." once per save (`005.5`).
- **Opening in place.** `marker-activated` carries the marker target's
  viewport rect, computed by the same `markerLayout` that fills the
  `highlighted` layout. `usePreviewCapture` keeps it as `activated`, and
  `editingItemRect` and `openDraftRect` use it until a matching `highlighted`
  layout arrives, so the box is placed in its first frame and the later
  layout lands on the same point. The preview's `SelectionCommentSurface`
  instances render with `animated={false}`; other surfaces keep the fade
  and zoom (`005.8`).
- **Target highlight.** The inspector tracks the hovered marker from
  document `mouseover` and `mouseout` events, because markers are redrawn on
  every scroll. It highlights the hovered marker's target and the
  `highlighted_id` target together. Text resolves its saved endpoints to a
  `Range` and is accepted only when the range's whitespace-normalized text
  starts with the marker's `selected_text`, which Kandev sends truncated to
  4,096 characters. Accepted ranges are painted through `CSS.highlights`
  where the browser has it, otherwise as fixed boxes from the range's client
  rects. A changed text falls back to the saved `text_anchor.rects`; an
  element uses its live bounding box, then its `capture_rect`. Boxes are
  pointer-transparent and appended outside `body`, so they never affect
  layout or the layout observers (`005.9`).

## Toolbar

- **Visibility.** On fine pointers the toolbar gets an opaque background, a
  2-pixel `ring-primary` outline, and `shadow-lg`, matching the mode's
  outline color so it reads as part of the mode on any page background.
- **Moving it.** A grip handle at the toolbar's left edge drags it anywhere
  over the preview frame. `hooks/use-drag-offset.ts` holds the shared drag
  session, `startMouseDrag` (the drag shield and document listeners), which
  both the comment surface's `useDrag`, moved there from
  `selection-comment/surface-parts.tsx`, and the toolbar use.
  `usePreviewToolbarPosition` stores the position in localStorage
  (`kandev:preview-annotation-toolbar-position`) as fractions of the free
  space, `left / (frameWidth - toolbarWidth)` and
  `top / (frameHeight - toolbarHeight)`, each clamped to 0 to 1. The toolbar
  keeps its relative place when the preview resizes and can never leave it.
  The default is top center. Double-clicking the handle resets to the
  default. Because the drag shield receives the release, the handle sees no
  `dblclick`; the second press (`event.detail` of 2) resets instead. With the
  handle focused, arrow keys move the toolbar 16 pixels. The toolbar measures
  itself in a layout effect, so it never paints at a wrong position first.
- **Stacking.** `PreviewAnnotationHost` renders the toolbar itself, positioned
  over the frame box, in a layer above the overlay panel, so it stays usable
  when dragged over the panel on a narrow host. The overlay panel still starts
  below the default toolbar position.
- **Panel toggle.** The toggle uses an outlined accent style
  (`border-primary text-primary`, with `bg-primary/10` while pressed) and a
  panel icon (`IconLayoutSidebarRight`) next to the count, so it differs from
  the filled tool buttons.
- Coarse pointers keep the bottom bar unchanged; it is not movable.

## Changing the tool

`usePreviewCapture` exposes `selectTool(tool)` and the toolbar calls it
instead of `annotation.setTool`. Choosing a tool other than the current one
first calls `closeComment()`, using part 4's reducer: an empty new capture is
discarded, a typed one is kept as a draft, and a saved item's editor closes
without saving. Closing ends the pause, so the bridge receives the new mode
immediately. Choosing the current tool does nothing. Escape's order is
unchanged.

## Region anchors

At capture time the inspector records the content under the region, and every
layout pass draws the outline from that content.

- **Choosing the anchor.** On drag end, the inspector starts at
  `elementFromPoint` at the region's center, skips inspector UI, and walks up
  to the first element whose bounding rectangle contains the whole region
  (1-pixel tolerance). `html` and `body` never anchor. The anchor needs a
  unique selector from the existing `getSelector`; otherwise the region is
  unanchored.
- **Stored fields.** `capture_rect` gains `anchor_selector`,
  `anchor_offset_x`, `anchor_offset_y` (region top-left minus anchor top-left,
  in CSS pixels), `anchor_width`, and `anchor_height`. The backend stores
  `capture_rect` as opaque JSON, so it needs no change. The agent receives the
  fields in the existing fenced capture JSON.
- **Layout.** `regionLayout` resolves the anchor and places the outline at the
  anchor's `getBoundingClientRect()` plus the stored offset, keeping the
  captured size. The bounding rectangle already includes document and
  container scrolling. When the scroll container is the anchor or one of its
  ancestors, content inside the container scrolls without moving the anchor,
  so the container's scroll delta is subtracted as today. Unanchored regions
  keep document coordinates.
- **When to lay out again.** Besides `scroll` and `resize`, the inspector
  observes `documentElement`, `body`, and each resolved anchor with a
  `ResizeObserver`, and the body subtree with a `MutationObserver` (child
  lists, `style`, and `class`). Every trigger is coalesced into one layout per
  animation frame, and an unchanged result is not resent, as today. The
  observers run only while regions are projected for the current route.
- **Moves and resizes.** A moved or resized region needs a new anchor. The
  parent sends `anchor-region {request_id, rect}` with the outline's viewport
  rectangle, and the inspector answers
  `region-anchored {request_id, anchor, container}` using the same rule, or
  `anchor: null`. `container` is the scroll container under the new position
  (selector and scroll offsets), or `null` when there is none; the moved
  region replaces its previous container with it, so a region moved out of a
  panel stops following that panel. The parent requests the anchor while
  rasterizing and waits at most 500 ms; without an answer, such as from an
  older inspector, the region is saved unanchored and keeps its previous
  container at that container's current scroll.
- **Overlapping captures.** Each capture or recapture of a draft's image
  takes a generation number, and only the latest generation applies its
  image or failure. An earlier capture that finishes later is dropped
  instead of overwriting a newer rectangle, and its failure no longer
  discards the draft. Saving checks that the uploaded image is still the
  draft's image; when a recapture replaced it during the upload, the stale
  upload is deleted and the current image and rectangle are saved.
- **Scroll containers.** A region scrolled partly outside its scroll
  container's box reports that box as `clip`, and the overlay cuts the
  outline, its controls, and its hit area to it; a region scrolled entirely
  outside is reported not visible. Moving or resizing draws the region whole.
  Revealing a region from the comment list scrolls its container, then the
  document, using its layout, because region captures have no in-page marker.
  Starting a move or a corner resize hides a shown saved capture.
- **What a capture contains.** Capture and recapture rasterize the outline's
  current viewport rectangle, so they take what is visible inside it at that
  moment. A layout change only moves or flags the outline; nothing is captured
  again until the user moves or resizes the region.

## Drift

`regionLayout` reports `drifted: true` when the outline cannot be placed
reliably:

- The anchor selector does not resolve, or the anchor has no rendered box
  (zero width and height, for example under `display: none`). The outline
  falls back to document coordinates.
- The anchor's size differs from `anchor_width` or `anchor_height` by more
  than 8 pixels or 5 percent, whichever is larger. Its content has probably
  reflowed. The outline still follows the anchor's top-left corner.
- An unanchored region whose current viewport width differs from the capture's
  `viewport_width` by more than 8 pixels.

`PreviewRegionView` gains `drifted`, and `RegionOverlay` gains an optional
`driftedIndexes` set and `labels`. The host passes the translated strings,
because the overlay is shared with question-review annotations. A drifted
outline is dashed amber (`border-amber-500`) and shows a warning badge
(`IconAlertTriangle`) centered on its top edge, clear of the number badge and
the corner handles. The badge is focusable and shows the
tooltip from `007.3`; on coarse pointers a tap opens it. Moving or resizing
the region recaptures it with a new anchor, which clears the drift.

## Showing the saved capture

`RegionOverlayItem` gains an optional `imageUrl`: the attachment URL for
saved items, or the draft's object URL once captured. When it is set, the
outline shows an eye button (`IconEye`, labelled "Show capture" or "Hide
capture", `aria-pressed`) centered on its top edge, next to any drift badge.
Its icons ignore pointer events, so swapping the icon during a press does not
cancel the click. On fine pointers the
button appears on hover or focus within the outline and shows the image while
pressed; a click without holding toggles it. On coarse pointers it is always
visible at 44 pixels and a tap toggles it. The image fills the outline with
`object-contain` over the live page, and the outline keeps its handles. It is
hidden again when the region starts moving or resizing.

## Bridge changes

The changes are additive, so the protocol stays at version 3:

- inspector to parent: `screenshot-region-selected` capture rectangles carry
  the anchor fields; `region-layout` regions gain `drifted`, and the payload
  gains `highlighted`, and regions may carry `clip`; new
  `region-anchored {request_id, anchor, container}`;
- parent to inspector: new `anchor-region {request_id, rect}`.

Parent validators bound `anchor_selector` like other selectors and treat a
missing `drifted` as false. An older inspector sends no anchors, so its
regions behave as before.

## Verification

- Vitest: route sanitizers dropping `kandev_` parameters, and the HTML preview
  URL builder; `selectTool` closing an open comment and resuming the bridge;
  the panel rendering no composer and Edit opening the floating box;
  `editingAnchor` from `highlighted`; toolbar position storage, clamping, and
  reset; `RegionOverlay` drift styling, eye toggle, and labels; validators for
  the new fields and messages; the anchor request timeout.
- Node-executed inspector tests: anchor choice (smallest containing element,
  no `html` or `body`, no inspector UI); layout following a moved anchor;
  container scroll for an ancestor anchor; drift for a removed, hidden, or
  resized anchor and for an unanchored width change; `anchor-region` replies;
  layouts recomputed after a resize and a DOM mutation; the `highlighted`
  rectangle.
- Desktop Chromium E2E: reopen an HTML preview and see the outline and number
  return, and a new capture numbered 2; open the panel and see a region stay
  on its content; make the page reflow at a narrower width and see the drift
  badge, then move the region and see it cleared; show the saved capture with
  the eye button; comment with the panel open in the floating box; switch
  from Text to Region with a comment open and draw a region immediately; drag
  the toolbar and find it in the same place after a reload.
- Mobile Chrome E2E: the eye button toggles on tap, and the bottom bar is
  unchanged.

## Implementation plans

- [Web preview annotation polish](../../../plans/web-preview-annotation-polish/plan.md)
