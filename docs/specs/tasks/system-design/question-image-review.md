---
status: current
system: tasks
created: 2026-11-07
requirements:
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-001
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-002
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-003
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-004
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-005
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-006
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-007
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-008
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-009
---

# Question image review system design

## Purpose and boundaries

Add a large, opt-in review surface to the shared clarification overlay. The
surface reads the same answer state, the same authorized image bytes, and the
same submit path as the compact panel. Only two backend contracts change: the
bundle image limit, and an optional `annotations` list on each answer.

The [requirements](../requirements/question-image-review.md) own observable
behavior. The [plan](../../../plans/question-image-review/plan.md) owns delivery
order. The [question image previews design](question-image-previews.md) remains
authoritative for image references, authorization, transport, and file limits.
The [answer attachments design](question-answer-attachments.md) remains
authoritative for answer files. All new symbols below are proposed interfaces.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-TASKS-QUESTION-IMAGE-REVIEW-001 | Review ownership, Form placement, Closing and focus |
| REQ-TASKS-QUESTION-IMAGE-REVIEW-002 | Layout, Keyboard scopes |
| REQ-TASKS-QUESTION-IMAGE-REVIEW-003 | Segments, Image loading |
| REQ-TASKS-QUESTION-IMAGE-REVIEW-004 | Viewport model |
| REQ-TASKS-QUESTION-IMAGE-REVIEW-005 | Comparison modes, Difference worker |
| REQ-TASKS-QUESTION-IMAGE-REVIEW-006 | Gallery capacity |
| REQ-TASKS-QUESTION-IMAGE-REVIEW-007 | Annotation contract, Annotation interface |
| REQ-TASKS-QUESTION-IMAGE-REVIEW-008 | Compatibility |
| REQ-TASKS-QUESTION-IMAGE-REVIEW-009 | Image labels |

## Current integration points

Paths are repository-relative.

| Boundary | Current owner and extension |
| --- | --- |
| Bundle image validation | `apps/backend/internal/clarification/protocol/images.go`: `ValidateQuestionImages` caps distinct IDs at four. Raise the cap to 16 through one named constant. |
| Operator tool schema | `apps/backend/internal/mcp/server/question_images.go`: `buildQuestionImageSchema` description states the limits. |
| Answer model | `internal/clarification/protocol/types.go`: `Answer` has question ID, selections, custom text, and attachments. Add `Annotations`. |
| Answer validation and normalization | `internal/clarification/outcome_validation.go`: `validateAnswers` and `normalizeAnswer` run before the claim. |
| Durable answer encoding | `internal/clarification/response_serialization.go`: `SerializeResponse` and `DeserializeResponse`. |
| Live agent result | `apps/backend/internal/mcp/server/handlers.go`: `extractQuestionAnswers` and `simplifyAnswer`. |
| Detached answer text | `internal/clarification/resolver.go`: `formatSingleAnswer`, `formatAnswerBody`, `withAnswerFiles`. The orchestrator wraps that text in `buildClarificationPrompt`. |
| Shared answer form | `apps/web/components/task/chat/clarification-input-overlay.tsx`: `ClarificationInputOverlay` owns the group, drafts, attachments, and `ClarificationCarouselBody`. The file is already above the 600-line limit; new logic lives in new modules. |
| Draft retention | `clarification-draft-context.tsx`: `useClarificationDraft` keys drafts by session, pending ID, and mode. |
| Image reads | `hooks/domains/session/use-question-images.ts` and `question-image-scheduler.ts`: authenticated, abortable, two concurrent reads, blob URLs revoked on cleanup. |
| Existing inspector | `question-image-viewer.tsx`: Fit and Actual size dialog. It stays unchanged. |
| Primitives | `@kandev/ui/dialog`, `@kandev/ui/resizable`, `@kandev/ui/toggle-group`, `@kandev/ui/slider`, `@kandev/ui/select`; `hooks/use-responsive-breakpoint.ts`. |
| History | `messages/clarification-request-message.tsx`: `AnswerSummary` renders the answered response. |

