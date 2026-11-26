# Threads deck

`/threads` renders stable conversation tiles in Columns or a two-row Grid.

`selectActiveThreads` (`lib/threads/active-threads.ts`) ranks tasks by attention, recency, and ID using board snapshots. `useStableThreadOrder` pins existing columns and appends new ones. Do not re-rank columns during a session; a reply must not move the column being edited.

The stable order lives in memory, so a reload ranks the deck again. The only
order that survives a reload is the `selection` sort: it ranks a
selected-scope view by the persisted `taskScope.taskIds` array, which the
picker appends to in click order. Descending selection sort reverses that order.
With the `all` scope it resolves to
ascending attention in `queryThreadView`, and the editor resets it in the same
draft update when the scope changes (`threadTaskScopeModeUpdate`).

A task reaches the deck through its **primary** session, matching the backend's
`GetPrimarySessionIDsByTaskIDs`. Once that task column is present, the Threads
switcher can select any existing sibling session, including a settled one.
Sessions created by the E2E seed harness are never primary, so a spec has to
run a real agent turn (create the task with an agent, open the task page to
launch the session, then wait) rather than seeding a RUNNING row.

Columns mount `TaskChatPanel` with **no `onSend`**. Its own submit path is what
honours the session's queue input mode, the selected model, plan mode, context
files and the optimistic store update; a custom sender posts straight to
`message.add` and drops all of it, so a reply to a busy thread jumps its queue.

They do pass `isVisible={false}`, for the same reason the kanban preview does: a
wall of columns is a glance across running work, and letting every mounted
column advance its own read cursor would mark threads read that nobody looked
at.

Columns share the board width (`flex-1` above a 360px minimum). Grid uses the
same direct task children in column-major order, two equal rows, and the same
width floor. One task fills the height; fewer than two 300px rows plus the 12px
gap temporarily selects Columns without changing the saved layout. Board
content size owns this fallback and reflow recovery. Both visible rows activate
selected conversations; stale observer callbacks cannot replace the current
window. The phone keeps one full-width snapping column and one active detail,
without overwriting the saved desktop layout.

`ThreadView` and `ThreadViewDraft` own `layout` and `autoHideComposer` through
the existing backend-owned user settings and view actions. Defaults are
Columns, auto-hide off, and five total chats; `maxColumns` still limits admitted
tasks across both rows. Presentation is excluded from `queryFingerprint`.
`ThreadsViewDisplay` is shared by the desktop editor and touch drawer. Layout
selection belongs only inside this configurator, not in the top bar;
do not add another persistence owner or autosave path. Render-only fallbacks
never write over the saved preference.

The phone title button opens one board-owned `MobileThreadPicker` using
admitted task summaries. Selection scrolls an existing shell; viewport
activation still owns transcript mounting. The picker restores focus without
scrolling back to the previous column. Phone headers prioritize task title,
status, and agent selection; workflow and step context remain in the picker.
Titles wrap to two lines. Inline topbar pagination shows position/count for
multiple threads and decorative dots only for decks of up to seven threads.
`ThreadsBoard.renderHeader` receives `mobileTaskId`, derived from board scroll
geometry by `useMobileThreadPosition`, and the board's grid-height fallback
reason for Display. The page derives the ordinal from stable
order without copying selection state. Do not derive pagination from loaded
chat or visibility-ID membership alone: both adjacent columns can stay
intersecting across a swipe midpoint. Position changes also refresh the nearest
visible detail calculation, retaining the one-phone-transcript limit.
Callback refs reconcile column additions/removals on the existing observer.
Do not rebuild it on every membership change: clearing surviving visibility
briefly unmounts readers' chats and loses editor focus. Recreate observation
for layout/width reflow or an empty/nonempty transition that replaces the board
element, measuring current geometry before asynchronous callbacks arrive.

`ThreadTaskActionsProvider` owns one task-action surface above removable
columns. Headers pass explicit task IDs; `useTaskManagementFlow` captures the
workspace/task identity and resolves current eligibility from shared snapshots.
`TaskManagementSurface` composes existing task menus, linking and confirmations;
`useTaskMenuActions({ stayOnListing: true })` shares destructive lifecycle
cleanup with the sidebar without task-detail navigation. Do not add task API
calls or mutation policy to Threads.

The page excludes task IDs from pending `archive` operations in
`taskRemoval.operationsByToken` before `queryThreadView` applies scope, filters,
limits or temporary deep-link admission. This unmounts outgoing chats at
acceptance and keeps counts, the phone picker and pagination consistent. Do not
mutate shared snapshots optimistically or include removal intent in the query
fingerprint/stable-order reset key. Successful reconciliation prunes snapshots
before releasing intent; failure readmits only currently eligible tasks in
normal arrival order. Pending deletion retains its existing timing.

Only noninteractive desktop task-header regions handle context menus. The
visible `TaskMenuButton` is also the keyboard/touch entry; phone and coarse
pointers use one `TaskManagementDrawer` with nested pages. Keep chat, editor,
session and native swipe events outside this boundary.

