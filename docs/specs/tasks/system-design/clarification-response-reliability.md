---
status: current
system: tasks
requirements:
  - REQ-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001
  - REQ-TASKS-CLARIFICATION-LIFECYCLE-001
---

# Clarification response reliability System Design

## Purpose and boundaries

This design removes transcript-size-dependent scans from clarification
response resolution and bounds the user-visible request lifecycle. It extends,
but does not change, the authority and recovery rules in
[Active clarification lifecycle](clarification-active-lifecycle.md).

The task repository owns the indexed pending-ID lookup and atomic claim. The
clarification resolver owns phase deadlines and idempotent outcome
reconstruction. The HTTP handler owns the retryable wire result. The shared web
clarification hook owns the client deadline and desktop/phone recovery state.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001` | [Pending-ID access path](#pending-id-access-path), [Bounded response flow](#bounded-response-flow), [Client recovery](#client-recovery), and [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- `internal/task/repository/sqlite` remains the shared SQLite/PostgreSQL task
  repository. It creates the additive lookup index and keeps the query
  expression text identical to the indexed expression.
- `internal/db/dialect` supplies the driver-specific JSON expression and index
  DDL so SQLite and PostgreSQL cannot drift.
- `internal/clarification.Resolver` uses one five-second pre-claim context for
  identity lookup, validation reads, and the atomic claim. Existing bounded
  delivery and durable recovery continue after a successful claim.
- `internal/clarification.Handlers` maps an exhausted internal pre-claim budget
  to a stable retryable REST result.
- `useClarificationGroup` applies a 40-second client deadline, preserves the
  submit-time bundle and answers, and reuses the existing idempotent response
  contract for Retry.
- `ClarificationStatusBanner` and the existing overlay actions render one
  shared recovery state on desktop and phone.

## Pending-ID access path

### Index shape

Add `idx_messages_metadata_pending_id_lookup_ordered` without removing
`idx_messages_metadata_pending_id` or the existing bare pending-ID lookup
index `idx_messages_metadata_pending_id_lookup`.

The new B-tree index has these keys, in order:

1. The driver-specific extracted `metadata.pending_id` expression.
2. `created_at`.
3. `id`.

The index includes only rows whose extracted pending ID is non-null. Equality
on the leading expression isolates one small interaction bundle. The remaining
keys serve deterministic bundle ordering. The existing session-ID-leading
index remains available for session-scoped access paths. The existing bare
pending-ID lookup index remains available for compatibility with installations
that already created it; the ordered partial index uses a distinct name so
startup can add it without attempting to alter an existing index definition.

SQLite uses `json_extract(metadata, '$.pending_id')`. PostgreSQL uses
`metadata::jsonb->>'pending_id'`. The lookup, claim, malformed-bundle guard,
and claimed-bundle reload must use the same dialect helper so their expression
matches the index definition exactly.

### Initialization and upgrade

The index is created by the existing ordered repository initialization path
with `CREATE INDEX IF NOT EXISTS` and a new name. A fresh database and an
existing database therefore converge without a row rewrite or a migration
version flag. Index creation is a startup-critical operation. A failure prevents
the backend from admitting work.

SQLite creates the index during its single-process startup after the message
table exists. PostgreSQL uses the same replay-safe startup step and a native
expression index. Kandev currently supports one backend replica and documents
stop-the-writer upgrades, so regular index creation is consistent with the
supported deployment contract. This design does not add concurrent index
creation, advisory coordination, or invalid-index repair.

Repository tests must prove fresh creation and replay for both dialects. Query
plan tests must prove that pending-ID-only reads and claims are eligible for the
new index with a large set of unrelated messages. PostgreSQL planner evidence
uses the environment-gated real PostgreSQL test suite rather than translating
SQLite `EXPLAIN` output.

## Bounded response flow

The response lifecycle has two budgets:

1. **Pre-claim budget, five seconds.** `ResolveBundle` derives a fresh bounded
   context. It uses this context for identity resolution, authorization inputs,
   validation reads, and `CompleteActiveClarificationBundle`. Exhaustion before
   a durable claim becomes a typed retryable error.
2. **Post-claim delivery budget, existing 30 seconds.** Live delivery
   confirmation, detached resume acknowledgement, compensation, and durable
   outcome reconstruction keep their existing ownership and timeout rules.

The client deadline is 40 seconds. It bounds the interface even if the backend
or an intermediary does not return. The durable recovery contract makes this
possible because a later Retry can reconstruct a committed winner.

`POST /api/v1/clarification/:pendingId/respond` adds one error outcome:

- HTTP 503, `{ "error": "clarification response is temporarily unavailable",
  "code": "temporarily_unavailable" }`, when the internal pre-claim budget
  expires.

Existing 200, 400, 404, 409, and unexpected 500 semantics remain unchanged.
The handler must distinguish the internal budget from caller cancellation. A
disconnected caller does not require a best-effort response write.

## Client recovery

`postClarification` owns an `AbortController` timer for the 40-second client
deadline and clears it on every completion path. A timeout, network error, HTTP
503, unexpected 5xx, or malformed 200 response body produces the existing
`error` submission state. The hook retains the selected answers and last
action. It releases its in-flight guard and restores normal overlay
interaction. A malformed response is never treated as an authoritative
success or used for an optimistic update.

Retry sends the same action through the response endpoint. The existing
resolver loser path is the authoritative reconciliation: it returns a prior
winner when the first request committed, or claims the still-pending bundle
when the first request did not. A 409 `not_active` remains an expired outcome,
not a successful submission.

### Inactive-response reconciliation and late answers

This section also maps `REQ-TASKS-CLARIFICATION-LIFECYCLE-001`, criteria
`.4` through `.9`. The [late-answer decision](../../../decisions/2026-09-18-late-clarification-messages.md)
separates operational tool authority from ordinary conversation.

`useClarificationGroup` captures question prompts, option labels, custom text,
answers, task ID, and session ID before submitting. An affirmative response
classified as inactive transfers that snapshot to a shared late-message action.
It does not clear the draft before message admission succeeds. Rejection instead
retires the obsolete surface without sending an ordinary message.

Use the existing `useMessageHandler` admission path for late messages. Format
question prompts and selected labels with custom answers as ordinary user text.
Do not interpolate them into a privileged system block. A directly opened
historical question uses this path without retrying the obsolete response endpoint.
A recognized inactive result after an affirmative submit uses the same path
without requiring the user to enter the answers again.

A typed host callback connects the shared question UI to message admission.
Resolve its task/session from the source question, not global active selection.
Retain the captured identity across navigation. Task chat and Quick Chat wire
this callback through their existing chat handler. A transcript question opens
an inline late-answer form using the same formatter and admission adapter.
Read-only run surfaces link to the source conversation rather than acquire a
second message transport. The Inbox History tab remains read-only; its existing
conversation navigation provides access to the transcript action.

Refresh message/session authority after an inactive result before deriving
promptability. The stale question itself must not force `hasPendingClarification`
to true and queue an otherwise idle session forever. A different active question
retains its barrier. Reuse `deriveSessionInputMode` and queue admission for busy
sessions; do not introduce forced interruption or a separate queue policy.
Unavailable sessions preserve the answer and use ordinary recovery feedback.
No automatic replacement task or session is created.

Keep the existing stable admission identity across retries, including when the
message surface closes and reopens while admission is unresolved. A rejected
admission preserves the draft; an uncertain admission reconciles by that same
identity. Do not send ordinary messages after timeouts, malformed responses,
unknown conflicts, or generic server failures in the tool-response path.
Those outcomes retain its existing retry flow.

Separate tool outcomes from message outcomes. `no_longer_active` is not a
successful tool response; sent/queued late answers need their own outcome.
Consumers must not unmount an answer draft merely on `no_longer_active` while
new-message admission is pending or has failed. Success-only `onResolved`
remains specific to the original response path.

Cache retirement preserves current row metadata, newer authoritative versions,
terminal siblings, deleted rows, and request-generation ownership. Keep the
per-row RFC3339Nano version checks from the inactive-dismissal repair. Retiring
operational state must not remove the transcript's late-answer entry point.
An expired surface has working Close and new-message actions. Its Escape guard
must match an actually mounted handler. New-message admission never marks the
historical tool request answered or reactivates its pending-action projection.

Desktop and phone use the existing inline question form. Keep the current scroll
owner and safe areas, provide 44px coarse-pointer actions, and return focus to
the transcript opener or composer after closing. Translate new labels in all
supported catalogs. The action explicitly says it sends a new message.

Local collapse, dismiss, Escape, and task navigation do not mutate the bundle.
Skip remains the explicit rejection path. These controls may be disabled only
while the bounded request is in flight and become available again on a
retryable failure.

The shared hook and status banner remain the composition for desktop and phone.
No phone-only state or action is introduced. The existing Retry button keeps a
44-pixel minimum touch target, and the overlay keeps its current viewport,
focus, and scroll behavior.

## Failure and recovery

- An index-creation failure blocks startup. It does not silently run the
  pending-ID-only response path without its required access path.
- A pre-claim timeout performs no delivery. Transaction cancellation or
  rollback leaves a current bundle pending and safe to retry.
- A failure after claim follows the active lifecycle's durable delivery intent,
  restore, winner reconstruction, and at-most-once rules. The client deadline
  does not weaken those boundaries.
- A client timeout preserves answers and offers Retry. The retry result, rather
  than the local timeout, determines whether the overlay resolves or remains
  answerable.
- A superseded or terminal bundle continues to return the existing 409
  `not_active` result and cannot be reopened.

## Persistence

No message row, metadata field, or clarification status is added. The only
persistent change is an additive partial expression index. Existing response
delivery intents and turn reservations remain the recovery source of truth.

The old session-leading pending-ID index is retained because removing it is not
required for this fix and could regress other session-scoped queries.

## Security

Authentication, task authorization, and pending-ID opacity remain unchanged.
The 503 result does not reveal whether a pending ID exists beyond the access
checks already performed. Logs can include pending ID for operator correlation.
metrics must not use it as a label.

## Observability

Emit a structured `clarification.response.phase` record for `identity`,
`claim`, and `delivery` completion or failure with `duration_ms`, outcome, and
database driver where available. Preserve pending ID in structured logs for
correlation.

Expose `clarification_response_timeout_total` through development `expvar`,
labelled only by phase. Tests pin the counter and log classification for a
pre-claim timeout. Query-plan tests are the delivery gate for accidental index
regression. Production does not run `EXPLAIN` per request.

## Related decisions

- [Current-turn clarification ownership](../../../decisions/2026-08-14-current-turn-clarification-ownership.md)
- [Bound clarification responses with indexed lookup](../../../decisions/2026-09-05-bounded-clarification-response-path.md)