## Review ownership

`ClarificationInputOverlay` owns review state through a new hook,
`useQuestionReview`, in `components/task/chat/question-review/use-question-review.ts`.
The hook returns `{ available, open, openReview, closeReview, triggerRef }`.
`available` is true only when the overlay is in active, answerable mode and any
question in `sortedMessages` has `question.images` or `options[].images`.
Late-answer, expired, submitted, and success states set it to false.

All hosts that mount the shared overlay gain the control: task chat, Quick Chat,
Office, Needs-you Inbox, and coordinator surfaces. No host-specific code is needed.

The Review images button renders in `ClarificationOverlayTopBar` beside the
existing panel actions. Its accessible name stays stable. Only one review view
can be open per overlay instance.

## Form placement

The answer form renders in exactly one place. While review is closed,
`ClarificationCarouselBody` renders in the compact panel as today. While review is
open, the overlay renders `QuestionReviewDialog` and passes the same
`ClarificationCarouselBody` element as its `form` slot. The compact panel shows a
short status line, "Reviewing in the large view". The modal dialog makes the
compact panel inert, so the status line has no controls.

Rendering the body once avoids duplicate test IDs, duplicate keyboard listeners,
and two text inputs editing one draft. All state stays in the overlay's existing
hooks: `useClarificationGroup`, `useClarificationDraft`, and
`useClarificationAnswerAttachments`. Moving the body therefore loses nothing.
`ClarificationCustomInput` must read its value only from the `draft` prop. Add a
test that a draft typed in the review view appears in the compact panel after
closing, and the reverse.

Inside the dialog, `ClarificationCard` receives `stageOwnsImages`. When it is set,
the card does not render question or choice thumbnails, because the stage shows
them. Choice rows show a small "Image N" text reference that links the choice to
its stage image. The active question index is the overlay's existing
`activeIndex`, so the segment strip, the carousel, and the compact panel share it.

## Closing and focus

`closeReview` restores focus to `triggerRef`. If the trigger is not mounted, focus
moves to the compact panel's status line. The dialog's Escape handler closes the
view and stops propagation, so the parent panel and Quick Chat dialog do not
collapse. Register a `useClarificationEscapeGuard` predicate while the view is
open, following `QuestionImageViewer`.

The overlay closes the view without submitting when any of these happen:
`group.submitState` becomes `ok` or `expired`, `group.pendingId` changes,
`sortedMessages` becomes empty, or the overlay unmounts. A successful submission
from the view closes it after the existing success path runs.

## Layout

`QuestionReviewDialog` uses `@kandev/ui/dialog` with `enterConfirms={false}` and
no animation, like the existing inspector.

Desktop (`useResponsiveBreakpoint().isMobile === false`): content is
`h-[min(94dvh,1200px)] w-[min(96vw,1800px)]`. A header row holds the title,
segment strip, and Close. Below it, a vertical `ResizablePanelGroup` holds the
stage panel (default 62%, minimum 30%) and the form panel (minimum 25%, scrolls
internally). The resize handle is keyboard operable through the primitive.
Remember the split per overlay instance in component state only.

Phone: content is `h-dvh w-screen` with safe-area padding. The header holds Close,
"Review N of M", and the segment strip as a horizontally scrolling row inside its
own container. The stage is fixed at `min(55dvh, 60%)`. The form scrolls below it.
Controls use 48px nominal targets. Each stage toolbar row (`reviewToolbarRow`)
wraps on desktop but is one horizontally scrolling line on phones, so the toolbar
does not take the stage's height; controls keep the desktop order. No
width-dependent layout renders choices beside the stage.

All dimensions use rem or viewport units. The page body never scrolls
horizontally. Only the stage canvas may pan.

## Keyboard scopes

Two scopes exist while the view is open.

- **Form scope:** `ClarificationCarouselBody` keeps `CarouselKeyboardShortcuts`
  enabled, with the dialog content as `scopeRef`. Digits select options, arrows
  move between questions, and the existing submit shortcut submits.
