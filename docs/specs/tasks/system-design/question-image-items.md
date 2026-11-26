---
status: current
system: tasks
created: 2026-11-24
requirements:
  - REQ-TASKS-QUESTION-IMAGE-ITEMS-001
  - REQ-TASKS-QUESTION-IMAGE-ITEMS-002
  - REQ-TASKS-QUESTION-IMAGE-ITEMS-003
  - REQ-TASKS-QUESTION-IMAGE-ITEMS-004
---

# Question image items and late-answer review system design

## Purpose and boundaries

Extend the [question image review design](question-image-review.md) in two ways:
open the review view from the late-answer form, and review each question's images
as agent-declared pairs and singles, or as a gallery when the agent declares none.
One backend contract changes: an optional `image_groups` list on each operator
question. Answers, image reads, and the late-answer delivery path do not change.

The [requirements](../requirements/question-image-items.md) own observable
behavior. The [plan](../../../plans/question-image-items/plan.md) owns delivery
order. The question image review design remains authoritative for layout,
viewport, comparison modes, the difference worker, and annotations. All new
symbols below are proposed interfaces.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-TASKS-QUESTION-IMAGE-ITEMS-001 | Late-answer review |
| REQ-TASKS-QUESTION-IMAGE-ITEMS-002 | Image group contract |
| REQ-TASKS-QUESTION-IMAGE-ITEMS-003 | Segment items, Item stage |
| REQ-TASKS-QUESTION-IMAGE-ITEMS-004 | Visible identity |

## Late-answer review

The late-answer form is the same `ClarificationInputOverlay`, mounted
with `mode="late"` from `ClarificationRequestMessage`, or the active overlay after
`group.lateAnswerState` leaves `idle`. Its `lateInteraction` flag already selects
the late submitter through `chooseSubmitter`, so `handleSubmit` sends a late answer
in both cases.

`isQuestionReviewAvailable` drops the `lateInteraction` exclusion and takes the
late status instead:

```ts
isQuestionReviewAvailable({ hasImages, submitState, lateStatus }): boolean
// false when !hasImages, submitState is "ok" or "expired",
// or lateStatus is "sent" or "queued"; true otherwise.
```

The overlay passes `lateMode ? effectiveLateStatus : group.lateAnswerState` as
`lateStatus`. A sent or queued late answer therefore makes `available` false, and
the existing effect in `useQuestionReview` closes the view before the overlay
renders its late success row. The bundle key stays `group.pendingId`, which a
late form keeps for its expired bundle.

`ClarificationHeaderActions` renders `reviewAction` in its `lateMode` branch,
before `attachmentActions`, Close, and Send. The control keeps its stable
accessible name and `triggerRef`.

`QuestionReviewSubmit` gains `label: "submit" | "late"`. The overlay sets
`show: true` and `label: "late"` while `lateInteraction` is true; the dialog then
renders the existing `task:lateAnswerSend` and `task:lateAnswerSending` copy on
its submit button (test ID `question-review-submit` unchanged). `enabled` stays
`allAnswered && attachmentsReady`, and `onSubmit` stays `handleSubmit`, so the
dialog cannot reach `group.submitCollected` in late mode.

The late error banner moves from beside the form into the shared `form` fragment,
after the existing submit-error banner. It therefore renders once, in the compact
panel or in the dialog, and its Retry keeps calling `retryLateAnswer`.

Image reads already work for expired bundles. `httpQuestionImage` authorizes by
pending ID through `AuthorizeBundleAccess` and reads saved message metadata, as
history thumbnails do today. The stage keeps calling `useQuestionImages` with the
same pending ID and owner. No backend change is needed for this issue.

The history message wraps the late form in `QuestionImageGallery`. While the view
is open, `stageOwnsImages` hides the form's thumbnails; the message's own inline
thumbnails above the form stay mounted and inert under the modal.

## Image group contract

Add to `protocol`:

```go
type QuestionImageGroupKind string

const (
    ImageGroupPair   QuestionImageGroupKind = "pair"
    ImageGroupSingle QuestionImageGroupKind = "single"
)

type QuestionImageGroup struct {
    Kind     QuestionImageGroupKind `json:"kind"`
    ImageIDs []string               `json:"image_ids"`
}
```

`Question` gains `ImageGroups QuestionImageGroups \`json:"image_groups,omitempty"\``.
`QuestionImageGroups` rejects a `null` list, `null` entries, and unknown fields
while decoding, like `QuestionImages`.
`Option` does not change. Web `ClarificationQuestion` gains
`image_groups?: QuestionImageGroup[]` with the same fields.

