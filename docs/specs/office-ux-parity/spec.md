---
status: shipped
created: 2026-05-03
owner: cfl
---

# Office: Task UX improvements

## Why

Today an office task feels static even while an agent is actively
working on it. You file "present yourself", hit send, and the only
way to know the CEO is doing something is the small live-agent badge
in the sidebar agents list. The task detail page itself looks idle:
comments appear abruptly when the agent finishes, with no inline
indication that work is in progress. The Dashboard sidebar row has
no badge at all, even when several runs are live. And the right-side
Properties panel shows status, priority, assignee, project, etc. as
plain text — none of it clickable.

Two complementary patterns to implement:

1. **Live presence is visible everywhere a task lives.** Sidebar
   "Dashboard" row has a `● N live` pill; per-agent rows do too.
   On the task detail page, while a session is running, a
   comment-shaped block appears **inline at its chronological
   position in the timeline** showing `RUNNING · Working · for 3
   seconds · ran 1 command`, with the agent's streaming chat
   embedded below. The task page header shows a small spinner +
   "Working" while a session is active. When the run finishes,
   the block collapses to a single-line summary that stays in the
   timeline and can be re-expanded.

2. **Properties are click-to-edit popovers, not labels.** Status,
   Priority, Labels, Assignee, Project, Parent, Blocked-by,
   Sub-issues, Reviewers, Approvers — every row is a popover
   trigger. Updates are fully optimistic with rollback + toast on
   failure.

A lot of pieces already exist: `SessionWorkEntry` already does the
collapsible card with embedded `<AdvancedChatPanel>`, the
`task_blockers` table has full repo CRUD, `priority` and `project_id`
columns exist, and the per-agent sidebar badge is already shipped.
Much of this work is plumbing existing pieces into the right
surfaces and exposing the missing fields.

### Scope notes

- **Assignees are agents only.** Kandev's office model is single-
  user — there is no concept of multiple human users to assign to
  ("Assign to me" / `assignee_user_id` / a creator field). The
  assignee picker offers "No assignee" + workspace agents only.
- **Reviewers/approvers are a static list on the task**, stored in
  a new `office_task_participants` table. Approvals are surfaced
  as approvals attached to a task, but kandev's `Approval` model
  has no FK to tasks (only JSON payload), and adding workflow-
  driven attached-approval semantics is much larger scope than a
  participants table. The static list is the reviewer/approver
  configuration; whether/how it gates approvals is a separate spec.

## What

Two parts. Either ships independently. Both are wired end-to-end —
no read-only stubs, no follow-ups.

### Part A — Live presence

A1. **Inline session entries in the comments timeline.** The chat
timeline gains a third entry kind alongside comments and timeline
events: a "session" entry, one per session for the task, ordered by
`session.startedAt`. Each entry renders a collapsible card.
- **Active session** (`state in {RUNNING, WAITING_FOR_INPUT}`):
  expanded by default, header shows `RUNNING · Working · for
  {elapsed} · ran {N} commands`, body embeds `<AdvancedChatPanel
  taskId sessionId hideInput />`. `{N}` is derived from
  `messages.bySession[sessionId]` filtered for `type === "tool_call"`.
- **Completed session** (`COMPLETED | FAILED | CANCELLED`):
  collapsed by default to one line — `{Agent} worked for {duration}
  · ran {N} commands` — click to re-expand the full transcript.
- The pinned `<SessionWorkEntry>` block above `<TaskChat>` and the
  `<SessionTabs>` selector are removed; their concerns dissolve
  into the timeline.
- For tasks with > 50 sessions, only the 50 most recent render
  inline; an explicit "Show older sessions" link expands the rest.

A2. **Topbar "Working" indicator on the task page header.** While
the task has at least one active session, render
`<IconLoader2 animate-spin /> Working` next to the task title in
the page header. Clicking it scrolls the timeline so the active
session entry is visible at the bottom of the viewport (via a ref
attached to that entry). Hidden when no active session — no layout
reservation.

