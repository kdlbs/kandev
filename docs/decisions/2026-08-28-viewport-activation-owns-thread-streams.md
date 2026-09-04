# ADR-2026-08-28-viewport-activation-owns-thread-streams: Viewport Activation Owns Threads Session Streams

**Status:** proposed
**Date:** 2026-08-28
**Area:** backend, frontend, protocol

## Context

Threads presents several task conversations in one horizontal deck. The first
implementation mounts one full `TaskChatPanel` for every eligible task. A
workspace with 30 active tasks therefore fetches 30 transcripts, creates 30
rich chat trees, and subscribes to 30 session streams before the user scrolls.

A task can also have several agent sessions. Showing those sessions as tabs
must not multiply the fan-out again. Unselected tabs still need correct
lifecycle and pending-action status, but their transcript message events are
session-scoped. The existing task summary is bounded and cannot carry an
unbounded session array.

## Decision

Threads will keep lightweight task-column shells for stable order, scroll
geometry, and deep links. Viewport activation will own expensive detail work.
Task-session membership can preload for visible columns plus one adjacent
column on each side. A full transcript and `session.subscribe` membership can
exist only for the selected session in an intersecting desktop column. Phone
Threads can have only the nearest snapped column detail-active.

Session selection is local to each task column. A tab switch replaces that
column's one detail stream and does not change another column or the task
workbench's active session.

Unselected tab status will use compact session projections. Existing
workspace-scoped `session.state_changed` events carry lifecycle and activity.
A new workspace-scoped `session.pending_action_changed` event will carry only
session identity, the complete pending action, and its revision. It will not
carry transcript content. The task status summary remains constant-size.

An ordinary `WAITING_FOR_INPUT` lifecycle state is not an attention action.
Question and permission indicators require an explicit pending-action
projection. Review-ready task state uses a completion indicator.

## Consequences

Initial transcript requests, rich chat trees, and full WebSocket session
subscriptions scale with the visible detail window instead of all eligible
tasks or all sibling sessions. Horizontal scrolling can show a short loading
state when a column was not preloaded, but the adjacent membership window
reduces that delay.

Lightweight task shells still scale with the number of eligible tasks. This is
intentional. It preserves current stable ordering and scroll behavior without
adding variable-width horizontal virtualization. A later change can virtualize
shells if profiling shows that small shell DOM nodes are the remaining limit.

The backend gains one compact semantic event. This avoids workspace delivery
of message bodies and avoids polling task-session lists. Reconnect and
preload-entry list hydration repair missed compact events.

## Alternatives Considered

- **Mount every task and session conversation.** Rejected because network,
  store, render, and WebSocket work grow with all tasks and all tabs.
- **Put all sessions in `TaskStatusSummary`.** Rejected because it breaks the
  constant-size task-list contract and duplicates session membership.
- **Use transcript message events to update inactive tabs.** Rejected because
  those events contain session detail and require the inactive subscriptions
  this decision removes.
- **Poll task-session lists for status.** Rejected because polling is less
  current, repeats complete lists, and creates work while nothing changes.
- **Fully virtualize task shells now.** Rejected for this delivery because the
  expensive work is transcript and subscription fan-out. Stable shells are a
  smaller and simpler scroll contract.

