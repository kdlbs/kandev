---
status: current
system: tasks
requirements:
  - REQ-TASKS-PLAN-COMMENTS-001
  - REQ-TASKS-PLAN-COMMENTS-002
  - REQ-TASKS-PLAN-COMMENTS-003
  - REQ-TASKS-PLAN-COMMENTS-004
---

# Task Plan Comments System Design

## Purpose and boundaries

The task system owns pending feedback on the current task plan. Session state
selects a delivery destination but never owns, filters, or persists the
comments. The backend is authoritative for comment content and consumption;
the web application projects that task state into Plan editors and every task
session composer.

This design replaces the browser-local, session-scoped ownership documented in
the superseded [Plan Comment Drafts design](../../ui/system-design/plan-comment-drafts.md).
Other comment sources remain session-scoped and keep their existing browser
persistence and client-side formatting.

The recovery refinements are pending in the
[Plan comment recovery package](../../../plans/plan-comment-recovery/plan.md).
The existing persistence, task ownership, and atomic admission boundaries remain
the basis for implementation.

## Requirement mapping

| Requirement                   | Design sections                                                                                                                 |
| ----------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-TASKS-PLAN-COMMENTS-001` | [Persistence model](#persistence-model), [Frontend projection](#frontend-projection), [Plan lifecycle](#plan-lifecycle)         |
| `REQ-TASKS-PLAN-COMMENTS-002` | [Composer delivery](#composer-delivery), [Atomic acceptance](#atomic-acceptance), [Failure and recovery](#failure-and-recovery) |
| `REQ-TASKS-PLAN-COMMENTS-003` | [Run routing](#run-routing), [Atomic acceptance](#atomic-acceptance)                                                            |
| `REQ-TASKS-PLAN-COMMENTS-004` | [Legacy migration](#legacy-migration), [Failure and recovery](#failure-and-recovery), [Responsive and accessibility behavior](#responsive-and-accessibility-behavior) |

## Ownership decision

A comment is pending task-plan state. It is not copied to sessions and it does
not carry a `session_id`. A session becomes relevant only when a user chooses
one of two dispatch paths:

- ordinary **Send** targets the selected session;
- plan-comment **Run** targets the current primary session.

Pending rows are not ambient model context. Opening a session, switching tabs,
or becoming primary does not expose them to an agent. The backend expands the
chosen rows into a prompt only during explicit delivery.

See [Persist Pending Plan Comments with the Task Plan](../../../decisions/2026-09-02-task-owned-plan-comments.md)
for the alternatives and rationale.

## Components and responsibilities

- `PlanService` and the task repository own comment CRUD, authorization, plan
  identity, collection revision allocation, and plan/task deletion.
- Task Plan WebSocket handlers expose comment snapshots and mutations. The
  task event broadcaster publishes authoritative replacement snapshots after
  committed changes.
- The direct-message and message-queue admission boundaries validate comment
  references, format their persisted contents, persist the target prompt, and
  consume the comments as one transaction.
- The production message adapter implements `AtomicQueuedPromptCoordinator`,
  forwarding queue capacity and committed-prompt notification to the
  orchestrator. A compile-time assertion keeps this capability wired through
  the adapter used by the actual WebSocket handler.
- The frontend task-plan state holds one comment snapshot per task. It does not
  place plan comments in `CommentsState.bySession` or under
  `kandev.comments.<sessionId>`.
- `TaskPlanPanel` renders the shared annotations. Every task chat composer
  reads the same task snapshot for its context item.
- `useRunComment` keeps existing behavior for non-plan comment types. Its plan
  branch resolves and guards the task's primary destination.

## Persistence model

### `task_plan_comments`

```text
id             text       primary key; caller-generated UUID
task_id        text       owning task
plan_id        text       current task plan
body           text       user feedback
selected_text  text       selected plan text used for display and fallback anchoring
anchor_from    integer    Tiptap document position at creation
anchor_to      integer    Tiptap document position at creation
version        integer    optimistic mutation version, starts at 1
created_at     timestamp  UTC
updated_at     timestamp  UTC
```

`task_id` and `plan_id` identify the same current plan. Database constraints
prevent a row from pairing a plan with another task. The task and plan foreign
keys use `ON DELETE CASCADE`. The list order is `created_at`, then `id`, so all
clients render a stable order.

`task_plans.comments_revision` is a monotonic integer, starting at zero. Every
committed create, edit, delete, orphan cleanup, migration, or delivery
consumption increments it in the same transaction. A snapshot has this shape:

```json
{
  "task_id": "task-id",
  "plan_id": "plan-id",
  "revision": 12,
  "comments": []
}
```

The plan ID distinguishes a newly created plan from a deleted plan whose
revision counter previously had the same value.

The service bounds bodies to 64 KiB and selected text to 256 KiB, measured in
UTF-8 bytes. Each create or update transaction also validates a maximum of 100
pending comments and 1 MiB of combined bodies and selected text. Exceeding a
limit rolls back the mutation and its revision. The shared `plancomments`
package owns these limits so SQLite and Postgres use the same policy.

### Delivery references

Message admission receives only identifiers and versions, never trusted
comment text:

```json
{
  "plan_comment_refs": [{ "id": "comment-id", "version": 3 }]
}
```

The backend loads the rows for the request's `task_id`, validates that they
belong to the current plan, and formats the stored body and selected text.
Message and queue metadata retain the accepted IDs and versions for provenance
and idempotent replay. The persisted message or queued prompt contains the
expanded Markdown, so later comment deletion cannot change what the agent
receives.

The transactional resolver checks the final rendered prompt against the 1 MiB
message limit before direct or queued insertion and comment consumption. This
also protects delivery of oversized legacy rows. Both transports return a
validation error without changing the pending snapshot.

## WebSocket contracts and synchronization

The task-plan handler family adds these authorized actions:

- `task.plan.comments.list`
- `task.plan.comments.create`
- `task.plan.comments.update`
- `task.plan.comments.delete`

Every request carries `task_id`. Create also carries `plan_id`, a
caller-generated `id`, selected text, anchor positions, and body. Update and
delete carry `plan_id`, `id`, and `expected_version`. A stale plan or version
returns a stable conflict error and the current snapshot. Repeating a create
with the same ID and identical task, plan, anchor, and text returns success;
reusing the ID for different data returns a conflict.

Every successful mutation returns the complete snapshot. The task event
broadcaster publishes `task.plan.comments.changed` with that same snapshot.
The frontend replaces its local set only when the event has the current plan
ID and a revision at least as new as its cached revision. It refetches after a
reconnect or any detected plan-identity mismatch. Full snapshots keep event
recovery simple because no client must replay missing deltas.

All four actions authorize through `task_id`. The gateway's deeper task-action
backstop requires that field, matching the existing task-plan actions. No
session authorization grants access to comments from an otherwise inaccessible
task.

## Frontend projection

The task-plan slice gains a comment snapshot keyed by task ID, plus loading,
mutation, and migration state. `usePlanComments(taskId)` owns initial load,
CRUD, event reconciliation, and retry. Transient selection and open-editor
state can remain local to `TaskPlanPanel`; it is reset only when the task or
plan identity changes, not when the selected session changes.

`TaskPlanPanel` no longer accepts an active session as the comment owner.
Selection-based **Add** remains available whenever a current plan exists. The
desktop Popover and mobile Drawer stay open with the entered body intact until
the backend acknowledges create or update. While a mutation is pending, the
relevant action is disabled; failure appears inline and can be retried.

Every mounted composer for the task derives its plan-comment context item from
the task snapshot. The item shows the shared count and opens the Plan surface.
It has no remove control: removing it from one session would either lie about
the shared context or become a surprising task-wide bulk delete. Users edit or
delete individual comments on the plan.

The Tiptap projection transaction marker introduced by the session-switch
repair remains useful. Backend snapshot reconciliation is presentation work
and must not trigger orphan deletion. Untagged destructive plan edits continue
to report truly removed anchors, and `TaskPlanPanel` turns that report into an
authorized backend delete.

## Composer delivery

At submit time, a session composer snapshots the visible comment IDs and
versions and passes them through `message.add` or `message.queue.add` alongside
the unexpanded composer content. It does not prepend plan-comment Markdown in
`buildSubmitMessage` or duplicate the feedback inside
`buildDocumentContext`. Other comment sources keep their current formatting
and clearing behavior.

The submitted `session_id` remains the ordinary delivery target. A primary
session mismatch does not redirect an ordinary Send. Existing workflow routing
may still replace a session under its documented turn-start rules; plan
comments introduce no primary-based rerouting to that path.

The client retains one caller-generated admission ID for an unchanged submit
payload across automatic reconciliation and explicit user retry. Changing the
message, comment versions, attachments, or destination creates a new identity.
Plan-comment Run retains its identity across a primary-session refresh because
a rejected stale-primary attempt has not been admitted.

An empty composer body is valid when `plan_comment_refs` is non-empty. The
backend canonical formatter prepends the existing visible `### Plan Comments`
shape to the base content for both ACP and passthrough sessions. It does not put
the feedback in a hidden system block, so the transcript, queue editor, and
passthrough terminal all show the same submitted prompt.

