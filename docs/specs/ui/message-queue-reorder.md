---
status: building
created: 2026-08-07
owner: kandev
---

# Reorder Queued Messages

## Why

Pending messages run in strict FIFO order, so a later, more urgent prompt
waits behind everything queued before it. Today the only workarounds are
remove-and-requeue (destructive) or Send Now (interrupts the active turn).
Users need to rearrange pending messages so execution order matches priority
without losing content.

## What

- Every visible pending message in the queue panel offers a **grab handle**
  on its left edge. The handle is a small dotted grip (2-column dot grid).
- The handle is a **floating overlay** anchored to the row's left edge,
  vertically centered. It does not participate in layout: no row or panel
  content moves to make room for it. While shown it may cover the row's
  `#N` position label; the label returns when the handle hides.
- On a fine pointer the handle is hidden by default and appears when the
  pointer hovers the row or keyboard focus reaches it. On a coarse pointer
  (touch) it is always visible, attached flush to the message box's left
  edge, and painted **behind** the row content — no chip surface, so the
  dotted grip reads as part of the box edge. The position label and sender
  icon stay painted above it (and pass touches through), and the interactive
  hit area is at least 44 by 44 CSS pixels.
- Dragging starts **only** from the handle. Pointer interaction with the row
  body (text selection, expand toggle, edit/merge/remove/send-now actions)
  is unchanged. Dragging requires pointer movement (activation distance),
  so a click on the handle does nothing by itself.
- Dropping the handle onto another row commits a new order. The queue panel
  updates immediately (optimistic) and reconciles to the backend result.
- The new order is **persisted server-side** and survives reload, session
  switch-back, and restart. All connected views reconcile to it.
- Displayed positions remain compact one-based ordinals (`#1` through
  `#N`) derived from the visible order, exactly as after remove/merge.
- Reordering applies to every visible pending row **regardless of
  provenance** (user, agent, workflow, system), matching the Remove action.
  Server-reserved rows already in flight stay hidden and are never
  reordered.
- The handle is disabled while a queue mutation or backend cancellation for
  the session is pending (the same `isLoading`/`cancellationPending` gate as
  Send Now), and is hidden while the row is being edited.
- Keyboard reordering is supported: focusing the handle and using
  Space/Enter plus the arrow keys moves the row, matching the platform's
  sortable-list convention.
- Reordering changes nothing else: merge, edit, Remove, Send Now, FIFO
  auto-drain, and capacity semantics are unchanged. Send Now **all** and
  FIFO drain dispatch in the current (reordered) order.

## API Surface

New WebSocket action:

```text
message.queue.reorder
```

Request:

```json
{
  "session_id": "session-id",
  "ordered_ids": ["entry-b", "entry-a", "entry-c"]
}
```

`session_id` is required. `ordered_ids` is required, non-empty, and must
contain each visible pending entry id of the session exactly once.

Successful response:

```json
{
  "session_id": "session-id",
  "reordered": 3
}
```

`reordered` is the number of entries in the submitted order — equal to the
visible pending count on success. A no-op reorder (submitting the current
order) reports the same count, since positions are rewritten to the compacted
`1..N` sequence.

Errors:

- `validation`: `session_id` missing, `ordered_ids` missing/empty, or
  duplicate ids in `ordered_ids`.
- `queue_changed`: the submitted id set does not match the current visible
  pending set — an entry was drained, removed, merged, or newly queued
  since the client's snapshot. The request is rejected atomically with no
  partial reorder; the client refetches the authoritative queue.
- Session access is required; the existing non-enumerating session-not-found
  response is used before any queue read or mutation.

On success the handler publishes the existing `message.queue.status_changed`
event so every connected view reconciles.

## Data Model

No schema change. Order lives in the existing `queued_messages.position`
integer column (lower = head). A successful reorder rewrites positions to a
strictly increasing sequence: reserved in-flight rows keep their place in the
sequence, and the visible rows appear in the submitted order. Positions are
compacted (`1..N`) by the same operation, so subsequent inserts take
`MAX(position) + 1` as today.

## Permissions

- Any caller with access to the session may reorder its visible pending
  entries, regardless of entry provenance (same rule as Remove and Send Now).
- The action accepts no caller-supplied content, `user_id`, `task_id`, or
  identity. Only `session_id` and the entry ids are taken from the caller,
  and the ids are validated against the persisted visible set.

## Failure Modes

- **Drain/remove/merge races the drag:** the submitted set no longer matches
  the visible set; the backend rejects the whole request with `queue_changed`
  and the client refetches silently (self-recovering, no error toast).
