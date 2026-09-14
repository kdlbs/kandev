---
status: current
system: ui
requirements:
  - REQ-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001
  - REQ-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001
---

# Message Queue Automation Controls System Design

## Purpose and boundaries

This design owns the task-workbench Auto-run and Auto-merge controls and their
end-to-end queue policy projection. The UI owns the operator outcomes; the
backend message queue remains the source of truth.

It changes no compatibility rule, manual merge operation, capacity value, Send
Now behavior, or Auto-run state machine. An eligible message can fold into the
tail at or above the positive row limit; staged attachments still require
insertion capacity.

## Requirement mapping

| Requirement                                    | Design sections                                                                                                                              |
| ---------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001` | [Queue header composition](#queue-header-composition), [Frontend state and mutation flow](#frontend-state-and-mutation-flow)                 |
| `REQ-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001` | [Policy resolution](#policy-resolution), [Persistence](#persistence), [Admission flow](#admission-flow), [Status ordering](#status-ordering) |

## Components and responsibilities

| Component                                       | Responsibility                                                                                                                                                                                                                                     |
| ----------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `internal/system/queuesettings.Service`         | Persists the install-wide automatic-merge value and revision together, then applies that committed policy to the queue service.                                                                                                                    |
| `internal/orchestrator/messagequeue.Service`    | Owns one atomic immutable global-policy snapshot and one process status epoch/generation source, resolves the effective session value, serializes session override writes with admissions, and applies the snapshotted value to automatic folding. |
| `internal/orchestrator/messagequeue.Repository` | Persists the nullable session override and its monotonic revision for memory, SQLite, and PostgreSQL implementations.                                                                                                                              |
| Session-scoped queue-status producers           | Receive an expected `QueueSessionIdentity` and publish only its identity-bound snapshot.                                                                                                                                                           |
| `internal/orchestrator/handlers.QueueHandlers`  | Authorizes the supplied task/session pair, passes the unchanged triplet to queue operations, and publishes status for that identity.                                                                                                               |
| `pkg/websocket`                                 | Defines queue action names.                                                                                                                                                                                                                        |
| `useQueue` and the queue session slice          | Supply current identities and order, reconcile, preserve, and disable queue state.                                                                                                                                                                 |
| `QueuePanelHeader`                              | Renders the compact automation pills with responsive and accessible interaction.                                                                                                                                                                   |

## Policy resolution

Automatic merge has two inputs:

```text
explicit session override present -> use override
no session override                -> use current global value
session override read fails        -> disable merge for this admission
```

`messagequeue.Service` replaces its independent automatic-merge boolean with one
immutable `GlobalAutoMergePolicy { Enabled, Revision }` stored through an
`atomic.Pointer`. `SetGlobalAutoMergePolicy(enabled, revision)` constructs and
publishes one new snapshot only after the settings value and revision commit.
Admissions and status reads load that pointer once; they cannot observe a value
paired with a revision from another settings generation. Reads allocate
nothing, while the infrequent settings update allocates one immutable snapshot.

At process startup, `backendapp/orchestrator.go` receives the persisted enabled
value and global revision from `queuesettings.Resolution` and calls
`SetGlobalAutoMergePolicy` before queue admissions become available. Startup
never reconstructs the runtime snapshot from the boolean alone. Reconnect tests
restart with a persisted nonzero revision and verify that the first status uses
the same pair.

Kandev has one orchestrator owner per installation, so the immutable global
snapshot is process-local under that deployment invariant. Each ordinary
admission carries `{task_id, session_id, expected_incarnation}` and loads the
global snapshot once before calling the repository operation that owns the
durable policy linearization point. SQL first locks the active task row, then
the `queue_session_locks` row, revalidates the exact task/session/incarnation,
and reads the nullable override plus inserts or directly folds in that
transaction. Override writes receive the same identity tuple and use the same
lock order and CAS validation. The result carries the one effective snapshot
used by later post-insert work. The service admission gate preserves
in-process call order but is not the cross-connection correctness boundary.

A session override committed before the repository admission lock is acquired
governs that admission. An override committed after the policy-and-insert
linearization point governs the next admission, even when the current admission
still has a staged-attachment claim and post-insert fold to complete. A
concurrent in-process global update is likewise ordered by the single atomic
load; it never changes policy halfway through one admission.

Updating the global snapshot does not scan or write session rows. Consequently
every session without an override uses the new committed global policy on its
next admission and successful queue-status read, while an overridden session
stays independent.

A user mutation always writes an explicit boolean. There is no clear-override
WebSocket operation or third UI state.

## Persistence

Queue-owned state gains a nullable override and revision:

```sql
CREATE TABLE queue_session_state (
    session_id          TEXT PRIMARY KEY,
    auto_run            INTEGER NOT NULL DEFAULT 1,
    auto_merge_override INTEGER NULL,
    auto_merge_revision INTEGER NOT NULL DEFAULT 0
);
```

`NULL` or a missing row means inherit. `0` means explicit OFF and `1` means
explicit ON. `auto_merge_revision` increments atomically on every successful
session override write. Existing rows migrate to `NULL` and revision `0`, so
upgrades preserve global inheritance. Fresh schema creation and the replay-safe
`ALTER TABLE` path define both columns using the repository's established
cross-dialect binding and duplicate-column handling.

`task_sessions` gains an opaque `queue_incarnation_id`. `CreateTaskSession`
generates a UUID once; it never changes while that row exists. The migration is
registered after both existing `task_sessions` table-rebuild migrations and
then adds the column and backfills every empty legacy row with a distinct
server-generated UUID. A restart resumes empty-row backfill without replacing
completed values. Fresh schema requires a non-empty value, and any future table
rebuild must project the column unchanged. Recreating a deleted textual session
ID therefore produces a different incarnation without retaining queue policy
state.

The persisted install-wide message-queue settings envelope also gains an
internal `auto_merge_revision`. It defaults to `0` for legacy values and
increments atomically only when `auto_merge_enabled` changes. Public settings
responses retain their existing shape; the revision is queue-status ordering
metadata, not an editable setting.

The memory repository stores override presence, value, and revision under its
existing mutex. SQL override writes increment and return the revision while
holding the durable session lock used by policy-aware admission. The settings
store saves the global value and revision in one write. Only after that commit
does the settings service publish the same immutable pair to
`messagequeue.Service`; a failed save leaves the prior runtime snapshot
unchanged.

`queuesettings.Resolution` carries the internal global revision alongside its
public configured and effective views. The backend application startup wiring
hydrates `messagequeue.Service` with that persisted pair. Live settings saves
and startup therefore use the same typed application boundary rather than a
boolean-only initialization path.

The queue package defines required authority seams instead of querying task
tables itself. `SQLSessionAuthority` exposes
`LockQueueTaskRows(ctx, tx, taskIDs)`,
`ValidateQueueSessions(ctx, tx, identities)`, and
`ClaimQueueAttachments(ctx, tx, identity, claim)`.
`QueueAttachmentClaim` carries authenticated owner/workspace and staged IDs.
`MemorySessionAuthority.WithQueueSessionIdentities` supplies an equivalent
claim-capable boundary to its callback. The task SQLite repository implements
the SQL seam on the queue transaction; production injects it into
`NewSQLiteRepository`. SQL fixtures use task/session/attachment rows, while
memory fixtures inject a seeded locked authority. Neither constructor accepts
nil; `tasksTablePresent` and missing-schema fail-open behavior are removed.

The common SQL transaction helper locks sorted unique task rows through the
authority, then sorted repository-local and durable queue-session locks, then
revalidates every exact identity before queue access. PostgreSQL uses ordered
`FOR UPDATE`; SQLite uses its writer fence. The service's outer admission mutex
is never acquired by lifecycle transactions and does not replace this order.
For memory, the authority read guard encloses validation and sorted bucket
locks; deletion removes identity under its write guard before post-commit
cleanup. Admission therefore finishes before deletion or sees a missing
incarnation.

The handler resolves authenticated claim metadata before admission and passes a
`QueueAttachmentClaim`, never a side-effect callback.
`QueueMessageWithMetadataAfterInsert` is removed. The policy-aware repository
operation validates identity, inserts, claims through the authority on the same
transaction, then folds if eligible. Claim failure rolls back queue and
attachment writes. Memory performs the same sequence inside its authority/bucket
critical section. At capacity, a staged claim still fails before insert or
claim. A two-connection test pauses inside the claim seam: deletion either wins
before validation and no claim occurs, or waits and removes the committed row
and claim without orphaning replacement state.

Queue policy state belongs to the exact task-session incarnation. SQL session
deletion receives the task ID and expected incarnation, locks the owning task
row before the durable session lock, revalidates the exact row, and atomically
deletes the task session, queued rows, and `queue_session_state`. Task and
workspace deletion already lock their authoritative task rows before acquiring
affected session locks in stable order and apply the same cleanup before
cascade removal. Admissions retain task-row-then-session-lock order and reject
a missing or mismatched session, preventing a waiter from inserting after
deletion or into a recreated textual ID.

The task repository adds a deletion-only post-commit callback carrying complete
`[]QueueSessionIdentity{TaskID, SessionID, SessionIncarnationID}` values. It is
separate from archive's queue purger/recount notifier. The memory repository
tags each bucket with the complete identity and CAS-purges only exact matches.
Its policy-aware admission validates the expected identity through the task
session authority while holding the bucket mutex, so validation and the
deletion callback order the old bucket. Archive never calls the deletion-only
callback and therefore preserves Auto-run and Auto-merge policy.

`DeleteSession` relies on this repository cleanup instead of calling
`messageQueue.CancelAll` after the session row is gone. After commit it
publishes only `publishTaskQueueStatusEvent(taskID, "")`, an internal
task-scoped recount. It never publishes a session-scoped queue status for the
deleted textual ID.

Workflow transfer supplies exact source and destination identities. In one
transaction it locks sorted unique task rows, then sorted source/destination
repository-local and durable session locks, and revalidates both incarnations
before reading or moving queue state. The move preserves existing Auto-run
semantics and the destination Auto-merge override/revision, while deleting the
source state. Source or destination mismatch returns session-not-found with no
row or policy change. Opposite-direction transfer and transfer versus
session/task/workspace delete/recreate tests use two connections to prove this
ordering is deadlock-free and leaves no old-incarnation rows. Snapshot/restore
also validates captured identities and never overwrites either policy.

`pending_moves` gains non-null `session_incarnation_id`; migration backfills
current identities and deletes unmatched legacy rows. `PendingMoveRecord` and
every set/read/take/CAS, retry, reaper, snapshot/restore, and transfer operation
retain `QueueSessionIdentity` and use the common authority/lock boundary.
Transfer writes the validated destination incarnation.

`applyPendingMove` passes captured identity, pending move ID, expected source,
and target to one deferred-transition transaction. It locks task then session,
revalidates, applies every direct write, increments the task workflow-transition
generation, and returns an immutable `TransitionEffectToken` containing that
generation and identity. The same-target marker uses this operation.

Every post-commit task effect CASes the token generation and committed target;
session/queue effects also validate the token identity and operation generation.
Publication, step history, exit/enter, prompt cleanup, drain, and pull-next fail
closed after recreation or a successor move. Tests pause before commit and
after commit before each effect, then race session/task/workspace recreation and
successor moves in both orders.

## WebSocket contract

The dispatcher adds:

```text
message.queue.auto_merge.set
```

Request:

```json
{
  "task_id": "task-id",
  "session_id": "session-id",
  "session_incarnation_id": "incarnation-uuid",
  "enabled": false
}
```

Successful response:

```json
{
  "session_id": "session-id",
  "session_incarnation_id": "incarnation-uuid",
  "queue_status_epoch": "server-epoch",
  "queue_status_generation": 42,
  "auto_merge_available": true,
  "auto_merge": false,
  "auto_merge_source": "session",
  "auto_merge_revision": 1
}
```

`task_id`, `session_id`, `session_incarnation_id`, and `enabled` are required.
The gateway and handler authorize the supplied task/session pair; persistence
revalidates the unchanged triplet under task-row-then-session-lock order.

Every browser queue request requires the triplet: `get`, `add`, `append`,
`cancel`, `update`, `remove`, `merge`, `reorder`, `drain`, `send_now`,
`auto_run.set`, and `auto_merge.set`. A stale identity returns the same
non-enumerating session-not-found response, changes nothing, and publishes no
status. The frontend refetches current state but never retries the operation.

`message.queue.get` and every `message.queue.status_changed` producer report
policy availability. A successful resolution carries the complete tuple:

```json
{
  "session_incarnation_id": "incarnation-uuid",
  "queue_status_epoch": "server-epoch",
  "queue_status_generation": 43,
  "auto_merge_available": true,
  "auto_merge": true,
  "auto_merge_source": "global",
  "auto_merge_revision": 4
}
```

`auto_merge_source` is `global` while the session has no override and `session`
after its first successful override write. The revision belongs to that source:
the current persisted global revision for inherited values, or the session
override revision for explicit values. The transport exposes source only for
ordering; the UI does not display inheritance state or offer reset.

Every session-scoped response/event uses an expected identity captured before
the operation: browser requests supply it; internal work retains it. The
producer allocates a generation, then calls the identity-bound snapshot. SQL
locks task then session, revalidates, and reads entries and policies together;
memory validates and reads under its bucket lock. The payload uses only the
expected identity. Mismatch returns session-not-found and publishes nothing, so
mutation commit followed by recreation cannot expose replacement state.

The payload receives a `queue_status_epoch` from the random process-start
identity and the allocated generation. The generation comes from one
process-local atomic counter and increases across sessions; it need not persist
because a restart changes the epoch.

If override resolution fails, the payload carries the same status ordering
fields plus `"auto_merge_available": false` and omits `auto_merge`,
`auto_merge_source`, and `auto_merge_revision`. Queue entries and independent
metadata still publish. The unavailable marker is not an authoritative OFF
value, and its generation lets clients reject a delayed older failure.

Older notifications without status ordering or `auto_merge_available` use a
complete policy tuple, when present, as an available rolling-upgrade payload;
otherwise they preserve prior frontend policy state. Current session-scoped
producers always encode status ordering and either a complete available tuple
or the unavailable marker.

Task-only `message.queue.status_changed` events used to trigger status-summary
recounts have no `session_id` and cannot resolve a session policy. They carry
`"queue_status_scope": "task"` and `task_id`, are exempt from session status
ordering and policy fields, and remain on the internal event bus for the
status-summary projector. `routeBroadcast` detects this scope and returns
without sending the event to a session or workspace browser channel. A
session-scoped publisher must provide a non-empty `session_id`, matching
`session_incarnation_id`, and the complete status encoder; an empty-session
queue event without explicit task scope is invalid and is dropped rather than
falling through to `BroadcastToWorkspace`. Producer and gateway tests cover all
three branches.

## Admission flow

`QueueSessionIdentity` is mandatory on every service/repository operation that
can insert, append, replace, restore, reserve, acknowledge, consume, update,
remove, merge, reorder, clear, transfer, or change policy. No mutating overload
accepts textual IDs. Browser callers supply current task-session identity;
handlers authorize its pair and persistence revalidates it. Internal and
deferred callers retain captured identity.

Reservations, `SendNowClaim`, queued-dispatch/cancellation records, and deferred
transition dispatch carry identity plus operation generation. Queue restore/ack
uses the common locked identity boundary. In-flight cleanup CASes identity and
generation. Before cancel or prompt, orchestration verifies task session,
execution incarnation, and operation ownership without holding a database lock
across agent I/O. Stale drain, Auto-run, or Send Now work cannot prompt, restore
into, block, or clear a replacement. Channel-controlled tests recreate after
claim and before prompt/restore for manual drain, Auto-run ON, and both Send Now
scopes. Restore/retry/transfer retains both endpoint identities.

1. Resolve the authoritative task session and carry its task ID, session ID,
   and immutable incarnation in `QueueSessionIdentity`. Load the current global
   policy snapshot once.
2. Call the repository's policy-aware admission operation. SQL locks the active
   task row, then the durable session lock, and confirms the exact
   task/session/incarnation still exists before reading policy or queue rows.
   The memory implementation performs identity validation and admission under
   its incarnation-tagged bucket mutex.
3. Under that boundary, read the nullable override, resolve it against the
   loaded global snapshot, and validate and insert the candidate under the
   current queue-capacity rules.
4. If insertion reports `queue_full`, effective Auto-merge is ON, and admission
   does not require a staged attachment claim, use the same transaction to try
   the existing direct candidate fold into the compatible tail. This applies
   when the row count is equal to or greater than the positive limit. If the
   fold skips, retain `queue_full`.
5. If override resolution fails, roll back that policy-aware attempt and retry
   only ordinary insertion with automatic merge disabled. The fallback repeats
   task/session/incarnation validation and the same lock order, never enters the
   direct-fold path, and still returns `queue_full` at capacity.
6. A staged-attachment admission at or above the positive limit returns
   `queue_full` before claiming the attachment or changing the prior tail.
7. For a successful ordinary insert, claim staged attachments in the same
   transaction, then apply the compatibility fold only when its policy snapshot
   is ON. Claim or identity failure rolls back both queue and attachment writes.
   A later override applies only to the next admission.
8. A successful fold preserves the target row identity and FIFO position, but
   assigns the later target/source queued time, matching the shipped
   `latestQueuedAt` behavior so pending activity reflects the newest prompt.
9. Publish final authoritative queue status.

Changing either global or session policy never scans or rewrites pending rows.
Lifecycle coalescing, retry, restore, and transfer keep their existing
coalesce, lifecycle-generation, FIFO, claim, and policy-copy semantics but pass
through the same required session-identity guard before replacing, restoring,
moving, or inserting a row; only ordinary admissions participate in automatic
merge. Manual merge and Send Now retain their existing explicit behavior.

## Frontend state and mutation flow

Wire-derived frontend types preserve the backend JSON names:
`TaskSession.queue_incarnation_id`, `TaskSession.task_id`, and
`session_incarnation_id` on queue payloads. There is no camel-case mapping
layer. Local function parameter names may be camel case, but every normalized
session ownership join and status comparison reads the snake-case
`TaskSession` properties.

`QueueStatus`, `QueueStatusChangedPayload`, and `QueueMeta` gain the
incarnation, status epoch/generation, policy availability, and effective
Auto-merge value, source, and revision. Every `setQueueEntries` call that
synthesizes metadata preserves the complete known incarnation, ordering, and
policy state alongside `autoRun` and `mergeEnabled` so optimistic clear and
reorder operations cannot reset or regress the control.

`queue-api.ts` requires `taskId` and `sessionIncarnationId` for every queue
request, including status reads. `useQueue` supplies the current `TaskSession`
identity. Stale outcomes refetch current state but never retry old work.

`QueueState` replaces the textual-session boolean loading entry with
`activeOperationBySessionId[sessionId]`, whose value contains
`sessionIncarnationId` and a monotonically allocated client
`operationGeneration`. `beginQueueOperation(sessionId, incarnation)` verifies
the current `TaskSession.queue_incarnation_id`, rejects a conflicting active
operation, installs a fresh token, and returns it.
`finishQueueOperation(sessionId, token)` deletes state only when both token
fields and the current session incarnation still match.

An independent status refetch begins and finishes its own token. A refetch
inside a mutation receives and retains the mutation's token instead of
installing another one, so the mutation and its authoritative refetch are one
terminal operation. Success, failure, cancellation, and early returns finish
through the same compare-and-clear API. Session removal deletes the token; a
replacement operation receives a new generation. A delayed old success, error,
refetch, or `finally` therefore cannot clear the replacement's token or
re-enable its controls.

One shared per-session predicate is true only for the active token matching the
current incarnation. It disables Auto-run, Auto-merge, row Send Now, and
reorder together. Auto-merge additionally disables when policy availability is
false.

The WebSocket handler applies the status-ordering rules below when an available
tuple is present. An unavailable payload marks the control unavailable but
preserves its last authoritative tuple; entries and unrelated metadata still
reconcile. This keeps refresh, reconnect, and live mutation paths consistent.

## Status ordering

Each producer allocates `queue_status_generation` and snapshots its expected
`QueueSessionIdentity`. Available and unavailable results use only that
incarnation; mismatch publishes nothing.

Frontend policy ordering is:

1. Reject the whole payload when current `TaskSession.queue_incarnation_id`
   differs, including queue entries.
2. A matching incarnation that differs from `QueueMeta` resets prior epoch,
   generation, availability, source, and revision.
3. Within one epoch/incarnation, apply availability only at or above the highest
   generation. A new non-retired process epoch retires the prior epoch; ignore
   later payloads from retired epochs.
4. Accept the first complete tuple, same-source revisions at or above the
   current revision, and the irreversible `global` to `session` transition.
   Never accept `session` back to `global` for that incarnation.
5. An accepted unavailable status preserves the tuple and disables Auto-merge;
   a newer complete tuple restores availability.

All session- and task-removal reducers delete queue entries, `QueueMeta`, and
the incarnation/generation-tagged operation token for each removed session.
`workspace.deleted` first snapshots affected session IDs by joining the
normalized current `TaskSession.task_id` map to locally cached tasks for that
workspace, then calls the same session cleanup before removing workspace/task
indexes. Queue metadata can exist only for a session present in that normalized
map because incarnation validation precedes status application, making this the
authoritative local cleanup set even for an empty queue.

A newly created `TaskSession` establishes `queue_incarnation_id` before queue
status is requested. Mutation responses stay narrow, and a post-mutation
refetch uses the same incarnation comparison instead of unconditionally
overwriting event state. Focused tests cover failure before success, a
lower-generation delayed failure after success, restart/reconnect with a new
epoch, session/task/workspace removal, and delete/recreate of the same textual
session ID while old-incarnation status, mutation success, error, refetch, and
`finally` completion are delayed. In the cleanup-race case a new-incarnation
mutation remains pending and all controls stay disabled until its own matching
operation generation completes.

## Queue header composition

`QueuePanelHeader` keeps one responsive header region with three logical groups:

1. queue icon, **Queued**, and capacity text;
2. compact bordered Auto-run and Auto-merge pills, each containing its visible
   label and shared `Switch`;
3. Clear all, the desktop-only pin, and collapse.

The groups use wrapping rather than truncating or horizontally scrolling the
controls. The former dedicated Auto-run help row is removed. Its localized ON
and OFF explanations remain available as the Auto-run switch's accessible
description, while Auto-merge has a localized accessible name. Both labels are
clickable.

Desktop retains the current dense header sizing. Coarse pointers enlarge each
pill's effective target to at least 44 CSS pixels without inflating desktop
controls. The existing inline panel remains the presentation on mobile; it owns
one internal message-list scroll region, uses dynamic viewport height, and does
not introduce a drawer, fixed footer, safe-area control, or document-level
horizontal overflow.

Mobile reuses `QueueChip` pill geometry and the current Auto-run coarse-pointer
target in this inline surface, not a compressed desktop workbench.

The global settings switch remains admin-only. Its localized description and
five locale catalogs explain that sessions inherit the default until changed.

## Failure and recovery

- **Write or identity failure:** publish no success; stale identity returns
  non-enumerating session-not-found with no mutation or retry. Refetch and show
  the localized error when applicable.
- **Admission policy read fails:** log and admit separately only when capacity
  permits; never auto-merge.
- **Status policy read fails:** publish ordered
  `auto_merge_available: false` without a tuple. Reconcile entries, preserve the
  last tuple, and disable Auto-merge until a newer success.
- **Global change:** advance persisted revision; later admission/status uses it
  without session migration or notification fan-out.
- **Delayed/legacy status:** epoch, generation, source, and revision reject
  regressions; compatibility preservation retains unrelated queue state.

## Security

The gateway and handler authorize the supplied task/session pair for every
browser queue request. They pass that pair plus the server-issued expected
incarnation unchanged to the identity-bound read or mutation. Persistence
revalidates under task-row-then-session-lock order, so authorization cannot
retarget a replacement. Existing admin-only global-setting permissions remain.

## Observability

Queue mutation failures and override-read failures use structured logs with
`session_id` and the operation. Successful status publication continues through
`message.queue.status_changed`. No new metric is required because this is a user
preference, not a background retry or availability loop.

## Verification strategy

- Queue service/repository tests cover policy, capacity, claims, timestamps,
  fail-closed reads, and required identities for every mutation category.
- Deterministic SQLite/PostgreSQL tests cover admission, policy, status,
  transfer, and deferred transition against lifecycle recreation. They pause
  before writes and after commit before every effect, race successor moves,
  assert task-then-session lock order, and prove no stale state or deadlock.
- Drain, Auto-run, deferred dispatch, and Send Now tests pause after claim and
  before prompt/restore. They prove identity/generation CAS protects replacement
  execution, queue, and in-flight ownership.
- Migration/lifecycle tests cover incarnation backfill, deletion cleanup,
  archive, transfer, task-only recount routing, and global-policy hydration.
- Handler and producer tests cover triplet authorization, expected-identity
  status snapshots, stale errors, and task-only browser suppression.
- Frontend tests cover wire fields, status ordering/removal, and operation-token
  CAS. Raw E2E helpers and sidebar, desktop, and mobile Playwright cover
  complete get/add/cancel payloads, mutation, persistence, layout, and
  stale-token rejection.

## Related decisions

- [Keep Queue Auto-run Server Owned](../../../decisions/2026-08-16-server-owned-queue-auto-run.md)
- [Inherit Queue Auto-merge Until Overridden](../../../decisions/2026-09-04-inherit-queue-auto-merge-until-overridden.md)