## Run routing

For a plan comment, `useRunComment` ignores the selected session as a target.
It reads the current task primary and submits only the clicked comment with
`require_primary_session: true` and plan mode enabled. A promptable primary uses
`message.add`; a busy primary uses a distinct `message.queue.add` entry rather
than `message.queue.append`. A distinct entry gives the action a durable
identity and avoids merging its comment-consumption boundary into unrelated
queued text.

Both endpoints validate under their task lock that the supplied session still
belongs to the task and is still primary. A `primary_session_changed` response
includes the safe current primary ID and state so the frontend can refresh its
routing state; it does not consume the comment. The user can retry without any
chance that the stale session received the feedback.

When no primary exists, or the primary is terminal and cannot accept direct or
queued input, the Run control is disabled and the Popover or Drawer explains
that a primary session must be available. **Add**, edit, and delete remain
enabled because comment ownership is independent of session availability.

## Atomic acceptance

Direct-message and durable-queue repositories already insert inside database
transactions. Direct delivery acquires a task-scoped admission lease before
its exact preflight and keeps it across task-state and turn-start hooks through
the final transaction. Plan-comment mutations and plan deletion join the same
lease, so a successful preflight cannot become stale only after irreversible
hooks run. SQLite uses a process-local task guard. PostgreSQL pairs that guard
with an advisory lock shared across backend processes; its writer pool must
allow at least two connections because the lease owns one while hooks and the
final transaction use another.

