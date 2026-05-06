---
status: shipped
created: 2026-05-03
owner: cfl
---

# Office: Task approval flow

## Why

The `office-ux-parity` work shipped reviewer/approver lists
on tasks: a static list of agents per role, persisted in
`office_task_participants`, exposed as multi-select chip pickers in
the right-hand Properties panel. **Today these lists are purely
cosmetic.** Concretely:

- Adding "CEO" as an approver on a task does not gate completion. The
  agent assignee can move the task `in_review → done` without the CEO
  ever being asked.
- Reviewers are not woken when a task enters `in_review`. They have
  no signal that their attention is needed and no action surface.
- There is no per-task record of who approved or rejected what.
- The existing `office_approvals` table tracks approval *requests*
  (e.g. "agent wants permission to do X"), not per-task signoff
  decisions, so it doesn't fill the gap.

The goal is to wire the configuration layer (the lists) into a real
gate so tasks meaningfully require sign-off before closing. This
when a task moves into review, listed approvers are pinged and must
approve before the task can complete.

## What

A coherent end-to-end flow that consumes the reviewer/approver lists,
tracks decisions per-task, gates the `in_review → done` transition,
and surfaces actionable UI to the right people.

### A. Schema — per-task decisions table

A new table records each reviewer/approver decision per task:

```sql
CREATE TABLE office_task_approval_decisions (
    id              TEXT NOT NULL PRIMARY KEY,
    task_id         TEXT NOT NULL,
    decider_type    TEXT NOT NULL CHECK (decider_type IN ('user','agent')),
    decider_id      TEXT NOT NULL,           -- '' for user
    role            TEXT NOT NULL CHECK (role IN ('reviewer','approver')),
    decision        TEXT NOT NULL CHECK (decision IN ('approved','changes_requested')),
    comment         TEXT NOT NULL DEFAULT '',
    created_at      DATETIME NOT NULL,
    superseded_at   DATETIME,                -- non-NULL when replaced by a newer decision
    FOREIGN KEY (task_id) REFERENCES office_tasks(id) ON DELETE CASCADE
);
CREATE INDEX idx_task_decisions_task ON office_task_approval_decisions(task_id);
```

Multiple decisions per (task, decider, role) are allowed; only the
most recent one (`superseded_at IS NULL`) counts toward gating.

### B. Reactivity hooks

In `scheduler/reactivity.go`:

- On `status → in_review`: queue a `task_review_requested` wakeup for
  each agent in `reviewers` AND each agent in `approvers`. Reason
  string is new (extending the existing wakeup-reasons list). Include
  `role` in the wakeup context so the agent's prompt builder can
  render an appropriate "you are the reviewer/approver" hint.
- On `decision = changes_requested`: queue a `task_changes_requested`
  wakeup for the assignee; include the comment.
- On `decision = approved` AND all approvers have approved AND task
  status is `in_review`: queue a `task_ready_to_close` wakeup for the
  assignee — they're cleared to mark it done.

### C. Status transition gate

In `dashboard/service.go` `UpdateTaskStatus`:

- When the target status is `done`:
  - If the task has any approvers, every approver must have a current
    `decision = approved`. If not, reject 409 with a typed error
    describing the missing approvals.
  - Reviewers do not gate (they're advisory, not authoritative —
    reviewers are advisory, not authoritative).
- The transition `in_review → todo|in_progress` (rework) **clears**
  prior decisions (sets `superseded_at`). Re-entering `in_review`
  starts a fresh round. This avoids stale approvals carrying over a
  rework cycle.

### D. Endpoints

- `POST /tasks/:id/approve { comment?: string }` — caller approves
  the task. 403 if the caller is not in the task's reviewers or
  approvers list. Returns the created decision row.
- `POST /tasks/:id/request-changes { comment: string }` — caller
  requests changes. Comment required. Same 403 rule. Returns the
  decision row.
