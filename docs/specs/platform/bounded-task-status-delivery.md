---
status: approved
created: 2026-08-01
updated: 2026-08-02
owner: kandev
---

# Bounded Task Status Delivery

## Why

Task rows currently obtain compact status indicators by observing large,
session-owned streams. With many agents working, one browser receives messages,
shell activity, model updates, MCP state, and Git events from sessions that are
not open. Switching tasks rebuilds those subscriptions. The resulting traffic
can delay or drop request responses, producing an unknown message-send result
and temporarily hiding active-session controls such as the model selector.

Task-level navigation needs a bounded status contract. Session detail still
needs rich streams, but opening a workspace must not make every session a
detail surface.

## What

- Every task snapshot may carry a `status_summary` containing the complete set
  of runtime facts needed by desktop and mobile task switchers.
- The backend derives that summary from authoritative task, session, message,
  Git, and pull-request state. It is a rebuildable read model, not a second
  source of truth.
- Summary updates use the existing workspace task feed. Task rows never
  subscribe to a session merely to obtain a badge.
- Full session snapshots and live session notifications are delivered only to
  explicitly opened session detail surfaces.
- Repeated subscribe/focus requests do not replay full session state. Git
  refresh has a targeted action that does not alter focus.
- Correlated WebSocket responses and errors are prioritized over unsolicited
  notifications and cannot be silently dropped by notification pressure.
- One session's streaming text or reasoning cannot monopolize persistence,
  notification delivery, or browser state work. Intermediate replacement
  updates are bounded while the final transcript remains lossless.
- `message.add` uses a stable client-generated message ID so an uncertain send
  can be reconciled or retried without creating or dispatching a duplicate.
- Existing desktop and mobile status/icon precedence remains unchanged.

## Task status summary

The wire field is `status_summary`; the frontend maps it to `statusSummary`.
The initial contract is:

| Field                                          | Meaning                                                             | Bound                                |
| ---------------------------------------------- | ------------------------------------------------------------------- | ------------------------------------ |
| `revision`, `updated_at`                       | Monotonic task-local version and projection time                    | Constant                             |
| `primary_session`                              | Primary session ID and lifecycle state                              | One session                          |
| `foreground_activity`, `active_subagent_count` | Existing task-level busy aggregate                                  | Constant                             |
| `pending_action`                               | `permission`, `clarification`, or absent                            | Constant                             |
| `active_error`                                 | Session ID, stable error stamp, occurrence time, and safe preview   | One preview, at most 512 UTF-8 bytes |
| `git`                                          | Aggregate additions, deletions, changed files, ahead, and behind    | Numeric totals only                  |
| `pull_request`                                 | Count, bounded representative identity, and aggregate display state | Constant regardless of PR count      |

`pull_request.aggregate_state` is one of `failure`, `blocked`, `pending`,
`awaiting_review`, `ready`, `passing`, `draft`, `merged`, `closed`, or
`neutral`. It preserves the existing most-attention-worthy task-row rule.

The summary never contains message bodies, transcript entries, file names,
patches, shell output, model lists, MCP payloads, or an unbounded array of
sessions, repositories, or pull requests. Existing flat task runtime fields may
remain during migration, but switchers use the summary when present.

## Derivation rules

- Pending permission outranks pending clarification for the row's primary
  state icon. Both outrank generating/background activity, which outranks
  coarse lifecycle state. This is the existing task-row precedence.
- A session's `pending_action` clears only when the specific request that armed
  it resolves — a `message.updated` on that same permission/clarification
  request row (matched by type and `pending_id`) reaching a terminal,
  non-`pending` status, or that row's deletion. An unrelated message on the
  same session (a tool call, script execution, or any other row that merely
  carries its own terminal status metadata) must not clear a different
  session's armed `pending_action`, and must not clear the current session's
  `pending_action` unless it is that same request row. Otherwise a session that
  is busy streaming ordinary tool activity right after a permission/
  clarification request clears the affordance before the user ever answers it.
- `active_error` is independent of the primary state icon. It represents the
  newest relevant recoverable agent error and clears after an authoritative
  dismissal or a newer agent response according to the existing error rules.
- Git totals aggregate the latest observation for every repository in the
  task. A multi-repository update must not expose a partial replacement that
  forgets unchanged repositories.
- Pull-request state aggregates open PRs before terminal PRs and chooses the
  most attention-worthy current status. Full PR details remain owned by the
  GitHub domain and are loaded only by surfaces that need them.
- A semantic no-op does not increment `revision` or emit an update.
- Clients ignore a summary delta whose revision is not newer than the stored
  revision.

## API and event surface