`ValidateQuestionBundleImages` runs `ValidateQuestionImages` and then
`validateImageGroups(index, q)` for each question. It returns an error that
names the question by one-based position and, when set, its ID
(`QuestionLabel`), and the rule that failed:

- Groups are present only when `q.ImageReferences()` is non-empty.
- At most `MaxQuestionImageReferences` groups.
- `Kind` is `pair` or `single`. A pair has exactly two different IDs. A single has
  exactly one ID.
- Every ID is in `q.ImageReferences()`, and no ID is in more than one group.

The bundle creation paths (MCP `validateQuestionImagePayloads`, HTTP and WebSocket
`normalizeAndValidateQuestions`) call `ValidateQuestionBundleImages`, so all of
them enforce the rules before a wait, durable question, or retry identity exists.
The MCP path decodes each question separately, so a decode error (a `null`
group, a wrong-typed ID, an unknown field) also names its question. The image
read route keeps calling `ValidateQuestionImages` only, so a saved group that no
longer validates never hides valid images; the web shows that segment as a gallery.

Persistence and identity:

| Boundary | Change |
| --- | --- |
| `backendapp/adapters.go` `clarificationQuestionData` | Writes `image_groups` when non-empty. |
| `clarification/bundle_question.go` question decode | Reads `image_groups` the same way it reads `images`. |
| `clarification/store.go` `questionsEqual` | Compares `ImageGroups` with a helper that compares kind and ID order. |
| `mcp/server/handlers.go` `askParentQuestionHandler` | Rejects `image_groups` on the raw payload, before validation, with "parent questions do not support images". |
| `mcp/handlers/parent_question.go` `parentQuestionRequestFromMessage` | Rejects `image_groups` on WebSocket parent questions with the same message. |

`buildOperatorQuestionSchemaItem` adds `image_groups` from
`buildQuestionImageGroupSchema()`: an array of at most
`MaxQuestionImageReferences` objects with `additionalProperties: false`,
`kind` as an enum, and `image_ids` with `minItems: 1` and `maxItems: 2`. Its
description states the rules and the guidance from AC-002.6. Exact pair and
single counts are enforced by `validateImageGroups`, because the JSON schema
cannot tie item count to kind portably. `questionImageSchemaDescription` replaces
"put the baseline first and candidates after it" with a pointer to `image_groups`.

The answer does not change. Native provider questions never carry images.

## Segment items

`lib/question-review/segments.ts` adds a pure item builder:

```ts
type ReviewItem =
  | { kind: "pair"; images: [ReviewImage, ReviewImage] }
  | { kind: "single"; image: ReviewImage }
  | { kind: "gallery"; image: ReviewImage };

type ReviewSegment = { ...existing; grouped: boolean; items: ReviewItem[] };
```

`buildSegmentItems(images, groups)` returns gallery items when `groups` is absent
or empty. Otherwise it maps each group to a pair or single over `images`, then
appends a single for each image no group names, in segment order. The web does not
trust the server blindly: when any group is malformed (unknown kind, wrong count,
unknown or repeated ID), the segment falls back to ungrouped gallery items. This
keeps older or corrupted data reviewable.

`itemIndexForImage(items, imageId)` returns the item that shows an image, for
region selection (AC-003.5).

## Item stage

The dialog owns `itemIndex` and `operatorCompare: { aId, bId } | null` per
segment in a `useReviewItems(segment)` hook. Changing the segment resets both.

`QuestionReviewStage` is split so image loading outlives item changes:

- `SegmentStage` (keyed by `segment.questionId`) calls `useStageImages` once and
  passes `images` down. Moving between items never remounts it, so no image is
  fetched again (AC-003.6).