- `GET /tasks/:id/decisions` — returns the current (non-superseded)
  decision rows for the task. Surfaced into the task DTO as a
  `decisions: TaskDecision[]` field on the existing task detail
  endpoint.

### E. Inbox integration

Tasks awaiting the current user's review/approval surface in the
inbox as a new item type `task_review_request`. Item content:
task identifier + title + the role(s) the user is in + a deep link
to the task detail page.

### F. UI

On the task detail page:

- **Action bar** at the top of the comments timeline, visible only
  when the current viewer (user or agent) is a listed reviewer or
  approver AND has no current decision recorded. Two buttons:
  "Approve" and "Request changes". Request-changes opens a small
  inline editor for the comment.
- **Status pill on the task header** when gated: instead of a plain
  `<StatusPicker>`, show the current status plus a small badge
  *"Awaiting approval from CEO, Eng Lead"* (the names of approvers
  who haven't decided yet). Hover for full list.
- **Decisions in the comments timeline**: each decision renders as
  a timeline event entry, like the existing status change events
  — *"CEO approved this task"* / *"Eng Lead requested changes:
  '...'"*.
- **Properties panel**: the existing `<ReviewersPicker>` and
  `<ApproversPicker>` chips gain a small status icon next to each
  agent showing their current decision (✓ approved, ✕ changes
  requested, ◯ pending).

### G. WS events

- `office.task.decision_recorded` payload `{ task_id, decision_id,
  role, decider_type, decider_id, decision, created_at }`.
- `office.task.review_requested` payload `{ task_id, role,
  reviewer_agent_id }` — fans out per reviewer for client-side
  inbox refresh.

The existing `office.task.updated` event continues to fire on
mutations to the lists themselves (already implemented).

### Out of scope

- **Conditional approval** (e.g. "any 1 of 3 approvers"). v1 is
  unanimous: *every* listed approver must approve.
- **Approval delegation** ("Eng Lead is OOO; let X approve in their
  place"). Delegation can be added later via a delegate column or a
  separate table.
- **Bulk approve** across many tasks at once. Single-task only.
- **External approvers** (humans who are not in the workspace's
  agent list). v1 only supports workspace agents and the single
  human user.
- **Approval expiry / TTL** ("approval valid for 7 days"). Out of
  scope; rework still clears decisions.
- **Reviewer role gating completion**. Reviewers are advisory only.
  If you want a hard gate, add the agent to `approvers`.

## Acceptance

1. Add CEO as an approver on TES-42, status `todo`. CEO does
   nothing. Move task to `done` via the status picker → 409, toast
   reads *"Cannot mark done: awaiting approval from CEO"*. Status
   stays `in_review` (we redirect the transition there as a
   convenience).
2. Move task to `in_review`. CEO receives a `task_review_requested`
   wakeup AND the task appears in CEO's inbox as a `task_review_
   request` item. The agent's wakeup prompt mentions the role.
3. CEO posts `POST /tasks/TES-42/approve`. Decision row created.
   `office.task.decision_recorded` event fires. Comments timeline
   shows *"CEO approved this task just now"*.
4. With CEO as the only approver and an approval recorded, status
   moves `in_review → done` successfully. The assignee receives a
   `task_ready_to_close` wakeup at the moment the final approval
   lands (so they can take action even before the user transitions
   the status).
5. Add a second approver Eng Lead. Eng Lead posts `request-changes`
   with comment *"please update the docs"*. Decision row created
   with `changes_requested`. Assignee receives a
   `task_changes_requested` wakeup carrying the comment. Status
   transition to `done` is still gated (CEO already approved but
   Eng Lead hasn't).
6. Assignee fixes the docs and moves status back to `in_review`.
   All prior decisions are superseded; CEO and Eng Lead must
   approve again.
7. Properties panel chips show the per-agent decision icon. Hover
   the gated status pill on the task header → tooltip lists
   pending approvers.
8. Reviewers (not approvers) get the wakeup and inbox item, but
   their decision (or lack of one) does not gate `in_review → done`.
