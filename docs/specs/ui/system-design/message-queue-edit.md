---
status: draft
system: ui
created: 2026-09-02
owners:
  - kandev
requirements:
  - REQ-UI-MESSAGE-QUEUE-MANAGEMENT-002
---
# Queued Message Editing System Design

## Context and boundaries

The queue service owns pending-entry identity, FIFO admission, reservation, and
mutation. The UI owns editor presentation and WebSocket request coordination.
This design covers the cross-surface contract for editing a user-owned pending
entry. It does not change agent, workflow, or server provenance rules, and it
does not define the separate Send Now operation.

The queue's existing per-session admission boundary is the serialization point
for queue mutations and automatic delivery. The edit hold is target-scoped: it
must not become a session-wide Auto-run pause.

## Components and responsibilities

- `internal/orchestrator/messagequeue.Service` owns edit leases, target
  revisions, operation fencing, lease expiry, and the interaction between an
  edit hold and automatic head reservation.
- `internal/orchestrator/handlers.QueueHandlers` authorizes the session before
  reading or mutating queue state, binds edit requests to the server-assigned
  WebSocket connection ID, translates typed queue conflicts to stable errors,
  and requests a post-save auto-run drain only after the edit lease is ended.
- `internal/orchestrator.Service` owns promptability checks and exposes the
  policy-preserving queue drain used after a successful edit save. This drain
  may reserve the FIFO head only when Auto-run is already enabled; it never
  enables Auto-run as a side effect.
- `internal/gateway/websocket.Client` places its connection ID in the dispatch
  context. The client never accepts a connection identity from a payload.
- `apps/web/lib/api/domains/queue-api.ts` carries begin, renew, end, and fenced
  update requests. `apps/web/hooks/use-queue-edit-protection.ts` owns the
  editor lease lifecycle. `queued-ghost-list.tsx` and
  `queued-ghost-message.tsx` keep the editor attached to the selected row and
  identify successful saves separately from cancellations.
- `apps/web/hooks/domains/session/use-queue.ts` reconciles mutation responses
  and queue status for the session captured by the action. Stale HTTP/WS
  responses must not overwrite a newer session snapshot.

## Data model and contracts

A lease is keyed by `(session_id, entry_id)` and contains a server-generated
lease ID, the entry's target revision at acquisition, an expiry timestamp, a
monotonic lease generation, and the server-side connection binding. Lease
connection identity is not exposed to the browser.

The queue edit end request may carry `dispatch_if_auto_run: true` only for a
successful save. The backend releases the validated lease first, then invokes
the policy-preserving drain for the same session. Cancel, failed save, stale
lease cleanup, and disconnect cleanup omit the field or set it false. The
response remains the existing session/entry acknowledgement; any dispatched
successor is observed through the existing queue and session events.

The queue update contract includes `lease_id`, `operation_id`, and
`expected_target_revision` when sent by a connected editor. The server returns
the resulting target revision and operation ID. The operation ID is scoped to
its live lease and its request content, attachments, and metadata. Repeating an
identical operation returns the original revision without applying the update
again. Reusing an operation ID with different content is rejected.

The ordinary compatibility update path remains available only for trusted
non-WebSocket callers. Browser queue edits must acquire and retain a live
target lease, pass its fencing fields on save, and refuse the save action when
the lease is absent or has ended. The browser must never send an unfenced queue
edit request.

## Control flow and state transitions

1. The editor requests `message.queue.edit.begin` for one visible user-owned
   row. The service checks the row under the session admission lock and creates
   one target lease without changing Auto-run.
2. Automatic queue advancement continues for other entries. A policy-aware head
   reservation skips only while the current visible head is the leased target;
   it may reserve earlier entries normally.
3. The editor renews the lease before expiry. Each renewal checks both lease ID
   and connection binding and advances the lease generation.
