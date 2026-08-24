---
status: draft
system: office
requirements:
  - REQ-OFFICE-TASKS-001
created: 2026-05-02
owners:
  - cfl
---
# Office Tasks System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-TASKS-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-TASKS-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Why

Office tasks are the unit of work in office mode: a single task carries a description, a workspace, an assignee, optional reviewers and approvers, blockers, parent/child relationships, a chat thread, and a per-agent session of working memory. Users and agents need to drive these tasks end-to-end - hand off context across a tree of subtasks, request review and approval, edit properties inline, see the agent's work in the chat, react to property changes without an agent having to poll, and preserve each participant's conversation across many short-lived agent runs.

This spec consolidates the office task surface: lifecycle, parent/child handoffs, approval flow, advanced execution mode, blocker cycle detection, the reactivity pipeline, per-(task, agent) session identity, inline editable properties, and the chat / activity views.

## What

### A. Lifecycle and identity

- A task progresses through statuses `todo → in_progress → in_review → done`, with `blocked` and `cancelled` as branchable states. Status transitions are user- or agent-driven and feed the reactivity pipeline (section E).
- Every office task has zero or more **task sessions**: one per `(task_id, agent_instance_id)` pair. A session represents one agent's persistent conversation thread on the task, not a single launch.
- Sessions cycle through `CREATED → STARTING → RUNNING → IDLE → RUNNING → IDLE → ...` for as many turns as the agent is woken. A session is terminal (`COMPLETED` / `FAILED` / `CANCELLED`) only when the agent leaves the task's participants list.
- Wakeups (section E) drive transitions IDLE → RUNNING. Turn-complete events drive RUNNING → IDLE, tearing down the executor and agent process entirely; the conversation is preserved via the stored ACP session token.
- Kanban / quick-chat sessions keep their per-launch model and `WAITING_FOR_INPUT` semantics - office's IDLE state is office-scoped.

### B. Parent / child handoffs

- Tasks form a tree via `parent_id`. Parent tasks act as the default home for shared specs, plans, and coordination documents.
- Child tasks can read parent-owned documents and write parent-owned coordination documents by default. Document handoffs reuse the existing **blocker mechanism**: a consumer task is blocked-by the producer task and reads the resulting documents from its wakeup prompt context. There is no separate "required documents" data type.
- Agents can list related tasks (parent, children, siblings, blockers, blocked) and can read/write allowed task documents via MCP or CLI tools.
- A parent task defines a default child workspace policy for its tree: children **inherit the parent workspace** OR **create their own workspace**.
- A parent task defines a default child ordering policy: children are created with dependency edges (sequential) or without (parallel).
- Each child task can override the parent workspace policy and run in the parent workspace, a new workspace, or an explicit shared workspace group.
- When a user creates a subtask from a Kanban task detail dialog, the dialog lets them choose to inherit the parent materialized workspace or create a new workspace from repositories, local folders, or remote URLs.
- When the user chooses to inherit the parent workspace, the dialog can identify the parent session branch because the child shares that worktree and file state.
- When the user chooses a new workspace, the dialog shows only the selected source and base-branch controls. It does not present the parent session branch as the branch of the isolated child workspace.
- When an agent creates a subtask through `create_task_kandev`, the MCP surface mirrors that choice: subtasks inherit the parent materialized workspace by default, and `workspace_mode="new_workspace"` requests a fresh materialized workspace/worktree.
- Subtasks inheriting a parent materialized workspace are represented through the same **shared workspace group** model used by Office-created tasks.
- Shared workspaces are visible in the UI: source task, member tasks, materialized path/environment, and whether tasks are ordered by dependencies.
- Workspace sharing does not lock execution in v1. Sequential behavior is expressed through task blocker / dependency edges.
- Wakeup prompt context names whether the agent should read parent documents, source-task documents, or shared workspace files; document bodies are NOT injected inline.
- Archiving or deleting a task releases it from shared workspace membership and can trigger cleanup when no active, non-archived members remain.
- Archiving a parent task cancels active descendant runs, recursively archives descendants, releases their workspace memberships, and runs cleanup.
- Deleting a parent task cancels active descendant runs, recursively deletes descendants, releases their workspace memberships, and runs cleanup.

### C. Approval flow

