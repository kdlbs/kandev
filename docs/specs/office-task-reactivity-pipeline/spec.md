---
status: shipped
created: 2026-05-03
owner: cfl
---

# Office Task Reactivity Pipeline

## Why

When a property on an office task changes — status, assignee, priority,
parent, blockers, comments — *something downstream should usually
happen*. Tasks waiting on a dependency should wake when their blocker
finishes. A reassigned task should hand off to the new agent with
context, not just sit there. A comment should ping the assignee
(and any @-mentioned agents) so they know there's new input. Closing
a parent should notify its children. Today, our backend writes the
new value to the DB, fires a thin notification event, and that's it —
the agents themselves are responsible for noticing, which they don't.

The target pattern is a **status-change pipeline**: a synchronous
function called on every property update that computes wakeups,
execution-policy transitions, and decision records *before* the DB
write. It produces a rich `contextSnapshot` per wakeup so the woken
agent knows *why*. We have none of this. The result is a feature that
looks reactive on the surface (events fly around) but is functionally
inert (nothing changes the agents' behavior).

## What

A backend pipeline that, on every relevant task property change, runs
a deterministic set of reactions before/after the DB write:

### A. Wakeup pipeline

- On **status → done**: query tasks blocked by this task and queue a
  wakeup for each, payload `{reason: "blocker_resolved",
  resolved_blocker_task_id}`. Also wake the parent task with
  `{reason: "child_completed"}` if all siblings are done.
- On **status from `blocked` → any unblocked state**: wake assignee
  with `{reason: "task_unblocked"}`.
- On **status from `done`/`cancelled` → `todo`/`in_progress`**: wake
  assignee with `{reason: "task_reopened", actor_id, actor_type}`.
- On **assignee change**: wake new assignee with `{reason: "assigned",
  comment_id?, actor_id}`. If old assignee had an active session,
  cancel it cleanly (`session.cancelled` notification) and surface
  that to the new assignee's context.
- On **comment created by user**: wake assignee with
  `{reason: "user_comment", comment_id}`. Also wake any @-mentioned
  agents in the same workspace with `{reason: "mentioned",
  comment_id}`.
- On **status → `cancelled`**: cancel the task's active execution
  immediately (interrupt the current turn, mark the session as
  cancelled), and emit `office.task.cancelled` so the UI updates.

### B. Execution-policy transition engine

- A new function `applyTaskExecutionPolicyTransition(task, change)`
  that runs synchronously inside `UpdateTaskStatus` (and any other
  property mutator that can advance a stage).
- It reads the task's `execution_policy` (work / review / approval
  stages) and the change being applied; it returns:
  1. The patch to apply alongside the user's change (e.g. advance
     `execution_state` to "review_pending").
  2. A list of wakeups to queue (e.g. wake the reviewer agent).
  3. A `decision` record if the transition encodes an approval verdict.
- The engine MUST be invoked from EVERY mutation entry point:
  - `UpdateTaskStatus` (status changes)
  - `MoveTask` (kanban moves) — already partially wired
  - `SetTaskAssignee`
  - `CreateTaskComment` when the comment marks an approval
- The engine MUST NOT poll or schedule — it returns its outputs
  synchronously and the caller persists them in one DB transaction.

### C. Wakeup context enrichment

Every wakeup written to the wakeup queue MUST carry a structured
`context` field (JSON) with:
- `reason`: enum (`blocker_resolved`, `task_unblocked`, `task_reopened`,
  `assigned`, `user_comment`, `mentioned`, `child_completed`,
  `stage_pending`, `stage_changes_requested`).
- `task_id` (always).
- `actor_id` + `actor_type` (`user` / `agent`) if known.
- `comment_id` if relevant.
- `resolved_blocker_task_id` / `child_task_id` for cascade reasons.
- `stage_id` + `allowed_actions` (e.g. `["approve","reject"]`) for
  execution-policy reasons.

The agent runtime reads `context.reason` to pick the right system
prompt template ("you've been asked to review X", "your blocker is
resolved", etc.).

### D. Decision audit table

A new `office_task_execution_decisions` table records every
review/approval verdict produced by the policy engine:
- `id`, `task_id`, `stage_id`, `verdict` (approved / changes_requested
  / rejected), `actor_id`, `actor_type`, `comment_id?`, `created_at`.

Used to reconstruct who approved what and when (compliance, debugging,
UI history).

## Scenarios

- **GIVEN** task A is `blocked_by` task B, **WHEN** B's status changes
  to `done`, **THEN** A's assignee receives a wakeup with
  `context.reason = "blocker_resolved"` and
  `context.resolved_blocker_task_id = B.id`.
- **GIVEN** task A has children B, C, D all `done`, **WHEN** the last
  child becomes `done`, **THEN** A's assignee receives a wakeup with
  `context.reason = "child_completed"`.
- **GIVEN** task A is assigned to agent X with a session running,
  **WHEN** the user reassigns to agent Y, **THEN** X's session is
  cancelled (clean shutdown), Y receives a wakeup with
  `context.reason = "assigned"` and `context.actor_id = <user>`.
- **GIVEN** the task assignee is agent X, **WHEN** the user adds a
  comment "@reviewer please look at this", **THEN** X receives a
  wakeup with `context.reason = "user_comment"` AND the agent named
  `reviewer` (if it exists in the workspace) receives one with
  `context.reason = "mentioned"`.
- **GIVEN** task A has `execution_policy: {work, review}` with state
  `work_in_progress`, **WHEN** the worker agent updates status to
  `in_review`, **THEN** the policy engine advances state to
  `review_pending`, the reviewer receives a wakeup with
  `context.reason = "stage_pending"` and `context.stage_id = "review"`,
  and an `office_task_execution_decisions` row is NOT yet written
  (decisions are written when the reviewer concludes the stage).
- **GIVEN** the reviewer is on stage_pending, **WHEN** they comment
  "approved" and update status to `done`, **THEN** the policy engine
  records a decision row with `verdict = "approved"`, the worker is
  not re-woken, and any approvers (if the policy has an approval
  stage) get the next stage's wakeup.
- **GIVEN** task A has an active session, **WHEN** status is set to
  `cancelled`, **THEN** the active turn is interrupted within 2
  seconds and the session shows `cancelled` state.

## Out of scope

- Polling fallbacks. If the wakeup queue worker isn't running, no
  reactions fire — that's already the case today and is a separate
  reliability concern.
- The UI for editing properties (covered by `office-editable-task-properties`).
- Permission/authorization model changes — assume current rules apply.
- Changing the wakeup queue / scheduler infrastructure itself; we add
  to the existing pipeline.
- Webhook / external notifications (Slack, email).
- Backfilling decisions for tasks that closed before this lands.

## Open questions

- Should we add a new event subject `office.task.cancelled`, or piggyback on `office.task.status_changed` with the
  new status? Plan picks one.
- For `mentioned` wakeups: do we resolve `@name` against agent names
  in the workspace, against agent slugs, or both? Plan picks one and
  documents the matching rule.
- Decision records on policy transitions: do we record one per stage
  entry or one per stage exit (with verdict)? Plan picks one.
