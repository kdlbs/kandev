---
status: draft
system: platform
created: 2026-08-28
owners:
  - kandev
---

# Viewport-bounded Session Delivery Requirements

## Overview

An intentional multi-conversation surface can make several session details
visible at the same time. It must not treat every task column or every sibling
session tab as an opened transcript. Full transcript streams must follow the
current viewport and selected session, while compact sibling status remains
live without transcript delivery.

The platform owns this delivery and subscription budget. The Threads UI that
uses it is specified in [Threads Conversation Deck](../../ui/requirements/threads-conversation-deck.md).

## Terminology

- **Session detail stream:** The subscribed transcript, message updates, shell
  activity, model state, MCP state, queue state, and other session-owned detail.
- **Compact session status:** Session identity, lifecycle state, foreground
  activity, and explicit pending action without transcript content.
- **Preload window:** Visible task columns plus at most one adjacent column on
  each side.
- **Detail window:** Desktop task columns that intersect the deck viewport, or
  the single nearest snapped phone column.

## Requirements

### REQ-PLATFORM-VIEWPORT-SESSION-DELIVERY-001: Viewport-owned detail streams

**Intent:** Bound full session traffic and frontend detail work by what the user
can currently inspect.

#### Acceptance criteria

- **AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-001.1:** A multi-conversation surface
  shall discover task columns from bounded task summaries without subscribing
  to any session detail stream.
- **AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-001.2:** A surface may load compact
  session membership for task columns in its preload window, and it shall not
  load that membership for every offscreen task on initial render.
- **AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-001.3:** For each task column in the
  detail window, the client shall subscribe to at most one full session detail
  stream, which shall be the user-selected session for that column.
- **AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-001.4:** An unselected sibling session
  shall not receive `session.subscribe`, a transcript list request, or rich
  message rendering solely because its selector item is visible.
- **AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-001.5:** When a task column leaves the
  detail window, the client shall release its selected session detail stream.
  On a phone, changing the nearest snapped column shall leave at most one task
  column detail-active.
- **AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-001.6:** With 30 eligible task
  columns, the number of full detail subscriptions shall be bounded by the
  detail window and shall not grow to 30 on initial render.

### REQ-PLATFORM-VIEWPORT-SESSION-DELIVERY-002: Lightweight sibling status

**Intent:** Keep every loaded session selector accurate without copying or
subscribing to its transcript.

#### Acceptance criteria

- **AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-002.1:** A task-session list response
  shall include the compact lifecycle and pending-action projection for each
  returned session without including transcript content.
- **AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-002.2:** Workspace-scoped session
  lifecycle events shall keep loaded sibling session state and foreground
  activity current without a session detail subscription.
- **AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-002.3:** A workspace-scoped compact
  pending-action event shall identify the task and session, carry the complete
  replacement pending action and its revision, and contain no message body or
  transcript metadata.
- **AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-002.4:** A client shall ignore an
  older pending-action revision and shall converge through a current
  task-session list after reconnect or when a task column re-enters the preload
  window.
- **AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-002.5:** A missing compact status
  event shall not cause a client to subscribe to an inactive session as a
  repair path.
- **AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-002.6:** Compact lifecycle and
  pending-action events shall use the owning workspace authorization boundary
  and shall fail closed when that workspace cannot be resolved.

## Out of scope

- Moving transcript, tool, shell, model, MCP, queue, or Git detail into compact
  task or session status.
- Adding an unbounded session array to `TaskStatusSummary`.
- Limiting full detail streams in a layout where the user explicitly makes
  several session details visible at the same time.
- Persisting viewport or selected-tab state on the backend.

