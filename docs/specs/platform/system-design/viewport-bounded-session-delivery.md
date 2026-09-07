---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-VIEWPORT-SESSION-DELIVERY-001
  - REQ-PLATFORM-VIEWPORT-SESSION-DELIVERY-002
---

# Viewport-bounded Session Delivery System Design

## Purpose and boundaries

This design applies the bounded task-summary boundary to an intentional
multi-conversation surface. Task summaries discover columns. A compact
task-session list discovers sibling sessions only near the viewport. Existing
workspace session lifecycle events and one new compact pending-action event
keep loaded selectors live. Only a selected session in the detail window owns a
full detail subscription.

Task, session, message, and pending-action records remain authoritative. This
design adds no transcript projection and no persisted viewport state.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-VIEWPORT-SESSION-DELIVERY-001` | [Delivery layers](#delivery-layers), [Subscription lifecycle](#subscription-lifecycle) |
| `REQ-PLATFORM-VIEWPORT-SESSION-DELIVERY-002` | [Compact pending-action event](#compact-pending-action-event), [Reconnect and repair](#reconnect-and-repair), [Security boundary](#security-boundary) |

## Delivery layers

The client uses three explicit layers:

| Layer | Source | Content | Activation |
| --- | --- | --- | --- |
| Task shell | workflow snapshots plus `task.status_summary.updated` | bounded task identity, ordering, review, aggregate attention and activity | task shells admitted by the active Threads view |
| Session selector | `GET /tasks/:taskId/sessions`, `session.state_changed`, compact pending-action event | bounded session rows, no transcript | preload window only |
| Conversation detail | `message.list` plus `session.subscribe` | selected session transcript and rich live state | detail window and selected session only |

The task summary remains constant-size and does not acquire a session array.
The existing task-session list already supplies `state`,
`foreground_activity`, `pending_action`, `pending_action_revision`, agent
profile identity, name, primary identity, and timestamps. The list is fetched
only after the UI marks a task as preloaded.

## Compact pending-action event

The existing `session.state_changed` action is workspace-scoped and updates
compact lifecycle and foreground activity without a session subscription.
Message events are session-scoped because they contain transcript detail. Their
current `pending_action` fields therefore cannot keep an unselected tab exact.

Add an internal semantic event and wire action named
`session.pending_action_changed`. Its payload is a complete compact replacement:

```json
{
  "workspace_id": "workspace-id",
  "task_id": "task-id",
  "session_id": "session-id",
  "pending_action": "clarification",
  "pending_action_revision": {
    "epoch": "projection-epoch",
    "sequence": 42
  }
}
```

`pending_action` is `clarification`, `permission`, or `null`. The event contains
no message ID, body, raw content, tool metadata, turn transcript, model state,
or shell data. It is emitted after the same authoritative projection read that
currently decorates pending-sensitive message events. A semantic no-op does
not need a duplicate event.

The gateway routes this action with `BroadcastToWorkspaceOrDrop`. The frontend
handler calls the existing revision-aware task-session pending projection
merge. If membership is not loaded yet, the store may retain only the keyed
pending projection until a task-session list supplies the complete row.

## Subscription lifecycle

The UI owns preload and detail activation. The transport contract is:

1. Task-summary hydration creates no session membership or detail request.
2. Preload activation mounts `useTaskSessions(taskId)` and can issue one
   deduplicated list request.
3. Detail activation mounts `TaskChatPanel` for exactly one selected session.
4. `useSessionMessages` registers that session with the WebSocket client and
   fetches its initial transcript.
5. Selecting a sibling unmounts the previous chat before or while mounting the
   replacement. Stable subscription membership prevents equivalent rerenders
   from replaying the initial snapshot.
6. Detail deactivation unmounts the chat and releases the session registration.
7. The compact session rows can remain cached. Lifecycle and pending-action
   events continue to update them without transcript traffic.

On desktop, several intersecting task columns can each own one selected detail
stream. On phone, only the nearest snap target can own one. An adjacent preload
column can fetch its session list but cannot subscribe to a transcript before
it enters the detail window.

## Reconnect and repair

The WebSocket reconnect path refreshes mounted `useTaskSessions` consumers, so
current preload columns rehydrate authoritative compact status. A column that
was offscreen performs the same refresh when it later re-enters preload.

`pending_action_revision` keeps the existing epoch and sequence comparison.
Older cross-channel events cannot overwrite a newer list or event projection.
A missing event can temporarily leave cached offscreen status stale, but the
client does not repair it with a detail subscription. Re-entry list hydration
is the repair boundary.

If a new session emits `session.state_changed` before its task-session list is
loaded, the current event upsert path seeds the compact row and invalidates a
previous complete list. A mounted preload consumer then reloads membership.

## Failure behavior

- A task-session list failure affects only its preloaded task column. Existing
  task shell summaries stay live.
- A compact event with no workspace identity is dropped instead of broadcast
  globally.
- A detail subscribe or transcript request failure uses the selected chat's
  existing retry behavior. It does not subscribe to sibling sessions.
- Rapid horizontal scroll can cancel or supersede React mounts. Stable task and
  session IDs ensure late responses merge into keyed caches without opening a
  detail stream for the now-offscreen column.

## Security boundary

Task-session list authorization remains unchanged. The compact pending-action
event contains no transcript content, but it is still task state and must use
the owning workspace boundary. The backend resolves `workspace_id` from the
authorized task/session relationship and fails closed if the relationship is
missing. The client does not trust URL `sessionId` until the authorized
task-session list proves membership.

## Persistence and migration

No new table or migration is required. `TaskSession.pending_action` and
`pending_action_revision` already exist in the authoritative list projection.
The new event is a delivery path for that existing projection. Task status
summary persistence and its constant-size contract remain unchanged.

## Observability and verification

Gateway tests assert that the compact pending event reaches workspace clients
without reaching a session-only or unrelated workspace client. Payload tests
assert that it contains no message content. Frontend reducer tests assert
revision ordering.

Threads component tests expose detail-active task IDs and mounted conversation
IDs through stable test seams. Browser tests record outgoing
`session.subscribe` and `session.unsubscribe` actions. For a many-column deck,
the initial subscribe set must equal selected sessions in the detail window,
not every task or every sibling tab.

## Related decisions

- [Viewport Activation Owns Threads Session Streams](../../../decisions/2026-08-28-viewport-activation-owns-thread-streams.md)
- [Separate Task Summary and Session Stream Traffic](../../../decisions/2026-08-01-separate-task-summary-session-stream-traffic.md)
