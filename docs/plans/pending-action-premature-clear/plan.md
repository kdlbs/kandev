---
spec: docs/specs/platform/bounded-task-status-delivery.md
created: 2026-08-08
status: completed
---

# Implementation Plan: Fix Premature Pending-Permission Clear

## Overview

Fix the sidebar/kanban "pending permission" shield icon
(`IconShieldQuestion`, `task-state-pending-permission`) flashing on and
disappearing immediately, even though the permission request was never
actually answered.

## Root Cause

`apps/backend/internal/task/statussummary/projector.go` derives each
session's `pending_action` (`permission` / `clarification` / absent) from two
independent event-bus subscriptions that both write `state.pending[sessionID]`:

1. `applyPermissionEventLocked` (~line 333) arms `pending_action = permission`
   immediately off the raw `PermissionRequestReceived` bus event.
2. `applyPendingMessageLocked` (~line 698) is meant to disarm it once the
   request's own message resolves. Its gate,

   ```go
   isTrackedMessage := isPendingMessage || stringField(metadata, "pending_id") != "" || status != ""
   ...
   if eventType == events.MessageDeleted || (status != "" && status != statusPending) {
       return p.clearPendingLocked(state, sessionID)
   }
   ```

   treats **any** message on the session with a non-empty, non-`"pending"`
   `metadata.status` as a resolution — not only the permission/clarification
   request's own row. Ordinary `tool_execute` / `tool_edit` / `tool_read` /
   `script_execution` messages all carry a `status` field as part of normal
   streaming (`"running"` → `"completed"`). `clearPendingLocked`
   (`projector_helpers.go` ~line 11) then unconditionally
   `delete(state.pending, sessionID)`s — wiping whatever was armed for that
   session, permission or clarification, regardless of which message
   triggered it.

   Because a busy session is constantly emitting these ordinary tool-status
   messages, the very next one after a permission request arms the flag
   disarms it again within a second or two — before the user ever sees or
   answers it. Confirmed empirically against a live task's DB
   (`task_session_messages`): zero `type='permission_request'` rows were ever
   persisted for the affected task's sessions, proving the observed shield
   flash did not correspond to a real, answered permission request.

Existing test coverage (`TestProjectorClearsPendingWhenRequestMessageResolves`
in `apps/backend/internal/task/statussummary/projector_test.go`) only exercises
resolution via the request's *own* message type, so it does not catch an
unrelated message clearing the flag. The comment above the clear branch
(lines 709-715) documents the original, narrower intent — "gating the clear
on 'not a request message' left the task-list affordance armed after the user
had already answered" — but the fix dropped type-scoping entirely instead of
matching the specific resolved request.

## Backend

### Scope the pending-clear to the request's own message

Update `applyPendingMessageLocked` in
`apps/backend/internal/task/statussummary/projector.go` so the terminal-status
clear only fires when the triggering message is the request that armed
`pending_action` for that session. The projector records that request's
normalized type and `pending_id` in a session-scoped `pendingRequests` map when
the permission event or request message arms the action. The terminal/deletion
branch must require both the request-shaped `isPendingMessage` predicate and an
exact identity match before clearing. The current clear branch uses the
broader `isTrackedMessage` (which also matches on bare `pending_id` metadata or
any non-empty `status`) and has no request identity check:

```go
// current (too broad):
if eventType == events.MessageDeleted || (status != "" && status != statusPending) {
    return p.clearPendingLocked(state, sessionID)
}

// fix: require a request-shaped message and the identity that armed the action
if isPendingMessage && state.pendingRequests[sessionID] == requestIdentity &&
    (eventType == events.MessageDeleted || (status != "" && status != statusPending)) {
    return p.clearPendingLocked(state, sessionID)
}
```

Keep `isTrackedMessage` as-is for the early-return gate (`state.pendingObserved`
bookkeeping still applies to any tracked message). Clear the stored identity
alongside the action in `clearPendingLocked` and direct pending-action updates.
This makes arm and clear symmetric: the same request type and `pending_id`
that armed `state.pending[sessionID]` are required to clear it.
Preserve existing behavior for:

- `events.MessageDeleted` on the matching request row (still clears).
- The detached-but-answerable clarification case
  (`TestProjectorKeepsPendingWhenRequestStaysAnswerable`): a `status: "pending"`
  update must still keep the affordance armed.
- A terminal update or deletion for a different request type or `pending_id`
  must leave the armed request untouched.
- Non-request tracked messages (bare `pending_id` metadata, or `requests_input`
  without a recognized request type) that were previously exercised by
  existing tests — keep their current arm/no-op behavior; only the
  clear branch's message-type gate changes.

Keep the clarification-event branch in `applySourceEventLocked`
(`events.ClarificationAnswered` etc. already clear correctly since they carry
their own session-scoped clear, independent of message type) unchanged beyond
removing the stored request identity when the shared clear helper runs.

## Tests

- **What:** an unrelated tool-status message on the same session must not
  clear an armed `permission`/`clarification` `pending_action`.
  **File:** `apps/backend/internal/task/statussummary/projector_test.go`.
  **How:** new regression test — arm via `PermissionRequestReceived`
  (or the clarification `MessageAdded` pattern already used in
  `TestProjectorClearsPendingWhenRequestMessageResolves`), then publish an
  unrelated `events.MessageUpdated` (or `MessageAdded`) for a
  `tool_execute`-typed message on the same session with
  `metadata: {"status": "completed"}` and a *different* `pending_id`
  (or none), and assert `PendingAction` is still armed. Then resolve a
  request-shaped row with a different `pending_id` and assert it remains
  armed, before resolving the matching request row and asserting it clears —
  reusing the existing
  `TestProjectorClearsPendingWhenRequestMessageResolves` table is the
  intended integration point (add a case, or a sibling test using the same
  `newProjectorTest`/`publishProjectorEvent`/`publishSessionState` helpers).
- **What:** existing regression suite for this package must keep passing
  unmodified in behavior:
  `TestProjectorClearsPendingWhenRequestMessageResolves`,
  `TestProjectorKeepsPendingWhenRequestStaysAnswerable`, and the rest of
  `apps/backend/internal/task/statussummary`.
  **How:** `go test ./internal/task/statussummary/...`.

## Implementation Waves

Wave 1 (sequential by default):

- [x] [task-01-scope-pending-clear-to-request-message](task-01-scope-pending-clear-to-request-message.md)

## Open Questions

None.
