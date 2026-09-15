---
status: current
system: tasks
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-001
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-002
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-003
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-004
---

# Web Preview Feedback System Design

## Purpose and boundaries

The task system owns unsent web-preview feedback because its lifecycle ends at
task-message admission, not when a preview panel unmounts or a session changes.
The backend is authoritative for the pending collection, screenshot ownership,
and exact consumption. Session state selects a destination only when the user
submits a composer.

The UI system owns pointer, touch, and keyboard capture inside existing preview
iframes and projects the task collection into responsive controls. The session
port proxy remains responsible for preview reachability and inspector-script
injection. Native HTML preview keeps its trusted-workspace execution contract.
The attachment service remains responsible for image bytes, and the existing
direct-message and durable-queue paths remain responsible for delivery.

This design extends the task-owned, exact-reference delivery model used by
[task plan comments](plan-comments.md). It creates a separate collection because
preview feedback is independent of a current plan and can own image attachments.
It does not generalize the session-local diff, file, pull-request, walkthrough,
or agent-message comment stores.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-WEB-PREVIEW-FEEDBACK-001` | [Ownership decision](#ownership-decision), [Persistence model](#persistence-model), [Synchronization](#synchronization), [Lifecycle](#lifecycle) |
| `REQ-TASKS-WEB-PREVIEW-FEEDBACK-002` | [Capture bridge](#capture-bridge), [Capture payloads](#capture-payloads), [Screenshot regions](#screenshot-regions), [Marker projection](#marker-projection) |
| `REQ-TASKS-WEB-PREVIEW-FEEDBACK-003` | [Composer projection](#composer-projection), [Canonical prompt](#canonical-prompt), [Atomic acceptance](#atomic-acceptance) |
| `REQ-TASKS-WEB-PREVIEW-FEEDBACK-004` | [Responsive interaction](#responsive-interaction), [Accessibility](#accessibility), [Failure and recovery](#failure-and-recovery) |

## Ownership decision

One pending preview-feedback collection belongs to a task. Pending rows do not
carry a delivery destination. They may record the session that supplied the
preview solely as source provenance; changing or deleting that session does not
change row ownership or remove the capture.

Pending feedback is not model context. Browser navigation, opening a composer,
loading an iframe, and restoring markers are read-only projections. The
ordinary composer **Send** action is the only delivery entry point added by
this feature and addresses the selected composer session. A promptable session
uses direct-message admission; a busy session uses the existing queue action.

The ownership and atomic-delivery choice is recorded in
[Persist Web Preview Feedback with the Task](../../../decisions/2026-09-15-task-owned-web-preview-feedback.md).

## Components and responsibilities

- `PreviewFeedbackService` and the task repository own authorization, CRUD,
  collection revisions, limits, screenshot claims, lifecycle cleanup, and
  ordered snapshots.
- Task WebSocket handlers expose list/create/update/delete/clear mutations and
  publish complete replacement snapshots after committed changes.
- Direct-message and message-queue admission resolve exact feedback references,
  build the canonical visible prompt, transfer screenshot claims, persist the
  target message or queue entry, and consume the referenced rows atomically.
- A task-level frontend slice owns one authoritative feedback snapshot per
  task. It is independent of `CommentsState.bySession` and browser storage.
- `usePreviewCapture` owns capture-mode state for one iframe. It accepts events
  only from that iframe, converts a completed selection into a local draft, and
  projects saved markers for the current page.
- `PreviewFeedbackControls` and `PreviewFeedbackPanel` provide shared desktop
  and phone behavior for `BrowserPanel` and `HtmlPreviewContent`.
- The chat composer reads the task snapshot and submits exact references with
  the same direct-or-queue routing, retry identity, and settlement rules as its
  existing content.
- The existing attachment service owns staged PNG bytes and their transition
  from unsent task feedback to a message or queue entry.

## Persistence model

### `task_preview_feedback_collections`

```text
task_id       text       primary key; owning task
revision      integer    monotonic collection revision, starts at zero
created_at    timestamp  UTC
updated_at    timestamp  UTC
```

The row is created lazily with the first list or mutation. Task deletion
cascades it. Every committed create, update, delete, clear, or delivery
consumption increments `revision` in the same transaction.

### `task_preview_feedback`

```text
id                        text       primary key; caller-generated UUID
task_id                   text       owning task
kind                      text       text | element | screenshot
comment                   text       required user feedback
source_kind               text       browser | html_file
source_session_id         text       nullable capture provenance
source_label              text       bounded display name
source_path               text       nullable workspace-relative HTML path
page_route                text       path and query at capture time
page_title                text       bounded document title
selected_text             text       nullable text selection
text_anchor_json          text       nullable DOM range and visual position snapshot
element_snapshot_json     text       nullable bounded semantic/HTML snapshot
capture_rect_json         text       nullable CSS-pixel rect and viewport data
screenshot_attachment_id  text       nullable unique attachment reference
version                   integer    optimistic version, starts at one
created_at                timestamp  UTC
updated_at                timestamp  UTC
```

`task_id` has `ON DELETE CASCADE`. `screenshot_attachment_id` is present only
for screenshot rows and references a task-owned claimed attachment. It is
unique, so one stored image cannot back two feedback items. Text and element
rows must not reference an attachment. Database checks enforce the closed kind
set and kind-specific nullable fields.

`source_session_id` is descriptive and has no cascading foreign key. Source
provenance therefore remains readable after a task session is deleted. A
Browser source label identifies the captured local service without persisting
credentials or a port-proxy capability URL. An HTML-file source also records
its repository-relative path. `page_route` contains the decoded pathname and
query, never the URL fragment, user information, cookies, or capability token.

Rows sort by `created_at`, then `id`. A complete snapshot has this form:

```json
{
  "task_id": "task-id",
  "revision": 8,
  "items": []
}
```

### Limits

A shared backend `previewfeedback` package defines and validates limits for
both database implementations and both delivery paths:

- 100 pending items per task;
- 64 KiB per comment;
- 256 KiB per selected-text or HTML-snippet field;
- 1 MiB of aggregate non-image payload in the pending collection;
- at most 10 pending screenshot images;
- PNG only, at most 10 MiB and 16 megapixels per screenshot;
- 1 MiB for the final formatted text prompt;
- the existing message-wide maximum of 10 attachments and 100 MiB after
  screenshots and ordinary composer attachments are combined.

The client prevents obviously invalid drafts, but the backend repeats every
bound. A collection mutation or delivery that crosses a bound rolls back
without changing its revision or attachment ownership.

## Capture bridge

The existing injected inspector becomes a selection bridge rather than the
owner of comments. `BrowserPanel` continues to proxy eligible executor-local
URLs before enabling capture. `HtmlPreviewContent` recognizes its already
proxied, inspector-capable URL directly, so it does not rewrite or remount that
URL merely to enable the controls.

The parent sends one versioned command envelope to its iframe:

```ts
type PreviewCaptureCommand =
  | { type: "kandev-preview-capture"; version: 1; command: "start"; mode: CaptureKind }
  | { type: "kandev-preview-capture"; version: 1; command: "cancel" }
  | { type: "kandev-preview-capture"; version: 1; command: "project"; anchors: MarkerAnchor[] };
