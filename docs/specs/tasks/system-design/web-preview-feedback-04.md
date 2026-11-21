---
status: current
system: tasks
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-005
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-006
---

# Web Preview Feedback System Design, Part 4: Comment Panel and Capture Feedback

## Purpose and boundaries

This part adds a hideable comment panel to annotation mode, changes how
unsaved captures are dismissed, makes markers open their comments, and fixes
the capture defects found while testing [part 3](web-preview-feedback-03.md).
Part 3 stays the design for the mode, tools, regions, recapture, and scroll
tracking. Where the two differ, this part wins; part 3's Drafts, Escape and
exit, and Layout sections point here.

Unchanged: the pending collection, composer delivery, the backend, the
screenshot limit, and comment-on-selection outside the mode.

[Part 5](web-preview-feedback-05.md) replaces this part's inline panel
composer with floating comments only, makes the toolbar movable, closes an
open comment on a tool change, and anchors region outlines to page content.
Where the two differ, part 5 wins.

## Requirement mapping

| Criterion | Section |
| --- | --- |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-005.2` | [Toolbar](#toolbar) |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-005.4`, `005.5`, `006.4` | [Closing a comment](#closing-a-comment) |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-006.1` | [Comment panel](#comment-panel) |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-006.2` | [Numbering and markers](#numbering-and-markers) |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-006.3` | [Where a comment is written](#where-a-comment-is-written) |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-006.5` | [Capture feedback](#capture-feedback) |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-006.6` | [Pausing the tool](#pausing-the-tool) |

## Defects found in testing

| Report | Cause |
| --- | --- |
| Region capture stays on "Capturing screenshot..." | `use-preview-screenshots` sets `mountedRef.current = false` in an effect cleanup and never sets it back. React StrictMode in dev builds runs mount, cleanup, mount, so every capture result is dropped and the busy counter is never decremented. Production builds and E2E were unaffected. html2canvas itself captures the same page with the injected inspector in about 100 ms. |
| No way to dismiss an empty comment box | The desktop surface has no close control and dismisses only on a parent-document `mousedown` or Escape. Clicks in the iframe never reach the parent document. |
| Escape leaves a numbered marker | Part 3 keeps every dismissed capture as a draft, including empty ones. |
| Clicking a marker does nothing | In-page markers are `pointer-events: none`; region outlines open only with the Region tool. |
| Count button opens the list but does not close it | The button's `onClick` sets open while the popover trigger also toggles. |
| Element comment box flashes; dragging it highlights other elements | The Element tool keeps tracking `mousemove` while the comment is open, and the drag handle loses `mousemove` to the iframe whenever the pointer crosses it. |
| Region disappears while capturing | The bridge hides its drag rectangle on release, and the parent draws outlines only for drafts and items, which do not exist until the image is ready. |

## Comment panel

`PreviewCommentPanel` replaces the pending popover in annotation mode. It
renders the existing collection list body from `preview-feedback-collection.tsx`
(route groups, Edit, Delete, Clear all) after an Unsaved group, so the list is
not duplicated. The header popover outside the mode keeps using the same body.

- **Fine pointers.** The layer's host becomes a row: the preview iframe, then
  a 320-pixel panel with a left border. When the host is narrower than
  720 pixels, the panel overlays the right edge instead, so the page keeps at
  least 400 pixels. Docking reflows the page when the panel opens or closes;
  the inspector already re-projects markers and regions on resize.
- **Coarse pointers.** The panel is a `Drawer` bottom sheet opened by the
  toggle. It starts hidden, because a sheet covers most of a phone page.
- **Preference.** `usePreviewCommentPanel` wraps `useLocalStorageBoolean`
  with the key `kandev:preview-comment-panel-open` (default open) for fine
  pointers. Coarse pointers start hidden on every entry and do not change the
  stored value. `PreviewAnnotationHost` lays out the frame, layer, and panel
  in both the Browser panel and HTML preview.
- Hiding the panel never discards anything. An entry being written when the
  panel hides moves to the floating box with its text, because the open draft
  and its comment live in the capture state, not in the panel.

## Toolbar

The count button becomes the panel toggle: a pressed toggle button with the
comment icon and the pending count (saved plus unsaved), named "Show
comments" or "Hide comments" by state through visually hidden text, so the
visible count stays part of the accessible name. It owns the open state directly, so it
opens and closes on each press. Tools and Exit are unchanged. The header
Annotate button keeps entering and leaving the mode.

## Where a comment is written

The shared surface's composer body (evidence, textarea, error messages, and
Add or Save, Delete, Close actions, with `usePopoverComposer`) is extracted
from `DesktopSelectionCommentSurface` into an exported
`SelectionCommentComposer`. The floating popover, the touch drawer, and the
panel's inline entry all render it, so the panel does not add a second comment
editor.

- **Panel open.** A new capture adds an entry at the end of its route group in
  the panel, scrolls it into view, and focuses the textarea. Editing a saved
  item replaces its row with the same composer.
- **Panel hidden.** A new capture opens the floating `SelectionCommentSurface`
  anchored next to the capture, as in part 3. Activating a marker opens the
  saved item's editor or the draft in the same surface.