4. Save validates the lease, connection, target revision, and operation ID in
   one admission-serialized operation. It claims any newly referenced staged
   attachments only after request preconditions that can reject the save have
   passed. The content, attachments, and entity-reference metadata replacement
   then commits atomically. Superseded attachment claims are released only
   after a successful replacement; rejected saves release only claims made by
   that request.
5. After a successful save, the UI ends the lease with
   `dispatch_if_auto_run: true`. The backend validates and releases that lease
   before attempting the policy-preserving drain. The drain serializes against
   cancellation and other queue dispatches, rechecks promptability, and
   reserves the FIFO head only when Auto-run is already enabled.
6. Cancel, failed save, lease loss, disconnect cleanup, and expiry end the lease
   without requesting a drain. A drain, remove, merge, session transfer, or
   replacement that wins first invalidates the target and causes the editor to
   refetch instead of applying stale state.
7. Delayed cleanup from the old connection is fenced by lease ID, connection ID,
   and session/entry key, so it cannot release a newer lease or trigger a
   successor turn after a later edit.

Session transfer and queue snapshot replacement must participate in the same
admission boundary as editing and invalidate leases for affected source and
destination entries. This prevents a transfer or restore from racing a stale
save and prevents a lease from surviving under an obsolete session identity.

## Failure and recovery behavior

- A second begin for the same target returns an edit conflict. A begin for a
  missing, reserved, or non-user row returns the non-enumerating not-found
  outcome.
- A stale lease, foreign connection, expired lease, or mismatched revision
  returns a typed edit conflict. The UI clears local edit state and fetches the
  authoritative queue; it does not apply an optimistic replacement.
- A duplicate operation with the same operation ID and request hash is
  successful and returns the original revision. A hash mismatch is rejected
  without a second mutation.
- Validation, lease, reference, or attachment-claim failures before the queue
  mutation release only attachments newly claimed by that request. Existing
  retained attachments remain owned by the queue.
- A requested post-save drain is never an implicit resume: Auto-run OFF,
  promptability guards, clarification, cancellation, or an in-flight dispatch
  leave the saved row pending for the next eligible trigger.
- If lease release succeeds but the post-save drain cannot dispatch, the saved
  row remains durable and the existing queue status event is authoritative; a
  later agent-ready or explicit Auto-run trigger may deliver it.
- Status events remain authoritative for every successful update. Reconciliation
  is scoped to the session and uses the existing request-generation guard so an
  older response cannot restore stale content after a newer event or session
  switch.

## Permissions and security

Session authorization runs before queue reads for all edit actions. The server
uses its connection-bound identity rather than payload data for lease fencing.
The post-save dispatch flag is honored only when the live lease records a
successful update for that target; a browser cannot turn a cancel or failed
save into a delivery request. A browser may submit only session ID, entry ID,
lease ID, operation ID, target revision, content, and replacement metadata. It
cannot choose `queued_by`, task ownership, or another connection's lease.
## Responsive behavior

The existing inline queue panel remains the mobile composition and its queue
list remains the only internal scroll owner. Edit, save, and cancel controls
remain discoverable without hover on coarse pointers, use at least 44 by 44 CSS
pixel hit areas where the shared row/action pattern requires them, and do not
introduce document-level horizontal overflow. Desktop keeps the compact row
layout. Both surfaces use the same lease and reconciliation state.
## Observability and tests

Queue edit conflicts, expired leases, rejected revisions, post-save drain
deferrals, and attachment rollback failures should retain the existing
structured queue-handler/service logging without logging message content or
attachment data. Focused backend coverage belongs beside the message-queue
service and handler tests and must cover save-after-turn-completion, Auto-run
OFF, cancel, and failed-save paths. Frontend unit coverage covers lease
lifecycle, operation fencing, and successful-save versus cancellation
completion. Playwright coverage must exercise desktop and `mobile-chrome`
edit/save behavior, including an Auto-run backlog and a session/view
replacement scenario.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-MESSAGE-QUEUE-MANAGEMENT-002` | Data model and contracts; Control flow and state transitions; Failure and recovery behavior; Responsive behavior |
