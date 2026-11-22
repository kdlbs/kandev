---
status: active
system: tasks
created: 2026-11-18
owners:
  - kandev
---

# Web Preview Annotation Mode Requirements

## Overview

Kandev shall let a user annotate a previewed page in a persistent mode: pick
text, elements, and screenshot regions with tools that stay selected, write
each comment next to its capture, review every annotation in a hideable
comment panel, and keep region outlines aligned with the page as its layout
changes.

This document extends [Web Preview Feedback Requirements](web-preview-feedback.md),
which owns the pending collection, capture contents, delivery, and
terminology. It continues that document's requirement ID sequence.

## Requirements

### REQ-TASKS-WEB-PREVIEW-FEEDBACK-005: Annotate a page in a persistent mode

**Intent:** Let a user make many annotations in one pass, with tools that stay
selected, instead of reopening a one-shot menu for every capture.

**User story:** As a user reviewing a page, I want to switch between annotation
tools like in an image editor and comment on any text I select, so that
reviewing a whole page takes few clicks.

#### Acceptance criteria

- **AC-TASKS-WEB-PREVIEW-FEEDBACK-005.1:** When feedback controls are enabled
  and the user selects text in the preview outside annotation mode, Kandev
  shall offer a comment action for that selection: a floating comment button
  next to the selection on fine pointers, and the shared bottom selection dock
  on coarse pointers, as in chat and plan comments. Activating it shall create
  a text capture and open the comment surface. Clearing the selection shall
  hide the action.
- **AC-TASKS-WEB-PREVIEW-FEEDBACK-005.2:** Entering annotation mode shall
  outline the preview area and show a toolbar with the Interact, Text, Element,
  and Region tools, a comment panel toggle that shows the pending count, and
  an Exit control. The toolbar and
  Exit control shall stay visible for the whole mode. On fine pointers the
  toolbar floats over the top of the preview, outlined so it stands out from
  any page background, and the user can drag it anywhere over the preview to
  reach the content beneath it. It shall stay inside the preview when the
  preview resizes, and the device shall remember where it was left. The panel
  toggle shall be styled as an accented control, distinct from the tools. On
  coarse pointers the toolbar is a bottom bar with 44-pixel controls.
- **AC-TASKS-WEB-PREVIEW-FEEDBACK-005.3:** The selected tool shall stay active
  after each capture is saved or dismissed, so the user can make any number of
  captures without reopening a menu. Interact shall pass all page input
  through while keeping existing annotations visible. Choosing a different
  tool while a comment is open shall close that comment as in `005.4` and make
  the new tool usable immediately.
- **AC-TASKS-WEB-PREVIEW-FEEDBACK-005.4:** A capture whose comment is closed
  while it has typed text and has not been saved shall stay as an unsaved
  draft. It is drawn dashed on the page, listed as unsaved in the comment
  panel, and can be resumed or discarded. A capture closed with no text shall
  be removed, together with its marker or outline. A new capture shall not
  require discarding earlier drafts.
  Unsaved and saved screenshot captures together shall stay within the pending
  screenshot limit, with a visible message when the limit is reached.
- **AC-TASKS-WEB-PREVIEW-FEEDBACK-005.5:** Escape shall step back one layer at
  a time, whether focus is in Kandev or in the previewed page: close an open
  comment (as in `005.4`), then switch the tool to Interact, then
  exit annotation mode. The Exit control shall exit directly. Exiting shall
  remove the outline, toolbar, region outlines and their markers, and
  gesture ownership, and unsaved drafts shall remain resumable. Text and
  element markers stay on the page outside the mode, as comments stay
  highlighted in chat and plans: activating one opens its comment in the
  floating box without entering the mode, and a press elsewhere in the page
  closes it. The rest of the page behaves as it did before the mode. Saving a
  comment shall announce it, with the pending count, to assistive
  technology.
- **AC-TASKS-WEB-PREVIEW-FEEDBACK-005.6:** The Region tool shall let the user
  draw any number of rectangles. Each rectangle shall be captured when the
  drag ends and opens its comment surface. Unsaved and saved rectangles on the
  current page shall show numbered outlines that can be moved and resized;
  releasing a move or resize shall capture the region again, keep its comment,
  and, for a saved item, replace its image in a version-checked update. A
  failed recapture shall keep the previous image and position and show an
  error.
- **AC-TASKS-WEB-PREVIEW-FEEDBACK-005.7:** Region outlines shall follow the
  page content as the document, or the scrollable element the rectangle was
  drawn over, scrolls. When the page scrolls in a way the preview cannot
  observe, such as inside a canvas, the outline shall stay where it was drawn.
  Outlines for other routes shall be hidden after navigation while their items
  remain in the pending collection. Sending feedback shall use the images
  already captured and shall never capture at send time.
- **AC-TASKS-WEB-PREVIEW-FEEDBACK-005.8:** Activating a marker shall open its
  comment beside the marker in its first rendered frame, with no opening
  animation and no later movement while the page does not scroll or reflow.
  The preview's floating comment boxes appear in place rather than animating
  in.
