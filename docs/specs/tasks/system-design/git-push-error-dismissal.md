---
status: current
system: tasks
requirements:
  - REQ-TASKS-GIT-PUSH-ERROR-DISMISSAL-001
created: 2026-09-25
owners:
  - kandev
---

# Git Push Failure Dismissal System Design

## Purpose and boundaries

The task system owns the session message and its durable metadata. This design adds manual acknowledgment to the existing Git push error message and its shared chat card. The workspace Git state remains owned by the workspace system; dismissal does not inspect or change it. The existing session-level `last_agent_error` dismissal is a separate contract and does not own this message.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-GIT-PUSH-ERROR-DISMISSAL-001` | Components and responsibilities, data and contracts, control flow, failure and recovery, persistence, security |

## Current behavior

The gateway persists a Git push failure as a task-session `error` message carrying the diagnostics and Fix action. A later successful push does not update or retract that message, and chat filtering does not treat its Git metadata as resolved state, so the stored footer card remains actionable. The design adds a separate manual acknowledgment on that exact row; it does not infer repair from later Git activity.

## Components and responsibilities

- `apps/backend/internal/backendapp/gateway.go` continues to create one `error` message for each failed Git operation, with `git_operation_error`, `operation`, `error_output`, and the existing Fix action. No push-status reconciliation or creation-path rewrite is required.
- `apps/backend/pkg/websocket/actions.go` and `apps/backend/internal/task/handlers/message_handlers.go` add the accepted client action, `message.dismiss_git_push_error`, whose payload contains only `message_id`.
- `apps/backend/internal/task/service/service_messages.go` owns the message-specific dismissal operation, validates that the addressed row is a Git push error, and delegates the first-write decision to a session-serialized repository transaction before publishing one update event for the winning write.
- `apps/web/lib/utils/git-push-error-message.ts` defines the strict Git push error and dismissed-message predicates used by the visibility filter and shared renderer.
- `apps/web/components/task/chat/messages/action-message.tsx` adds a client-side Dismiss control only when the row has `type: error`, `git_operation_error: true`, and `operation: push`; it does not depend on a new persisted action entry, so existing messages remain dismissible. The dismissal-specific handler owns pending state, localized failure feedback, retry behavior, and application of the persisted success response to the existing client message.
- `apps/web/hooks/processed-message-filtering.ts` removes a persisted dismissed row before it is split into regular transcript messages and footer action messages; `apps/web/components/task/chat/message-renderer.tsx` also suppresses that row for direct callers, covering both action-card and status-message rendering.
- `apps/web/components/task/chat/messages/action-message-actions.tsx` keeps the existing Fix action path unchanged; this capability does not refactor generic `StandardActionButton` error handling.
- `apps/web/lib/ws/handlers/messages.ts` already applies `session.message.updated` by message ID. The Dismiss handler patches the existing row identified by the clicked message's original session and message ID from the success response, so a session switch during the request cannot redirect the acknowledgment and an active view converges when the best-effort broadcast is missed.
- `apps/web/lib/state/slices/session/session-slice.ts` keeps a nonempty dismissal timestamp when merging later message updates, so a delayed broadcast without the persisted marker cannot make the acknowledged card visible again.

## Data and contracts

The accepted WebSocket request is `message.dismiss_git_push_error` with payload `{ "message_id": "<message-id>" }`. The server loads the message by ID and derives its task and session from the stored row; the request does not accept caller-supplied task, session, metadata, or timestamp values. The success response payload is exactly `{ "message_id": "<message-id>", "dismissed_at": "<UTC RFC3339Nano timestamp>" }`, wrapped in the existing WebSocket response envelope for the request ID and action. This acknowledgment contains no diagnostics. Standard WebSocket error responses report rejection or persistence failure without returning diagnostic content.

The service loads the message and its stored session association by `message_id`. It rejects a sessionless row, then uses the authenticated WebSocket context and `AuthorizeSessionAccess` to authorize that actual session before validating message eligibility. This follows the existing informational-error acknowledgment contract. Only after authorization does it require `type: error`, `git_operation_error: true`, and `operation: "push"`. The repository then serializes the current-row read and first-write decision with the session's conversation mutation lock. The first accepted dismissal adds `git_operation_error_dismissed_at` to that message's existing metadata using a UTC timestamp. It leaves `content`, `error_output`, `actions`, and all other diagnostic metadata intact. Concurrent and repeated requests return the original stored timestamp without another write or update event.

The frontend supplies a localized Dismiss action through the existing `ws_request` path using the exact `Message.id`. The label reuses `task:dismiss`, which already exists in all six supported locales. The existing `common:requestFailed` key is available in those catalogs for the save-error toast, so this change needs no new locale entries. Dismiss is generated by the UI from strict message metadata rather than persisted in the row's `actions` array, preserving compatibility with existing failure messages.

## Control flow

1. The existing Git failure callback creates the diagnostic message and Fix action; existing rows use the same `type: error`, Git push metadata, and action shape.
2. The shared action card recognizes a Git push error from strict metadata and adds Dismiss for that message ID. The card uses the current action layout: buttons stack on phone and align in a row on desktop. The client does not hide the row optimistically; it waits for a success response that proves persistence or for the existing message update.
3. The handler passes only `message_id` to the task service. The service loads the row, rejects a missing session association, authorizes the stored session with `AuthorizeSessionAccess`, then validates the stored message type and Git push marker before adding the dismissal timestamp.
4. A session-serialized repository transaction reads the current row, writes the timestamp only if no acknowledgment exists, and returns the authoritative message plus conversation receipt. The winning request publishes the existing `session.message.updated` broadcast; overlapping requests return the winner's timestamp without another write or event.
5. The success response patches the same message in the client store under the session captured by the clicked card with the authoritative timestamp. The normal WebSocket update does the same when delivered, and store merges retain an already acknowledged timestamp if a delayed event omits it. Visibility filtering removes the row before transcript/footer separation, and the shared renderer suppresses direct render calls.
6. Reload, reconnect, or returning to the task reads the same persisted row and keeps it hidden. A later failure has a different message ID and remains visible; an unrelated error never meets the action's strict server-side validation.

## Failure and recovery

If the message is missing, sessionless, belongs to a session the caller cannot read, or is not a Git push error, the service makes no update and publishes no event. If the metadata write fails, the handler returns the action error without a dismissal timestamp; the dismissal-specific handler shows `toast.error(t("common:requestFailed"))` using the existing `@/lib/toast/sonner` and `@/lib/i18n` helpers, following the same pattern in `apps/web/components/task/chat/messages/use-permission-handlers.ts`. It clears pending/disabled state in `finally`, leaves the complete card and both Fix and Dismiss controls visible and retryable, and does not optimistically hide the row. After a success response, it applies the returned timestamp to the current store row; this recovers immediately when the persisted update's best-effort broadcast is missed. A focused retry test proves that a failed request shows feedback and leaves the card intact, then a retry can persist dismissal and hide it without relying on the broadcast. The existing generic `StandardActionButton` error path remains unchanged.

## Persistence

The existing message repository stores metadata in the message row's JSON metadata field. Adding `git_operation_error_dismissed_at` requires no schema migration, new table, task-session metadata projection, or diagnostic copy. The repository's narrow metadata first-writer uses the existing conversation mutation transaction and receipt, changes only metadata and `updated_at`, and returns the current row unchanged when another request already stored a non-empty timestamp. The service publishes `session.message.updated` only for the winning write.

## Security

The service derives the session from the stored message ID and authorizes that session with `AuthorizeSessionAccess`, which requires `authz.ScopeWorkspaceRead`, in addition to the authenticated WebSocket connection checks. This matches the existing informational-error acknowledgment contract. Dismissal is a shared acknowledgment for a task reader, not a generic message-metadata mutation permission; the narrow action still rejects sessionless rows and validates the exact Git push error shape after authorization. No error output is copied into the action response or toast.

## Observability

The design adds no metrics or logs. Existing message update behavior remains the source of live client notification.

## Related decisions

- [ADR: Isolate Replaceable Session Stream Traffic](../../../decisions/2026-08-02-isolate-replaceable-session-stream-traffic.md) defines `session.message.updated` as replaceable full-state delivery; the persisted message remains authoritative if an intermediate update is missed.

## Contract boundary

`message.dismiss_git_push_error` is one new public WebSocket action because `UpdateMessage` is service-internal and no generic message metadata update action exists. Dismissal is a shared acknowledgment for authorized task readers, not a general right to mutate message metadata.
