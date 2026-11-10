---
status: current
system: ui
requirements:
  - REQ-UI-WALKTHROUGH-STEP-FEEDBACK-001
  - REQ-UI-WALKTHROUGH-STEP-FEEDBACK-002
  - REQ-UI-WALKTHROUGH-STEP-FEEDBACK-003
---

# Walkthrough Step Feedback System Design

## Boundary and current evidence

This design extends the existing UI-owned feedback interaction. It uses
`WalkthroughStepInner`, `useCommentsStore`, and `useRunComment`. It does not
change backend walkthrough storage, agent prompts, or delivery routing.

The current card always mounts `CommentForm`. It adds `WalkthroughComment`
records but does not render them. `markSentInState` removes all accepted
comments. `persistSessionComments` writes the session collection to
`sessionStorage`. These are the implementation boundaries to change.

`CreateTaskWalkthrough` in
`apps/backend/internal/task/repository/sqlite/walkthrough.go` preserves the
walkthrough ID on replacement. Matching only that ID and a step index is unsafe.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-UI-WALKTHROUGH-STEP-FEEDBACK-001 | Identity, retention, delivery, persistence |
| REQ-UI-WALKTHROUGH-STEP-FEEDBACK-002 | Composer, presentation, accessibility |
| REQ-UI-WALKTHROUGH-STEP-FEEDBACK-003 | Drafts, counts, and step picker |

## Identity and selection

Add optional `walkthroughUpdatedAt: string` to `WalkthroughComment`. New records
capture the walkthrough's exact `updated_at` value alongside existing identity
and snapshot fields. Do not change the wire model or database schema.

A pure selector matches source, active session ID, task ID, walkthrough ID,
revision, and step index. Verify the snapshot's repository, file, normalized
line range, and explanation against the selected step. Order matches by
`createdAt`, then ID for ties. Use the same selector on every viewport.

For older records without a revision, require an exact walkthrough ID, index,
repository, file, range, and explanation match. Missing walkthrough IDs or
snapshot mismatches fail closed. Do not infer ownership from a file path alone.
Unmatched pending comments stay in the existing pending-feedback tray.

No history is transferred automatically to a replacement walkthrough. A late
submission acknowledgement updates its captured comment ID, never the newly
selected step. Revision comparison uses the string value without date rounding.

## Retention and persistence

Change `markSentInState` only for `source: "walkthrough"`: retain its canonical
record with `status: "sent"` and remove its ID from `pendingForChat`. Other
comment sources keep their existing removal behavior. Clear matching editing
state and persist every affected session. Repeated acknowledgements are safe.

Keep `bySession` and `byId` as the single history source. Do not create a second
history store. `hydrateSessionInState` already restores pending IDs only for
pending records. Guard `addToPending` and pending selectors against retained
sent walkthrough records, including stale persisted indexes and stale callers.

The step surface must hydrate the active session even if chat has not mounted.
Reuse the existing hydration action. `clearSessionComments` removes pending
and sent history from memory and storage. No cross-tab synchronization,
server persistence, or new retention timer is introduced.

## Delivery and failure

Make accepted submission paths converge on `markCommentsSent`. Direct and
queued Run already use it. Ordinary chat and passthrough chat currently call
`handleClearWalkthroughComments`, which deletes the rows. Replace those success
cleanup calls with acknowledgement of the exact submitted comment IDs.
Do not change explicit user removal through the context tray.

Cover direct Run, queued Run, and feedback attached to ordinary or passthrough
chat. Inspect `chat-input-area.tsx`, `passthrough-chat-composer.tsx`, and
`use-pending-comments.ts`. Capture submitted IDs before awaiting delivery.
An acknowledgement must not mark newly added comments as submitted.

The stored `sent` status renders as **Submitted**. Queue admission is not agent
completion. Do not invent separate processed or failed states from absence of
events. Existing success notifications can distinguish immediate and queued
submission.

Close the composer after saving. Repeated Run requests for the same comment
share the active promise within the hook. After acknowledgement, a retained sent
record cannot be submitted through stale local controls. Do not re-enqueue it
on hydration. This prevents duplicate submission caused by retained history.
It does not establish network-wide exactly-once delivery.

If Run fails, the saved record remains pending and the existing error toast
appears. The pending-feedback tray supplies the existing retry route. Do not
recreate a second comment when the user retries. Submitted rows have no send or
edit action. Display plain comment text safely, preserving line breaks.

## Composer state

The composer starts closed for each task/session/walkthrough revision/step key.
Comment opens it and moves focus to the textarea. Its button uses
`aria-expanded` and `aria-controls`. Cancel and Escape close it and return focus
to the trigger. Escape propagation must not dismiss the surrounding walkthrough.

Reuse `CommentForm` with its real `onCancel` capability. Return an explicit save
outcome from the walkthrough feedback handler before clearing input or closing.
If this requires an additive shared-form callback result, preserve existing
callers that return `void`, and cover those callers' existing behavior.

Add closes after the record enters the store. Run also closes after saving its
record, while the pending row remains visible during delivery. Missing context
leaves the draft and composer intact. A step change closes the transient composer. Cancel preserves unsaved input.
The separate draft state described below supplies input when Comment reopens.

## Desktop and phone presentation

Use the existing floating desktop card and phone walkthrough panel. Do not add
a nested modal for a short comment. The closest shipped surface is
`WalkthroughFloatingWindow`. `MobilePickerSheet` provides the reference pattern
for fixed controls around one scrolling region.

The body order is title, file anchor, explanation, and comment list. When empty,
omit the list and its heading. Keep Comment visible in the footer. When open,
place the composer immediately above navigation. Let the card fit its content
within the available viewport height.