- **AC-TASKS-WEB-PREVIEW-FEEDBACK-005.9:** With a fine pointer, hovering a
  text or element marker shall highlight what its comment points at: the
  commented text, or the commented element's outline. The highlight shall
  clear when the pointer leaves the marker, and shall stay while that
  comment is open. When the commented text is no longer on the page, the
  highlight shall use the text's boxes from capture time, or show nothing
  when those are unknown, rather than highlight other text. Highlighting
  shall not change the page's layout. Region markers are not highlighted,
  because their outline already shows the region.

### REQ-TASKS-WEB-PREVIEW-FEEDBACK-006: Review annotations in a comment panel

**Intent:** Keep every annotation on the page in view in one list, while
still allowing the full preview width.

**User story:** As a user annotating a page, I want a comment list beside the
preview that I can hide, so that I can find and revisit comments while writing
each one next to what it describes.

#### Acceptance criteria

- **AC-TASKS-WEB-PREVIEW-FEEDBACK-006.1:** In annotation mode, a comment panel
  shall list the saved items and unsaved drafts for the task. On fine pointers
  it is docked beside the preview and is open the first time the mode is
  entered on a device; on coarse pointers it is a bottom sheet that starts
  hidden. The toolbar's panel toggle shall show and hide it without leaving
  the mode, and hiding it shall return the full preview width to the page.
  The device shall remember whether the panel was open or hidden.
- **AC-TASKS-WEB-PREVIEW-FEEDBACK-006.2:** Panel entries and in-page markers
  and region outlines for the current page shall share one number per
  capture. In annotation mode, activating a marker or region number, or a
  panel entry's edit action, shall open that capture's comment for editing or
  deletion in the floating comment box. Selecting a panel entry shall
  highlight its marker and scroll it into view when the capture belongs to the
  current page.
- **AC-TASKS-WEB-PREVIEW-FEEDBACK-006.3:** Comments shall always be written
  and edited in the shared floating comment box next to the capture, whether
  the panel is open or hidden. The panel lists comments and does not contain a
  comment form; a new capture appears in it when saved or kept as a draft.
- **AC-TASKS-WEB-PREVIEW-FEEDBACK-006.4:** Every comment box shall have a
  visible close control that behaves as `005.4`, without requiring text.
  Clicking into the previewed page shall count as an outside click for an open
  comment box.
- **AC-TASKS-WEB-PREVIEW-FEEDBACK-006.5:** After a region drag ends, its
  outline shall stay visible with an animated border until the image is
  captured. The image shall show the content that was under the rectangle when
  the drag ended, even if the page or the scrollable element under it scrolls
  before capture finishes, and the outline shall follow that content in the
  meantime. A capture that fails or does not finish within a fixed time shall
  remove the outline and show an error, and it shall never leave a capturing
  state on screen.
- **AC-TASKS-WEB-PREVIEW-FEEDBACK-006.6:** While a comment for a text or
  element capture is open, the selected tool shall pause: the captured element
  or text stays highlighted, and moving the pointer or dragging the comment
  box shall not highlight or select other elements. The tool shall resume when
  the comment closes.

### REQ-TASKS-WEB-PREVIEW-FEEDBACK-007: Keep region annotations aligned with the page

**Intent:** Keep each region outline on the content it was drawn over when the
page's layout changes, and make it obvious when that is no longer possible, so
that adjusting a region never silently captures the wrong content.

**User story:** As a user annotating a page, I want region outlines to stay on
their content when the preview resizes or the page reflows, and to see when a
region no longer lines up, so that I can trust what each region captures.

#### Acceptance criteria

- **AC-TASKS-WEB-PREVIEW-FEEDBACK-007.1:** A region outline shall follow the
  content it was drawn over when the page's layout moves that content: when
  the comment panel opens or closes, when the preview or window resizes, and
  when the page reflows. This adds to the scroll tracking in `005.7`.
- **AC-TASKS-WEB-PREVIEW-FEEDBACK-007.2:** Every capture and recapture shall
  take exactly the content visible inside the outline at that moment. Kandev
  shall never capture content that is not visible, and shall never capture
  again on its own because the layout changed.
- **AC-TASKS-WEB-PREVIEW-FEEDBACK-007.3:** When a region can no longer be
  placed reliably, because the content it was drawn over was removed, hidden,
  or changed size, or because the page reflowed at a different width with
  nothing to follow, its outline shall be drawn in a distinct warning style
  with a warning badge. The badge's tooltip shall say that the page layout
  changed since the capture, that the screenshot still shows the original
  area, and that moving or resizing the region captures it again from its new
  position. Moving or resizing the region shall clear the warning.
- **AC-TASKS-WEB-PREVIEW-FEEDBACK-007.4:** Each region with a captured image
  shall offer a control that shows the saved capture inside the outline in
  place of the live page, so the user can compare them: on fine pointers it
  appears when hovering or focusing the region and shows the capture while
  pressed or until pressed again; on coarse pointers a tap toggles it. The
  control shall have an accessible name and state.

## Out of scope

- Freehand drawing, arrows, or shapes other than rectangles in annotation mode.
- Moving or resizing text and element captures; only region captures change
  geometry after capture.
- Automatic recapture after the page's layout changes.