- Boot state, task-list responses, and workflow snapshots include the latest
  `status_summary` without per-task queries.
- `task.status_summary.updated` is workspace-scoped and carries `task_id`,
  `workspace_id`, and the complete replacement summary. It uses the same
  authorization and subscription boundary as other task updates.
- `session.subscribe` sends the full initial session snapshot only when that
  client newly joins the session. A duplicate subscribe only acknowledges the
  request.
- `session.focus` changes focus/poll priority and acknowledges the request; it
  does not replay a session snapshot.
- `session.git.refresh` requests a fresh Git observation for the selected
  session without changing focus or subscription membership.
- WebSocket request responses/errors use a reserved control path. If the
  server cannot enqueue control traffic, it closes the connection so the
  client enters explicit reconnect/reconciliation instead of waiting on a
  response that was silently discarded.
- Web clients send a stable `client_message_id` with `message.add` (the
  backend also accepts `message_id` as a compatibility alias). The first
  accepted request owns that ID. A retry in the same authorized task returns
  the persisted message and skips session-state transitions, turn-start hooks,
  message creation, and prompt dispatch. Reuse outside that scope is rejected.

## Persistence guarantees

- The latest task status summary is stored by task ID with workspace ID,
  revision, JSON payload, and update time using migrations supported by both
  SQLite and Postgres.
- Summary persistence and revision changes are serialized per task. A
  compare-and-update or equivalent transaction prevents concurrent source
  events from publishing duplicate revisions or losing a newer value.
- Missing rows are rebuilt from authoritative records. List and boot loaders
  batch summary reads and may batch repairs; they do not perform an N+1 query.
- Live Git observations remain coalesced before persistence/publication.
  Running executions maintain the slow monitoring baseline independently of
  browser subscribers; active focus may request fast monitoring. Settled tasks
  retain the latest stable snapshot.
- Message IDs remain stable through reconnect, retry, response hydration, and
  notification hydration. Duplicate notifications and responses upsert the
  same frontend message.

## Session stream overload isolation

Agent stream chunks are an ingress format, not a required persistence or
browser-render cadence. The lifecycle boundary coalesces adjacent assistant or
reasoning chunks for the same Kandev message record before publishing them to
the orchestrator:

- the first non-empty chunk creates the streaming record without waiting for a
  coalescing interval;
- subsequent adjacent chunks for that record publish at most once per 100 ms
  window while streaming continues;
- concatenation preserves byte order and content exactly; coalescing may not
  truncate, summarize, or rewrite agent output; and
- pending content flushes before a tool call/result, permission request,
  completion, error, stream disconnect, execution replacement, or backend
  shutdown so persisted transcript order remains authoritative.

The per-agent ACP adapter and process handoff apply cancellation-aware
backpressure when their bounded handoff queues fill. They must not silently
drop normalized assistant/reasoning chunks before the lifecycle coalescer; a
noisy session may slow its own agent process, and shutdown cancellation may
discard only work after that session is terminal. This handoff is scoped to the
agent instance and must not block unrelated backend sessions.

The WebSocket gateway treats `session.message.updated` as a replaceable full-
state notification keyed by client, session, and message. At most one unsent
replacement for a key occupies delivery capacity. A newer update replaces the
older payload in place. Replaceable stream notifications use bounded
per-session capacity and fair scheduling, separate from correlated control
traffic and semantic notifications such as message creation/deletion, task or
session state changes, pending-input occurrences, and turn completion. When a
session exceeds its replaceable allowance, that session's oldest queued
replaceable entry may be evicted; it cannot consume another session's
allowance or the semantic-notification lane. Reconnect, snapshot, and
turn-settle reconciliation repair any intermediate replacement that was not
delivered because the database remains authoritative.

The frontend applies the same defense at the render boundary. It keeps only the
newest `session.message.updated` payload for each message during one animation
frame and performs one store update per changed message when the frame flushes.
Message add/delete and turn-settle events remain ordered barriers. Intentional
multi-session detail surfaces may subscribe to every session they display, but
rerendering or refetching equivalent session objects must not tear down and
recreate unchanged subscription membership.

Overload handling is observable. Structured metrics/logs identify the client,
session, action, queue class, coalesced chunk count, replacement count, and
eviction count without logging transcript content. Reconnect, a selected-
session snapshot, or turn-settle reconciliation repairs any skipped
intermediate replacement.

## Failure modes

- If projection fails, the last valid summary remains visible, the failure is
  logged with task/source context, and the next semantic source event or
  authoritative rebuild can repair it.
- If a summary delta is dropped, reconnect or a task-list/workflow refresh
  supplies the complete current summary. The browser does not fall back to
  background session subscriptions.
