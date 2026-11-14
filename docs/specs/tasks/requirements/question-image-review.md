---
status: active
system: tasks
created: 2026-11-07
owners:
  - kandev
---

# Question image review

## Overview

An operator answers a question about visual changes while the evidence stays
large and visible. The operator opens an opt-in review view, compares a baseline
image with up to three candidates, and answers in the same form. The form stays
visible below the image, so a long and specific question is not lost while the
operator inspects details.

This extends [question image previews](question-image-previews.md). The task
system owns it because the clarification bundle owns question identity, image
references, answer state, persistence, and delivery. The web interface exposes
that contract; it does not own a separate review state.

Source: [issue #5135](https://github.com/kdlbs/kandev/issues/5135). The paired
[system design](../system-design/question-image-review.md) defines the
implementation boundaries. The [plan](../../../plans/question-image-review/plan.md)
defines delivery. Local changes implement this contract; it does not yet describe
a released version.

## Terms

- **Review view:** The large image stage with the active question's answer form below it.
- **Segment:** One question in the review view, with its ordered images.
- **Baseline and candidates:** In a segment, image A is the baseline, the first
  image by default. The other images are candidates. Image B is the selected candidate.
- **Stage:** The area of the review view that shows images.
- **Region annotation:** A rectangle drawn on one image with an operator comment.

## Requirements

### REQ-TASKS-QUESTION-IMAGE-REVIEW-001: Opt-in review with shared answer state

**Intent:** Offer a large view without making quick answers slower.

**User story:** As an operator, I want to open a large review only when I need it,
so that small questions stay quick and visual questions stay reviewable.

#### Acceptance criteria

- **AC-TASKS-QUESTION-IMAGE-REVIEW-001.1:** When a pending, answerable bundle has
  at least one question or choice image, the question panel shall show a labelled
  Review images control. Image-free bundles, historical messages, and submitted,
  expired, or late-answer success states shall not show it.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-001.2:** The compact panel shall remain the
  default. The review view shall open only from an explicit operator action.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-001.3:** Selections, custom-text drafts, answer
  attachments, and the active question shall be one shared state between the
  compact panel and the review view. A change in one shall appear in the other.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-001.4:** Opening or closing the review view
  shall not submit, skip, reject, or resolve the bundle. Submitting from the review
  view shall use the existing submission behavior, then close the view.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-001.5:** When the bundle is answered elsewhere,
  replaced, expires, or its host unmounts, the review view shall close without
  submitting stale answers. Escape and Close shall close the view without
  dismissing or collapsing the question, and focus shall return to the Review
  images control.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-001.6:** Choosing an answer in the review view
  shall record it without moving to the next question, so the compared images stay
  on screen. The Next control shall then be the emphasized action. The compact panel
  keeps its existing advance-on-answer behavior.

### REQ-TASKS-QUESTION-IMAGE-REVIEW-002: Stage above a visible answer form

**Intent:** Keep the image and the question visible together.

#### Acceptance criteria

- **AC-TASKS-QUESTION-IMAGE-REVIEW-002.1:** The review view shall show the stage
  above the active question's title, prompt, choices, custom text, answer
  attachments, navigation, and submit controls.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-002.2:** On desktop viewports of at least
  1280 by 720 CSS pixels, the review view shall use most of the viewport. The
  stage and the form shall both be visible on entry. The operator can resize the
  split with a pointer or keyboard, and each part shall keep a usable minimum height.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-002.3:** Below the 768px phone breakpoint, the
  review view shall use the full dynamic viewport height with safe-area clearance.
  The stage shall stay fixed above a separately scrolling form. The comparison mode
  and rarely used actions shall sit in one row above the image, and the frequently
  repeated actions (flip, candidate stepping, image choice, swap, mode sliders)
  shall sit in a fixed bottom bar within thumb reach, hidden while a text field has
  focus. Zoom level and status shall appear as chips on the image. No toolbar shall
  scroll horizontally. Controls shall measure at least 44px in both dimensions, and
  the document shall not overflow horizontally.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-002.5:** On wider screens, the stage toolbar
  shall be one row holding only the comparison controls: the mode, an image-pair
  control (A, B, swap, and candidate stepping), and the current mode's action.
  Zoom, Mark area, and status shall sit on the image.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-002.4:** While the review view is open, the
  form shall keep the existing option and submit shortcuts when focus is in the
  form. Stage shortcuts shall apply only when focus is in the stage, and shall not
  select an option, change the question, submit, or skip.

### REQ-TASKS-QUESTION-IMAGE-REVIEW-003: Segment navigation and image loading

**Intent:** Move through several visual questions without loading every image.

#### Acceptance criteria

- **AC-TASKS-QUESTION-IMAGE-REVIEW-003.1:** Each question in the bundle shall be
  one segment, in bundle order. A segment strip shall show each segment's position,
  title or prompt summary, image count, and answered state. The operator can move to
  any segment by pointer, touch, or keyboard. Questions without images remain
  segments and show an image-free stage state.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-003.5:** Moving to another segment shall keep the
  comparison mode and its slider values (wipe position, opacity, tolerance). Image
  choices and zoom shall start fresh for the new segment.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-003.2:** A segment's images shall be its question
  images in authored order, then its choice images in option order. Each image
  shall show its agent-authored description, and each choice image shall also show
  its choice label.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-003.3:** Only the active segment's images shall
  be fetched. Moving to another segment shall abort pending reads for the previous
  segment and release its loaded bytes. The view shall not preload other segments.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-003.4:** Loading, unavailable, unsupported,
  invalid, and too-large images shall show the existing localized preview states
  with Retry. A failed image shall not block answering or comparing other images.

### REQ-TASKS-QUESTION-IMAGE-REVIEW-004: Zoom and pan

**Intent:** Make one-pixel differences visible.

#### Acceptance criteria

- **AC-TASKS-QUESTION-IMAGE-REVIEW-004.1:** The stage shall offer Fit, Actual size
  (100%), and zoom steps from 25% to 800%, with the current zoom level visible.
  Fit shall be the default and shall always contain the whole image, even when
  that needs less than 25%. Zooming out shall stop at the smaller of Fit and 25%.
  A zoom level menu shall offer Fit, 100%, 200%, 400%, and 800%, and a double-click
  on the image shall switch between Fit and 100%.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-004.2:** The operator can pan and zoom with mouse
  drag and modified wheel input, with touch drag and pinch, and with documented
  keyboard keys while the stage has focus. Zooming with a pointer shall keep the
  point under the pointer in place.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-004.3:** Above 100%, images shall render with
  nearest-neighbor scaling so individual pixels stay sharp.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-004.4:** Zoom and pan shall be measured in image
  pixels. Changing the segment shall reset the view to Fit.

### REQ-TASKS-QUESTION-IMAGE-REVIEW-005: A/B comparison

**Intent:** Show exactly what changed between a baseline and a candidate.

#### Acceptance criteria

- **AC-TASKS-QUESTION-IMAGE-REVIEW-005.1:** With one image, the stage shall show it
  alone. With two or more images, the operator can choose images A and B and pick
  one of five modes: Toggle, Wipe, Side by side, Onion skin, and Difference.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-005.2:** Toggle shall show A or B in the same
  place. A visible control and a stage keyboard shortcut shall flip between them.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-005.3:** Wipe shall show A and B divided by a
  slider that the operator can move by pointer, touch, or keyboard. A drag shall
  reach 0% and 100% without the pointer reaching the screen edge, the handle
  shall stay fully visible, and a toolbar slider shall set the same position. On
  touch-first devices the line shall stay pinned at the center of the stage and a
  drag shall scrub both images beneath it, far enough for either image edge to
  reach the line.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-005.4:** Side by side shall show A and B with
  synchronized zoom and pan. Below the phone breakpoint, the images shall stack vertically.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-005.5:** Onion skin shall show B over A with an
  opacity control from 0% to 100%.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-005.6:** Difference shall highlight pixels whose
  per-channel difference exceeds a tolerance the operator can change. It shall
  report the changed pixel count and proportion. It shall compare full-resolution
  pixels and shall not downscale either image.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-005.7:** Switching between Toggle, Wipe, Side by
  side, and Onion skin, or flipping or swapping images, shall keep the zoom level
  and pan position.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-005.8:** When A and B have different dimensions,
  the stage shall show both sizes. Images shall align at their top-left corner at the
  same scale. Difference shall count pixels outside the shared area as changed.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-005.9:** When the device cannot compute a
  difference for the selected images, Difference shall show a localized unavailable
  state. The other modes and the form shall remain usable. Computing a difference
  shall not block input in the form.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-005.10:** Comparison shall use only image bytes
  already loaded through the authorized question-image read. It shall not add a
  server-side image transformation, image archive, or new file-read route.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-005.11:** In Toggle and Side by side with three
  or more images, Previous and Next controls, the `[` and `]` keys, and a horizontal
  swipe at Fit shall step B through the other images while A stays fixed, without
  wrapping. A label shall show B's name and position.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-005.12:** Pressing and holding the image, or
  holding Space while it has focus, shall show A until released. A press that
  moves shall pan instead.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-005.13:** Difference shall outline the area that
  contains every changed pixel. Zoom to changes shall fit that area in every mode;
  outside Difference it shall compute the area only when asked, keep it for the
  image pair and tolerance, outline it briefly after zooming, and report when no
  pixels changed.

### REQ-TASKS-QUESTION-IMAGE-REVIEW-006: Larger image galleries

**Intent:** Let a bundle carry an A/B set for each of its questions.

#### Acceptance criteria

- **AC-TASKS-QUESTION-IMAGE-REVIEW-006.1:** A bundle shall accept up to 16 distinct
  image IDs. This replaces the limit of four in
  [AC-TASKS-QUESTION-IMAGES-001.1](question-image-previews.md) and
  [AC-TASKS-QUESTION-IMAGES-005.1](question-image-previews.md). The limits of
  four references per question and one image per choice shall remain.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-006.2:** A bundle with 17 or more distinct image
  IDs shall fail validation before a wait, durable question, or retry identity is created.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-006.3:** The compact panel and history shall keep
  loading only the active question's images, or an explicitly expanded history entry.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-006.4:** The agent tool schema and agent-facing
  documentation shall state the new bundle limit.

### REQ-TASKS-QUESTION-IMAGE-REVIEW-007: Region annotations

**Intent:** Tell the agent exactly which area a comment concerns.

#### Acceptance criteria

- **AC-TASKS-QUESTION-IMAGE-REVIEW-007.1:** In the single-image and Toggle views,
  the operator can draw a rectangle on the shown image, comment on it, edit its
  comment and bounds, and delete it. Each question shall accept at most eight
  regions, and regions shall be numbered in creation order. Finishing a region
  shall open a comment popover next to it (a bottom drawer on touch screens), the
  same comment control chat uses for selected text. Cancelling the comment of a
  new region shall discard the region; clicking or tapping an existing region, or
  pressing Enter on a selected one, shall reopen the popover to edit or delete it.
  The Mark area control shall look active while on, a hint with Done shall show on
  the image, and Escape shall close an open comment, then leave marking, and only
  then close the view.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-007.2:** Each region shall record the image ID,
  an integer pixel rectangle within the image, the image width and height observed
  when it was drawn, and a comment of 1 to 500 characters.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-007.3:** Regions shall be part of the question's
  answer state and draft. They shall supplement the answer and shall not count as a
  selection or custom text.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-007.4:** The server shall reject a region that
  references an image outside its question, has a rectangle outside its observed
  size, has an empty or over-long comment, or exceeds the count limit. Rejection
  shall not claim the bundle.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-007.5:** The agent shall receive the regions in
  the keyed answer, both in the live tool result and in a detached answer delivered
  in a later turn. Prompt text shall encode each comment so it cannot add, end, or
  change another structured entry.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-007.6:** Accepted regions shall persist with the
  answer and appear in answered question history with their number, image
  description, and comment.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-007.7:** Keyboard and touch users shall be able
  to create, adjust, and delete regions. Regions shall be listed as read-only text
  with Edit and Delete controls, in a side panel next to the image on wider screens
  and in a sheet on phones, so screen-reader users can review and change them.

### REQ-TASKS-QUESTION-IMAGE-REVIEW-008: Compatibility

**Intent:** Leave existing questions and clients unchanged.

#### Acceptance criteria

- **AC-TASKS-QUESTION-IMAGE-REVIEW-008.1:** Image-free questions, compact image
  previews, and the existing image inspector shall keep their current behavior.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-008.2:** Answers without regions shall keep their
  current wire shape, serialization, retry identity, and agent result.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-008.3:** Older web clients and external answer
  clients can omit regions and answer through the existing contract.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-008.4:** All application copy shall be localized.
  Reduced-motion and forced-colors preferences shall keep every control usable.

### REQ-TASKS-QUESTION-IMAGE-REVIEW-009: Image labels

**Intent:** Let the agent name images so pickers read before, after, alt.

#### Acceptance criteria

- **AC-TASKS-QUESTION-IMAGE-REVIEW-009.1:** An agent can give each question or choice
  image an optional one-word label of 1 to 16 letters, digits, underscores, or
  hyphens. Pickers, choice references, candidate labels, and the thumbnail
  inspector counter shall show the label instead of the image number, and fall
  back to the number without one. Labels are agent data and are not translated.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-009.2:** Malformed labels, and the same label on
  two different images of one question, shall fail validation before the bundle is
  accepted. A repeated image ID shall carry the same label.
- **AC-TASKS-QUESTION-IMAGE-REVIEW-009.3:** Labels shall persist with the question
  and survive reload, history, and exact retries; a changed label shall change the
  request identity.

## Out of scope

The review view for already-answered history, freehand or non-rectangular
annotations, annotations in Wipe, Side by side, Onion skin, and Difference,
server-side comparison, more than four images per question, images on parent
questions or native provider questions, image formats other than PNG and JPEG,
and saving unsent drafts across a full page reload.

The [question barrier](user-question-turn-boundary.md),
[active lifecycle](clarification-active-lifecycle.md),
[response reliability](clarification-response-reliability.md), and
[answer attachments](question-answer-attachments.md) remain authoritative.