Comment-bearing variants then extend the final transaction boundary in this
order:

1. Lock or guard the active, unarchived task row, then take the existing
   per-session admission lock. This preserves the repository's
   task-before-session lock order.
2. If `require_primary_session` is set, verify the target session is the
   current primary. Re-read the target state and reject a terminal session or
   a direct-delivery state that changed since routing was selected.
3. Load every referenced comment for the current plan and require the exact
   submitted version. Missing, duplicated, stale, or cross-task references
   reject the whole operation.
4. Build the canonical prompt from those persisted rows and the submitted base
   content.
5. Claim any staged file attachments, insert the user message and durable
   delivery receipt, conditionally delete every referenced comment, and
   increment `task_plans.comments_revision`. A promptable direct send uses the
   same receipt as a deferred send so a process crash after commit cannot lose
   its dispatch intent.
6. Commit all of those changes together, then publish the ordinary
   message/queue event and the complete
   `task.plan.comments.changed` snapshot.

A shared leaf transaction helper owns reference validation, conditional
deletion, and revision allocation so the task-message and message-queue
repositories do not implement different semantics. The direct-message path
retains `client_message_id`. Comment-bearing queue additions add a
caller-generated `client_queue_id`; the queue repository treats an exact replay
as a read and rejects reuse for different content or ownership. Immediate
auto-merge is skipped for these entries so the idempotency identity remains
observable through admission. Reserve/dispatch retains that queue row as a
replay receipt and writes a deterministic, idempotent transcript before
external delivery. The transcript metadata retains the caller queue ID, so a
lost response can still reconcile after the queue receipt is acknowledged.

Every pre-dispatch reservation has a random ownership token and a short lease.
Only that token may release, acknowledge, or cross the delivery boundary. A
live lease blocks another backend process; after expiry a new owner receives a
new token and every mutation from the stale owner fails closed. Immediately
before provider or passthrough I/O, the repository atomically verifies the
task is still active and marks the receipt `delivery_attempted`. Failures before
that marker release the receipt for retry. Once marked, no automatic retry is
allowed, including after an ambiguous transport failure or failed
acknowledgement, because the external system may already have accepted the
prompt. Startup reconciliation resumes expired pre-attempt receipts and clears
attempted receipts without redispatching them.