- **Stage scope:** The stage area (toolbar and canvas) stops the propagation of
  every key except Tab and Escape, so window-level carousel listeners never see
  them. The canvas is a focusable, labelled `region` whose description lists the
  stage keys; it calls `preventDefault()` for each key it consumes.

Stage keys: `+` and `-` zoom; `0` Fit; `1` Actual size; arrow keys pan by 10%
of the visible area, or 1 image pixel with Shift; `t` flips A and B; `s` swaps A
and B; `[` and `]` move the wipe divider or the onion opacity by 5%. In annotate
mode, `n` adds a region at the view center, arrow keys move the selected region
(Shift moves 10 pixels), Alt with an arrow key resizes it, and Delete or
Backspace removes it. Digits and Enter are not stage keys. Escape closes the view;
a region that is still being dragged is discarded.

`CarouselKeyboardShortcuts` already ignores events outside its scope. Add a
test that a digit or arrow pressed with focus in the stage does not change the
selection or question.

## Segments

`buildReviewSegments(messages)` in `lib/question-review/segments.ts` is a pure
function. For each message in bundle order, it returns
`{ questionId, index, label, answered, images: ReviewImage[] }`.
`ReviewImage` is `{ id, alt, choiceLabel?, ref: QuestionImage }`. Images are the
question's images, then each option's images in option order. An image ID that
repeats within a segment appears once, at its first placement.

The segment strip is a labelled `nav` with an ordered list of buttons and
`aria-current="step"` on the active segment. Each button shows the position, the question title or a truncated
prompt, an image count, and an answered check icon with a text alternative.
Selecting a segment calls the overlay's `setActiveIndex`. Segments without images
show a stage message, "This question has no images", and the form stays usable.

## Image loading

The stage calls `useQuestionImages(pendingId, questionId, segment.images.map(i => i.ref), active, owner)`
for the active segment only, with the owner key from `useQuestionImageOwner`,
which the compact previews also use. The existing hook already aborts work, revokes
blob URLs, and clears its snapshot when its key changes, so moving to another
segment releases the previous segment's bytes. The compact panel does not mount
its thumbnails while the view is open, so the same images are not fetched twice.

Comparison needs pixels as well as URLs. Extend the loaded state to keep the
`Blob`: `{ kind: "loaded"; url: string; blob: Blob }`. Existing consumers keep
using `url`. The blob is released with the URL. No global cache is added.

## Viewport model

`useReviewViewport` in `question-review/use-review-viewport.ts` owns
`{ mode: "fit" | "manual"; scale: number; x: number; y: number }`, where `x` and
`y` are the image-pixel coordinates at the stage's top-left corner. `fit`
derives its scale from the stage size and the larger of A and B, with no lower
bound, so the whole image always shows (for example in stacked phone panes).
Zoom steps are 0.25, 0.5, 1, 2, 4, and 8. Wheel input with Ctrl or Meta and pinch
input change scale continuously and keep the focal image point fixed. Every zoom
path stops at `minScaleFor(fit)`, the smaller of the Fit scale and 0.25, and at 8. Plain
wheel input pans. Pointer drag pans through pointer capture. The stage sets
`touch-action: none` only on the canvas so the form keeps native scrolling.

Every layer renders as an `<img>` inside a transformed container,
`transform: translate(-x*scale px, -y*scale px) scale(scale)` with
`transform-origin: 0 0`. Set `image-rendering: pixelated` when `scale > 1`.
Because all layers share one transform, mode switches and flips cannot move the
view. Changing the segment resets to `fit`. Changing A or B keeps the current
manual view and clamps it to the new bounds.

## Comparison modes

`useReviewCompare` holds `{ mode, aId, bId, wipe: 0.5, opacity: 0.5, tolerance: 16 }`.
A defaults to the first image and B to the second. With one loaded image, the
stage shows it alone and hides comparison controls.

