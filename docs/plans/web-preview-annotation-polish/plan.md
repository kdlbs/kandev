---
created: 2026-11-18
status: implemented
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-002
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-005
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-006
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-007
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback-04.md
  - ../../specs/tasks/system-design/web-preview-feedback-05.md
legacy_specs: []
---

# Implementation plan: Web preview annotation polish

## Overview

The third test round of the [annotation review](../web-preview-annotation-review/plan.md)
reported seven issues:

1. Comments should always be written in the floating comment box, even when
   the panel is open.
2. The floating toolbar is easy to miss on a similar background, and it covers
   the page beneath it. It should be outlined and movable.
3. Choosing another tool while a comment is open changes the pressed tool, but
   the new tool does nothing until the comment is closed. Switching should
   close the comment instead.
4. The panel toggle on the toolbar should be accented so it is clearly there
   and important.
5. Opening the panel shifts the page under region outlines, and the outlines
   do not follow.
6. After leaving and reopening an HTML preview, the markers are gone from the
   page but still listed in the panel. Editing them does not bring them back,
   and new captures start again at 1.
7. Resizing the window moves the page under a region outline. Where that
   cannot be solved, flag it, because adjusting a drifted region captures a
   different place.

Decisions from the round:

- Captures always take exactly what is visible inside the outline. A layout
  change only moves or flags an outline; it never captures on its own.
- Regions follow the content they were drawn over. When that is no longer
  reliable, the outline turns into a dashed amber warning with a tooltip.
- Every captured region gets an eye button that shows the saved capture over
  the live page, for comparison.
- Switching tools closes an open comment by the existing rules: an empty
  capture is removed and a typed one is kept as a draft.