Send Now follows the same rule for every comment-bearing source. It persists
one deterministic transcript for the combined ordered envelope before marking
all source receipts attempted in one transaction; source-level transcript
markers never suppress that combined transcript.

This boundary gives each comment one accepted delivery. If another request
consumes or edits a referenced row first, the loser rolls back its message or
queue insert and returns `plan_comments_changed`. The frontend refreshes the
snapshot and leaves its composer text intact rather than sending a different
prompt than the user reviewed.

## Plan lifecycle

Plan content updates and revision reverts keep the same current plan row and
therefore keep pending comments. On projection, the editor first tries saved
positions and then the existing selected-text fallback. If an untagged user
edit truly removes the marked range, the client requests deletion. A comment
can remain pending while no Plan panel is mounted; opening the plan performs
the same reconciliation.

`task.plan.delete` deletes the current `task_plans` row and cascades its pending
comments. Task deletion cascades both. Plan revision history contains plan
content only and does not retain pending comments. Recreating a plan creates a
new plan ID and an empty comment collection.

## Legacy migration

### Separate discovery, migration, and snapshot loading

`usePlanCommentMigration` owns only promotion of legacy browser drafts.
`usePlanComments` continues to own current-plan and authoritative comment reads.
The latter's `commentsErrorByTaskId` is not evidence that a legacy draft exists
and must not be converted into migration failure.

Discovery reads `kandev.comments.<sessionId>` only for sessions attributed to the
task by `useTaskSessions` or current task-session state. It can inspect known
session records while the authoritative session list is pending, and rescans
when membership becomes available or the browser returns to the foreground.
Failed or incomplete discovery is not a completed scan, but has no delivery
restriction in the absence of identified task-owned draft content. Do not
attribute an unrelated session's stored rows to the open task. No server request
is required by migration when its task scan contains zero legacy records.

Once identified, retain each pending record in the recovery state until exact
backend acknowledgement and selective storage cleanup. A later failed or empty
storage read must not erase that known pending set. Keep other comment sources,
malformed-but-readable payload entries, original UUIDs, and concurrent local
edits intact. `listLegacyPlanComments` and
`removeAcknowledgedLegacyPlanComment` remain the persistence boundaries; any
additional read outcome must distinguish unavailable storage from an
authoritative empty result without changing unrelated storage helpers.

For a task with identified drafts:

1. Resolve its current plan while connected. If no plan exists, retain the
   drafts and wait for plan availability; do not repeatedly upload or delete.
2. Upload each draft with its existing UUID and content. Each create response
   already supplies an authoritative snapshot; reconcile it by task, plan ID,
   and revision before acknowledging the exact stored record.
3. Re-read after acknowledgement and retain failed or concurrently edited rows.
   Retry only unresolved rows. A conflict snapshot alone does not acknowledge
   the conflicting local body.
4. Complete migration when every identified row has been acknowledged. Do not
   append an unconditional list request: a failed background snapshot refresh
   must not undo successful migration or keep an empty migration blocked.

### Recovery state and lifetime

Replace the status-only `commentsMigrationStatusByTaskId` projection with one
task-keyed recovery record in the existing task-plan slice. The proposed
`PlanCommentMigrationState` contains status, identified pending-record count,
and actionable failure classification; the
`commentsMigrationByTaskId` map and `setTaskPlanCommentMigrationState` action
update that projection together. The old map and setter are removed, not kept
as a second source of truth. Phases distinguish discovery, active migration,
retry waiting, missing plan, actionable failure, and completion.

Promise, timer, pending-record, and subscriber bookkeeping stays in one
store-scoped coordinator keyed by task with a plan-identity generation. React
consumers share it, including simultaneous
Plan, structured-composer, and passthrough mounts. At most one operation and one
retry timer per task are active. StrictMode and repeated focus events must not
create duplicate uploads or reset the retry budget on every render.