- If Git monitoring fails, the summary retains the last observation and does
  not claim a clean tree from missing data.
- If a message response is interrupted, the client reconciles the same
  `client_message_id`/persisted message ID from the response, notification, or
  message list, then retries that ID when needed. It reports an unknown outcome only after bounded
  reconciliation cannot determine acceptance.
- Notification overload may coalesce or drop replaceable notifications, but
  it cannot silently discard a correlated response/error.
- If one session exceeds its replaceable stream allowance, other sessions and
  semantic notifications continue to make progress. The noisy session retains
  exact persisted content and converges through its newest replacement or
  normal reconciliation.
- If `status_summary` is temporarily absent during rollout, task rows use the
  existing coarse task fields and omit unavailable decorations; they do not
  subscribe to inactive sessions.

## Scenarios

- **GIVEN** a workspace with 27 tasks and one selected session, **WHEN** the
  task switcher loads, **THEN** no inactive session is subscribed and no
  inactive message, shell, model, MCP, or Git stream is delivered for a row.
- **GIVEN** the same workspace, **WHEN** the user switches tasks repeatedly,
  **THEN** subscription work is proportional to the sessions opened/closed by
  each switch and does not grow with the number of workspace tasks.
- **GIVEN** a background task asks a question, encounters a recoverable error,
  changes its Git tree, or receives a PR update, **WHEN** its summary revision
  arrives, **THEN** desktop and mobile rows update without a session
  subscription.
- **GIVEN** a recoverable error is dismissed or followed by a newer agent
  response, **WHEN** the projector processes that occurrence, **THEN** the
  independent error indicator clears on both task switchers.
- **GIVEN** a session's agent requests permission and, before that request is
  answered, the same session emits an unrelated tool call/execute/read message
  that reaches its own terminal status, **WHEN** the projector processes that
  unrelated message, **THEN** `pending_action` for the session remains
  `permission` (the affordance does not flash and disappear before the user
  answers it).
- **GIVEN** notification traffic saturates its queue, **WHEN** the selected
  session sends `message.add`, **THEN** the correlated response is delivered
  first or the connection closes for deterministic reconciliation.
- **GIVEN** the first message response becomes uncertain, **WHEN** the client
  retries the same `client_message_id`, **THEN** exactly one user message is stored and
  the duplicate request does not dispatch a second prompt.
- **GIVEN** many background sessions are active, **WHEN** the selected agent
  publishes model configuration, **THEN** the selected chat retains its model
  selector and can submit a follow-up message.
- **GIVEN** one ACP session emits 30,000 tiny reasoning chunks for one message,
  **WHEN** the stream crosses the lifecycle, persistence, gateway, and browser
  boundaries, **THEN** the final reasoning content is byte-for-byte complete,
  append publications occur no more than once per 100 ms window, and pending
  replacement work remains bounded.
- **GIVEN** a noisy session is continuously streaming, **WHEN** the user opens
  another task or submits a message in another selected session, **THEN** that
  navigation and its correlated response complete without waiting for the
  noisy session's backlog.
- **GIVEN** an Office task detail refetch returns new objects for the same set
  of session IDs, **WHEN** React rerenders the page, **THEN** the client sends no
  unsubscribe, subscribe, or initial-snapshot replay for unchanged membership.
- **GIVEN** a phone viewport, **WHEN** task summaries change, **THEN** the
  existing task-switcher sheet shows the same badges and precedence as the
  desktop sidebar without new navigation, scroll, or touch behavior.

## Out of scope

- Moving full transcripts, diffs, shell output, model configuration, or MCP
  state into tasks.
- Replacing authoritative session, message, Git, or GitHub persistence.
- Redesigning desktop or mobile task-switcher layout and interactions.
- Preventing detail surfaces such as Office or explicit multi-session views
  from subscribing to sessions they intentionally display.
- Guaranteeing external agent prompt execution exactly once across a complete
  backend process crash; this contract prevents duplicate handler dispatch on
  client retry.
- Treating a larger timeout or queue capacity as the fix.
- Limiting how much reasoning or assistant content an agent may ultimately
  produce. This contract bounds intermediate work, not transcript length.
- Automatically stopping or penalizing an agent solely because it emits many
  valid chunks.

## Implementation plan

- Original delivery redesign:
  [`../../plans/bounded-task-status-delivery/plan.md`](../../plans/bounded-task-status-delivery/plan.md)
- Stream overload repair:
  [`../../plans/session-stream-overload-isolation/plan.md`](../../plans/session-stream-overload-isolation/plan.md)
