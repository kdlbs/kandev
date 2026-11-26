---
status: active
system: tasks
created: 2026-11-24
owners:
  - kandev
---

# Question image items and late-answer review

## Overview

An operator reviews a question's images in the interface that fits each one, and
can do so even after the question expired. Agents declare which images form a
before/after pair and which stand alone. The review view compares pairs, shows
singles on their own, shows ungrouped images as a gallery with on-demand
comparison, and always shows which image is on screen. The same view opens from
the late-answer form.

This extends [question image review](question-image-review.md), which remains
authoritative for the view, its layout, zoom, comparison modes, and annotations.
The task system owns it for the same reason: the clarification bundle owns
question identity, image references, answer state, and delivery.

Sources: [issue #5494](https://github.com/kdlbs/kandev/issues/5494) (REQ-001) and
[issue #5495](https://github.com/kdlbs/kandev/issues/5495) (REQ-002 to REQ-004).
The paired [system design](../system-design/question-image-items.md) defines the
implementation boundaries. The [plan](../../../plans/question-image-items/plan.md)
defines delivery.

## Terms

- **Late-answer form:** The answer form for a bundle that can no longer be
  answered in place. It opens from **Answer as new message** on an expired
  question, or stays open after an expired submission falls back to a new message.
  Its Send action delivers the answer as a new chat message.
- **Image group:** An agent-declared set of a question's images. A **pair** has
  exactly two images meant to be compared. A **single** has one image shown alone.
- **Item:** One screen of a segment's stage: a pair, a single, or one gallery image.
- **Grouped and ungrouped segments:** A segment is grouped when its question
  declares image groups. Otherwise it is ungrouped and its images form a gallery.
- **Operator comparison:** A comparison the operator starts in an ungrouped
  segment by choosing any two of its images.

## Requirements

### REQ-TASKS-QUESTION-IMAGE-ITEMS-001: Review in the late-answer form

**Intent:** Keep the large view available when the operator answers an expired
image question as a new message.

**User story:** As an operator who missed a question's timeout, I want to compare
its images in the large view before I send a late answer, so that a late answer is
as well informed as an on-time one.

#### Acceptance criteria

- **AC-TASKS-QUESTION-IMAGE-ITEMS-001.1:** A late-answer form for a bundle with
  question or choice images shall show the Review images control in its header
  beside Close and Send. A late-answer form for an image-free bundle shall not.
- **AC-TASKS-QUESTION-IMAGE-ITEMS-001.2:** The review view opened from a late-answer
  form shall offer the same segments, items, zoom and pan, comparison modes,
  identity display, and region annotations as for a pending bundle.
- **AC-TASKS-QUESTION-IMAGE-ITEMS-001.3:** Selections, custom-text drafts, answer
  attachments, regions, and the active question shall be one shared state between
  the late-answer form and the review view. Opening or closing the view shall not
  send the late answer.
- **AC-TASKS-QUESTION-IMAGE-ITEMS-001.4:** The review view shall show a Send as new
  message action, enabled under the same conditions as the late-answer form's Send.
  It shall deliver the answer through the late-answer path, never through the
  pending-bundle submission. When the answer is sent or queued, the view shall
  close and the late-answer success state shall show.
- **AC-TASKS-QUESTION-IMAGE-ITEMS-001.5:** When late delivery fails, the review view
  shall stay open, show the existing late-answer error with Retry, and keep the
  answer state.
- **AC-TASKS-QUESTION-IMAGE-ITEMS-001.6:** Images for an expired bundle shall load
  through the existing authorized question-image read, with the same lazy loading
  as a pending bundle. No new file-read path shall be added.

### REQ-TASKS-QUESTION-IMAGE-ITEMS-002: Agent-declared pairs and singles

**Intent:** Let an agent say which of a question's images belong together.

**User story:** As an agent asking for visual approval, I want to declare which
images form a before/after pair and which stand alone, so that the operator
compares only images that are comparable.

#### Acceptance criteria

- **AC-TASKS-QUESTION-IMAGE-ITEMS-002.1:** An operator question can declare an
  optional ordered list of image groups. Each group is a pair of exactly two
  different image IDs or a single of exactly one image ID. Each ID shall name an
  image on that question or its choices.
- **AC-TASKS-QUESTION-IMAGE-ITEMS-002.2:** A pair with other than two different
  IDs, a single with other than one ID, an unknown kind, an ID that is not one of
  the question's images, an ID in more than one group, and image groups on a
  question without images shall fail validation before a wait, durable question,
  or retry identity is created. The error shall name the question and the rule
  that failed.
- **AC-TASKS-QUESTION-IMAGE-ITEMS-002.3:** Images of a grouped question that no
  group names shall appear as singles after the declared groups, in segment image
  order.
- **AC-TASKS-QUESTION-IMAGE-ITEMS-002.4:** Parent questions shall reject image
  groups with the same error as images.
- **AC-TASKS-QUESTION-IMAGE-ITEMS-002.5:** Image groups shall persist with the
  question and survive reload, history, late answers, and exact retries. A changed
  group list shall change the request identity.
- **AC-TASKS-QUESTION-IMAGE-ITEMS-002.6:** The agent tool schema and agent-facing
  documentation shall describe image groups, state the pair and single rules, and
  explain when to use each: a pair for two versions of the same view, a single for
  an image that has no counterpart.
- **AC-TASKS-QUESTION-IMAGE-ITEMS-002.7:** Questions without image groups shall keep
  their current wire shape, validation, request identity, and agent result. Older
  web clients shall ignore image groups and keep their existing review behavior.

### REQ-TASKS-QUESTION-IMAGE-ITEMS-003: Item-by-item review

**Intent:** Show each image in the interface that fits it.

#### Acceptance criteria

- **AC-TASKS-QUESTION-IMAGE-ITEMS-003.1:** A grouped segment's items shall be its
  declared groups in order, then its ungrouped images as singles. An ungrouped
  segment's items shall be one gallery item per image, in segment image order.
- **AC-TASKS-QUESTION-IMAGE-ITEMS-003.2:** On a pair, the stage shall offer the
  comparison modes of
  [REQ-TASKS-QUESTION-IMAGE-REVIEW-005](question-image-review.md) with the pair's first image as A and its second as B.
  A pair's order is fixed: its stage shall not offer A and B pickers or Swap, and
  Flip shall switch between its two images. On a single or gallery item, the stage
  shall show the image alone with zoom, pan, and region annotations, and no
  comparison controls.
- **AC-TASKS-QUESTION-IMAGE-ITEMS-003.3:** A segment with two or more items shall
  show its position as "Item N of M" with the item's kind, and Previous and Next
  item controls. The operator can move between items with these controls and with
  `PageUp` and `PageDown` while the stage has focus. A drag on the image shall
  never move to another item: it pans, adjusts the comparison, or marks a region.
  Item navigation shall not wrap and shall stay within the segment.
- **AC-TASKS-QUESTION-IMAGE-ITEMS-003.4:** An ungrouped segment with two or more
  images shall offer a Compare images control. It starts an operator comparison
  with the current gallery image as A and the next image as B (the previous one on
  the last image). A Back to gallery control shall end it and return to the
  gallery item last shown as A. An operator comparison keeps the A and B pickers
  and Swap, because the gallery can hold more than two images. On phones, both
  controls shall show a text label, not only an icon.
- **AC-TASKS-QUESTION-IMAGE-ITEMS-003.5:** Selecting a region in the region list
  shall move to the item that shows the region's image.
- **AC-TASKS-QUESTION-IMAGE-ITEMS-003.6:** Moving between items shall not fetch
  images again. The active segment's images shall load once, as in
  [AC-TASKS-QUESTION-IMAGE-REVIEW-003.3](question-image-review.md).
- **AC-TASKS-QUESTION-IMAGE-ITEMS-003.7:** On phones, item navigation shall sit in
  the fixed bottom bar with 44px controls, and the document shall not overflow
  horizontally.
- **AC-TASKS-QUESTION-IMAGE-ITEMS-003.8:** On phones, tapping the zoom level on
  the image shall open zoom out, zoom in, Fit, and Actual size controls next to
  it, so zooming does not need the More menu. A tap outside shall close them.
- **AC-TASKS-QUESTION-IMAGE-ITEMS-003.9:** While the review view stays open,
  returning to an item or question shall show its image at the zoom and pan the
  operator left it at. A newly opened image with the same pixel size as the last
  one viewed shall start at that zoom and pan; any other new image starts at Fit.
- **AC-TASKS-QUESTION-IMAGE-ITEMS-003.10:** In the review view's answer form, a
  choice with an image shall carry a passive marker naming that image inside the
  choice itself. Selecting the choice, by pointer, touch, or number key, shall
  move the stage to that image, including when it is selected again after the
  operator navigated away. Selecting a choice without an image shall leave the
  stage where it is.

### REQ-TASKS-QUESTION-IMAGE-ITEMS-004: Visible image identity

**Intent:** Keep the operator certain which image is on screen.

#### Acceptance criteria

- **AC-TASKS-QUESTION-IMAGE-ITEMS-004.1:** While a pair or an operator comparison is
  on the stage, the stage shall always show which image is visible, or which image
  is on which side, as a slot letter with the image's label or number. Toggle and
  hold to compare shall name the shown image. Wipe shall name the image on each
  side of the divider. Side by side shall caption each pane. Onion skin shall show
  a two-row layer list: the top image with its opacity, above the bottom image.
  Difference shall name both images. The identity shall sit at the stage's bottom
  left, opposite the zoom controls, except in Side by side, where each pane's
  caption sits at that pane's bottom left and the zoom controls sit at the second
  pane's bottom right, and in Wipe, where each image's name sits on the Wipe
  handle beside the line, on that image's side, as upright letters read top to
  bottom in the divider's color with a light outline in every theme. A Wipe name
  shall hide when its side is too narrow for it.
- **AC-TASKS-QUESTION-IMAGE-ITEMS-004.2:** The identity shall update in the same
  render as a flip, swap, image pick, candidate step, item change, mode change, or
  hold, so it is never stale.
- **AC-TASKS-QUESTION-IMAGE-ITEMS-004.3:** The identity shall be available to screen
  readers through the stage's status region and shall stay visible in forced-colors
  mode. The bottom-left identity shall not cover the wipe handle, the zoom
  controls, or Mark area.
- **AC-TASKS-QUESTION-IMAGE-ITEMS-004.4:** The Wipe handle shall sit low on the
  divider and move with it. Dragging the handle shall move the divider sideways
  and the handle up or down along the line; on phones, where the line stays at the
  center, it moves the handle up or down only. The handle shall stay inside the
  stage, also after a resize, and keep its height for the rest of the question
  round: across mode, item, and question changes, and when the review view closes
  and reopens. A new question round shall start at the default height.
- **AC-TASKS-QUESTION-IMAGE-ITEMS-004.5:** The Wipe names shall show only while
  the handle is hovered, has keyboard focus, or is held; while the pointer or a
  finger is on the Wipe divider slider; and for about a second after the split
  moves on the image (the divider changes, or the image pans or zooms beneath it)
  or after a touch or drag ends. Entering Wipe shall not show them.

## Out of scope

Groups of more than two images, operator comparison inside a grouped segment,
item navigation across segments, image groups on choices, parent questions, or
native provider questions, the review view for already-answered history, and any
server-side image transformation.
