---
status: current
system: office
requirements:
  - REQ-OFFICE-UNREAD-DIVIDER-001
---

# Office: Slack-Style Unread Divider System Design

## Purpose and boundaries

Office owns the session-global read cursor and the visit-scoped unread boundary.
The task transcript consumes that boundary as one of its scroll-placement
owners. This design covers the backend cursor write, frontend request
correlation, local cache update, divider lifecycle, and diagnostic events.

The generic transcript auto-scroll coordinator remains owned by the
[UI transcript auto-scroll design](../../ui/system-design/transcript-auto-scroll.md).
It must honor an active unread-divider target, but it does not own whether that
target is current.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-OFFICE-UNREAD-DIVIDER-001` | [Cursor persistence](#cursor-persistence), [Frontend request correlation](#frontend-request-correlation), [Visit and placement flow](#visit-and-placement-flow), [Observability](#observability) |

## Cursor persistence

`task_sessions.last_read_message_id` stores the newest message seen for each
session. `POST /api/v1/task-sessions/:id/mark-read` validates that the message
belongs to the session and calls `UpdateTaskSessionLastReadMessageID`. The
repository compares message `(created_at, id)` order so delayed requests can
advance, but never regress, the persisted cursor.

The response contains only `session_id` and `last_read_message_id`. The web
client applies it through `updateSessionReadCursor`; it never merges a full
session snapshot over newer WebSocket state.

## Frontend request correlation

`useSessionReadTracking` snapshots the cursor before dispatching the live
mark-read request. Request freshness is scoped by session, not by the hook's
currently rendered session or component lifetime.

The frontend keeps one latest-request generation for each session with a
request in flight. A response can update the local cursor only when its
generation is still the latest generation for that same session. A request for
task B cannot supersede task A's response. A later request for task A does
supersede an earlier task A response, including when the earlier component was
hidden or unmounted.

Settling the latest response removes its in-flight generation. Older responses
remain stale after that removal and cannot regress the local cursor. This keeps
the coordinator bounded by active requests rather than all sessions ever
opened.

## Visit and placement flow

When a chat panel becomes visible, `useSessionReadTracking` captures the local
cursor before advancing it. After the initial message window is ready, the hook
freezes a divider anchor only when the captured cursor precedes the rendered
tail. The mark-read request advances the backend and local cursors independently
of that visit-scoped anchor.

When the user leaves and returns, the next visit reads the newest local cursor.
If the prior visit reached the tail, the next visit is divider-ineligible and
normal enabled transcript placement selects the bottom. This remains true when
the successful mark-read response arrived while another task was active.

If a genuine unread divider exists, `message-list-native.tsx` owns the one-time
divider placement and `message-list-native-scroll.ts` delegates initial bottom
placement to it. Running-agent message or work-state updates may cause later
bottom-follow writes, but those writes must not be required to repair a stale
completed-session cursor.

## Failure and recovery

- A failed mark-read request leaves the local cursor unchanged and reports the
  existing error. A later visible render can retry it.
- An out-of-order response for the same session is ignored by the frontend and
  cannot regress either the local or persisted cursor.
- A successful response received while another task is active still updates
  its own session's local cursor when no newer request for that session exists.
- A task/session switch, panel hide, or component unmount does not invalidate a
  still-current response merely because visibility changed.

## Responsive behavior

Desktop Dockview and the mobile task layout use the same read-tracking hook,
session store, and native transcript coordinator. The transcript stays the only
vertical scroll owner. This repair changes no navigation, layout, touch target,
safe-area, or user-facing copy.

## Observability

Development diagnostics use `createDebugLogger` and carry `sessionId` so the
debug logger can append the owning task ID. The `messages:read-tracking`
namespace records visit capture, mark-read dispatch, response apply, stale
response discard, and failure. It includes request generation and message IDs,
but never message content.

The existing `messages:scroll-placement` namespace also records when initial
placement delegates to another owner and when a running-state or message update
performs the bottom write. Logs are emitted once per lifecycle decision or
actual write, not per render or scroll event. Together the two namespaces show
whether a repeated divider came from a stale cursor or whether a later running
agent update only masked failed initial placement.

## Verification boundaries

- Hook tests hold task A's response, dispatch task B's request, and prove both
  sessions apply their own latest cursor without allowing same-session
  out-of-order regression.
- Native scroll tests prove the unread divider owns initial placement and that
  running-state/message writes are distinguishable from initial placement.
- Desktop and mobile Playwright tests use completed tasks, delay task A's
  mark-read response across a task switch, return to task A, and prove the stale
  divider does not move the transcript away from the bottom.

