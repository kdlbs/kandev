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

## Requirement mapping

| Requirement                   | Design sections                                                                                                                 |
| ----------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-TASKS-PLAN-COMMENTS-001` | [Persistence model](#persistence-model), [Frontend projection](#frontend-projection), [Plan lifecycle](#plan-lifecycle)         |
| `REQ-TASKS-PLAN-COMMENTS-002` | [Composer delivery](#composer-delivery), [Atomic acceptance](#atomic-acceptance), [Failure and recovery](#failure-and-recovery) |
| `REQ-TASKS-PLAN-COMMENTS-003` | [Run routing](#run-routing), [Atomic acceptance](#atomic-acceptance)                                                            |
| `REQ-TASKS-PLAN-COMMENTS-004` | [Legacy migration](#legacy-migration)                                                                                           |

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
transactions. Comment-bearing variants extend those transaction boundaries in
this order:

1. Lock or guard the task row, then take the existing per-session admission
   lock. This preserves the repository's task-before-session lock order.
2. If `require_primary_session` is set, verify the target session is the
   current primary.
3. Load every referenced comment for the current plan and require the exact
   submitted version. Missing, duplicated, stale, or cross-task references
   reject the whole operation.
4. Build the canonical prompt from those persisted rows and the submitted base
   content.
5. Insert the user-message or queued-message row, conditionally delete every
   referenced comment, and increment `task_plans.comments_revision`.
6. Commit, then publish the ordinary message/queue event and the complete
   `task.plan.comments.changed` snapshot.

A shared leaf transaction helper owns reference validation, conditional
deletion, and revision allocation so the task-message and message-queue
repositories do not implement different semantics. The direct-message path
retains `client_message_id`. Comment-bearing queue additions add a
caller-generated `client_queue_id`; the queue repository treats an exact replay
as a read and rejects reuse for different content or ownership. Immediate
auto-merge is skipped for these entries so the idempotency identity remains
observable through admission.

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

The first frontend version with backend comments runs a bounded migration for
the open task after it knows the current plan and the task's session IDs:

1. Inspect `kandev.comments.<sessionId>` for every known session in the task.
2. Extract only records whose source is `plan`; retain their existing UUID,
   text, selected text, and anchor positions.
3. Create each backend row against the current plan. Exact UUID replays are
   successful, so interrupted migration can restart safely.
4. After each acknowledgement, remove only that plan record from its legacy
   session payload. Preserve all other comment sources and every failed plan
   record.
5. Fetch the authoritative snapshot and mark migration complete for the task.

Composer Send and plan-comment Run wait for this migration gate. Comment CRUD
and other local comment types are not blocked. If no current plan exists, the
legacy records remain in storage for recovery rather than being discarded.

## Failure and recovery

- Failed load or migration shows a retryable comment-context error. Kandev does
  not allow a comment-bearing delivery until it can establish the task
  snapshot, preventing silent omission.
- Failed create or edit keeps the entered body and selection in the open
  editor. Failed delete keeps the annotation visible.
- A direct or queue admission error leaves both the comments and composer draft
  unchanged.
- A lost response is reconciled with the caller-generated message or queue ID.
  An accepted replay returns the durable row; an absent row leaves the comments
  pending.
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
- Desktop Playwright coverage distinguishes selected-session Send from
  primary-session Run in a two-session task and proves task-wide removal after
  acceptance.
- Mobile Playwright coverage proves the same shared context and routing through
  the session picker and Plan Drawer, with no horizontal overflow or undersized
  actions.

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