Use connected, visible consumers to admit network attempts. Replace the
uncoalesced focus/visibility listeners in `usePlanComments` with
`useForegroundRefresh`; defer work while disconnected and resume on the next
connected transition. Discovery and legacy recovery use the same foreground
trigger and connection check. `plan-comment-loading.ts` owns the ordinary
read promise, bounded retry timer, and plan generation behind `usePlanComments`;
`plan-comment-migration.ts` owns identified drafts and acknowledgements behind
`usePlanCommentMigration`. Both layers preserve successful snapshots through
read failures; no application-wide transport timeout or retry policy changes.

Transient failures get three connected attempts with delays of one and two
seconds after the first two failures. After that burst, schedule single
background attempts at 30, 60, then at most once per 120 seconds while visible
and connected. These are implementation constants, not operator settings.
Coalesced reconnect, foreground, and explicit Retry triggers can bring the next
attempt forward, but share the in-flight operation. Only explicit Retry or
successful recovery resets the failure budget. Disconnected or hidden periods
do not spend attempts. Stop scheduled work when the last consumer unmounts;
remount rescans and resumes without a browser-global completed marker.

Typed content, limit, authorization, and UUID-conflict rejections are actionable
and do not enter an automatic mutation loop. Resolve a plan-not-found response
through current-plan lookup. Untyped transport failures and server read failures
use bounded retry. Manual Retry rechecks prerequisites as well as remaining
records, and cannot clear identified draft evidence just to release Send.

Each operation captures task/plan identity and a generation. Recheck before
each upload, state write, and acknowledgement. Task or plan invalidation cancels
future work and rejects late completion from the previous generation. A
successful current-generation upload removes only its exact legacy row; it
does not delete an entire storage key containing other records.

### Delivery restriction

Use one shared selector for composer migration blocking: at least one
identified legacy record for that task remains unresolved. Neither an idle
phase nor a general read error satisfies that predicate. The structured
`useSubmitHandler` and passthrough submission guard both use it. A blocked
attempt preserves the typed message and returns localized pending-context
feedback; recovery never resubmits it.

Persisted snapshot comments remain visible during failed background reads.
Normal Send continues to freeze their IDs and versions with
`toTaskPlanCommentRefs`, so backend exact-version admission protects their
content. A successful refresh, including an empty snapshot or confirmed absent
plan, clears obsolete read errors but never acknowledges unresolved local rows.

Run selects one persisted comment. Remove the unrelated task-wide migration
status check from `runTaskPlanComment` and the selection toolbar; keep persisted
version, primary availability, exact reference, and server acceptance checks.
Other legacy records remain pending. Add-and-Run must still await persistence
of the selected new comment before invoking Run.

## Failure and recovery

- Discovery and transient background recovery are quiet. A comment restoration
  notice is eligible only for identified unresolved drafts after the initial
  retry burst or for an actionable failure, including an absent current plan.
  General read failures never produce that notice. A task with no identified
  drafts has no migration restriction, independently of plan availability.