```

The bridge emits only bounded, structured capture metadata:

```ts
type PreviewCaptureEvent = {
  type: "kandev-preview-capture-result";
  version: 1;
  kind: "text" | "element" | "screenshot";
  page: { route: string; title: string };
  rect?: CaptureRect;
  selectedText?: string;
  textAnchor?: TextAnchorSnapshot;
  element?: ElementSnapshot;
};
```

The parent validates `event.source === iframe.contentWindow`, the protocol
version, closed discriminators, field types, and limits before creating a
draft. It ignores unknown or oversized events. The inspected page never sends
feedback directly to the backend and never supplies trusted task, workspace,
attachment, or session ownership identifiers.

The bridge owns hover outlines, region drag overlays, touch gesture capture,
keyboard cancellation, selection extraction, route discovery through
`window.__kandevProxyPrefix`, and best-effort marker positioning. The Kandev
parent owns all localized controls, comment editing, upload state, mutation
errors, and draft settlement.

## Capture payloads

### Text

Text mode reads the browser `Selection` after a completed pointer, touch, or
keyboard selection. It normalizes whitespace for display while retaining the
bounded exact string and a frozen `TextAnchorSnapshot`. The anchor contains the
nearest rendered containing-element snapshot, start and end element selectors,
DOM child-node index paths and text offsets, every bounded selection client
rectangle, the union rectangle in document coordinates, scroll position,
viewport dimensions, device-pixel ratio, and page identity. Node paths and
offsets describe the rendered DOM at capture time; they are evidence, not a
promise that a later page can resolve the same nodes. A collapsed selection does
not create a draft.

The persisted anchor and containing-element HTML are independent of the live
page. Script-generated or transient text therefore remains understandable to
the agent even if source search fails or later navigation produces different
DOM. Marker restoration uses selectors and node paths as best-effort hints, but
failure to restore never removes the captured runtime snapshot.

### Element

Element mode resolves the deepest eligible element beneath the pointer or
keyboard focus. Its snapshot contains tag, ID, classes, role, accessible label,
visible text, stable selector when derivable, and a bounded `outerHTML` string.
Kandev displays `outerHTML` only as escaped code. It never inserts the string
with `innerHTML`.

Before selection, the bridge positions a non-interactive outline and translucent
fill over the current candidate. A small overlay label shows the shortest useful
identity in tag, `#id`, and `.class` order, while the full selector remains in
the eventual capture details. Mouse or pen hover, keyboard focus movement, and
touch press update the same candidate state. On touch, the hint appears on
press before release commits the capture. Candidate chrome is marked as
inspector-owned, excluded from hit testing, and removed on completion, cancel,
mode change, navigation, or iframe teardown.