A3. **Sidebar "Dashboard" live badge.** The sidebar Dashboard row
gains a `● N live` pill. Visual identical to the existing per-agent
badge. Count is the total number of `RUNNING | WAITING_FOR_INPUT`
sessions in the workspace.

A4. **Auto-scroll behavior.** When a new active session entry first
appears in the timeline, scroll the chat container to the bottom —
**only** if the user is already at the bottom (within ~80px). If
they've scrolled up, do not yank focus. Same rule applies to new
streaming message chunks within an active entry.

A5. **Reactive only — no polling.** All updates flow through
existing WS events (`session.state_changed`,
`session.message.added`, `session.message.updated`,
`office.comment.created`, plus a new `office.task.updated` for
property mutations — see B7). No `setInterval`.

### Part B — Properties panel popover selectors

B1. **Schema migrations (backend).**
- `priority`: change column type from `INTEGER DEFAULT 0` to `TEXT
  DEFAULT 'medium'`. Map all existing values to `"medium"` (the
  current int values are not surfaced anywhere meaningful).
- New table `office_task_participants`:
  ```
  task_id            TEXT NOT NULL,
  agent_instance_id  TEXT NOT NULL,
  role               TEXT NOT NULL,  -- 'reviewer' | 'approver'
  created_at         DATETIME NOT NULL,
  PRIMARY KEY (task_id, agent_instance_id, role),
  FOREIGN KEY (task_id) REFERENCES office_tasks(id) ON DELETE CASCADE
  ```

B2. **Click-to-edit pickers.** All optimistic with rollback + toast
on failure (optimistic cache update + `onError → snapshot rollback + toast`).

- **Status** — popover with the canonical status list (Backlog, Todo,
  In Progress, In Review, Blocked, Done, Cancelled). Calls existing
  `PATCH /tasks/:id` with `status`.
- **Priority** — popover with the four-value enum: Critical, High,
  Medium, Low. Calls `PATCH /tasks/:id`
  with `priority`. Always required (no "none").
- **Labels** — refit existing inline label editor into the same
  popover-row visual pattern.
- **Assignee** — searchable popover: "No assignee" + workspace
  agents. Substring match on display name. Calls `PATCH /tasks/:id`
  with `assignee_agent_instance_id` (empty string clears).
- **Project** — searchable popover: workspace projects + "No
  project". Calls `PATCH /tasks/:id` with `project_id`.
- **Parent** — searchable popover: workspace tasks (excluding the
  current task) + "No parent". Backend rejects only direct self-
  reference (`parent_id == task_id`). No descendant scan, no depth
  check.
- **Blocked by** — multi-select popover. Chip rows show blocker
  task identifier + title; "x" removes. New endpoints `POST
  /tasks/:id/blockers` and `DELETE /tasks/:id/blockers/:id` wrap
  the existing repo CRUD.
- **Sub-issues** — list of children + "+ Add sub-issue" button.
  Opens the existing `<NewTaskDialog>` pre-filled with
  `parentTaskId`, `defaultProjectId`, `defaultAssigneeId` (the
  dialog already accepts these props). No "link existing" path.
- **Reviewers / Approvers** — multi-select agent picker per role.
  Add via `POST /tasks/:id/reviewers` (or `/approvers`) with
  `{ agent_instance_id }`; remove via `DELETE
  /tasks/:id/reviewers/:agentId`. Chip rows.

B3. **Reused `<PropertyRow>` primitive.** Single component for
label/value layout. Searchable pickers reuse
`apps/web/components/combobox.tsx`. Fixed-list pickers (Status,
Priority) are small bespoke components. Multi-selects share a
`<MultiSelectPopover>` primitive that wraps the combobox + chip rows.