- Known pending feedback remains protected as described in
  [Delivery restriction](#delivery-restriction). A partial upload cannot release
  a Send that would omit a remaining row; an unrelated pending row cannot block
  Run of an already persisted comment.
- Failed create or edit keeps the entered body and selection in the open
  editor. Failed delete keeps the annotation visible. Desktop outside-click or
  Escape dismissal and mobile Drawer dismissal are ignored while any mutation
  is pending.
- A direct or queue admission error leaves both the comments and composer draft
  unchanged.
- A lost response is reconciled with the caller-generated message or queue ID.
  A create retry reuses its pending caller ID. An accepted replay returns the
  durable queue row or transcript; an absent delivery leaves the comments
  pending.
- A primary-session conflict triggers an authoritative task-session refresh,
  including an explicitly absent primary or a replacement not yet in the local
  store. A request-local store subscription detects changes to primary identity,
  session state, and queue incarnation while the request is pending. Once any
  routing change occurs, neither the older response nor its error fallback may
  replace that projection, even if routing changes back before the response.
  The subscription is disposed when the request settles.
- Quick Chat and config chat clear their automatic launch descriptor when the
  first eligible attempt starts, keeping a session-scoped recovery draft. A
  rejected attempt restores that draft to an empty composer for explicit retry;
  reopening the panel must not automatically resend it. Settlement remains
  bound to the original session and never replaces newer user-entered text.
- Reconnect reloads a complete snapshot. A stale WebSocket snapshot cannot
  replace a newer revision.
- A queue-capacity failure does not consume comments. Cancellation or editing
  of an already accepted queued prompt does not resurrect them because the
  queue content is now the durable delivery record.

## Responsive and accessibility behavior

Desktop keeps the anchored selection Popover. Phones and coarse pointers keep
the bottom Drawer, inset containment, safe-area clearance, one internal scroll
owner, focus return, and 44 px actions established by the responsive Plan
contract. Both surfaces use the same task-level query and mutations.

The pending state is announced on the action being performed. Mutation and
routing errors are visible text, not color-only or tooltip-only feedback. The
Run control exposes why it is unavailable when no eligible primary exists.
Session switching must not dismiss or mutate a persisted comment merely as a
side effect of changing responsive navigation.

`PlanCommentMigrationNotice` remains an inline context notice in the existing
Plan surface and both composers, but renders only actionable recovery state.
No progress banner is mounted for discovery or automatic retry. Retain draft
focus when it appears or disappears. The existing task mobile composition in
`task-layout.tsx` and the Plan Drawer are the shipped exemplars: chat remains
the primary phone destination and its scroll/safe-area ownership is unchanged.
Do not add a recovery modal or a second scroll region. Retry keeps a 28 px
desktop control and a minimum 44 px coarse-pointer target. Shared logic owns
eligibility; responsive wrappers own wrapping and containment. Pending-Send
copy describes preserved feedback and automatic recovery through translations,
without instructing users to perform a migration.

## Verification

- Repository tests cover schema replay on SQLite and Postgres, task/plan
  cascades, stable ordering, optimistic versions, idempotent create, complete
  snapshots, and authorization.
- Direct-message and queue tests prove canonical server formatting, empty-body
  submission, exact-version consumption, rollback on stale references or queue
  capacity, idempotent transport replay, task/session mismatch rejection, and
  the primary guard.
- Frontend tests prove one task snapshot drives every Plan and composer,
  session changes do not filter it, async editors retain failed input, Run
  chooses the primary, and legacy migration removes only acknowledged plan
  records.
- Recovery tests cover empty and unknown plan state, initial discovery failure,
  automatic reconnect and foreground recovery, exhausted retries, mixed
  acknowledged/unacknowledged rows, unmount and plan-identity races, unrelated
  task records, and persisted-comment Run during another draft's recovery.
- Desktop Playwright coverage distinguishes selected-session Send from
  primary-session Run in a two-session task and proves task-wide removal after
  acceptance.
- Mobile Playwright coverage proves the same shared context and routing through
  the session picker and Plan Drawer, with no horizontal overflow or undersized
  actions.
- Desktop and phone recovery scenarios inject correlated comment/read failures
  while leaving unrelated chat frames live, send plain text without Retry on
  empty tasks, and prove actual legacy feedback recovers without a manual
  action. Existing selected-session Send and primary-session Run scenarios
  remain part of those focused suites.

## Observability

The existing task event bus carries authoritative snapshots after mutations
and delivery consumption. Publication failures log identifiers and collection
state where available, never comment bodies or selected plan text. Stable
transport error codes distinguish stale comments, stale primary selection,
queue capacity, and replay conflicts. No new production metric is required
initially because deterministic repository and E2E coverage owns the
correctness boundary.

## Related decisions

- [Persist Pending Plan Comments with the Task Plan](../../../decisions/2026-09-02-task-owned-plan-comments.md)
- [Keep Queue Auto-run Server Owned](../../../decisions/2026-08-16-server-owned-queue-auto-run.md)
- [Separate Message Queue Provenance, Cancellation, and Capacity](../../../decisions/2026-08-03-separate-message-queue-provenance-cancellation-and-capacity.md)
- [Keep Saved-Prompt Expansion Server-Owned](../../../decisions/2026-09-01-server-owned-saved-prompt-expansion.md)

## Implementation plans

- [Task-owned plan comments](../../../plans/task-owned-plan-comments/plan.md)
- [Plan comment recovery](../../../plans/plan-comment-recovery/plan.md)
