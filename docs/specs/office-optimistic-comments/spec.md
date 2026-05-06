---
status: partially-shipped
created: 2026-05-03
owner: cfl
---

# Office Optimistic Comments

## Status

**Phase 1 — per-comment run status badge: shipped**
(`feat(office): per-comment run status badge`).

What landed:
- Backend publishes `office.run.queued` on `Service.QueueRun` and
  `office.run.processed` on `FinishRun` / `FailRun` / `cancelRetry` /
  `cancelStaleRun`. WS broadcaster fans both out.
- `CommentDTO` carries `runId` / `runStatus` / `runError`, joined
  through `office_runs.idempotency_key = "task_comment:<comment_id>"`
  via a batched `GetRunsByCommentIDs` lookup.
- `<UserCommentRunBadge>` renders five states (queued / claimed /
  finished / failed / cancelled) under a user comment, gated on no
  later agent reply existing. Hides reactively when the agent reply
  lands.
- E2E (`apps/web/e2e/tests/office/comment-run-status.spec.ts`)
  covers initial render, live queued → claimed transition,
  failed-with-tooltip, and gating-on-later-agent-reply.

**Phase 2 — still draft** (this document below):
- Optimistic local rendering (`Sending…` before server confirm, draft
  restore on error).
- "Awaiting agent (N ahead)" indicator driven by
  `selectActiveSessionsForAgent`.
- "Queued — agent paused" branch + `<AgentPausedNotice>` above the
  textarea.

The shipped run-status badge already covers the queued / claimed /
failed states with WS reactivity, so Phase 2's remaining pieces are
about the *cause* of the wait (paused vs busy vs just-queued) rather
than its mere presence.

## Why

Adding a comment to an office task is currently a black box: the user clicks
send, the input clears, and nothing visibly happens until the server returns
and the WS refetch fires. There's no spinner, no "Sending..." state, and no
optimistic rendering of the comment. On a slow connection or after a backend
hiccup, users second-guess whether the action went through and click again,
creating duplicates or simply giving up. The comment should appear
immediately with a faded "Sending..." badge and restore the draft to the
input on error.

## What

- All updates MUST flow through the existing WebSocket event stream
  (`office.comment.created`, `session.message.added` / `.updated`). No
  polling. The optimistic comment is purely client-side state until the
  server-confirmed event arrives.
- When the user submits a comment, the comment MUST appear in the chat
  thread immediately, before the server responds, with a visual `pending`
  styling (e.g. reduced opacity and a subtle "Sending…" label).
- When the server confirms the comment, the pending styling MUST resolve to
  the normal comment appearance with no visible flash or layout shift.
- When the server fails (network error, 5xx, validation error), the pending
  comment MUST be removed AND the draft text MUST be restored to the input
  textarea so the user can retry without re-typing. A toast or inline error
  MUST explain the failure.
- The send button MUST stay disabled while a submission is in flight, so the
  same comment can't be submitted twice.
- File attachments and pasted images that accompany the comment MUST follow
  the same lifecycle (pending appears immediately, error restores to input).

### Comment lifecycle states

A user comment goes through three observable states:

1. **Sending** — between user click and server confirmation. Optimistic
   render with reduced opacity + "Sending…" sub-label.
2. **Awaiting agent** — server confirmed the comment is persisted, but the
   assignee agent has not yet replied. This can take seconds (busy agent
   freeing up) to hours (queue depth, low-priority task) or never (agent
   paused). The chat MUST surface this state so the user understands
   *whose turn* it is.
3. **Resolved** — agent has posted a reply comment after this user comment.
   The waiting affordance disappears.

The "awaiting agent" sub-label MUST adapt to the agent's actual situation:

- **Agent paused / stopped / pending_approval** → "Queued — agent paused"
  (with a link to the agent so the user can resume it).
- **Agent currently working on this same task** (a session for this task
  is `RUNNING`) → "Agent is replying…" with a typing-style indicator.
- **Agent currently busy with N other tasks** → "Awaiting agent
  ({N} ahead)" — gives the user a sense of queue depth without making
  promises about timing.
- **Agent idle, not paused** → "Awaiting agent" (default, the agent
  scheduler should pick it up momentarily).

All sub-label transitions MUST be driven by existing WS events
(`office.agent.updated`, `office.agent.status_changed`,
`session.state_changed`, `office.comment.created`). No polling.

### Agent paused — input area notice

In addition to per-comment styling, the chat input area MUST show a single
inline notice when the agent is paused, so the user understands replies
won't come at all (vs. just being slow):

- Wording: "Agent is paused — resume it for replies" with a link to the
  agent.
- Notice appears above the textarea before the user types.
- Notice updates reactively from `office.agent.updated` /
  `office.agent.status_changed` events.

## Scenarios

- **GIVEN** the user types "looks good" and clicks send, **WHEN** the request
  is in flight, **THEN** the comment appears at the bottom of the thread with
  a faded styling and a "Sending…" indicator within 50 ms, and the send button
  is disabled.
- **GIVEN** a pending comment is showing, **WHEN** the server returns 201,
  **THEN** the pending styling is removed and the comment renders with the
  normal author + timestamp from the server response.
- **GIVEN** a pending comment is showing, **WHEN** the server returns 500,
  **THEN** the pending comment is removed from the thread, the draft text is
  restored to the textarea, the send button re-enables, and a toast says
  "Failed to send comment — please try again."
- **GIVEN** the user attaches a file and types "see attached", **WHEN** they
  click send, **THEN** the pending comment shows with both the text and the
  attached filename. On error, both the text and the file selection are
  restored.
- **GIVEN** the assignee agent (or workspace CEO) is paused, **WHEN** the
  user opens the task chat, **THEN** the input area shows "Agent is paused
  — resume it for replies" before the user types anything.
- **GIVEN** the assignee agent is paused, **WHEN** the user submits a
  comment, **THEN** the comment is saved and shows "Queued — agent
  paused" instead of "Sending…".
- **GIVEN** the inline "agent paused" notice is showing, **WHEN** the user
  resumes the agent, **THEN** the notice disappears within 2 seconds
  without a page reload, driven by `office.agent.updated`.
- **GIVEN** the user just posted a comment and a session is RUNNING for
  this task, **WHEN** the comment confirms, **THEN** it shows "Agent is
  replying…" with a typing-style indicator until the agent posts a reply
  comment.
- **GIVEN** the user posted a comment and the assignee agent is busy
  running 2 other tasks, **WHEN** the comment confirms, **THEN** it shows
  "Awaiting agent (2 ahead)" — driven by `selectActiveSessionsForAgent`
  on the existing `taskSessions` store (same selector the sidebar uses).
- **GIVEN** a user comment is showing "Awaiting agent", **WHEN** an agent
  reply comment for this task arrives via `office.comment.created`,
  **THEN** the awaiting indicator disappears and the comment renders
  normally.

## Out of scope

- Polling fallbacks — if the WS connection is down, the chat stays as-is
  until it reconnects.
- Editing comments after submission (separate feature).
- Optimistic rendering of agent-generated comments (those still flow through
  the WS stream and are not user-initiated).
- Retry-on-error UI (clicking a "retry" button on a failed comment). The
  draft restoration covers the common case; explicit retry can come later.
- Auto-resuming a paused agent when the user posts a comment. The notice
  invites the user to resume manually; a "send + resume" combined action
  is a future iteration.

## Open questions

- How do we ID a pending comment locally so the server response can match it
  back? Options: client-generated UUID sent in the request body and echoed
  back, or just append to the thread on success and remove the pending one
  with the same body. Plan should pick one.
