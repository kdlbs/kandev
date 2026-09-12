---
status: current
system: ui
requirements:
  - REQ-UI-THREADS-DECK-001
  - REQ-UI-THREADS-DECK-002
  - REQ-UI-THREADS-DECK-003
  - REQ-UI-THREADS-DECK-004
  - REQ-UI-THREADS-DECK-005
---

# Threads Conversation Deck System Design

## Purpose and boundaries

This design adds local multi-session selection and viewport-owned conversation
activation to `/threads`. Task snapshots remain the task-column discovery
source. Task-session membership remains backend-owned. The frontend does not
copy session selection into global task-page state and does not make Threads a
session-management surface.

[Threads Saved Views](threads-saved-views.md) owns the task query, admitted
task set, saved order, optional chat limit, and persistence of presentation
preferences. This design applies after that query admits a task shell.

The paired platform design owns network hydration, compact status events, and
full session subscription limits. This UI design owns the task-column shell,
the desktop tab row, the mobile picker, selection rules, status presentation,
and deep-link behavior.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-THREADS-DECK-001` | [Session membership and selection](#session-membership-and-selection), [Header controls](#header-controls), [Deep links](#deep-links) |
| `REQ-UI-THREADS-DECK-002` | [Status projection](#status-projection) |
| `REQ-UI-THREADS-DECK-003` | [Viewport activation](#viewport-activation), [Responsive behavior](#responsive-behavior), [Failure and recovery](#failure-and-recovery) |
| `REQ-UI-THREADS-DECK-004` | [Layout composition](#layout-composition), [Layout transitions](#layout-transitions) |
| `REQ-UI-THREADS-DECK-005` | [Composer disclosure](#composer-disclosure), [Composer geometry](#composer-geometry) |

## Components and responsibilities

- `ThreadsPageClient` keeps workspace scope, derives task-column
  summaries, resolves `taskId` and `sessionId` from the URL, and owns routing
  back to the full task page.
- `selectActiveThreads` remains a bounded task-level selector. It returns the
  task ID, title, workflow and step metadata, task review state, summary
  attention state, activity, and primary session identity. It does not return
  a transcript or an unbounded session array.
- `ThreadsBoard` keeps every lightweight task-column shell in stable order and
  observes its horizontal viewport.
- `useThreadColumnActivation` produces two sets. The preload set
  contains visible columns plus at most one adjacent column on each side. The
  detail set contains only intersecting desktop columns. On phone it contains
  only the nearest snapped column. The same viewport owner exposes a separate
  lightweight `mobileTaskId` for pagination and picker context; that identity
  does not depend on membership hydration or detail mounting.
- `ThreadColumn` always renders the task shell. It mounts the session-list
  adapter only while preloaded and mounts `ThreadConversation` only while
  detail-active.
- `ThreadColumn` owns one local selected session ID. `selectThreadSessionId`
  applies deterministic initial and removal fallback rules without using
  `tasks.activeSessionId`.
- `ThreadSessionSwitcher` renders compact desktop tabs or a mobile picker from
  the same session view model.
- `ThreadConversation` mounts `TaskChatPanel` only for the selected session and
  keeps its standard message submission path by omitting a custom `onSend`.

## Session membership and selection

`useTaskSessions(taskId)` is mounted only for a column in the preload set. The
hook supplies the current compact `TaskSession` rows and uses its current list
and reconnect behavior. The store can retain loaded rows after a column leaves
the preload set, but no offscreen hook performs a list request or foreground
refresh.

Selection is column-local and keyed by task ID. A status change never changes a
valid selection. The initial or replacement selection uses this order:

1. The URL `sessionId`, if it belongs to the URL `taskId`.
2. The existing column-local selection, if it remains a member.
3. A session with a pending action, with permission before clarification.
4. A `STARTING` or `RUNNING` session, ordered by the existing session sorter.
5. The task's primary session.
6. The newest remaining session from the existing session sorter.

If no session exists, the column shows its local empty state. A newly created
session can enter the loaded task-session collection through the existing
global lifecycle event and invalidation path. A preloaded column then refreshes
membership without loading any transcript for the new session.

## Header controls

The first header row keeps the task attention icon, title, and Open task
button. The second row is one flex row:

```text
+------------------------------------------------------------------+
| ?  Task title                                         [Open task] |
| Ready for review  Development  Review   [1 Mock] [2 Mock running] |
+------------------------------------------------------------------+
```

On desktop, metadata owns the left flexible region. The switch-only tab list
owns a bounded right region and can scroll horizontally inside that region.
The tab list does not wrap onto a new header row. Tabs expose standard tab-list
and selected-tab semantics. They do not expose add, close, context-menu,
double-click, drag, or management callbacks.

The implementation can reuse `SessionTabs`, session sort helpers, agent labels,
`AgentLogo`, and `GridSpinner`. It must not reuse
`PreviewSessionBody`, because that component supplies a custom sender instead
of the standard `TaskChatPanel` message path.

## Status projection

The session selector uses agent identity as its leading presentation. It uses
the effective agent profile name as its label. If profile data is not available,
it uses the custom session name or the existing fallback label. A settled
session shows `AgentLogo`. A `STARTING` or `RUNNING` session shows `GridSpinner`
in place of that logo.

One pure helper creates the task-column status from compact `TaskSession`
fields. The column presentation uses this precedence:

1. `pending_action === "permission"`: permission indicator and `Permission needed`.
2. `pending_action === "clarification"`: question indicator and `Question from agent`.
3. `state === "STARTING"`: spinner and `Starting`.
4. `state === "RUNNING"` or foreground activity is present: spinner and `Working`.
5. Terminal failure or cancellation: the existing terminal status treatment.
6. Plain `WAITING_FOR_INPUT`, `IDLE`, or `COMPLETED`: no question indicator.

Task-column status uses explicit task-wide pending action only to say that some
session needs attention. It never assigns that action to the primary session.
When the task is at its review workflow outcome and no explicit action exists,
the column uses a completion indicator and `Ready for review`.

All new text uses the `threads` i18n namespace in every supported catalog. The
question and permission variants use different accessible names as well as
different icons.

## Viewport activation

The board uses `IntersectionObserver` with the board as its root. Each stable
task shell registers by task ID. The observer and current horizontal order
derive:

- **preload:** all intersecting desktop shells and at most one adjacent shell
  on each side;
- **desktop detail:** all intersecting shells;
- **phone detail:** the nearest snap target only.

Preload mounts only the compact session-list adapter. Detail activation mounts
only the selected `ThreadConversation`. Leaving detail activation unmounts the
chat panel, which releases its message hook and session subscription. The
column-local selection and task shell remain.

Keeping every lightweight shell mounted preserves `useStableThreadOrder`, CSS
scroll geometry, browser find and accessibility order, and deep-link scrolling.
It also avoids a variable-width horizontal virtualizer. With 30 columns, the
DOM contains 30 small shells, but network, transcript state, rich chat trees,
and session subscription counts follow the current viewport.

If `IntersectionObserver` is unavailable, the board activates the focused URL
column or first column. Pointer, focus, and scroll interactions can promote a
new column. The fallback must not activate every conversation.

### Phone position feedback

`useThreadColumnActivation` also derives the phone's presentation position from
the existing board and registered shell geometry. A passive board scroll
listener schedules at most one `requestAnimationFrame` measurement. It chooses
the shell nearest the board center and updates `mobileTaskId` only when that
identity changes. It measures registered shells in stable order, not the
IntersectionObserver's asynchronously delivered membership set.

Both adjacent shells can remain intersecting as a swipe crosses their
midpoint. Equal intersecting-ID sets therefore cannot suppress phone position
updates. Position must not be read from `detailTaskIds`, a loaded transcript,
the focused URL marker, or a completion callback.

Initial measurement, board resize, and admitted-order changes reconcile the
same position. Before usable geometry exists, the focused admitted task or
first admitted task is the fallback; empty decks yield null. Native swipe,
picker scrolling, and deep-link scrolling all use this one path. There is no
touch interception, scroll-end debounce, second shell registry, stored
preference, or network request. Leaving phone mode or unmounting removes
listeners/resize observation and cancels any pending frame. Leaving phone mode
also clears its measured identity, so returning uses the current fallback
until the new phone geometry is available.

The indicator and thread picker's selected row consume `mobileTaskId` directly
through the board. Their update never enlarges the detail/preload window or
mounts a second phone transcript. Existing viewport-bounded loading and
column-local session selections keep their separate responsibilities.
Position changes also invalidate the nearest-visible detail calculation and
provide its conservative phone fallback. An edge-adjacent column can remain
intersecting at zero area after snapping; unchanged visibility membership must
not leave the previous column detail-active. The detail set remains bounded to
one visible phone column and is never gated by transcript loading completion.

## Deep links

`linkToThreads` accepts an optional `sessionId` and produces a URL with
`taskId`, `sessionId`, and the existing optional workspace scope. The page
first validates the task ID against the stable task shells. It validates the
session ID after the target column loads its session list.

A valid session request sets the target column's initial selection. An invalid
or removed session request falls back locally and can remove only the invalid
`sessionId` parameter. It does not redirect away from Threads or select a
session in another task.

## Responsive behavior

Desktop keeps direct compact tabs in the metadata row. Fine-pointer tab targets
can use the existing compact `SessionTabs` height because they are not the
phone interaction.

Phone keeps horizontal swipe for task columns and does not add a second
horizontal gesture region. The metadata row shows a compact
`MobilePillButton`, for example `Agent 2 of 3`, on the right. It opens a
`MobilePickerSheet` with one 44-pixel-or-larger row per session. Rows contain
the label, selected state, and the same agent identity or grid spinner used by
desktop tabs. The sheet owns vertical scrolling and safe-area padding.

Phone columns use the board's full content width, without an inset outer card
or a neighboring-card peek. The board retains horizontal snap ownership; each
transcript owns vertical scrolling. Flex ancestors allow width shrinking, and
the composer stays within the dynamic-height app shell and its safe-area rules.
Code and tables retain their local horizontal scrolling.

The phone column header uses a two-line title button, an Open task action, and
a second row for attention and the existing session picker. Position/count
appears inline beside the view control in the page topbar, with no instruction
text or extra task-header row. Decks of up to seven threads also show decorative
page dots; larger decks retain the numeric position without an overflowing dot
row. Empty and single-thread decks omit the indicator. The board's header
render slot receives `mobileTaskId` from phone position feedback, so the page
derives the ordinal from stable order during the swipe. The page does not copy
that identity into state or synchronize it through a loading callback.
The title opens one board-owned `MobileThreadPicker`, using the inset
`MobilePickerSheet` pattern. It lists the admitted task summaries in stable
order, including workflow, step, and attention. Selecting a task scrolls its
existing shell into view; viewport activation still owns transcript mounting.
This reuses the focused-column navigation pattern from `MobileColumnTabs` and
does not duplicate session selection or store state.

`KanbanHeaderMobile` uses the shared
[Mobile Workspace Topbar](mobile-quick-chat-topbar.md) composition. Threads
supplies its page identity and selected saved view in the unboxed stacked title
control, with pagination alongside it. The saved-view editor continues to use
its existing drawer; listing navigation and secondary workspace actions use
the shared menu. Saved-view semantics are not introduced into Kanban or List.

Desktop metadata and tabs keep their existing regions. Phone pickers own their
internal vertical scrolling, clear safe areas, and return focus on dismissal.
If the opening task disappears while its picker is open, focus returns to the
current mounted phone thread or the first remaining column, without scrolling.
Leaving phone layout closes the thread picker; returning does not reopen it.
Long labels truncate inside controls without document-level horizontal overflow.

## Layout composition

The `ThreadView` and matching draft gain `layout: "columns" | "grid"` and
`autoHideComposer: boolean`, through the saved-view persistence design.
`queryThreadView(...).effectiveView` is the page's single presentation source.
Pass its fields to `ThreadsBoard`; do not derive another active view inside
individual tiles. Layout and disclosure do not enter `queryFingerprint`.

Keep each `ThreadColumn` as a direct board child keyed by task ID. Its existing
`data-thread-column-id` remains the registry, selection-recovery, context-menu,
and deep-link boundary. Do not introduce per-pair wrappers or change React
keys when layout changes. Use ordinary labelled sections, not ARIA grid roles
that would impose spreadsheet keyboard behavior on a conversation.

Columns retains the existing flex arrangement. Grid uses CSS grid with
column flow, two `minmax(0, 1fr)` row tracks, and
`ceil(admittedTaskCount / 2)` equal flexible column tracks with the existing
360px minimum. The board owns horizontal overflow and keeps its existing
12px desktop gap and padding. Each tile owns its transcript's vertical scroll.
The sequence A, B, C, D produces A/C on the upper row and B/D on the lower row;
DOM, keyboard, picker, and screen-reader sequence remains A, B, C, D.

For one task, the effective row count is one. With an odd number greater than
one, the last lower cell stays empty; the last task is not duplicated or
stretched across rows. With no tasks, reuse the existing empty/loading state
and keep view controls available. `maxColumns` still counts admitted tasks;
it never becomes a grid-column count.

Use `useResponsiveBreakpoint().isMobile` for the phone boundary. Phones render
the existing snap deck and one detail-active conversation regardless of the
saved layout. Tablets and wider touch displays may render Grid but retain
normal visible composers. Width and pointer precision are separate inputs.

A small `thread-layout.ts` helper derives effective layout, row count,
and track count from saved layout, phone mode, task count, and available board
content height. A board-owned `ResizeObserver` measures content height after
padding; two rows require at least `2 * 300 + 12` CSS pixels. Below that bound,
render Columns. Keep Grid selected as the stored choice, and show a localized
reason in its layout control/editor. This is render-only fallback: hydration,
resize, keyboard appearance, and breakpoint changes never save preferences.
Before height is measurable, use Columns as a conservative render placeholder.

### Layout transitions

Extend `useThreadSelectionRecovery` with the effective layout identity so its
existing before/after geometry path also handles composition changes. Retain
the last interacted surviving task; if it is no longer a usable anchor, use
the current visible task, then the first admitted task. Preserve its horizontal
offset where the new scroll range permits, otherwise scroll only enough to
reveal it. Keep input focus when the same editor remains mounted. The task
ID, local selected session, and session draft are never keyed by row/column.

The board's `IntersectionObserver` still observes individual task tiles, so
both grid rows become detail-active. Preserve the existing preload limit of
visible tasks plus at most one neighboring task ID on each side of the stable
sequence; do not preload an entire extra grid column. Layout changes must
refresh geometry even if admitted IDs are identical. Update activation with an
effective-layout generation and reject callbacks from a disconnected observer
or replaced shell. Publish measured shell intersections synchronously when
rebuilding observation, retaining still-visible chats while dropping offscreen
ones before asynchronous delivery. Reconcile without briefly activating every
task, and preserve the one-phone-detail rule throughout responsive transitions.
Callback refs reconcile membership on the existing observer; only reflow or
board replacement rebuilds it, so removing a tile preserves surviving readers.
Keep the short-height explanation in Display, outside the measured board
allocation, so crossing the Grid threshold is independent of resize history.
Pointer, focus, and wheel input all record the interacted task for recovery.
Deep-link scrolling is instant when reduced motion is requested.
Recovery skips zero-offset scroll assignments so it does not cancel an
in-flight browser deep-link scroll when the first size measurement arrives.
Deep-link retirement uses the raw workspace/task/session request identity.
`useThreadFocusRequest` retains the initial activation fallback after reader
interaction until the target leaves the deck; failed archive readmission must
not revive a consumed mark or fallback.

The selected task remains the owner for context menus and removal recovery in
the lower row. `scrollIntoView` keeps `block: "nearest"`, and board scroll
recovery remains horizontal. Open task still routes through the page's existing
`handleOpenTask` callback. Hover does not call any focus/read/session API, and
`ThreadConversation` retains `embedded` and `isVisible={false}`.

## Composer disclosure

Use an optional Threads-only disclosure context, scoped to the task tile and
selected session, with a null default for other chat hosts. The
`use-composer-disclosure.ts` owns timing and interaction state; its provider
adds no DOM wrapper between the board and the stable task section. Tile
pointer/focus events feed this controller, while the selected chat reports
draft, operation, and required-action state from their existing owners.

Keep the input and plugin trees mounted while detail-active. Collapse one
routine region with a CSS grid-track transition, `aria-hidden`, and inert
interaction; do not duplicate `ChatInputArea`. The only persistent footer
content is the existing CI status/popover family in `ComposerCIStatus`: `PRStatusChip`,
`MRStatusChip`, `AzureDevOpsTaskPullRequestChip`, and
`RegisteredChangeRequestStatus`. Keep these instances in place so disclosure
does not remount a CI popover or add a background-sync consumer. An empty CI
row has no padding or height. No compact Reply/Stop/plugin row is added.

When auto-hide is effective, `ChatInputArea` places that one CI family outside
`ComposerDisclosureRegion`; `ChatStatusBar` omits its usual inline CI family.
This branch depends on the effective preference, not transient expansion, so
hover never remounts CI. Hints, routine status, queue, editor, and toolbar share
one animated region, avoiding independently snapping margins or padding.

All other status controls, queue affordances, context controls, editor, and
toolbar collapse together. Stop remains in the normal composer; reuse its
existing eligibility and cancellation-pending behavior without another sender
or cancel action. Existing task/status header cues remain. Pending
clarification, selected-session permission, or actionable recovery forces the
normal input area open, keeping `DynamicRouteRecovery`, recovery banners, and
clarification content in their current owners. Do not create a second action
renderer from task summary data. Do not assign another sibling's task-wide
pending action to this composer.

### Disclosure state and event ownership

| State/input | Effect |
| --- | --- |
| Auto-hide off, phone, or coarse pointer | Normal input area; no hover timers; preserve the saved preference. |
| Fine-pointer dwell | Open after 150ms; leaving before then cancels. Never focus the editor. |
| Pointer leaves | Close after 300ms only if no interaction, draft, busy operation, or required action holds it open. |
| Tile receives keyboard focus | Reveal without stealing focus; normal Tab navigation reaches its composer. |
| Explicit editor focus | Reveal before `ChatInputContainerHandle.focusInput` or the existing plugin composer focus operation. |
| Editor/owned overlay focus | Hold open, including React-owned portals outside the tile's DOM bounds. |
| Unsent text/attachments or host-owned upload/send | Hold open for automatic hiding. An existing stored draft also opens on detail activation. |
| Required clarification/permission/recovery | Force open without introducing a new autofocus or agent action. |
| Explicit Collapse | Available inside the revealed composer; retain draft and focus the task section with `preventScroll`. Suppress that focus event's immediate reopen and cancel the current pointer-entry dwell. A new pointer entry can reveal again. Block for a host-owned operation or required action. |
| Session change/detail deactivation | Cancel timers and overlay holds; release the normal chat subscription and retain only the existing session-keyed draft. |

Explicit collapse suppresses draft-driven reopening until a new fine-pointer
entry, explicit keyboard reveal, or required action. It never clears a draft.
The task section is a keyboard focus target only when auto-hide is effective;
Enter on that section can reopen an explicitly collapsed composer. Child
controls keep their own Enter behavior. Phone and coarse-pointer composers
stay visible because the user requested no persistent Reply control. Use the
existing breakpoint/pointer model and ignore touch compatibility hover events.

`ChatInputContainer` already receives `value`, attachments, upload state, and
input focus from `useChatInputContainer`/`useChatInputState`. Expose a narrow
optional `useComposerActivity` report through that context from the native
owner; do not infer drafts by
polling `getValue()`, duplicating draft storage, or querying the DOM. Use
React focus capture across owned portals and existing controlled open-change
callbacks where a host overlay deliberately does not move focus. Any local
hold aggregation remains session-scoped, never a global overlay registry.
Release holds on close, failure, session change, and unmount. Parent `:hover`
or DOM `contains()` alone cannot represent portal ownership.

Plugin buttons hide with the composer, with no visibility exception. Their
components and existing `PluginComposerCapability` remain live while hidden;
do not infer recording state, cancel an operation, or revoke a capability on
collapse. This implements the user's CI-only direction without a new public
plugin activity API or an external voice-plugin change. Host-observable focus
and drafts still keep the composer open; opaque plugin activity alone is not
a keep-open condition. The existing session-change/unmount revocation remains.

Collapsed editors must not autofocus from mount/recovery effects. Reveal
before an existing explicit focus request is fulfilled. Keyboard focus and
the focusable tile make the feature usable without pointer hover. Escape belongs to
an open child menu/dialog first; collapsing the composer must never dismiss
the parent task surface. Auto-hide and explicit collapse cannot call submit,
cancel, clear, start, or stop as incidental side effects.

### Composer geometry

Composer disclosure changes only the transcript/input allocation inside the
fixed task tile. It must reclaim actual height; opacity-only hiding leaves the
space occupied, and overlaying the editor covers the latest message. Use the
existing grid-track disclosure pattern: `0fr` to `1fr`, a zero-minimum clipped
inner wrapper, and a 6px vertical offset with opacity on the routine content.
CSS transitions take 200ms to reveal and 160ms to collapse with eased motion;
they retarget from the current position on interruption. Do not animate CI,
mount/unmount the editor, introduce animation timers, or use `transition: all`.
Remove inert immediately on reveal, apply it immediately on collapse, and defer
visual `visibility: hidden` until exit finishes. `prefers-reduced-motion`
disables transitions and the visibility delay. Outside effective auto-hide,
wrappers use normal layout without motion. Initial render does not animate.

Use the existing native transcript scroll owner in
`message-list-native-scroll.ts`, `transcript-auto-scroll.ts`, and
`clamped-scroll-restore.ts`. Its `transcript-viewport-resize.ts` helper observes
height-only changes for a visible, initially placed, auto-following transcript.
It retains the pre-resize bottom state even when native clamping emits a scroll
event before resize delivery. Width reflow, history, frozen scroll, and an
active programmatic scroll lock keep their existing owners.
On footer allocation changes, preserve its current
follow/reading policy. A following viewport stays at the bottom; a history
reader retains the first visible message and offset, clamped to the new scroll
range. Add a narrowly scoped resize/anchor seam there only if the existing
resize path does not preserve the required position. Do not add a competing
scroll loop to `ThreadColumn` or change the stored auto-scroll preference.

The footer's expanded maximum height is the tile body height minus an 80px
transcript floor. Long expanded content scrolls within that bounded footer;
the input and its actions stay reachable. Apply this bound to Threads even
when auto-hide is off. The editor's ordinary
internal text scroll and existing portal menus continue to work. Both regions
use `min-h-0`; no content can increase the grid row height. Pending question
content can scroll inside the bounded footer without clipping its response
actions. Phones use the existing dynamic-height app shell and keyboard/safe-
area handling, with 44px touch actions; the composer is inline, not a second
chat drawer. Readable transcript and reachable actions take precedence over
animation.

Session draft text, rich content, and staged attachments continue to use
`useChatInputState` and `getChatDraftText`/`getChatDraftContent`/
`getChatDraftAttachments`. They remain transient session drafts, distinct from
backend-owned view preferences. When offscreen detail unmounts, existing draft
restoration owns recovery; an expanded composer never pins an offscreen stream.
Queue transitions must preserve the mounted input and focus. Keep the keyed
input as `QueueAffordance`'s single child, with the collapse action outside that
wrapper, so adding or draining queued messages cannot remount the editor.

## Failure and recovery

- Before membership loads, a preloaded shell shows a local session-loading
  state. Other columns remain navigable.
- A list request failure shows an in-column retry control. It does not mount a
  primary-session transcript as a fallback.
- Reconnect refreshes only currently preloaded task session lists. Compact
  status revisions reject stale pending-action replacements.
- If the selected session disappears, local fallback runs once against the
  newest membership and never moves the task column.
- If transcript hydration fails, the selected tab and other lightweight tab
  identities remain available while the chat shows its existing retry state.

## Verification design

Pure tests cover status precedence and deterministic selection. Component
tests install a controllable `IntersectionObserver`, render 30 task shells,
and assert membership/chat mount counts as columns enter and leave the viewport.
They also prove that only selected sessions mount `ThreadConversation`.

Desktop Playwright covers same-row tabs, switch-only controls, exact
conversation switching, agent identity, running spinners, deep links, and
horizontal scroll activation. Phone Playwright covers one active snapped
conversation, the picker sheet, 44-pixel rows, safe-area containment, and zero
document overflow.

Phone position tests cross the midpoint while the same two shells remain
intersecting, reverse that geometry, resize, remove the current task, and
unmount with a scheduled frame. Browser coverage holds a real touch drag past
the midpoint before touch release and verifies the topbar's new position while
the destination's detail loading is delayed. It must not wait for the new chat
to mount before asserting position. Settled swipes and picker navigation still
prove the one-phone-transcript budget.

## Related decisions

- [Viewport Activation Owns Threads Session Streams](../../../decisions/2026-08-28-viewport-activation-owns-thread-streams.md)
- [Separate Task Summary and Session Stream Traffic](../../../decisions/2026-08-01-separate-task-summary-session-stream-traffic.md)

## Implementation plans

- [Threads layouts and composer disclosure](../../../plans/threads-layouts/plan.md)
  implements the draft layout/disclosure additions. Existing deck/session and
  mobile behavior remains covered by the completed
  [windowing](../../../plans/threads-multi-session-windowing/plan.md) and
  [mobile](../../../plans/mobile-threads/plan.md) packages.
