# Threads deck

`/threads` renders one column per live agent conversation, TweetDeck-style.

`selectActiveThreads` (`lib/threads/active-threads.ts`) derives the columns from
the workflow snapshots the board already keeps in the store, so opening the view
costs no extra request and stays live on the same WebSocket updates the cards
use. Its ordering is total (attention bucket, then recency, then task id).

**That ranking decides where a column first appears and nothing after that.**
`useStableThreadOrder` (`lib/threads/stable-order.ts`) pins each column to its
slot, and new threads append rather than sorting in. Do not re-apply the ranking
live: replying to a thread refreshes its activity and flips it out of the
"needs a human" bucket, so a live re-rank slides the column the reader is typing
into across the deck, then slides it back when the turn ends. The E2E
`holds a column's slot and the deck's scroll while you reply to it` fails if this
is undone.

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
(`PreviewSessionBody` in the kanban preview still passes one and has the same
defect.)

They do pass `isVisible={false}`, for the same reason the kanban preview does: a
wall of columns is a glance across running work, and letting every mounted
column advance its own read cursor would mark threads read that nobody looked
at.

Columns share the board width (`flex-1` above a min-width floor) rather than
taking a fixed slice, so two threads fill the deck and a busy deck scrolls
horizontally instead of shrinking into slivers. The phone keeps one full-width
snapping column, which is a deliberately different layout.

The page reads `?workspace=` into the route it hands `useKanbanRouteBootstrap`.
Without it a cross-workspace link loads whichever workspace the cookie last
named. Scope changes from the shared header go through `listingHistoryHref`,
which keeps the deck's own path: those handlers `pushState` without routing, so
a task-overview href would leave the deck rendered under a Home URL.

## Round trip with the task page

`linkToThreads(workspaceId, taskId, sessionId)` produces `/threads?taskId=…` and,
when supplied, the target session id. The link asks the deck to scroll that
column into view and ring it. The focus id is resolved against the rendered
deck (`resolveFocusedThreadId`), never trusted from the URL: the thread may
have settled between the link being offered and followed.

The scroll effect is keyed on `isFocused` and the column is keyed by task id, so
a column that only mounts once a later snapshot lands still scrolls, while a
column re-rendering with new messages does not yank the deck back.

The mark retires on the first pointer or focus interaction with the deck, since
it only ever answered "where is the column I asked for". Dismissal is keyed to
the requested id, so a later deep link earns a fresh mark rather than being
swallowed by an earlier dismissal.

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

`kanban`, `pipeline`, `list` and `threads` share one device-local preference in
`lib/task-listing/view-preference.ts`. `resolveTaskListingNavigation`
(`lib/task-listing/view-navigation.ts`) is the single mapping from a toggle pick
to the view to remember and the page to go to; the desktop topbar
(`components/kanban/kanban-header.tsx`) and the phone menu sheet
(`hooks/use-mobile-menu-sheet-state.ts`) both call it, so a new view means one
entry there plus a toggle item on each surface.

A view that owns its own route also needs an entry in
`ROUTED_TASK_LISTING_VIEWS`, so Home hands off to it instead of rendering the
board, and one in `lib/navigation/core-destinations.ts` — a guardrail test fails
when a top-level route has no navigation destination. `currentPage` is
`TaskListingPage` internally; `toPluginTopBarPage` narrows it to the two values
the plugin contract has always published.