| Mode | Rendering |
| --- | --- |
| Toggle | One visible layer. Flip changes which layer is visible without remounting it. An `aria-live="polite"` status announces "Showing A: <alt>". |
| Wipe | B above A, clipped at the divider in image pixels. The divider is a `role="slider"` with value text, operable by drag and keys. `wipeFromPointer` maps the drag over the stage minus 24px at each side, so 0% and 100% are reached away from the screen edge. CSS `clamp()` keeps the handle inside the stage while the line reaches the edges. A toolbar slider sets the same value. |
| Side by side | Two stage panes, each with the same transform, split horizontally on desktop and vertically on phones. |
| Onion skin | B above A with CSS `opacity`. The opacity control uses `@kandev/ui/slider`. |
| Difference | A worker-generated mask over A. The toolbar shows the changed count and percentage and a tolerance slider from 0 to 64. |

When A and B dimensions differ, the toolbar shows "A: W×H, B: W×H" and both
layers align at image coordinate (0, 0) at the same scale.

The compare toolbar uses `ToggleGroup` for the modes and for the A and B
pickers. Picker buttons show the stage number; their accessible names carry each
image's description and choice label. Choosing the same image for A and B swaps them.

## Difference worker

`lib/question-review/diff.worker.ts` runs off the main thread, created with
`new Worker(new URL("./diff.worker.ts", import.meta.url), { type: "module" })`,
as in `components/diff-worker-pool-provider.tsx`. `lib/question-review/pixel-diff.ts`
holds the pure comparison function so Vitest can test it without a worker.

Input: the two blobs and the tolerance. The worker decodes both with
`createImageBitmap`, draws each to an `OffscreenCanvas` at its natural size, reads
`ImageData`, and compares each pixel in the union of both sizes. A pixel is changed
when any RGBA channel differs by more than the tolerance, or when it is outside
one image's bounds. The worker writes an RGBA mask, with changed pixels in opaque
magenta and unchanged pixels transparent, and transfers its `ArrayBuffer` with
`{ changed, total, width, height }`. The stage draws the mask into a canvas inside
the shared transformed layer, so it follows zoom and pan.

The worker never resizes inputs. Peak memory for two 16-megapixel images and a
mask is about 192 MB. If `Worker`, `OffscreenCanvas`, or `createImageBitmap` is
unavailable, or decoding or allocation throws, the worker posts `{ ok: false }`
and the stage shows the unavailable state. Each request carries an increasing ID,
and the stage ignores responses to older requests. The worker keeps the decoded
pair for the current blobs, so tolerance changes, debounced by 150ms, do not
decode again; picking another image replaces the pair. The stage remounts per
segment and terminates its worker on unmount.

## Gallery capacity

Add `const MaxBundleImages = 16` and `const MaxQuestionImageReferences = 4` in
`protocol/images.go`. `ValidateQuestionImages` uses both constants, and its error
messages include the numbers. `buildQuestionImageSchema` builds its description
from the same values, so the schema text cannot drift. Update
[question image previews requirements](../requirements/question-image-previews.md)
AC-001.1 and AC-005.1, the previews design, and `docs/public/agent-communication.md`
in the same change. No web change is needed for the limit, because previews load
per question. The existing `slice(0, 4)` in `QuestionImages` remains a
per-placement bound.

## Annotation contract

Add to `protocol`:

```go
type AnswerAnnotation struct {
    ImageID     string `json:"image_id"`
    X           int    `json:"x"`
    Y           int    `json:"y"`
    Width       int    `json:"width"`
    Height      int    `json:"height"`
    ImageWidth  int    `json:"image_width"`
    ImageHeight int    `json:"image_height"`
    Comment     string `json:"comment"`
}
```

`Answer` gains `Annotations []AnswerAnnotation \`json:"annotations,omitempty"\``.
Web `ClarificationAnswer` gains `annotations?: ClarificationAnswerAnnotation[]`
with the same snake_case fields.

`validateAnswers` calls a new `validateAnswerAnnotations(i, a, q)` before the
claim. It enforces all of the following and returns a `validationError`:

- At most eight entries per answer.
- `ImageID` matches an image in `q.ImageReferences()`.
- `ImageWidth` and `ImageHeight` are positive, and their product is at most
  16,000,000, computed without overflow.
- `X` and `Y` are at least 0, `Width` and `Height` are at least 1, and
  `X+Width <= ImageWidth` and `Y+Height <= ImageHeight`.
- `Comment`, after trimming, has 1 to 500 Unicode code points and contains no
  NUL characters.

The server does not read the image to check its dimensions. The observed size is
operator-reported context that lets the agent rescale if the file changed.

`normalizeAnswer` trims comments and drops an empty list. Annotations alone do not
make an answer: the existing selection or custom-text requirement is unchanged.
A rejected bundle ignores annotations.

`SerializeResponse` and `DeserializeResponse` add a `serializedAnnotation` slice
with `omitempty`, so answers without regions produce byte-identical output.
`metadata.response.annotations` therefore persists with the existing claim
transaction. No migration is needed.

`simplifyAnswer` copies `annotations` into the live keyed result beside
`selected_option`, `custom_text`, and `attachments`. Update the output schema to
allow it, and keep the JSON text and structured content identical.

`withAnswerFiles` becomes `withAnswerEvidence`. After attached file names, it
appends one line per region:

```text
Region 1: {"image_id":"after","x":12,"y":40,"width":200,"height":32,"image_width":1440,"image_height":900,"comment":"Label contrast too low"}
```

Each line is produced by `json.Marshal` of a fixed struct, so quotes, newlines,
and the text `Region 2:` inside a comment are escaped inside one JSON string and
cannot start another entry. The region number comes from the slice position, not
from operator input. Detached answers reach the agent through
`buildClarificationPrompt`, which already embeds this text.

Native provider questions cannot carry images, so their bridges never receive
annotations. Parent questions reject images, so parent answers never carry them.

## Annotation interface

Annotation state is per question in the overlay:
`annotationsByQuestion: Record<string, ReviewRegion[]>`, owned by
`useReviewAnnotations` and stored in the existing draft cache beside
`customDrafts`, under the same draft key. `enrichAnswers` adds
`annotations` to each answered question, as it already does for attachments.
Skip and bundle replacement clear them.

Annotate mode is available in the single-image view and in Toggle. In Toggle,
regions attach to the image shown when drawing starts. Drawing uses pointer events
in image coordinates through `useReviewViewport`, rounded to integers and clamped
to the image bounds. Each region renders as an outlined rectangle with its number.
Regions drawn on other images are hidden but stay listed.

Finishing a region (pointer up, or `n`) opens `RegionCommentPopover`, which reuses
the chat's `PlanSelectionPopover`: a floating popover anchored below the region on
fine pointers and the same bottom drawer on touch. It renders into the review
dialog (`portalContainer`) so the modal does not make it inert. While it is open,
the dialog's `onEscapeKeyDown` calls `preventDefault()`, so Escape closes only the
popover and the carousel shortcut ignores it. `useReviewAnnotations` tracks
`commenting` (question, index, new or existing). Cancelling a new region discards
it; saving sets its comment. Enter on a selected region, or Edit in the list,
reopens the popover with Delete.

Below the choices, the form shows a read-only "Regions" list: number, image
description, comment text, Edit, and Delete. Adding a ninth region does nothing,
and the Annotate control explains the limit. Submission stays blocked while a new
region is waiting for its comment.

History: `AnswerSummary` lists `metadata.response.annotations` as
"Region N on <image alt>: <comment>", rendered as plain text, below the selection.

## Usability round (2026-11-09)

- **Answering:** `buildQuestionHandlers` takes `stayOnAnswer`; the review view
  passes it, so choosing an answer stays on the question, and
  `ClarificationCarouselNav` emphasizes Next (`data-emphasized`).
- **Phone composition:** `ReviewLayoutProvider` gives stages `phone`, `touch`, and the
  dialog's bottom bar element. Stages render the mode sheet button and More above
  the image, `ZoomChip` and `StatusChips` on it, and frequent actions through
  `ReviewBottomBar` (a portal into the bar). The dialog hides the bar while a text
  field has focus (`onFocusCapture`).
