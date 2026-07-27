---
status: shipped
created: 2026-07-27
owner: cfl
---

# Office: Slack-Style Unread Divider

## Why

Navigating back into a task session that kept running in the background
previously dropped the reader at the bottom of the transcript with no
indication of what was new. Slack solves this with a "New" divider frozen at
the boundary between read and unread messages for the duration of the visit.
This spec documents the persisted cursor this feature is built on, since it's
new product state whose scope (global vs. per-user) and edge-case behavior
(first visit, pagination, initial scroll) aren't otherwise obvious from the
UI alone.

## What

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
- The divider's position is decided once per **visit** (a transition from
  hidden/inactive to the active, visible chat panel), not on every render:
  the hook captures the cursor's value at the instant the session becomes
  visible (before this visit's live-advance can touch it) and freezes it as
  the anchor. Re-rendering while still visible never moves the divider;
  leaving and returning (or switching sessions and back) captures a fresh
  anchor from whatever the cursor advanced to since.
- **First visit**: a session with no persisted cursor (`last_read_message_id`
  is empty) has no divider — there's nothing to distinguish as "already
  read." The first visit begins live-advancing the cursor as normal.
- **Initial scroll**: on visit start, if the divider falls on a
  currently-loaded message that's outside the initial viewport, the message
  list performs a one-time scroll to bring it into view (`block: "start"`)
  instead of the default scroll-to-bottom. This applies once per visit and
  does not fight a pending explicit layout scroll restoration
  (`pendingChatScrollTop`), which always wins when set.
- **Pagination**: `findUnreadDividerItemId` looks for the divider's anchor
  message among the *currently loaded* render items. If the anchor is older
  than everything loaded so far (paginated out), the whole loaded window is
  treated as unread and the divider renders before the first loaded item —
  matching Slack's behavior of showing the marker at the top of what's
  visible rather than requiring a full backward fetch before showing
  anything. The synthetic frontend-only `task-description` placeholder
  (rendered for a session with zero real messages yet) is never a valid
  anchor or divider boundary — it doesn't exist in the backend messages
  table.

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
  the user navigates away and back, **THEN** the divider appears before the
  first message added since the frozen anchor, and clears (no anchor) on
  the *next* visit after that.
- **GIVEN** a transcript long enough that the divider falls outside the
  initial viewport, **WHEN** the session becomes the visible chat panel,
  **THEN** the view scrolls once to the divider instead of the newest
  message.
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
