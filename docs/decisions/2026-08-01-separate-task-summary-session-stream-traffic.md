# ADR-2026-08-01-separate-task-summary-session-stream-traffic: Separate Task Summary and Session Stream Traffic

**Status:** accepted
**Date:** 2026-08-01
**Area:** backend, frontend, protocol

## Context

Task switchers need a small set of facts for every visible task: runtime state,
pending user input, recoverable agent errors, aggregate Git changes, and pull
request status. Those facts currently come from several stores. In particular,
the desktop sidebar subscribes to every task's primary session to obtain live
Git status, while desktop and mobile also inspect loaded session messages and
metadata to derive pending and error indicators.

A captured reload with 27 sessions produced 700 WebSocket frames (3.23 MB) in
five seconds, including 74 `session.subscribe` and 47 `session.unsubscribe`
requests. Switching tasks rebuilds the subscription set. Each session
subscription can replay the full session snapshot and then carry messages,
shell activity, model configuration, MCP status, and other data that a task row
does not use.

The WebSocket client currently puts correlated responses and unsolicited
notifications on one bounded FIFO queue. When that queue is full, a frame can
be dropped without closing the socket. This makes a fast, persisted
`message.add` operation appear to time out, and delayed model snapshots can
temporarily remove the model selector. Increasing the message timeout would
hide the symptom without removing the fan-out or the ambiguous response loss.

## Decision

Kandev will maintain a bounded, rebuildable `TaskStatusSummary` read model for
task-level surfaces. It is carried on task snapshots and updated through the
existing workspace-scoped task event path. The summary contains only the facts
needed to classify and decorate a task row; transcripts, file paths, diffs,
shell output, model configuration, and MCP payloads remain session-owned and
are never copied into it.

Session, message, Git, and pull-request domains remain authoritative. The
summary projector consumes their semantic changes, persists the latest compact
projection with a monotonic per-task revision, and emits an update only when
the projected value changes. A missing projection is rebuilt from authoritative
state and never causes the browser to subscribe to background sessions.

Task switchers consume task snapshots plus task-summary deltas. They do not
subscribe to session streams. Full session streams are limited to sessions
explicitly opened in an active detail surface. Repeated subscription and focus
requests are idempotent: only a new subscription receives a full initial
snapshot, and focus changes polling priority without replaying session data.
Targeted refresh operations use targeted actions rather than `session.focus`.

Git-summary freshness is tied to runtime ownership rather than browser
interest. A live execution keeps a slow monitoring baseline, active focus may
upgrade it to fast monitoring, and settled tasks use the latest persisted
snapshot. Removing a browser's background subscriptions therefore cannot pause
the source that feeds task-level Git status.

Correlated WebSocket responses and errors use a reserved, prioritized delivery
path separate from best-effort notifications. A response must be queued or the
connection must fail explicitly; it must not be silently discarded behind
session notification traffic. User message submission also carries a stable
client-generated message ID. Retrying that ID returns the accepted message
without rerunning turn-start hooks or dispatching the prompt twice.

## Consequences

Workspace traffic scales with task-summary changes and explicitly opened
sessions rather than with every task's full session activity. Switching tasks
has constant subscription work and no longer tears down and recreates an
all-task subscription fan-out. Desktop and mobile task switchers read the same
status contract and preserve their existing icon precedence.

The summary is eventually consistent by design. Revisions prevent stale
deltas from overwriting newer task state, and reconnect/task-list hydration
repairs missed updates. Projection failures leave the last valid summary in
place and are observable; detailed domain data remains available on demand.

The implementation adds a compact persistence model and projection wiring
across several domains. It also requires explicit queue semantics in the
WebSocket gateway and an idempotency contract for `message.add`. These are more
moving parts than a frontend-only optimization, but they remove the coupling
that caused the traffic amplification and protect every request type from the
same queue-pressure failure mode.

## Alternatives Considered

- **Keep all session subscriptions but reduce individual payloads.** Rejected
  because task switching would still perform O(number of tasks) subscription
  churn and task rows would remain coupled to session-internal events.
- **Derive one task aggregate entirely in the frontend.** Rejected because the
  browser would still need every authoritative stream, every client would
  duplicate precedence rules, and a suspended browser could not repair the
  aggregate without replaying those streams.
- **Copy full session, Git, or PR records onto the task row.** Rejected because
  it would create competing sources of truth and make a bounded list payload
  grow with transcripts, repositories, or pull requests.
- **Poll task details from REST.** Rejected because polling introduces stale
  badges and repeated full-list reads while the workspace task event channel
  already provides an appropriate bounded delivery path.
- **Only enlarge the WebSocket queue or message timeout.** Rejected because
  queue pressure would remain proportional to background sessions and a full
  queue could still make accepted writes ambiguous.