- A task carries two participant lists: **reviewers** (advisory) and **approvers** (gating). The lists are edited inline from the Properties panel.
- When a task enters `in_review`, every reviewer AND approver is woken with `task_review_requested`. The wakeup carries `role` so the agent's prompt renders an appropriate hint.
- A task SHALL NOT transition `in_review → done` while it has any approvers without a current `decision = approved`. Reviewers are advisory and do not gate completion.
- Transitioning `in_review → todo|in_progress` (rework) supersedes all prior decisions; re-entering `in_review` starts a fresh round.
- Approving a task while approvers are pending wakes the assignee with `task_changes_requested` (when changes are requested) or `task_ready_to_close` (when the final approval lands).
- Recording a decision does NOT terminate the deciding agent's session. The reviewer's session stays IDLE between cycles and is re-used on a subsequent `task_review_requested` wakeup.
- Tasks awaiting the current user's review/approval surface in the inbox as item type `task_review_request` only when the user/agent is a reviewer or approver participant with a required decision. Runner/assignee participation alone does not create a review inbox item.
- v1 approval is **unanimous**: every listed approver must approve. There is no quorum / any-of-N mode.

### D. Inline editable properties

- Every property row that holds an enumerable, lookup-able, or free-text value is editable inline by clicking the value: Status, Priority, Assignee, Project, Parent task, Blocked by, Reviewers, Approvers, Labels.
- Read-only rows stay read-only: Created by, Started, Completed, Created, Updated, Tree cost metrics. They reflect events, not user intent.
- Edits are **optimistic**: the new value renders immediately; on API failure the row reverts to the prior value and a toast surfaces the error.
- Pickers are searchable when the candidate list could exceed roughly 10 items (Assignee, Project, Parent, Blocked-by). Status and Priority pickers use a fixed list. The threshold for switching to a search input is around 8 candidates.
- The editor works inline on the task detail page - no separate dialog, no navigation.
- Property changes broadcast over the existing WS event stream so other open clients update reactively. No polling.

### E. Reactivity pipeline

A backend pipeline runs synchronously on every relevant task property change. Property mutators (`UpdateTaskStatus`, `MoveTask`, `SetTaskAssignee`, `CreateTaskComment`, blocker mutations, participant mutations) feed the pipeline; the pipeline returns wakeups, patches, and decision records that are persisted in one DB transaction alongside the user's change.

**Wakeup triggers:**

- `status → done`: query tasks blocked by this task; wake each with `{reason: "blocker_resolved", resolved_blocker_task_id}`. Wake the parent task with `{reason: "child_completed"}` if all siblings are done.
- `status: blocked → unblocked`: wake assignee with `{reason: "task_unblocked"}`.
- `status: done|cancelled → todo|in_progress`: wake assignee with `{reason: "task_reopened", actor_id, actor_type}`.
- **Assignee change**: wake new assignee with `{reason: "assigned", comment_id?, actor_id}`. If old assignee had an active session, cancel it cleanly (`session.cancelled` notification) and surface that to the new assignee's context.
- **Created task with assignee**: if a newly-created task or subtask already has a runner, queue the same assigned wakeup idempotently with `{reason: "assigned", task_id}`.
- **Comment created by user**: wake assignee with `{reason: "user_comment", comment_id}`. Wake any @-mentioned agents in the same workspace with `{reason: "mentioned", comment_id}`.
- `status → in_review`: wake every reviewer and approver with `{reason: "task_review_requested", role}`.
- `decision = changes_requested`: wake assignee with `{reason: "task_changes_requested", comment}`.
- All approvers have approved AND status is `in_review`: wake assignee with `{reason: "task_ready_to_close"}`.
- `status → cancelled`: cancel the task's active execution immediately (interrupt the current turn, mark the session cancelled) and emit `office.task.cancelled`.

**Execution-policy transition engine** runs alongside wakeups: reads the task's `execution_policy` (work / review / approval stages) and returns (1) a patch to apply alongside the user's change (e.g. advance `execution_state` to `review_pending`), (2) wakeups to queue, (3) a decision record if the transition encodes an approval verdict. The engine is invoked from every mutation entry point and MUST NOT poll or schedule.

**Wakeup context** carries a structured JSON payload: `reason`, `task_id`, optional `actor_id`/`actor_type`, optional `comment_id`, cascade-specific fields (`resolved_blocker_task_id`, `child_task_id`), policy fields (`stage_id`, `allowed_actions`). The agent runtime reads `context.reason` to select the system prompt template.

### F. Blocker cycle detection

- Adding a blocker `B blocks A` SHALL be rejected when a path already exists from `B` back to `A` through existing blockers. This catches cycles of length 3 or more (existing checks already cover self-blocking and direct two-node cycles).
- The detection runs as a breadth-first walk starting at the proposed `blockerTaskID`, traversing the blocker chain via existing repository queries, bounded by a visited set.
- A rejected attempt returns a typed error including the cycle path (e.g. `"would create cycle: A → B → C → A"`). The frontend blockers picker surfaces the message as a toast and rolls back its optimistic chip.