- Root causes are recorded in
  [part 5](../../specs/tasks/system-design/web-preview-feedback-05.md#defects-found-in-testing).
  Issue 6 is the HTML preview's `?v=` cache buster leaking into page routes.

## Requirements and designs

| Area | Criteria | Design |
| --- | --- | --- |
| Page identity | `002.5` | [part 5: Page identity](../../specs/tasks/system-design/web-preview-feedback-05.md#page-identity) |
| Floating comments and tool changes | `005.3`, `006.2`, `006.3`, `006.4` | [Floating comments only](../../specs/tasks/system-design/web-preview-feedback-05.md#floating-comments-only), [Changing the tool](../../specs/tasks/system-design/web-preview-feedback-05.md#changing-the-tool) |
| Toolbar | `005.2` | [Toolbar](../../specs/tasks/system-design/web-preview-feedback-05.md#toolbar) |
| Region anchors and drift | `007.1`, `007.2`, `007.3` | [Region anchors](../../specs/tasks/system-design/web-preview-feedback-05.md#region-anchors), [Drift](../../specs/tasks/system-design/web-preview-feedback-05.md#drift) |
| Showing the saved capture | `007.4` | [Showing the saved capture](../../specs/tasks/system-design/web-preview-feedback-05.md#showing-the-saved-capture) |

All criteria IDs above are `AC-TASKS-WEB-PREVIEW-FEEDBACK-*`. `REQ-005` to `007` moved to
[Web Preview Annotation Mode Requirements](../../specs/tasks/requirements/web-preview-annotation-mode.md)
in this package, because the feedback requirements reached the 20 KiB limit;
their IDs are unchanged.

## Work orders and dependency order

| ID | Work order | Wave | Depends on |
| --- | --- | --- | --- |
| 01 | [Page identity without Kandev parameters](task-01-page-identity.md) | 1 | none |
| 02 | [Region anchors and drift in the bridge](task-02-region-anchors.md) | 1 | none |
| 03 | [Floating comments only and closing on tool change](task-03-floating-comments.md) | 1 | none |
| 04 | [Movable, outlined toolbar with an accented panel toggle](task-04-toolbar.md) | 2 | 03 |
| 05 | [Drift warning and saved-capture view on regions](task-05-region-overlay.md) | 2 | 02 |
| 06 | [E2E coverage and public docs](task-06-e2e-and-docs.md) | 3 | 01, 04, 05 |

Wave 1 touches separate files: the route sanitizers and URL builder (01), the
inspector script and bridge validators (02), and the capture hook, panel, and
layer (03). Task 04 follows 03 because both edit the layer and the annotation
host. Task 05 consumes 02's `drifted` and anchor messages.

## ASCII UI previews

The drawings show structure only. All copy goes through `t()` in seven
locales.

### UI-01: Desktop, panel open, comment in the floating box (`005.2`, `006.2`, `006.3`)

```text
+-- Browser ------------------------------------------------ [Annotate 3] --+
|#################################################| Comments           [x] |
|# [[ :: [Interact][Text*][Element][Region] [P 3] [Exit] ]]| /pricing       |
|#                                                 #|  1  [thumb] "bigger"    |
|#   +1-------------+ [o] <- eye on hover          #|      [Edit] [Delete]    |
|#   |              o                              #|  2  "Buy now"  < open   |
|#   +--------------o                              #|      [Edit] [Delete]    |
|#   +------------------------+                    #|            [Clear all]  |
|#   | Buy now  [=====]       |  <- highlighted    #|                          |
|#   +------------------------+                    #|                          |
|#      +-- :: ------------------------------+     #|                          |
|#      | <button> "Buy now"                 |     #|                          |
|#      | [Make this the primary action   ]  |     #|                          |
|#      |                     [Close] [Save] |     #|                          |
|#      +------------------------------------+     #|                          |
|#################################################|                          |
+---------------------------------------------------------------------------+
```

`[[ ... ]]` marks the toolbar's 2-pixel primary outline and stronger shadow;
`::` is its drag handle. `[P 3]` is the accented panel toggle: a panel icon
and the count, outlined in primary and tinted while pressed. The panel has no
comment form; entry 2 is marked as the one open in the floating box.
Requirement: comments are written only in the floating box. Illustrative:
exact spacing and icon glyphs.

### UI-02: Toolbar moved off the content beneath it (`005.2`)

```text
|##########################################################################|
|#   Pricing                                                              #|
|#   +1---------------------------+                                       #|
|#   |  Starter   Pro   Team      |     [[ :: [I][T][E][R*] [P 3][Exit] ]]#|
|#   +----------------------------+                                       #|
|##########################################################################|
```

The device remembers the position. Double-clicking `::` returns the toolbar
to the top center. The toolbar stays inside the preview and above the overlay
panel on narrow hosts.

### UI-03: A drifted region after a narrower reflow (`007.3`)

```text
|#   + -1- - - - - - - - -+ [!]   <- dashed amber, warning badge         #|
|#   :   (content moved)  :                                               #|
|#   + - - - - - - - - - -+                                               #|
|#      +----------------------------------------------------------+     #|
|#      | The page layout changed since this capture. The          |     #|
|#      | screenshot still shows the original area. Moving or      |     #|
|#      | resizing this region captures it again from its new      |     #|
|#      | position.                                                |     #|
|#      +----------------------------------------------------------+     #|
```

The tooltip opens on hover or focus of `[!]`, and on tap for coarse pointers.

### UI-04: Showing the saved capture (`007.4`)

```text
|#   +1-----------------------+[o]  <- pressed: "Hide capture"            #|
|#   |#### saved image #######|                                           #|
|#   |########################|                                           #|
|#   +------------------------+                                           #|
```

### UI-05: Phone (`005.2`, `007.4`)

```text
+---------------------------+
|###########################|
|#  +1------+[o]  (2)      #|   <- eye always visible, 44 px, tap toggles
|#  +-------+              #|
|###########################|
| [I][T][E][R*] [P3] [Exit] |   <- bottom bar, not movable
+---------------------------+
```

Comments open in the shared drawer composer, as before.

## Verification strategy

Each work order lists its exact commands. Across the package:

- Vitest for route sanitizers and the URL builder (01), validators and the
  anchor request (02), tool changes and the list-only panel (03), toolbar
  position storage (04), and `RegionOverlay` drift and eye controls (05).
  The question-review annotation tests stay green after 05.
- Node-executed inspector tests for anchors, drift, observers, and the
  `highlighted` rectangle (02).
- Playwright `chromium` and `mobile-chrome` for the flows in part 5's
  verification section (06), plus the existing preview specs as regression.
- `make fmt`, then `make typecheck test lint`, and `pnpm run i18n:check`,
  before the commit.

## Risks

- **02:** a `MutationObserver` on a busy page (animations, live counters)
  fires often. Layout is coalesced per frame, unchanged results are not
  resent, and observers run only while the current route has regions.
- **02:** the smallest containing element can be a wrapper that changes size
  with unrelated content, so a region may be flagged as drifted while it is
  still correct. The 8-pixel or 5-percent tolerance limits this, and moving
  the region clears the flag.
- **02:** anchor fields reach the agent inside the capture JSON. They are
  selectors and pixel numbers, the same kind of data as the existing
  `scroll_container_selector`.
- **03:** the floating box can cover part of the docked panel or the content
  being commented on. It stays draggable, as today.
- **04:** rendering the toolbar at the host level moves it out of the
  annotation layer. Escape handling, `stopPropagation` on presses, and the
  E2E test IDs must keep working.
- **05:** `RegionOverlay` is shared with question-review annotations. Every new
  prop is optional, and copy comes from the host.

## Exclusions

- Re-anchoring text or element markers; they already follow their elements.
- Migrating pending HTML-preview items saved with a `?v=` route.
- Automatic recapture after a layout change.
- Moving the coarse-pointer bottom bar.

## Implementation record

Delivered as planned, with these refinements recorded in part 5:

- The region view helpers moved from `use-preview-capture.ts` to
  `hooks/preview-region-views.ts`, keeping the capture hook under the file
  size limit.
- The toolbar shares `startMouseDrag` with the comment surface's `useDrag`
  rather than reusing `useDrag` itself, because it places an absolute
  position instead of accumulating an offset. A double-click on its handle is
  read from the second press, because the drag shield receives the release.
- The drift badge and eye button sit centered on the region's top edge. The
  first placement, at the top-right corner, covered the corner resize handle.
- `requestRegionAnchor` tests live in `lib/preview-region-anchor-request.test.ts`
  because the bridge validator suite runs in the Node environment.
- The inline `SelectionCommentComposer` was deleted; no other surface used it.
- Mobile Chrome E2E keeps the existing bottom-bar and sheet coverage. The eye
  button's tap toggle is covered by Vitest and the coarse-pointer styles,
  because drawing a region by touch is not reliable in the emulated E2E device.
- Test feedback: text and element markers stayed visible, but inert, after
  leaving annotation mode. Following the chat and plan pattern, they stay
  visible and are now clickable outside the mode, opening the comment in the
  floating box; region outlines and their markers stay mode-only, and each
  save is announced to screen readers. `005.5` records this.
- Test feedback: a marker's comment box first rendered at the fallback
  corner, then jumped beside the marker when the page reported its layout,
  replaying the open animation. The click now carries the target's position,
  and the preview's boxes do not animate (`005.8`). Hovering a marker now
  highlights the commented text or element, and the highlight stays while
  the comment is open (`005.9`). The inspector script test harness moved to
  `lib/preview-inspector-script.test-helpers.ts` so the new highlight suite
  can share it.
- PR review: overlapping captures of one draft now apply only the latest
  image, and saving re-uploads when a recapture replaced the image mid-upload.
  Regions are clipped to, or hidden outside, their scroll container; region
  captures are revealed from the comment list through their layout; moved
  regions take the scroll container the page reports; a corner resize hides
  a shown saved capture. The part 5 design records each rule.
