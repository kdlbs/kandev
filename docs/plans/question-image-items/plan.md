---
created: 2026-11-24
status: implemented
requirements:
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-001
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-003
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-005
  - REQ-TASKS-QUESTION-IMAGE-ITEMS-001
  - REQ-TASKS-QUESTION-IMAGE-ITEMS-002
  - REQ-TASKS-QUESTION-IMAGE-ITEMS-003
  - REQ-TASKS-QUESTION-IMAGE-ITEMS-004
system_design:
  - ../../specs/tasks/system-design/question-image-items.md
  - ../../specs/tasks/system-design/question-image-review.md
legacy_specs: []
---

# Implementation plan: Question image items and late-answer review

## Overview

Two follow-ups to the [question image review package](../question-image-review/plan.md):

- [Issue #5494](https://github.com/kdlbs/kandev/issues/5494): offer **Review
  images** in the late-answer form, with the same view, shared answer state, and a
  send action that uses the late-answer path.
- [Issue #5495](https://github.com/kdlbs/kandev/issues/5495): let agents declare
  image pairs and singles, review each item in the interface that fits it, show
  ungrouped images as a gallery with on-demand comparison, and always show which
  image is on screen.

Read the [requirements](../../specs/tasks/requirements/question-image-items.md)
(REQ-TASKS-QUESTION-IMAGE-ITEMS-001 to 004, shortened below to AC-ITEMS-NNN.N),
the amended AC-001.1, AC-003.5, AC-005.1, and AC-005.11 in
[question image review](../../specs/tasks/requirements/question-image-review.md),
and the [system design](../../specs/tasks/system-design/question-image-items.md).

Decisions made in this design turn, without a user interview:

- The contract is an additive `image_groups` list of ID references per question,
  not a nested `images` shape.
- Images that no group names become singles after the groups; they do not fail
  validation.
- Item navigation stays within a segment and does not wrap.
- Operator comparison is offered only in ungrouped segments.
- `PageUp` and `PageDown` navigate items; `[` and `]` keep their meaning.

## Work orders

| Wave | Work order | Depends on | Result |
| --- | --- | --- | --- |
| 1 | [01 Late-answer review](task-01-late-answer-review.md) | none | Review images in the late-answer form; send uses the late path. |
| 1 | [02 Image group contract](task-02-image-group-contract.md) | none | `image_groups` validated, persisted, in identity, schema, and docs. |
| 2 | [03 Item-by-item review](task-03-item-review.md) | 02 | Pairs, singles, gallery, item navigation, and operator comparison. |
| 3 | [04 Visible image identity](task-04-visible-identity.md) | 03 | Identity badges and announcements for pairs and comparisons. |
| 4 | [05 Test-round controls](task-05-test-round-controls.md) | 04 | Pair controls, identity placement, phone labels, and phone zoom. |
| 5 | [06 Wipe handle names](task-06-wipe-handle-names.md) | 05 | Names on the Wipe handle, Onion layer list, and no swipe navigation. |
| 6 | [07 View memory and Show image](task-07-view-memory-show-image.md) | 06 | Remembered zoom and pan, Show image on choices (superseded by 08), picker inside the dialog. |
| 7 | [08 Selecting a choice shows its image](task-08-select-shows-choice-image.md) | 07 | Image marker inside the choice; selecting the choice shows its image. |

Wave 1 work orders touch separate files. Work order 01 touches the overlay and
header; 02 is backend plus the web type. Work order 03 needs the web type from 02.
Work order 04 builds on the item stage from 03.

## ASCII UI preview

Structural requirements: the Review control in the late form header, the late
send action in the dialog, item navigation placement, the absence of comparison
controls on singles and gallery items, the Compare images and Back to gallery
controls, and identity badges at the stage's top left (top of each side in
Wipe). Spacing, icons, and exact copy are illustrative. Use existing `@kandev/ui`
primitives and localized copy.

### UI-01: Late-answer form header

Entry: an expired image question, after **Answer as new message**. Maps to
AC-ITEMS-001.1.

```text
| ⏱ Timed out. The agent moved on.                                          |
| [thumb: before] [thumb: after]                    (history inline images) |
| +-----------------------------------------------------------------------+ |
| | Question 1 of 2   [Review images] [clip] [Close] [Send as new message]| |
| | Is the new header spacing acceptable?                                 | |
| | (1) Approve   (2) Revise   [custom answer..........................]  | |
| +-----------------------------------------------------------------------+ |
```

### UI-02: Desktop review view on a pair, opened from a late form

Maps to AC-ITEMS-001.4, AC-ITEMS-003.2, AC-ITEMS-003.3, AC-ITEMS-004.1.

```text
+------------------------------------------------------------------------------------+
| Review images [1 Header ✓] [2 Screens]               [Send as new message] [Close] |
| [‹] Item 1 of 3 · Pair [›] | [Toggle|Wipe|Side|Onion|Diff]  [Flip]                  |
|  +------------------------------------------------------------------------------+  |
|  | [A · before]                                                  [✎ Mark area] |  |
|  |                              image area                                      |  |
|  | [Sizes differ]                                    [⌖] [-] [100% v] [+] [?]  |  |
|  +------------------------------------------------------------------------------+  |
|================================= drag to resize ==================================|
| Is the new header spacing acceptable?                                              |
| (1) Approve   (2) Revise   [custom answer..........]                               |
+------------------------------------------------------------------------------------+
```

Wipe on the same pair: one badge on each side of the line.

```text
|  | [A · before]            ‖            [B · after]               [✎ Mark area] |  |
|  |                  A side ‖ B side                                             |  |
```

### UI-03: Desktop single and gallery items

Single (grouped segment): no comparison controls. Maps to AC-ITEMS-003.2.

```text
| [‹] Item 3 of 3 · Single [›]                                               |
|  +----------------------------------------------------------------------+  |
|  | New billing screen (1440×900)                          [✎ Mark area] |  |
```

Gallery (ungrouped segment with two or more images). Maps to AC-ITEMS-003.4.

```text
| [‹] Item 2 of 3 · Image [›]   [Compare images]                             |
```

Operator comparison started from the gallery. A and B pickers remain:

```text
| [Back to gallery] | [Toggle|Wipe|...]  A [2 v] ⇄ B [3 v] ‹ 1/2 ›  [Flip]    |
|  | [A · 2]                                                     ...       |  |
```

### UI-04: Phone review view

Fixed header, top row, image, and bottom bar (hidden while typing). Item
navigation leads the bottom bar. Every control is at least 44px; nothing scrolls
sideways. Maps to AC-ITEMS-003.7, AC-ITEMS-004.1.

```text
+--------------------------------------+
| [Close]  Review 1 of 2  [Send new]   |  late label when late
| [1 ✓] [2]                            |
| [Mode: Toggle v]  [Regions] [...]    |
|  +--------------------------------+  |
|  | [B · after]                    |  |  identity badge
|  |        image area       [100%] |  |
|  +--------------------------------+  |
|--------------------------------------|
| question and answers (scrolls)       |
|--------------------------------------|
| [‹] 1/3 Pair [›] | [⇄] [ Flip A/B ]  |  thumb-zone bar
+--------------------------------------+
```

On a single or gallery item the bar holds item navigation, Mark area, Regions,
and (gallery only) Compare. Work order 06 removed swipe navigation: items change
only through these controls and `PageUp`/`PageDown`.

## Verification strategy

Each work order lists its exact commands. In summary:

- Backend: `go test` for `./internal/clarification/...`, `./internal/mcp/...`,
  and `./internal/backendapp/` focused tests.
- Web unit: `vitest run` on `lib/question-review`,
  `components/task/chat/question-review`, and touched overlay tests.
- Web static: `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and
  `pnpm run i18n:ratchet`.
- E2E: `question-image-review.spec.ts` on `chromium` and
  `mobile-question-image-review.spec.ts` on `mobile-chrome`.

User-facing requirements get end-to-end evidence in work orders 01, 03, and 04.

## Risks

- `clarification-input-overlay.tsx` is above the 600-line limit. Work order 01
  changes only arguments and moves the late error banner; any new logic goes in
  `question-review/`.
- Late form inside chat history: Escape in the review view must not close the late
  form. The dialog already stops propagation and registers an escape guard; the
  E2E checks it.
- Splitting `QuestionReviewStage` must keep image bytes across item changes. The
  E2E records image requests and asserts no refetch.
- Swipe ownership (superseded by work order 06, which removed swipe navigation):
  one `onSwipe` callback meant item navigation or candidate stepping.
- Identity badges can collide with Mark area, the wipe handle, or the annotate
  hint. Check desktop and phone screenshots in Wipe and while marking.
- Agents may declare groups inconsistently. Server validation rejects malformed
  groups; the web falls back to the gallery for any malformed persisted group.

## Exclusions

Groups of more than two images, operator comparison in grouped segments, item
navigation across segments, image groups on choices or parent questions, review
for already-answered history, and any server-side image transformation.