- `ItemStage` (keyed by item index and operator compare state) chooses the view:
  - `pair`: `ReviewCompareStage` with `compareImages = item.images`, `pair: true`.
    With exactly two images it already hides candidate stepping; `pair` also
    hides the A and B pickers and Swap (see the pair's fixed order below).
    Phones hide the picker sheet button for pairs.
  - `single` and `gallery`: `SingleImageStage` with the item image.
  - operator comparison: `ReviewCompareStage` over all segment images with the
    chosen A and B, as today.

`useCompareStage` and `useSingleImageStage` take the loaded `images` from props
instead of calling `useStageImages`. `ReviewCompareStage` takes an initial
`{ aId, bId }` so an operator comparison starts from the current gallery image.

`ItemNav` renders "Item N of M" with the item kind (`Pair`, `Single`, `Image`)
and Previous and Next buttons. It sits at the start of the desktop stage toolbar.
On phones `ItemNavRow` puts a compact "N/M · kind" version in its own row of the
bottom bar (the bar stacks its rows), so the comparison sliders and Flip keep
their full touch size at 320px. It renders only
when the segment has two or more items. The stage handles `PageUp` and `PageDown`
in its key handler, which already stops propagation to the form.

Each item mounts its own stage, so an item change removes the focused canvas.
`QuestionReviewStage` wraps the item stage and, when focus was inside it, moves
focus to the new item's canvas (`useKeepStageFocus`), so `PageUp` and `PageDown`
keep working from the keyboard.

No drag navigates. A drag on the image pans, adjusts the comparison (the pinned
touch Wipe scrubs), or marks a region; items change only through the item
controls and `PageUp`/`PageDown`, and candidates through the stepper and `[`/`]`.
`useReviewViewport` has no swipe callback.

Gallery items in an ungrouped segment with two or more images show a
**Compare images** button: in the stage toolbar on desktop, and with the short
label **Compare** in the top row on phones. It sets `operatorCompare` to the
current image and its neighbor. The comparison toolbar (desktop) then shows
**Back to gallery** in the slot item navigation uses; on phones a **Gallery**
button takes the bottom-bar row item navigation uses, so the top row keeps the
mode button and More at 320px. Either clears `operatorCompare` and sets
`itemIndex` to the gallery item for the current A.

A pair's order is fixed. Its desktop toolbar has the modes and `ModeControls`
but no `ImagePairControl` (no pickers, Swap, or candidate stepping), its phone
bottom bar has no Swap, and `useReviewCompare` ignores the `s` key for it. An
operator comparison keeps the pickers and Swap. Flip uses the primary button
style on desktop and phones.

Phone zoom: `ZoomChip` opens a popover, anchored to the chip, with
`ReviewZoomControls` (zoom out, level, zoom in, Fit, Actual size), which wrap at
narrow widths. More no longer holds zoom; a single or gallery item has no More
button, because its Mark area and Regions are in the bottom bar.

Region selection: `useRegionReveal` turns each new selection (a new
`annotations.selected` object) into one reveal. It moves to
`itemIndexForImage(items, imageId)` and holds the image ID until the stage that
shows it consumes it: a comparison stage dispatches `show`, and a single or
gallery item consumes it at once. A selection is never replayed when the operator
later navigates or a stage remounts, so it cannot override Back to gallery or the
starting images of a new comparison.

Compare settings keep carrying over through the dialog's `CompareSettings`, so
the mode and sliders survive item changes ([AC-TASKS-QUESTION-IMAGE-REVIEW-003.5](../requirements/question-image-review.md)).
The dialog owns one `ReviewCarry` holder (`lib/question-review/review-carry.ts`)
that every stage reads when it mounts and writes as values change: the
comparison settings (`read`/`write`) and each stage's zoom and pan
(`recallView`/`rememberView`, AC-003.9). Item stages mount below the dialog, so
they read the latest values directly, and dragging a slider re-renders only the
stage, not the dialog and its answer form. `useRememberedView` recalls a view
once, when the image size is first known: the view kept for the stage's key
(`questionId` plus the single image, the pair's two images, or `compare` for an
operator comparison), or else the last view when the image size matches. Fit is
kept as a stage's view, so a stage left at Fit returns at Fit. A comparison
recalls only once both of its images are measured (`measuredPairSize`). The
holder lives as long as the dialog.

**Choice images.** In the dialog's answer form, `ClarificationOptions` renders
`ChoiceStageImageHint` inside each choice that has an image: a photo icon and the
image's short name, announced as "Image {name}" (AC-003.10). It is not a
separate control. Selecting a choice calls `ShowImageContext` with the choice's
first image (`choiceStageImageId`); clicks and number-key picks share that
handler. The dialog keeps an `ImageRequest` (`useImageRequest`), and each call
makes a new request, so re-selecting a choice after navigating away moves the
stage again. `useRegionReveal` turns a request into the same one-time reveal as
a region selection: move to the item with that image, then let a comparison
show it.

The image pickers', zoom level, and shortcut-help popovers render into the
dialog's popover container, like the region and zoom-chip popovers. A popover
portaled outside the modal dialog lands in content the dialog has made inert,
which kept the A/B picker from opening in Safari.

## Visible identity

`lib/question-review/identity.ts` holds a pure function:

```ts
visibleIdentity(state: CompareState, holding: boolean, names: { a: string; b: string })
  : { kind: "one"; slot: "a" | "b" }
  | { kind: "sides"; left: "a" | "b"; right: "a" | "b" }   // Wipe
  | { kind: "panes" }                                       // Side by side
  | { kind: "over"; top: "b"; bottom: "a"; opacity: number } // Onion skin
  | { kind: "versus" }                                      // Difference
```

It reads `effectiveCompareState`, so hold to compare reports A. Because it is
derived from reducer state in the same render, flips, swaps, picks, steps, and
mode changes cannot leave it stale (AC-004.2).

`IdentityBadges` (`review-identity-badges.tsx`) renders the result at the
stage's bottom left, opposite the zoom controls, in the same corner as the
status chips, styled like the zoom control (a rounded pill). Text wraps instead
of truncating, so full labels and the opacity stay readable:

- `one`: "A · before" or "B · after".
- `sides`: no badge; the names are on the Wipe handle (below).
- `panes`: a caption at the bottom left of each pane. Zoom sits at the bottom
  right of the second pane, the stage's far corner.
- `over`: `LayerBadge`, a two-row layer list: "Top  B · after  40%" above
  "Below  A · before".
- `versus`: one pill, "A · before vs B · after".

**Wipe handle.** `WipeHandle` (desktop) and `PinnedWipeLine` (touch) render
`WipeGripHalves` (`review-wipe-grip.tsx`): a 6px rounded grip on the 2px line,
with A's name left of it and B's right of it. Names are stacked letter elements
(`wipeNameLetters`), not a vertical writing mode, because WebKit fails to paint
clipped vertical text. Letters use the divider's color with a 1px white outline
(an eight-direction `text-shadow`) in every theme. `wipeLabelSides` hides a name
when its side is narrower than 32px; `wipeNameMaxLetters` cuts long names to fit
the stage height with a vertical ellipsis. The names are `pointer-events: none`,
so only the grip's 44px touch core grabs and scrubbing beside it still works.

`useWipeGrip` sizes the names and positions the handle by `bottom`, clamped in CSS to stay inside the
stage after a resize. A drag on the handle moves the divider sideways (desktop)
and the handle vertically. The height is a per-round value owned by
`useQuestionReview` (`wipeLift`, keyed by the pending bundle ID, cleared when the
round changes) and provided through `WipeLiftContext`; the handle commits it on
release (AC-004.4).

`useWipeNamesReveal` decides when the names show (AC-004.5): while any source
holds them (the handle during a press, the divider slider under the pointer or a
finger), and for 1.2 seconds after the last release or a `pulse`.
`usePulseOnSplitMove` pulses when the wipe value or the view's position or scale
changes in Wipe, skipping the first value after entering Wipe, and clears every
hold when Wipe ends, since a control that held the names (the slider under the
pointer) may unmount without a pointer-leave. The handle sets `data-active`; CSS
also shows the names on hover and keyboard focus. At 0% and 100% the desktop
handle is clamped inward by half its width, so the grip, hit area, and focus
ring stay inside the stage. In forced colors the line and grip use
`CanvasText` and the names get a `Canvas` backing.

Names come from `imageShortName`. Labels carry `forced-color-adjust: auto` with
a visible border, and are `aria-hidden`. The
stage's existing `role="status"` announcement uses the same identity value and
the same slot names (`useSlotText`) in every mode, each followed by its image's
description (for example "Wipe: A · before, Header before on the left; B · after,
Header after on the right"). Images with the same description stay distinct. The
identity sits bottom left; the zoom controls stay bottom right and Mark area top
right.

## Accessibility and localization

Item navigation buttons, Compare images, and Back to gallery have localized
accessible names. "Item N of M" and the item kind are text, not icons. All copy
uses `t()` with keys in all supported locales. Image labels and descriptions are
agent data and are never translated.

## Compatibility

| Producer or consumer | Behavior |
| --- | --- |
| Agents without `image_groups` | Unchanged validation and identity. Segments review as a gallery with on-demand comparison. |
| Older web client | Ignores `image_groups` and keeps the A/B review over all images. |
| Malformed persisted groups | The web falls back to the gallery for that segment. |
| Late-answer clients | Unchanged delivery. The review view only reads and edits the same form state. |

## Design choices

Image groups are a separate, ordered list of ID references rather than a nested
`images` shape. The flat `images` list, its validation, the image read route, and
older clients stay unchanged. Ungrouped leftovers become singles instead of
failing validation, because choice images often have no counterpart, and requiring
every image in a group would make the common case verbose.

The late-answer form reuses the review view and the overlay's existing late
submitter. A separate review surface for late answers would duplicate the state
and shortcut handling that the shared form already owns.

Identity is derived from the compare reducer state in the same render, so no
second source of truth can drift from what the stage draws.