### Screenshot regions

Region mode records a CSS-pixel rectangle inside the current iframe document.
The parent uses the proxied iframe document and the maintained `html2canvas`
library to rasterize that rectangle to PNG. The parent downsizes output that
would exceed 16 megapixels, validates the encoded result, and shows a thumbnail
and dimensions before the user can save it.

This is a DOM rendering of the selected region. Unsupported CSS and cross-origin
images or canvases may not be reproducible; the library documents those
limits in its [official documentation](https://html2canvas.hertzen.com/documentation/)
and [FAQ](https://html2canvas.hertzen.com/faq/). The confirmation thumbnail is
therefore part of the capture contract. A renderer exception, empty canvas,
tainted canvas, encoding failure, or limit violation produces a visible error
and no persisted feedback.

On save, the parent uploads the PNG as a staged prompt attachment, then creates
the feedback row with the returned attachment ID. The create transaction
validates owner, workspace, task, MIME signature, size, kind, and uniqueness,
and changes the attachment to a task-owned feedback claim. If the create fails,
the staged upload remains available for the unchanged draft retry and expires
through the existing staged-attachment lifecycle.

The Screen Capture API is not the primary path because it requires a separate
user permission prompt and lets the browser mediate the captured surface rather
than guaranteeing the active preview region. Its constraints remain documented
by [MDN](https://developer.mozilla.org/en-US/docs/Web/API/MediaDevices/getDisplayMedia)
and the [W3C specification](https://www.w3.org/TR/screen-capture/).

## Marker projection

Saved text and element items are immutable captures even when the live page
changes. On iframe readiness and route changes, the parent sends only current
source-and-route anchors to the bridge. The bridge first resolves a saved
selector and then uses the saved rectangle as a visual fallback for the same
viewport geometry. A marker that cannot be placed remains available in the
pending list and composer.

Markers are presentation state. Projecting, failing to project, navigating, or
unmounting an iframe never mutates the backend collection. Selecting a marker
opens the corresponding task-owned item in Kandev chrome.

## Synchronization

The task WebSocket family adds authorized actions:

- `task.preview_feedback.list`
- `task.preview_feedback.create`
- `task.preview_feedback.update`
- `task.preview_feedback.delete`
- `task.preview_feedback.clear`

Every request carries `task_id`. Create carries a caller-generated item ID,
capture payload, source provenance, and optional staged screenshot attachment
ID. Update carries `id`, `expected_version`, and the new comment; anchors and
capture content are immutable. Delete carries `id` and `expected_version`.
Clear carries the collection revision it was rendered from so it cannot erase
concurrently added items.

Repeating a create with the same ID and identical fingerprint returns success;
reusing the ID with different data returns a conflict. Successful mutations
return the complete snapshot. The task event broadcaster publishes
`task.preview_feedback.changed` with the same snapshot. Clients replace local
state only with an equal or newer revision and refetch after reconnect or a
detected gap.

The gateway authorizes every action through task access. An accessible session
or preview URL does not grant access to another task's feedback.

## Responsive interaction

### Desktop

Browser and HTML-preview toolbars expose one **Annotate** control with the
pending count. Its menu offers Text, Element, and Screenshot region modes and
opens the pending panel. Choosing a mode closes the menu and puts the iframe in
that single active mode. A completed capture opens an anchored Kandev Popover
with capture summary, screenshot thumbnail when applicable, required comment,
Cancel, and Add.

The pending panel lists every page group in stable order and supports inspect,
comment edit, and delete. **Clear all** uses a destructive confirmation and the
rendered collection revision. There is no Send or Run control in the preview;
the normal task composer owns delivery.

### Phone and coarse pointers

The focused preview header exposes one 44-pixel **Annotate** action with a count
badge. It opens a bottom Drawer with mode choices and the pending list. Choosing
a mode closes the Drawer so the preview retains the available viewport. A
completed capture reopens the Drawer directly on the comment form. The Drawer
has one internal vertical scroll owner, safe-area padding, focus return, and no
horizontal page overflow.

While text or element capture is active, ordinary page interaction remains
available until a selection completes. Screenshot-region mode alone owns drag
and touch-move gestures inside the iframe; its visible mode bar offers Cancel.
Completing or cancelling the mode removes the gesture listeners and restores
ordinary page scrolling.

The shipped mobile HTML focused viewer supplies the page composition and the
existing task Drawer patterns supply containment and focus behavior. Shared
capture and mutation hooks own behavior; responsive wrappers own arrangement.

## Accessibility

- Capture-mode controls expose pressed or selected state and announce mode
  changes through visible status plus a polite live region.
- The element candidate label mirrors the visual outline in accessible status
  text, so keyboard and assistive-technology users receive the same preview of
  what activation will capture.
- Escape cancels an active mode or draft. Keyboard element mode can capture the
  focused element, and text mode accepts normal keyboard selection.
- Focus returns to the invoking control after cancel, save, or responsive
  surface closure. A pending item links its marker and list row with stable IDs.
- Screenshot thumbnails include localized alternative text based on source and
  comment. HTML snippets use code semantics and remain selectable as text.
- Counts, active modes, upload progress, and failures use text or accessible
  names in addition to icon, color, or outline changes.

## Composer projection

Every composer for a task derives one context item from the task snapshot. It
shows the current count and opens the preview-feedback panel. It has no local
remove control because removing a task-owned item from one composer would
misrepresent the shared pending collection.

At submit time, the composer freezes the visible IDs and versions:

```json
{
  "preview_feedback_refs": [
    { "id": "feedback-id", "version": 2 }
  ]
}
```

It passes the references alongside unexpanded typed content, plan-comment
references, entity references, and ordinary staged attachment IDs. It does not
prepend client-formatted feedback. An empty typed body is valid when either
preview-feedback or plan-comment references are present.

The submission identity includes destination, message content, feedback
versions, plan-comment versions, and attachment identities. An unchanged retry
reuses that identity; modifying any of those inputs creates a new identity.

## Canonical prompt

The backend loads referenced rows for the submitted task, preserves snapshot
order, and appends one visible `### Web Preview Feedback` section. Every item
contains its kind, source label, page route, user comment, and captured context.
Text is quoted and followed by its containing element, DOM range endpoints,
document-coordinate rectangles, scroll position, and viewport size. Element
HTML is in a bounded code fence and includes its rendered position. A screenshot
names its attached PNG and includes dimensions. A safe serializer chooses fence
lengths and escapes labels so captured page content cannot break the enclosing
shape.

Captured strings remain user-authored prompt content. They are never placed in
`<kandev-system>` markup or assigned higher instruction priority. The persisted
expanded prompt is the value shown in queue editing, the transcript, ACP, and
passthrough delivery.

## Atomic acceptance

Preview-bearing admission joins the task-scoped lease and final repository
transaction already used for plan comments and staged attachments. The shared
admission envelope supports plan-comment refs, preview-feedback refs, and
ordinary attachment IDs together rather than nesting independent transactions.

The final transaction performs these steps:

1. Lock or guard the active task, then the selected session, using the existing
   task-before-session order.
2. Load every preview-feedback row by task and require unique IDs with exact
   submitted versions. Load any plan comments through their existing resolver.
3. Revalidate the session route, queue capacity, feedback limits, final prompt
   limit, and the combined attachment count and byte limit.
4. Format the canonical visible prompt from persisted rows.
5. Transfer each screenshot from its task-feedback claim to the accepted
   message or queue entry, claim ordinary staged attachments, persist the
   message or queue receipt, conditionally delete the exact feedback and plan
   comment rows, and increment their collection revisions.
6. Commit, then publish the ordinary delivery event and authoritative changed
   snapshots.

If any conditional delete or attachment transfer loses a race, the entire
operation rolls back and returns a stable `preview_feedback_changed` or
attachment conflict. New or edited rows outside the submitted versions remain
pending. No compensating delete or restore is used as an atomicity mechanism.

The queue row and direct-message receipt retain reference fingerprints for
idempotent replay and use the existing pre-dispatch reservation and at-most-once
external-I/O boundary. Accepted queue cancellation or editing never resurrects
the consumed feedback because the queued prompt and attachments are then the
durable user message.

## Lifecycle

- Task deletion cascades pending rows and collection state and schedules all
  still-owned screenshot objects for deletion through attachment cleanup.
- Deleting one screenshot row or clearing the collection deletes its task-owned
  image after the database transaction commits. A failed object deletion stays
  discoverable by the existing attachment cleanup path.
- Session deletion, task reassignment, primary changes, panel closure, and
  executor restart do not mutate pending feedback.
- A stopped runtime can make source pages unavailable, but the stored comment,
  text, element snapshot, and screenshot remain reviewable and sendable.
- Accepted delivery removes the pending rows. The expanded prompt, message or
  queue metadata, and claimed message attachments become the durable record.
- Staged screenshots that never reach feedback creation expire through the
  existing 24-hour staged-attachment sweep.

## Failure and recovery

- An iframe that is not on an inspector-capable session proxy shows no capture
  control and does not accept bridge events. Existing browsing remains usable.
- A failed selection keeps the current mode active with localized guidance. A
  screenshot rasterization or upload failure keeps the local draft and permits
  retry or cancel.
- A failed create or comment update keeps the form, capture, and typed comment
  open. A failed delete keeps the row visible. Controls that would duplicate a
  pending mutation are disabled until settlement.
- Reconnect obtains a complete collection snapshot. Stale events cannot replace
  a newer revision.
- Direct or queue rejection preserves feedback and composer state. A changed
  snapshot refreshes the shared collection and requires the user to submit the
  reviewed current version rather than silently omitting stale items.
- If source navigation or DOM changes invalidate an anchor, the immutable
  capture stays in the pending list with an unavailable-marker state.

## Security

Previewed HTML already executes as trusted workspace code under
[ADR-2026-09-05](../../../decisions/2026-09-05-trusted-browser-html-preview.md).
This feature does not widen that execution claim. It adds these boundaries:

- Parent windows accept bridge events only from the mounted iframe window and
  validate a versioned closed schema and byte limits.
- The page cannot provide task ownership or attachment claims. The backend
  derives authorization from the authenticated request and verifies every
  referenced row and attachment.
- Page routes omit credentials, fragments, cookies, and capability tokens.
  Logs omit comments, selected text, HTML snippets, and screenshot bytes.
- Captured HTML renders only as escaped code. Captured page content and comments
  are user prompt data and cannot create system messages.
- Screenshot upload verifies the PNG signature, MIME, size, pixel bounds, owner,
  workspace, and task before claim. Content is served through the existing
  authorized attachment endpoint.
- Cross-task IDs, duplicated references, stale versions, and attachment reuse
  fail the entire mutation or delivery.

## Observability

Structured logs identify task ID, feedback ID, kind, source kind, mutation,
delivery mode, outcome, and stable error code. They never include captured or
comment content. Existing attachment-cleanup and message/queue diagnostics cover
image lifecycle and delivery receipts.

Initial correctness is exposed through snapshots, conflicts, and deterministic
repository/E2E coverage. No production metric is added until operating data
shows a need for rates such as capture failure or stale-send conflict.

## Verification

- Repository tests cover SQLite and PostgreSQL schema replay, task cascades,
  stable order, revision allocation, optimistic versions, idempotent create,
  screenshot claim transfer and cleanup, limits, and task authorization.
- Direct-message and queue tests cover canonical server formatting, mixed plan
  and preview feedback, empty typed content, exact-version consumption,
  attachment rollback, queue capacity, idempotent replay, and concurrent sends.
- Frontend tests cover bridge origin/schema validation, all capture payloads,
  screenshot crop and bounds, task snapshot reconciliation, Browser and native
  HTML eligibility, draft preservation, composer references, and localized
  responsive controls.
- Desktop Chromium E2E covers a multi-route local development page, all three
  captures, preserved prior-page feedback, explicit direct and queued Send, and
  real screenshot attachment delivery.
- Mobile Chrome E2E covers the HTML-file preview Drawer, touch region capture,
  44-pixel controls, one scroll owner, source return, and no horizontal overflow.
- Container E2E covers a port published by a Docker task session through the
  same Browser-panel annotation flow.

## Related decisions

- [Persist Web Preview Feedback with the Task](../../../decisions/2026-09-15-task-owned-web-preview-feedback.md)
- [Persist Pending Plan Comments with the Task Plan](../../../decisions/2026-09-02-task-owned-plan-comments.md)
- [Treat HTML Preview as Trusted Workspace Code](../../../decisions/2026-09-05-trusted-browser-html-preview.md)
- [Separate Message Queue Provenance, Cancellation, and Capacity](../../../decisions/2026-08-03-separate-message-queue-provenance-cancellation-and-capacity.md)

## Implementation plans

- [Interactive web preview feedback](../../../plans/web-preview-feedback/plan.md)