### G. Per-(task, agent) session lifecycle

- `task_sessions` rows are keyed by `(task_id, agent_instance_id)` for office tasks. The same pair reuses one row across many wakeups. Kanban / quick-chat sessions leave `agent_instance_id` NULL and keep their per-launch + `is_primary` model.
- A single task may carry multiple sessions: assignee, each reviewer, each approver, each @-mentioned agent. Each maintains its own conversation; one agent's notes never appear in another's buffer.
- On wakeup, `EnsureSessionForAgent(task, agent_instance)`:
  1. Looks up the row by `(task_id, agent_instance_id)`.
  2. If found and IDLE: flip to RUNNING.
  3. If found and RUNNING/STARTING: return as-is (idempotent).
  4. If found and terminal: create a new row (prior pair was retired).
  5. If not found: insert with state `CREATED`.
- ACP init: if `acp_session_id` is stored, call `session/load`. On failure (expired session, agent CLI version mismatch), fall back to `session/new` and overwrite the stored token. Row identity is preserved at the kandev level even when the underlying conversation is reset.
- On turn complete for office sessions, the agent process AND executor backend (container, standalone process, sprites instance, agentctl) tear down completely. The conversation is preserved via the stored ACP token. The next wakeup recreates the executor cold.
- A session goes terminal (`COMPLETED`) only on **participation removal**:
  1. Reassignment retires the previous assignee's session.
  2. Removing a reviewer/approver via the picker retires that agent's session.
  3. Deleting the agent instance at workspace level cascades and retires all of its sessions across all tasks.
- Decisions (approve / request-changes) do NOT terminate the deciding agent's session - they cycle RUNNING → IDLE so a subsequent review round resumes the same conversation.
- Re-adding the same agent after a terminal row creates a fresh row with a fresh conversation thread.

### H. Advanced mode

- `/office/tasks/[id]?mode=advanced` exposes dockview panels (files, terminal, changes) backed by a running agentctl execution.
- Office tasks usually have no warm execution (the agent ran and tore down), so entering advanced mode SHALL ensure an execution exists for the resolved session without sending a new prompt to the agent.
- The session resolved for advanced mode is:
  1. The current viewer's session if the viewer is an authenticated agent.
  2. Otherwise (singleton human user), the current assignee's session.
  3. If no session exists yet (user enters advanced mode before the first wakeup), create one for the assignee on demand, then ensure the executor.
- Panels gate on `agentctlStatus.isReady`; existing WS events `session.agentctl_starting` / `session.agentctl_ready` drive their loading states. No panel directly creates executions.
- If executor resume fails, the call is non-fatal: the session still returns and panels show their existing "not available" states.

### I. Task chat

The Chat tab on the office task detail page is a unified timeline of comments, agent work, and system events.

**Auto-bridging agent responses to comments:**

- When an agent completes a turn, its final text message is auto-posted as a `task_comment` with `author_type=agent` and `source=session`.
- Only messages of type `message` (not `status`, `script_execution`, etc.) are bridged.
- The comment carries the agent's name as `author_id`.
- Duplicate comments are prevented by checking for an existing `source=session` comment for the same turn.

**Merged thread:**

The Chat tab displays a chronological merge of:
1. **User comments** posted via the comment input or API.
2. **Agent comments** auto-bridged from session messages, or posted via `kandev comment add`. Agent comments from auto-bridge show a "via session" indicator.
3. **Timeline events** for status changes (SCHEDULING, IN_PROGRESS, REVIEW, DONE) and assignee changes, rendered as compact system entries.
4. **Inline approval-decision events** ("CEO approved this task" / "Eng Lead requested changes: '...'").

**Expandable agent session in chat (v2):**

- Between comments, each agent work session appears as a collapsible entry, one per (task, agent) pair, ordered by most-recent activity.
- Collapsed: shows "Agent worked for Xm Ys" with a chevron. If running, shows a spinner and "Agent working..." and auto-expands.
- Expanded: renders the `AdvancedChatPanel` component (reused from advanced mode) with the full session transcript - tool calls, thinking, messages.
- A task with assignee + 2 reviewers shows up to 3 entries. Each entry collapses when its agent's session is IDLE; expanded by default while RUNNING.
- The "ran N commands" header derives from messages on that session, accumulated across launches.

