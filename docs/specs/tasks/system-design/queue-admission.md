---
status: draft
system: tasks
requirements:
  - REQ-TASKS-QUEUE-ADMISSION-001
---

# Queue Admission System Design

## Boundary and mapping

The task system owns durable prompt admission. This design extends the existing queue transport and identity guards.

| Criteria (AC-TASKS-QUEUE-ADMISSION-001) | Sections |
| --- | --- |
| .1, .2, .3, .7 | Server admission, Persistence |
| .4, .5 | Client recovery |
| .6, .8 | Composer feedback |

## Server admission

`QueueHandlers.wsQueueMessage` already accepts optional `client_queue_id`. Preserve this field rather than adding a parallel `client_message_id`.
Validate its length for every identified admission. Keep legacy unidentified callers on the existing path.
Plan-comment admissions retain their existing transaction and merge exclusion.

For ordinary identified requests, add a typed admission request to the messagequeue service.
Capture the immutable `QueueSessionIdentity`, normalized payload, and caller identity. Fingerprint content, model, plan mode, attachments, context, references, and admission options.
Exclude request IDs, timeouts, and transport timestamps. Use the existing canonical fingerprint utility.

Under the existing task/session admission guards, validate authorization and incarnation before reading any receipt.
An exact receipt replay returns acceptance without changing queue state. A mismatched fingerprint returns a typed conflict.
Replay lookup precedes capacity evaluation and attachment claim preparation that can reject already-claimed uploads.
Mutable reference validation applies to a first admission. Exact replay must not fail because an accepted reference later changed.

For a new admission, keep the existing insertion, full-capacity candidate fold, and staged attachment rules.
Write the receipt in the same transaction as the accepted insertion or fold and any attachment ownership changes.
A transaction failure commits neither content nor receipt. A database uniqueness constraint closes concurrent replay races.
Do not implement a separate receipt write after queue commit or a read-then-insert deduplication check outside the transaction.

Do not put each request ID into merge compatibility metadata. Ordinary entries must retain their current compatibility behavior.
The earlier tail still survives automatic merging. A receipt identifies admission, not the continued existence of a visible source row.
Post-admission readiness checks retain current behavior and cannot convert committed acceptance into rejection.

## Persistence

Add a queue-owned `queue_admission_receipts` table through `repository_sqlite.go` initialization and a focused repository helper.
The shared SQL implementation must work with SQLite and PostgreSQL using established rebinding and lock conventions.

Use `(task_id, session_id, session_incarnation_id, client_queue_id)` as the receipt key.
Store the normalized request fingerprint, accepted queue ID, bounded response snapshot, and acceptance timestamp.
The response retains the current `QueuedMessage` shape. Omit inline attachment data from stored and replayed snapshots.
Clients already hold the submitted attachment payload and refresh authoritative queue state separately.

Retain receipts after manual edits, merges, removal, clearing, reservation, and dispatch. These operations do not reverse admission.
Session/task deletion purges associated receipts through existing lifecycle transactions. Reset invalidates old incarnation requests before replay lookup.
Session transfer must not retarget a receipt. Reject an obsolete original identity under current ownership guards.
Do not add a time-based expiry that allows a delayed duplicate within a retained session.
Existing rows need no backfill. Historical unidentified requests have no replay guarantee.

## Client recovery

`useMessageHandler` passes its existing `clientAdmissionId` for every queue submission, not only plan-comment submissions.
`useQueueAdmissionAction` preserves that ID and treats acknowledged admission as successful even if its terminal queue refresh fails.
`queueMessage` keeps an explicitly supplied ID. It does not generate a replacement ID inside retry handling.

Use a shared request closure with numeric timeout `10000`, or `30000` for nonempty attachments, for both attempts.
Classify timeout and connection closure using existing transport errors. Explicit server rejection is not a transport failure.
Keep existing queue/transcript reconciliation as an optimization. Bound ordinary reconciliation to one queue read and one newest transcript page per pass.
Each read has a 5-second timeout. Wait at most 3 seconds for reconnection before one replay-safe retry.
After retry failure, one bounded reconciliation pass can confirm acceptance. Otherwise propagate typed rejection or uncertain delivery.
The server receipt, not exhaustive transcript scanning, supplies deduplication when the original row has disappeared.
Preserve existing plan-comment recovery coverage while extracting any shared helper.

A returned replay snapshot must not insert a phantom pending row into the store.
Use the existing authoritative queue refresh and incarnation-aware mutation token.
After unresolved failure, preserve the draft. A later deliberate Send remains a new admission and can duplicate an earlier uncertain submission.
Feedback must not claim otherwise. Persistent draft retry identities are outside this package.

## Composer feedback

Keep typed queue error codes through the API boundary. Map capacity, validation, identity conflict, and unavailable-session errors to localized rejection copy.
Do not display raw server diagnostics as translated UI copy. Preserve existing plan-comment-specific normalization.
Only an unresolved transport outcome uses the existing uncertain-delivery message.
Use shared failure presentation for Task chat and Quick Chat. Audit the passthrough composer if it receives the same error types.

No new controls or layout are needed. Reuse the current toast, composer, and `chat-input-toolbar-mobile.tsx`.
The phone Task layout and Quick Chat retain their scroll ownership, safe-area spacing, and 44-pixel Send target.
The plan's UI-01 preview defines the affected error state. Add desktop and phone rendered regression coverage.

## Security and observability

Apply existing queue authorization before receipt lookup. Never disclose another task's receipt by a guessed key.
Fingerprinting and receipt handling must not log prompt content, attachment bytes, or credentials.
Use bounded structured fields for initial acceptance, replay, conflict, and uncertain client recovery.
No new metrics, high-cardinality labels, or global transport logging are required.

## Related decisions and implementation

- [Durable admission receipts](../../../decisions/2026-09-14-durable-queue-admission-receipts.md) records the proposed persistence choice.
- [Server-owned Auto-run](../../../decisions/2026-08-16-server-owned-queue-auto-run.md) remains authoritative for dispatch.
- [Implementation plan](../../../plans/queue-admission-reliability/plan.md)