At most one comment is open, as in part 3.

## Closing a comment

`SelectionCommentSurface` gains an optional `onCancel`. When provided, a
Close button sits beside Add in the popover, the inline form, and the drawer
(44 pixels on coarse pointers); other surfaces are unchanged until they opt in.
Preview passes `onCancel` on every surface and panel entry. An `addDisabled`
prop keeps Add disabled while a region's image is still being captured.

Closing by X, Escape, or outside click calls one reducer:

- Empty comment on a new capture: discard the draft, its PNG, its staged
  upload, and its marker or outline.
- Typed comment on a new capture: keep it as an unsaved draft (dashed).
- Editing a saved item: close without saving.

While a floating comment box is open, the bridge reports `pointerdown` inside
the page as an outside click. It is sent as `page-pointerdown` only while the
parent has marked a surface open through `set-annotation-active`'s new
`surface_open` field, so pages are not observed otherwise.

The Escape order from part 3 is unchanged; step 1 now uses this reducer.

## Numbering and markers

The parent numbers captures once: per route, saved items in collection order,
then drafts in creation order. The number is sent with each marker and region
in `project-markers` (`number` field), and the panel shows the same number.
The bridge stops computing `index + 1`.

While annotation mode is active, the bridge makes marker badges
`pointer-events: auto` with a pointer cursor and sends `marker-activated {id}`
on click. Outside the mode they stay pointer-transparent. `RegionOverlay`
gains `onSelect(index)`; its number badge is clickable in every tool, while
moving and resizing stay limited to the Region tool. A region drag that starts
on a marker badge still draws a region; a click without a drag opens it.

`project-markers` carries `highlighted_id` (the open comment or the selected
entry), drawn with a thicker ring, and `reveal_id`, set only when an entry is
picked in the panel. The bridge scrolls only the revealed marker into view, so
opening a new capture's comment never moves the page.

## Capture feedback

- **Lifecycle bug.** The `mountedRef` effect sets `true` in its body and
  `false` in cleanup. A StrictMode `renderHook` test covers the regression.
- **Capturing outline.** `screenshot-region-selected` immediately adds a
  draft in a `capturing` state with its capture rectangle and no image. It is
  projected as a region like any draft, so the outline follows the document or
  the inner scroll container while capture runs. `RegionOverlay` gains
  `busyIndexes`; a busy outline shows a rotating conic-gradient border
  (`@property --angle` animation, static dashed border under
  `prefers-reduced-motion`). The comment surface opens at the same time, so
  the user can type while the image is captured. Saving waits for the image.
- **Content under the rectangle at release.** html2canvas renders from the
  document coordinates recorded at release, so document scrolling during
  capture does not matter. For inner scroll containers, `onclone` restores the
  recorded `container_scroll_x` and `container_scroll_y` on the cloned
  container before rendering.
- **Timeout.** Rasterization races a 15-second timer. On failure or timeout
  the capturing draft is removed, its outline disappears, and the existing
  screenshot error is shown. Recapture keeps the previous image, as in
  part 3.
- A capturing draft counts toward the screenshot limit.

## Pausing the tool

When a text or element comment opens, the parent sends
`set-capture-mode {mode: null}` while keeping annotation active, and projects
the captured element or text as a highlighted pending marker. When the comment
closes, the parent restores the selected tool. The toolbar still shows the
selected tool as pressed. Region captures do not pause, because the Region
tool has no hover state.

The shared `useDrag` adds a transparent full-viewport drag shield
(`position: fixed; inset: 0`) for the duration of a header drag, so
`mousemove` stays in the parent document when the pointer crosses an iframe.
This also fixes dragging the surface over other embedded frames.

## Bridge changes

The changes are additive, so the protocol stays at version 3:

- inspector to parent: `marker-activated {id}`, `page-pointerdown {}`;
- parent to inspector: `set-annotation-active` gains `surface_open`;
  `project-markers` markers and regions gain `number`, and the payload gains
  `highlighted_id` and `reveal_id`.

Parent validators accept the new events. An older inspector never sends them,
and missing `number` falls back to list position.

## Verification

- Vitest: the screenshot hook under StrictMode; the 15-second timeout removing
  the capturing draft; `onclone` restoring container scroll; the close
  reducer (empty discards, typed keeps, saved closes); numbering across items
  and drafts; the panel preference; `SelectionCommentSurface` X and Close
  controls; `useDrag` shield; validators for the new events.
- Node-executed inspector tests: badges clickable only in the mode and
  sending `marker-activated`; `page-pointerdown` only while `surface_open`;
  `number` rendered from the payload.
- Desktop Chromium E2E (the StrictMode regression is covered by Vitest,
  because E2E serves a production build): draw a region and see the busy
  outline then the image; scroll during capture; close
  an empty capture with X and see its outline gone; close a typed one and see
  it dashed in the panel; hide the panel and see the page full width; click a
  marker to edit; the toggle opens and closes; dragging an element comment box
  does not change the highlight.
- Mobile Chrome E2E: the sheet starts hidden, opens from the toggle, and a
  region capture opens the drawer composer.

## Implementation plans

- [Web preview annotation review](../../../plans/web-preview-annotation-review/plan.md)