`useThreadSelectionRecovery` preserves the surviving reader's column offset
and uses `resolveRemainingThreadId` for successor/predecessor/first-new/empty recovery
when membership changes. Layout/width changes retain that identity too;
resize-generated scroll or same-membership snapshots must not overwrite it
before reflow recovery. This does not replace stable ordering, the parent's
scroll-derived pagination, or transcript activation. Action focus restoration
resolves a currently visible trigger and never scrolls to a removed opener.

Pass `?workspace=` to `useKanbanRouteBootstrap` so cross-workspace links select the right workspace. Use `listingHistoryHref` for shared-header scope changes to retain the Threads route.

## Composer disclosure

`ThreadColumn` owns a selected-session-scoped `useComposerDisclosure` controller
and `ComposerDisclosureContext`. Only fine-pointer desktop layouts apply
auto-hide. Phone/coarse-pointer layouts retain the normal composer and stored
preference. Tile hover and keyboard focus reveal without autofocus; explicit
Collapse cancels the current hover dwell and returns focus without reopening.

Native owners report drafts, attachments, pending operations, required actions,
and owned overlays through `useComposerActivity`. `useComposerFocus` reveals
before native or plugin focus requests. Keep these reports scoped to the
selected session and clean up holds/timers on deactivation. React-owned portal
focus belongs to the tile even when its DOM is outside the tile.

`ComposerDisclosureRegion` wraps the entire non-CI `ChatInputArea` content in
one interruptible grid-track/opacity transition (200ms in, 160ms out). Inert
and `aria-hidden` change immediately; visual hiding waits for exit completion.
Reduced motion disables transitions. Keep editor and plugin instances mounted,
with no nested disclosure wrappers or separate animation timers. The existing
provider-specific `ComposerCIStatus` sits outside the animated region while
auto-hide is effective; `ChatStatusBar` retains inline CI for other hosts.
An empty CI row occupies no height. No Reply, Stop, plugin, or queue strip remains when
collapsed. Opaque plugin activity does not hold the composer open; hiding must
not cancel operations or revoke plugin capabilities. Required native actions
and recovery force the regular surface open. Cancellation stays in the native
composer, including its pending-state hold.

`ComposerFooterAllocation` bounds the entire Threads footer, including required
questions and when auto-hide is off, leaving an 80px transcript floor. Long
footer content scrolls inside that allocation. The native transcript owner in
`message-list-native-scroll.ts` uses `transcript-viewport-resize.ts` to preserve
bottom-follow across height-only allocation changes without overriding history,
frozen scroll, or width reflow. Do not add a competing tile scroll loop or pin
offscreen details to retain a composer; session draft restoration owns return.

## Round trip with the task page

`linkToThreads(workspaceId, taskId, sessionId)` builds a `/threads` URL with
the supplied workspace and task IDs. It includes a session ID only when a task
ID is also supplied. The link asks the deck to scroll that
column into view and ring it. The focus id is resolved against the rendered
deck (`resolveFocusedThreadId`), never trusted from the URL: the thread may
have settled between the link being offered and followed.

The scroll effect is keyed on `isFocused` and the board's layout/width key;
the column is keyed by task id. Initial measurement or resizing can interrupt
smooth scroll, so the still-marked target is reasserted after reflow. Ordinary
message updates do not scroll the deck.

The mark retires on the first pointer, wheel, or focus interaction with the deck,
since it only ever answered "where is the column I asked for". Dismissal is keyed to
the raw workspace/task/session request identity, independently of its currently
resolved column. Temporary exclusion and failed archive readmission must not
revive a consumed mark; an actual new deep link still earns a fresh mark.
`useThreadFocusRequest` keeps the initial activation fallback alive after mark
dismissal until its target leaves the deck. Early interaction must not unmount
that chat before the first visibility callback, and failed readmission must
not restore a consumed fallback. URL-driven callers pass `focusRequestKey`;
the optional task-ID default deliberately retains legacy dismissal semantics.

`OpenInThreadsButton` is the other half, living in the chat status row. It has
two gates, and both matter:

- `useIsDeckThread` — the session must belong to a task with a Threads column,
  which can come from a live primary or a task-level review outcome. It reads
  already-loaded sessions and the cached task summary/state and must not fetch;
  a status-row control triggering a request would be a surprise. A settled
  sibling is therefore eligible when that task keeps a column in the deck.
- pathname — the deck's own columns render the same chat panel, so without this
  the button would appear inside every column offering a jump to the view
  already on screen.

## Adding a task-listing view

Task-listing views share `lib/task-listing/view-preference.ts`. Add navigation to `resolveTaskListingNavigation` and toggle items to the desktop and phone surfaces. For a routed view, update `ROUTED_TASK_LISTING_VIEWS` and `lib/navigation/core-destinations.ts`. Preserve the published plugin top-bar page contract.