- **Carried-over settings:** the dialog keeps `CompareSettings` (mode, wipe,
  opacity, tolerance); `useReviewCompare` starts each segment from it and reports
  changes.
- **Candidate stepping:** `compareReducer` `step` moves B through the images other
  than A without wrapping; `CandidateStepper` renders it; `[`/`]` step in Toggle and
  Side by side; `useReviewViewport` `onSwipe` steps on a swipe that starts at Fit.
- **Hold to compare:** `useHoldToCompare` shows A after a 300ms still press or while
  Space is held; the stage renders an effective Toggle-on-A state.
- **Pinned wipe:** on touch, Wipe uses `clampPinnedView`, which lets either image
  edge reach the center line, and `pinnedWipeFraction`; `PinnedWipeLine` replaces
  the draggable handle.
- **Zoom to changes:** `diffPixels` returns `bounds`; `ChangeOutline` marks them and
  `rectView` fits them.
- **Marking:** **Mark area** (pen icon, solid primary when on), `AnnotateHint` with
  Done, a primary ring on the stage, Escape order in the dialog's
  `onEscapeKeyDown`, and `useRegionClick`, which opens a region on a still press.
  `RegionsSidePanel` lists regions beside the image on wider screens and
  `RegionsSheet` on phones.

## Image labels

`QuestionImage.Label` is an optional one-word label matching
`protocol.ImageLabelPattern`. `ValidateQuestionImages` rejects a malformed label
and the same label on two different images of a question. The label is part of the
struct, so persistence, `questionsEqual`, and the request hash include it. The tool
schema offers it. The web keeps only valid labels (`buildSegmentImages`) and
`imageShortName` shows the label or the stage number.

## Accessibility and motion

Every icon button has a localized accessible name. The stage announces the mode,
the shown image, and the zoom level through one `role="status"` region. The
difference result is also announced as text. Focus outlines remain visible in
forced-colors mode, and the difference mask uses `forced-color-adjust: none` so
the highlight stays visible. No animation is required. Mode changes are instant.

All copy uses `t()` with keys in all seven locales. Generate Traditional Chinese
with `pnpm run i18n:zh-hant`. Image descriptions, choice labels, and comments are
agent or operator data and are never translated or rendered as HTML.

## Failures

| Failure | Result |
| --- | --- |
| Image read fails | Existing localized error and Retry in that image's stage slot. Other images and the form keep working. |
| One of A or B is not loaded | Comparison controls are disabled with a status explaining which image is missing. |
| Difference cannot run | Localized unavailable state. Other modes stay available. |
| Bundle resolves elsewhere while open | View closes. The existing outcome banner shows in the compact panel. |
| Annotation validation fails | Existing submit error banner with the server message. The bundle is not claimed and drafts remain. |
| Older backend rejects `annotations` | Same submit error. The operator can delete regions and resubmit. |

## Compatibility

| Producer or consumer | Behavior |
| --- | --- |
| Image-free bundles | No Review control and no new code path. |
| Existing compact previews and inspector | Unchanged while the view is closed. |
| Answers without annotations | Field omitted. Serialization, retry identity, MCP result, and prompt text are unchanged. |
| Older web client | Never sends annotations. Bundles with up to 16 images still load one question at a time. |
| External answer clients | May omit annotations. May send them with the same validation. |
| Older agents | Ignore the additive `annotations` key. Prompt text remains readable. |

## Design choices

Rendering the existing form inside the dialog keeps one answer state and one
submit path. A second form bound to shared state would need duplicated shortcut
and focus handling.

Client-side comparison reuses the authorized bytes and satisfies the issue's
constraint against new server routes and transformations. Full-resolution
comparison in a worker keeps one-pixel changes visible. The cost is high memory
on large images, which fails visibly instead of silently downscaling.

Annotations extend the answer the same way attachments did. They reuse the
existing claim, persistence, and delivery path, so no ADR is needed.