- **Backend mutation fails for another reason:** the client refetches to
  restore the authoritative order and shows a localized error toast.
- **Concurrent clients:** the last successful reorder wins. A client with a
  stale snapshot receives `queue_changed` and reconciles on its next refetch.
- **Reserved in-flight rows:** they are invisible to the client and excluded
  from validation; they are never reordered and keep their delivery
  reservation.
- **Rapid repeated drags:** while a reorder request is in flight the handles
  are disabled, so overlapping reorder requests cannot be issued from one
  client.

## Persistence Guarantees

The reordered sequence persists in `queued_messages.position` in SQLite and
survives a Kandev restart. It is queue state, not a client preference, and is
shared by every view of the session. A process crash mid-reorder leaves the
transaction uncommitted and the previous order intact.

## Responsive and Mobile Behavior

- The existing inline queue panel remains the desktop and phone composition;
  the queue list stays the single internal scroll owner and the composer
  stays visible.
- Desktop: the handle is a hover/focus-revealed floating overlay on the
  row's left edge; nothing shifts in layout.
- Phone/coarse-pointer: the handle is always visible, attached flush to the
  box's left edge, and painted behind the row content (background, no chip
  surface). The row's position label / sender icon render above it and are
  `pointer-events: none`, so touches reach the handle's 44 by 44 CSS-pixel
  target while the label stays readable. `touch-action: none` on the handle
  keeps a touch-drag reordering instead of scrolling; vertical scrolling of
  the queue list is unaffected elsewhere.
- The queue hook, backend action, optimistic order, and reconciliation are
  shared across viewports; only the handle's visibility and size differ.
- Mobile Playwright coverage proves the same reorder result with touch input.

## Scenarios

- **GIVEN** three messages are queued in order A, B, C, **WHEN** the user
  drags C's handle above A, **THEN** the panel shows `#1` C, `#2` A, `#3` B
  and reloading the page keeps that order.
- **GIVEN** a fine-pointer desktop with a queued message, **WHEN** the
  pointer is not over the row, **THEN** no handle is visible, and the row
  box has the same width and content position as without the feature.
- **GIVEN** the pointer hovers a queued message row, **WHEN** the pointer
  moves onto the left edge, **THEN** the dotted grab handle appears as a
  floating overlay and the row content does not shift.
- **GIVEN** a queued message row, **WHEN** the user presses and drags on the
  row body (not the handle), **THEN** no drag starts; text selection and row
  actions behave as before.
- **GIVEN** a row is being edited, **WHEN** the user looks at it, **THEN**
  no grab handle is shown for that row.
- **GIVEN** a queue mutation or cancellation is in flight, **WHEN** the user
  looks at any row, **THEN** the handles are visible but disabled.
- **GIVEN** the user starts dragging entry A and another client drains A
  before the drop, **WHEN** the drop commits, **THEN** the backend returns
  `queue_changed`, the panel silently refetches, and no error toast appears.
- **GIVEN** a queue whose head is a hidden reserved in-flight lifecycle row,
  **WHEN** the user reorders the visible entries, **THEN** the visible
  entries assume the submitted order around the reserved row and its
  delivery/acknowledgement is unaffected.
- **GIVEN** the backend reorder fails for a non-race reason, **WHEN** the
  drop commits, **THEN** the panel refetches the authoritative order and
  shows the localized "failed to reorder" toast.
- **GIVEN** a queue containing user, agent, workflow, and system rows, **WHEN**
  the user drags any of them, **THEN** the row moves to the dropped position.
- **GIVEN** a reordered queue and the user clicks Send Now **all**, **THEN**
  the bulk prompt concatenates the bodies in the reordered FIFO order.
- **GIVEN** a phone viewport with queued messages, **WHEN** the user opens
  the queue panel, **THEN** every row shows an always-visible touch-sized
  handle attached flush to the box's left edge and painted behind the row
  content (position labels stay readable), and a touch drag reorders the
  queue with the same persisted result.
- **GIVEN** the user focuses a row's handle with the keyboard, **WHEN** they
  press Space, Arrow Up, then Space, **THEN** the row moves one position up
  and the order is persisted.

## Out of Scope

- Bulk selection or multi-row drag.
- Dedicated up/down move buttons as an alternative to dragging (keyboard
  reordering is covered by the sortable keyboard sensor).
- Reordering hidden reserved in-flight rows.
- Cross-session or cross-task reordering.
- Changing merge, edit, Remove, Send Now, FIFO drain, or capacity semantics.