**Reply wakes agent:**

- Posting a comment on a task with an `assignee_agent_instance_id` queues a `task_comment` wakeup for that assignee.
- The agent receives the comment text in its wakeup context and can respond on its next turn.

**Activity tab:**

- Task status transitions (CREATED → SCHEDULING → IN_PROGRESS → REVIEW → DONE) are logged to `office_activity_log` with `target_type=task` and `target_id=taskID`.
- The Activity tab renders these entries with the status transition and timestamp.

**Sidebar counters:**

- The office sidebar shows count badges next to Tasks, Skills, and Routines, fetched from the dashboard API or dedicated count endpoints, updated on navigation or data change.

## Data model

```
task_sessions
  id                     TEXT PK
  task_id                TEXT FK -> office_tasks.id
  agent_instance_id      TEXT  nullable  -- office: set; kanban/quick-chat: NULL
  acp_session_id         TEXT  nullable  -- preserved across IDLE turns
  state                  enum   CREATED | STARTING | RUNNING | IDLE
                                | WAITING_FOR_INPUT | COMPLETED | FAILED | CANCELLED
  is_primary             bool   -- kanban resume; office never reads this
  ...

  UNIQUE INDEX uniq_office_task_session
    ON task_sessions(task_id, agent_instance_id)
    WHERE agent_instance_id IS NOT NULL;

office_task_approval_decisions
  id              TEXT PK
  task_id         TEXT FK -> office_tasks.id ON DELETE CASCADE
  decider_type    enum   user | agent
  decider_id      TEXT   '' for user
  role            enum   reviewer | approver
  decision        enum   approved | changes_requested
  comment         TEXT   default ''
  created_at      DATETIME
  superseded_at   DATETIME nullable  -- non-NULL when replaced by a newer decision

  INDEX idx_task_decisions_task (task_id)

office_task_execution_decisions
  id           TEXT PK
  task_id      TEXT FK -> office_tasks.id
  stage_id     TEXT
  verdict      enum   approved | changes_requested | rejected
  actor_id     TEXT
  actor_type   enum   user | agent
  comment_id   TEXT  nullable
  created_at   DATETIME

task_blockers                         -- existing
  task_id         TEXT FK
  blocker_task_id TEXT FK
  CHECK (task_id != blocker_task_id)

task_comments                         -- existing, extended
  ...
  author_type   enum   user | agent
  source        enum   manual | session   -- session = auto-bridged from agent turn

office_activity_log                    -- existing, extended
  target_type   enum   ... | task
  target_id     TEXT
  event_type    TEXT  -- 'status_changed', ...
  ...
```

Multiple rows per `(task, decider, role)` in `office_task_approval_decisions` are allowed; only the most recent (`superseded_at IS NULL`) counts toward the gate.

## API surface

**HTTP (office task service):**

- `POST /tasks/:id/approve { comment?: string }` - caller approves. 403 if caller is not in reviewers or approvers. Returns the created decision row.
- `POST /tasks/:id/request-changes { comment: string }` - caller requests changes. Comment required. Same 403 rule. Returns the decision row.
- `GET /tasks/:id/decisions` - current (non-superseded) decisions. Also surfaced on the task detail endpoint as a `decisions: TaskDecision[]` field.
- `PATCH /tasks/:id` - inline property edits (status, priority, assignee, project, parent, blocked-by, reviewers, approvers, labels). Returns the updated task.
- `POST /tasks/:id/blockers { blocker_task_id }` - returns 400 with a body carrying the cycle path when the addition would create a cycle.
- `DELETE /tasks/:id/reviewers/:agentId` / `DELETE /tasks/:id/approvers/:agentId` - removes the participant AND terminates that agent's session for the task.

**WebSocket:**

- `session.ensure { task_id, ensure_execution?: bool }` - idempotent; returns the resolved session. When `ensure_execution` is true and the session exists without a running execution, the backend ensures the executor resumes. Non-fatal on resume failure.
- `office.task.decision_recorded { task_id, decision_id, role, decider_type, decider_id, decision, created_at }`.
- `office.task.review_requested { task_id, role, reviewer_agent_id }` - fans out per reviewer for client-side inbox refresh.
- `office.task.cancelled { task_id }`.
- `office.task.updated` / `office.task.status_changed` - existing events continue to fire on property and status mutations.
- `session.agentctl_starting` / `session.agentctl_ready` - existing events drive advanced-mode panel readiness.

**Go interfaces:**