B4. **Permission gates.**
- Users always allowed.
- Agent callers ungated for status / priority / labels / project /
  parent / blockers (matches today's status-mutation posture).
- Agent callers gated by `PermCanAssignTasks` for assignee
  (existing rule).
- Agent callers gated by `PermCanApprove` for reviewers /
  approvers CRUD (governance action — agents shouldn't silently
  add or remove their own reviewers).
- Sub-issue creation inherits `PermCanCreateTasks` via the
  unchanged task-create endpoint.

B5. **Reactivity.** The existing reactivity pipeline already runs
on status and assignee mutations. New mutations get the following
treatment:
- Priority: no wakeup, no activity entry (matches existing
  decision #4 — priority is metadata only).
- Project / parent: emit an activity entry. No wakeup.
- Blockers added/removed: emit an activity entry. No new wakeup;
  the existing on-blocker-resolved cascade keeps working.
- Reviewers/approvers added: emit an activity entry. No wakeup
  in v1 (workflow integration is a separate spec).

B6. **`PATCH /tasks/:id` extensions.**

```go
type UpdateTaskRequest struct {
    Status                  string  `json:"status"`
    Comment                 string  `json:"comment"`
    AssigneeAgentInstanceID string  `json:"assignee_agent_instance_id"`
    ModelProfile            *string `json:"model_profile"`
    Reopen                  bool    `json:"reopen"`
    Resume                  bool    `json:"resume"`
    Priority                *string `json:"priority,omitempty"`
    ProjectID               *string `json:"project_id,omitempty"`
    ParentID                *string `json:"parent_id,omitempty"`
}
```

Validation:
- `priority`: one of `"critical" | "high" | "medium" | "low"`.
- `project_id`: empty (clears) or a project belonging to the same
  workspace.
- `parent_id`: empty (clears) or a task in the same workspace,
  not equal to the task itself.

B7. **WS events for property mutations.** Today only
`office.task.status_changed` is broadcast. Add a generic
`office.task.updated` event published on every successful property
mutation (priority / project / parent / assignee /
blockers-add/remove / participants-add/remove). Payload:
`{ task_id, workspace_id, fields: ["priority", ...] }`. Frontend
subscribes via the existing office-task subscription path and
patches the local cache by re-fetching the task DTO. Status and
comment events keep their existing dedicated channels.

### Out of scope

- Approval-flow integration for reviewers/approvers (the static
  list configures *who*; the *flow* is a follow-up spec).
- Cross-workspace task linking for Parent / Blocked-by / Sub-issues.
- Replacing the existing per-agent sidebar badge (A3 reuses the
  same component for the new Dashboard-row sibling badge).
- Multi-step blocker cycle detection (B blocks A → C blocks B →
  reject A blocks C). v1 only rejects direct self-blockage,
  v1 only rejects direct self-blockage.

## Acceptance

A user creates a task assigned to an agent and the agent starts
working.

1. The sidebar Dashboard row shows `● 1 live`. The agent's row in
   the sidebar agents list also shows `● 1 live`. Both update via
   WS without a page refresh.
2. The task page header shows `<spinner /> Working` next to the
   title while a session is active. The indicator disappears the
   moment the session reaches a terminal state. Clicking it
   scrolls the timeline to the active session entry.
3. On the task detail page, an inline session entry appears at its
   chronological position in the comments timeline, expanded by
   default, with header `RUNNING · Working · for {seconds} · ran
   {N} commands` and a streaming agent transcript below.
4. When the agent finishes, the entry collapses to one line —
   `{Agent} worked for {duration} · ran {N} commands`. The
   resulting comment lands in the timeline below it. Clicking the
   collapsed entry re-expands the full transcript.
5. The pinned `<SessionWorkEntry>` block and `<SessionTabs>` are
   gone from the page.
6. Right-side Properties panel: every row is a popover trigger.
   - Status, Priority, Labels — fixed-list pickers
   - Assignee, Project, Parent — searchable single-select
   - Blocked by — multi-select with chip rows
   - Sub-issues — "+ Add sub-issue" opens `<NewTaskDialog>` with
     `parentTaskId` set
   - Reviewers, Approvers — multi-select agent pickers with chip
     rows; gated for agent callers behind `PermCanApprove`
7. Edits are fully optimistic. On error: cache rolls back to the
   pre-edit snapshot and a toast surfaces the failure. No inline
   per-field error state.
8. No `setInterval` is added anywhere. Live updates flow purely
   through existing WS events plus the new
   `office.task.updated` event.