Explanation and history share `min-h-0`, `overflow-y-auto`, and
`overscroll-contain`. Avoid a second comment-list scrollbar. Preserve the
existing floating-card/phone presentation boundary at 640px. New phone control
sizing follows the 768px phone boundary or coarse pointer condition. Use the
available viewport height below 768px, including landscape phones between
640px and 767px. Apply the 78dvh desktop height cap only at 768px and above.
Test both boundaries. Do not refactor unrelated breakpoint policy.

On phones, the current panel remains the focused surface, with safe-area
clearance and dynamic viewport sizing. Keep the composer and navigation usable
when the keyboard reduces available height. Allow the body to shrink first.
If necessary, constrain the input height without obscuring actions. Long text
must wrap without document overflow.

## Visual details and accessibility

Reuse existing Button, Textarea, typography, and color tokens. Use quiet
dividers between comments, not a stack of elevated cards. Retain visible focus
rings. Use 28px ordinary desktop buttons and at least 44px phone/coarse targets.
Keep the existing icon family and `currentColor` styling.

Routine disclosure needs no expressive animation. If a transition is necessary,
name its properties, cap routine feedback at 150ms, and respect reduced motion.
No global theme, shadow, or button-motion redesign belongs in this package.

Add labels through i18next in all seven shipped locales. Generate Traditional
Chinese through `i18n:zh-hant`. Use semantic headings and accessible names for
the comment list, input, disclosure, and submission status.

## Verification and records

Unit tests cover selection, retention, hydration, cleanup, and pending guards.
Component tests cover disclosure, focus, step changes, and save failures.
Desktop and phone Playwright tests cover real Add and Run paths, history after
submission and reload, stable navigation, overflow, and hit-target geometry.

The [plan](../../../plans/walkthrough-step-feedback/plan.md) owns exact commands.
The walkthrough section of `docs/public/sessions-and-review.md` describes the
implemented interaction and browser-tab storage limits.

No ADR is needed: this local interaction change retains existing ownership and
storage boundaries. This design records the retention choice and its limits.

## Drafts, counts, and step picker

Keep unsaved drafts in a small page-lifetime Zustand store, separate from saved
comments. Key drafts by session, task, walkthrough ID, revision, and step index.
Retain drafts through disclosure, navigation, and responsive remounts.
Existing layout remounts can close the walkthrough panel. Reopening restores
the selected step and its draft through Comment. Replace
stale revision entries for a task/session when its current walkthrough changes.
Session cleanup clears its drafts. Do not persist drafts across reloads.

Use a controlled CommentForm mode for walkthrough drafts. Preserve the existing
uncontrolled mode for other consumers. Successful saving clears the captured
step's draft. A stale completion cannot clear another step's draft.

Derive per-step counts from the same matching selector as the comment list.
Show counts in the list heading and step picker. Do not count unsaved drafts.
Use the shared `@kandev/ui/select` for the desktop step picker. Its portal avoids
card clipping. Use its default item-aligned positioning, as in task filters,
with viewport width limits, keyboard selection, and selected-item indication.
Do not add walkthrough-specific popper offsets or alignment overrides.
Phones and coarse pointers use the
shared `MobilePickerSheet` with ordered `CommandItem` rows. The sheet owns one
scroll region and remains open through viewport-height changes. Its trigger and
rows have 44px targets. One tap selects the step and closes the sheet. Escape or
dismissal restores trigger focus; selection retains existing file-reveal focus.
Use the shared drawer's standard bottom-sheet entrance, exit, backdrop, and drag
handle, matching the session and terminal selectors. Remove the walkthrough's
animation opt-out. For reduced-motion preferences, disable animation and
transition on this selector's content and backdrop. Keep the list background
transparent so it reads as part of
the sheet. Rows show a step number, title, and separate muted comment count.
Use the same bordered active-row treatment as the session and terminal lists.
The confirmed direct-touch activation remains responsible for reliable taps.
Rendered tests wait for the sheet's actual entrance or dismissal before sending
a single raw tap at its visible controls; they must not retry failed taps.
Both surfaces use Kandev styling instead of an operating-system picker.
Selection uses setWalkthroughActiveStep, including existing file reveal behavior.
Exclude the select from drag initiation. Keep titles and counts localized and
constrain long option labels to the card width.

## Content-driven height

Remove the floating card's fixed height. Let the explanation and comments take
natural space until the viewport limit is reached. The body is the single scroll
owner. Put its padding inside the scroller so the region can collapse to zero
height. The header, picker, and navigation do not shrink. The open composer
can shrink its input to 44px while its action row retains full touch targets.
Reserve the combined minimum input, action row, and spacing height so neither
row can overlap navigation. On desktop,
observe card resizing and clamp its position to the viewport after content or
viewport changes. On phones,
retain the bottom-sheet placement, status-bar clearance, and safe-area inset.

## Touch activation

The mobile picker and Previous/Next buttons handle completed primary touch
taps on pointer release. Opening, selection, and navigation must not depend on
a later browser compatibility click. Reuse `useWalkthroughTap` in both the
picker and navigation components. Disabled boundary buttons do not attach touch
handlers, so navigation cannot pass the first or last step.
Track the pointer ID, origin, duration, and control bounds. Cancel on movement
over 8 CSS pixels, pointer cancellation, a secondary touch, release outside the
control, or a press lasting 500 ms or more. Keep browser scrolling enabled.
A control-owner document capture guard consumes the subsequent pointer click,
including retargeted clicks after a row unmounts. Clear the guard on the next
pointer gesture or owner unmount. Keyboard clicks remain available. Mouse and
keyboard activation continue through the shared controls' normal handlers.