- `EnsureSessionForAgent(ctx, task, agentInstance) (*TaskSession, error)` - office wakeup launch path.
- `applyTaskExecutionPolicyTransition(task, change) (patch, []Wakeup, *Decision)` - policy engine, invoked synchronously inside every mutator that can advance a stage.
- Wakeup queue producers emit a structured `context` JSON (`reason`, `task_id`, optional fields per section E).

## State machine

**Task session (office):**

```
CREATED ──► STARTING ──► RUNNING ──► IDLE ──► RUNNING ──► IDLE ──► ...
                              │           │
                              ▼           ▼
                          COMPLETED   COMPLETED        (on participation removal)
                          / FAILED    / FAILED
                          / CANCELLED / CANCELLED
```

- `IDLE → RUNNING`: a wakeup for the matching `(task, agent_instance)` fires. `EnsureSessionForAgent` flips the row.
- `RUNNING → IDLE`: turn-complete event for an office session. The agent process and executor backend tear down. `acp_session_id` is preserved.
- `RUNNING → CANCELLED`: status set to `cancelled`, or an explicit hard cancel from the reactivity pipeline (e.g. reassignment of the prior assignee).
- `* → COMPLETED`: participation removal (reassignment of assignee, picker-removal of reviewer/approver, workspace-level deletion of the agent instance). Plain decisions do NOT cause this.

The `updateTaskSessionState` terminal-state guard relaxes for office: COMPLETED / FAILED / CANCELLED stay terminal, but IDLE → RUNNING is allowed.

Kanban / quick-chat sessions keep `CREATED → STARTING → RUNNING → WAITING_FOR_INPUT → ...` and warm executors between turns; office-only states are not used.

**Task status (relevant transitions for reactivity):**

```
todo ──► in_progress ──► in_review ──► done
   │           │              │           ▲
   │           ▼              │           │
   └───────► blocked          └── todo / in_progress (rework, supersedes decisions)
   └───────► cancelled (from any non-terminal state)
```

## Permissions

There are two authorization layers: **agent capability** (what a running agent can do via runtime APIs) and **HTTP caller identity** (what a UI user or JWT-bearing agent can do via task endpoints). See `agents.md` for the agent role / permission model that supplies the capability set on each run.

**Agent runtime capabilities** (resolved from role + permission overrides; enforced by the office runtime handlers):

| Task action | Required capability | Default holders |
|---|---|---|
| Post a comment / reply | `post_comment` | All roles (assignee, reviewers, approvers, mentioned agents). |
| Change task status | `update_task_status` | All roles. |
| Create a subtask | `create_subtask` | Roles with `can_create_tasks` (CEO, worker). |
| Request approval on a task | `request_approval` | Roles with `can_approve` (CEO). |
| Spawn an agent run on a task (assign and wake) | `spawn_agent_run` | Roles with `can_assign_tasks` (CEO, worker). |

An agent's run JWT also pins an `AllowedTaskIDs` scope. The runtime rejects mutations against any task outside that scope with `ErrTaskOutOfScope` (403, `shared.ErrForbidden`), even when the capability is granted - this prevents a worker on task T1 from mutating task T2 just because both belong to its workspace.

**HTTP endpoint gates** (`internal/office/approvals`, task service):

- `POST /tasks/:id/approve`, `POST /tasks/:id/request-changes`: when called by an agent (caller has an agent JWT), the handler additionally checks: (1) `caller.WorkspaceID == task.WorkspaceID` (403 otherwise), (2) `has_permission(can_approve)` (403 otherwise), (3) the caller is not the requester of the approval (403 self-decide). Reviewers / approvers list membership is the user-facing gate; the `can_approve` permission is the underlying agent gate. UI callers (no agent JWT) are accepted without these checks today.
- `DELETE /tasks/:id/reviewers/:agentId`, `DELETE /tasks/:id/approvers/:agentId`: caller must be a workspace UI user; no agent-side endpoint exposes participant removal.
- `PATCH /tasks/:id` (inline property edits) and `POST /tasks/:id/blockers`: no per-field authorization in v1; any UI user with workspace access can edit any task, and any agent with the appropriate runtime capability can mutate within its task scope.
- Approver gate on `done`: independent of caller identity. A `done` transition is rejected 409 while any approver lacks a current `approved` decision (see Failure modes).

**Out of scope for this iteration:**

- A "no permission" UI state for fields a user cannot edit, and per-field user-side permission rules (see `out of scope`).
- Granular per-task ACLs beyond workspace membership.
- Workspace-level rules for who can be added as assignee / reviewer / approver - those follow the existing agent / workspace membership rules and are not redefined here.
