---
status: shipped
created: 2026-07-27
owner: cfl
---

# Office: Slack-Style Unread Divider

## Why

Navigating back into a task session that kept running in the background
previously dropped the reader at the bottom of the transcript with no
indication of what was new. A "New" divider can identify that visit-start
boundary, but must not remain after the reader's current, visible transcript
has been acknowledged. This spec documents the persisted cursor this feature
is built on, since it's new product state whose scope (global vs. per-user)
and edge-case behavior (first visit and initial scroll) aren't otherwise
obvious from the UI alone.

## What

- Each user can enable or disable the divider from **Settings > General > Task
  Actions > Unread Messages**. It defaults **on** and takes effect immediately:
  when disabled, the frontend does not compute or render a divider, the message
  list performs its normal scroll behavior, and `useSessionReadTracking` does
  not dispatch `POST /task-sessions/:id/mark-read`; existing
  `last_read_message_id` values remain stored for a future re-enable.
- Each `task_sessions` row carries one `last_read_message_id` column — the
  ID of the newest message the session has "seen." This is **session-global,
  not per-user or per-device**: kandev tasks do not currently model multiple
  independent viewers of the same session, so there is exactly one read
  cursor per session, shared by whoever opens it. If a second viewer (or the
  same user on a second device/tab) opens the same session, they observe and
  advance the same cursor as the first.
- The cursor advances **live** while the session is the visible chat panel:
  as new messages render, `useSessionReadTracking` dispatches
  `POST /task-sessions/:id/mark-read` with the newest currently-rendered
  message id, and the backend persists it via
  `UpdateTaskSessionLastReadMessageID` (monotonic — see below).
- The divider is a **pending visit-start marker**, not a frozen marker for
  the whole visit. On a transition from hidden/inactive to the active,
  visible chat panel, the hook captures the persisted cursor before this
  visit's live advance can touch it. It may render before the first unread
  message only until the visible transcript has been acknowledged.
- When the `mark-read` request for the newest visible message succeeds, the
  hook clears the captured marker immediately. New messages arriving while
  the chat panel remains visible are acknowledged live and never create or
  move a divider. Leaving and returning (or switching sessions and back)
  captures a fresh marker from the cursor that was persisted while away.
- **First visit**: a session with no persisted cursor (`last_read_message_id`
  is empty) has no divider — there's nothing to distinguish as "already
  read." The first visit begins live-advancing the cursor as normal.
- **Initial scroll**: while a visit-start marker is pending, the message list
  may scroll it into view (`block: "start"`) when it is currently loaded but
  outside the initial viewport. Once the visible transcript is acknowledged
  and the marker clears, normal scroll behavior resumes. A pending explicit
  layout scroll restoration (`pendingChatScrollTop`) always wins.

## Persistence guarantees

- `UpdateTaskSessionLastReadMessageID` is a narrow, single-column write
  (like `RenameTaskSession`) so it never collides with a concurrent
  full-row or metadata write.
- The write is **monotonic**, comparing each message's `(created_at, id)` —
  portable across SQLite and Postgres, unlike a rowid-based comparison. A
  delayed or out-of-order mark-read request (e.g. two overlapping requests
  where an older message's response is processed after a newer one) can
  never regress the cursor and resurrect a stale divider.
- A cursor referencing a deleted message is treated as having no rank, so
  the monotonic guard never wedges.
- The frontend applies the mark-read response narrowly
  (`updateSessionReadCursor`), never as a full-session merge — a delayed
  HTTP response can't clobber a newer `session.state_changed` WebSocket
  update to unrelated fields.

## Failure modes

- **Unexpected repository/DB failure marking a session read**: logged and
  returned as a sanitized `500`, distinct from `400` for genuine bad input
  (empty id, unknown message, message belonging to a different session).
- **Mark-read targeting the synthetic placeholder**: prevented client-side —
  `lastRenderedMessageId` skips it, so an empty session never issues a
  mark-read request the backend would reject.

## Scenarios

- **GIVEN** a session with messages read before a rewound cursor, **WHEN**
  the user navigates away and back, **THEN** the divider may identify the
  first message added since that cursor while the initial acknowledgment is
  pending, and disappears as soon as the newest visible message is marked
  read — it must not remain while the session is actively in view.
- **GIVEN** new messages arrive while the session is already the visible chat
  panel, **THEN** live mark-read advances the cursor without rendering or
  moving a divider.
- **GIVEN** two overlapping mark-read requests for the same session where
  the response for an older message resolves after a newer one, **WHEN**
  both complete, **THEN** the persisted cursor reflects the newer message,
  never regressing.
- **GIVEN** a brand-new session with no messages yet, **WHEN** the chat
  panel renders, **THEN** no mark-read request is issued for the synthetic
  task-description placeholder.

## Out of scope

- Per-user or per-device read state. Would require a join table
  (`session_id`, `user_id`) → `last_read_message_id`; not needed while
  kandev has no multi-viewer session model.
- Unread *counts* (e.g. a badge showing "12 new") — this feature only
  positions a single boundary marker, matching Slack's divider, not its
  unread-count badges.
