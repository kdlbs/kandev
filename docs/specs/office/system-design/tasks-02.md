---
status: draft
system: office
requirements:
  - REQ-OFFICE-TASKS-001
created: 2026-05-02
owners:
  - cfl
---
# Office Tasks System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-TASKS-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-TASKS-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Failure modes

- **Approver gate violation.** A `done` transition while approvers are pending is rejected with 409 and a typed error naming the missing approvals. The UI redirects the transition to `in_review` as a convenience.
- **Blocker cycle.** Adding a blocker that would create a cycle returns 400 with the cycle path. The frontend rolls back the optimistic chip and toasts the message.
- **Optimistic property edit failure.** The UI reverts the row to the prior value and shows a toast. No partial server state.
- **ACP `session/load` failure.** Fall back to `session/new`, overwrite the stored `acp_session_id`, treat the conversation as fresh. Row identity at the kandev level persists; chat history at the office level (comments, decision rows, timeline events) is unaffected.
- **`session.ensure { ensure_execution: true }` resume failure.** The session is still returned; the executor is not started; panels show their normal "not available" / "Preparing workspace..." states.
- **Reactivity pipeline DB transaction failure.** The user's change rolls back along with any pending wakeups, patches, and decision records produced by the policy engine - all writes are in one transaction. No partial reactions.
- **Wakeup queue worker not running.** Reactions do not fire. Already the case today; no polling fallback is introduced.
- **Workspace cleanup with active sessions.** Cleanup is deferred until no active, non-archived members reference the workspace AND no active sessions hold it.
- **User-owned local folders.** Never deleted by workspace cleanup, even when the last task referencing them is deleted.

## Persistence guarantees

**Survives restart:**

- Task rows, comments, decision rows, blocker edges, parent/child links, project / assignee / reviewers / approvers assignments.
- `task_sessions` rows including `agent_instance_id`, `acp_session_id`, and `state`. Office IDLE sessions resume on the next wakeup via `session/load`.
- Activity log entries.
- Materialized workspaces owned by Kandev (worktrees, clones, plain folders) - lifecycle tied to task membership, not process lifetime.
- Shared workspace group membership.

**Does NOT survive restart:**

- In-memory execution entries in `lifecycle.Manager`. Office sessions in IDLE are expected to have zero in-memory executions until the next wakeup.
- Agent subprocesses, executor backends (container, standalone, sprites), agentctl HTTP connections for IDLE office sessions.
- Optimistic UI state for unsaved property edits.

**Cleanup rules:**

- Kandev-owned materialized workspaces are cleaned up after the last active member is archived or deleted AND no active sessions reference the workspace. Kandev does NOT snapshot workspace contents before cleanup; files in cleaned folders, clones, or worktrees are intentionally discarded.
- Unarchiving a task with a cleaned Kandev-owned workspace recreates a fresh materialized workspace from stored source configuration when possible. If reconstruction is impossible, the task becomes active with a workspace-requires-configuration status visible to the user.
- User-owned local folders and existing local checkouts are never deleted by workspace cleanup.

## Scenarios

**Handoffs:**

- **GIVEN** a coordinator creates a planning task and an implementation task with the implementation blocked-by the planner, **WHEN** the planner writes `spec` and `plan` documents up to the parent and completes, **THEN** the blocker resolves and the implementation task wakes; its prompt names the parent's available document keys so the agent fetches them via the task document tool.
- **GIVEN** a parent policy says children inherit the parent workspace and run sequentially, **WHEN** the coordinator creates child tasks, **THEN** the children reuse the parent materialized workspace and receive dependency edges that order their execution.
- **GIVEN** a child task inherits a parent materialized workspace, **WHEN** the child is archived or deleted, **THEN** the inherited parent workspace remains available to the parent and any other active child that still references it.
- **GIVEN** a user opens a Kanban task detail page, **WHEN** they create a subtask, **THEN** they can choose to inherit the parent task workspace or create a new workspace by selecting repositories, local folders, or a remote URL.
- **GIVEN** a parent session has an active worktree branch, **WHEN** the user selects **Inherit parent workspace** in the subtask dialog, **THEN** the dialog identifies the parent branch as shared with the child.
- **GIVEN** a parent session has an active worktree branch, **WHEN** the user selects **Create new workspace** in the subtask dialog, **THEN** the parent branch indicator is absent and the selected source's base-branch control is the only branch context shown for the child.
- **GIVEN** an agent creates a subtask via MCP, **WHEN** it omits `workspace_mode`, **THEN** the subtask inherits the parent materialized workspace; **WHEN** it sets `workspace_mode="new_workspace"`, **THEN** the subtask launches in its own materialized workspace/worktree.
- **GIVEN** two tasks share a workspace group without dependency edges, **WHEN** the scheduler starts both, **THEN** they may run concurrently in the same materialized workspace.
- **GIVEN** a parent task has descendant tasks, **WHEN** the user deletes the parent, **THEN** Kandev cancels active descendant runs, deletes every descendant, releases their workspace memberships, and runs cleanup.
- **GIVEN** an archived task's workspace cannot be recreated from stored configuration, **WHEN** the task is unarchived, **THEN** the task becomes active with a workspace-requires-configuration status visible to the user.

**Approval flow:**

- **GIVEN** a task with CEO as the only approver in `todo`, **WHEN** the user attempts to move it directly to `done`, **THEN** the transition is rejected 409 with a toast "Cannot mark done: awaiting approval from CEO" and the status is redirected to `in_review`.
- **GIVEN** a task moves to `in_review`, **WHEN** the reactivity pipeline runs, **THEN** each reviewer and approver receives a `task_review_requested` wakeup AND an inbox item of type `task_review_request`.
- **GIVEN** CEO is the sole approver and approves via `POST /tasks/:id/approve`, **WHEN** the decision is recorded, **THEN** `office.task.decision_recorded` fires, the assignee receives `task_ready_to_close`, and the comments timeline shows "CEO approved this task".
- **GIVEN** an approver requests changes with comment "please update the docs", **WHEN** the decision is recorded, **THEN** the assignee receives `task_changes_requested` carrying the comment AND the `done` transition remains gated.
- **GIVEN** the assignee returns the task to `in_review` after rework, **WHEN** the transition happens, **THEN** all prior decisions are superseded and approvers must approve again.

**Editable properties:**

- **GIVEN** a task is `in_review`, **WHEN** the user clicks the Status value and selects "Done", **THEN** the row updates optimistically within ~100ms and other open clients on the same task observe the change via `office.task.status_changed`.
- **GIVEN** the user clicks Priority and chooses "High", **WHEN** the request fails, **THEN** the priority reverts to its previous value and a toast says "Failed to update priority".
- **GIVEN** the user clicks Parent, **WHEN** they search "KAN" and pick a candidate task, **THEN** the row shows the new parent and `parent_id` updates server-side.

**Blocker cycles:**

- **GIVEN** existing rows `A blocks B` and `B blocks C`, **WHEN** `POST /tasks/A/blockers { blocker_task_id: C }`, **THEN** the response is 400 with a body containing the cycle path "A → B → C → A".
- **GIVEN** the blockers picker, **WHEN** a cycle is rejected, **THEN** the optimistic chip is removed and the toast displays the cycle path.
- **GIVEN** the existing single-step rejection `A blocks B` then `B blocks A`, **WHEN** the second insert is attempted, **THEN** rejection continues to work (no regression).
- **GIVEN** a non-cycling addition `D blocks A` while `A blocks B blocks C`, **WHEN** the insert is attempted, **THEN** it succeeds.

**Reactivity pipeline:**

- **GIVEN** task A is blocked-by task B, **WHEN** B moves to `done`, **THEN** A's assignee receives a wakeup with `context.reason = "blocker_resolved"` and `context.resolved_blocker_task_id = B.id`.
- **GIVEN** task A has children B, C, D all `done`, **WHEN** the last child becomes `done`, **THEN** A's assignee receives `context.reason = "child_completed"`.
- **GIVEN** task A is assigned to agent X with a session running, **WHEN** the user reassigns to agent Y, **THEN** X's session is cancelled cleanly, Y receives `context.reason = "assigned"` with `context.actor_id = <user>`.
- **GIVEN** the assignee is agent X, **WHEN** the user adds a comment "@reviewer please look at this", **THEN** X receives `context.reason = "user_comment"` AND the agent named `reviewer` (if it exists in the workspace) receives `context.reason = "mentioned"`.
- **GIVEN** task A is `in_progress` with stage state `work_in_progress`, **WHEN** the worker advances status to `in_review`, **THEN** the policy engine advances stage state to `review_pending`, the reviewer receives `context.reason = "stage_pending"` with `context.stage_id = "review"`, and no `office_task_execution_decisions` row is yet written.
- **GIVEN** task A has an active session, **WHEN** status is set to `cancelled`, **THEN** the active turn is interrupted within 2 seconds and the session shows `cancelled` state.

**Session lifecycle:**

- **GIVEN** a task with no prior session, **WHEN** the first wakeup runs, **THEN** a single `task_sessions` row is created with `agent_instance_id` matching the assignee and an empty `acp_session_id` until ACP handshake fills it.
- **GIVEN** an office session in IDLE, **WHEN** a second wakeup for the same agent fires, **THEN** no new row is created; the state cycles RUNNING → IDLE → RUNNING; the agent process is launched with `session/load` and resumes the conversation.
- **GIVEN** a task with assignee CEO and reviewer QA, **WHEN** both have been woken, **THEN** `(TES-1, CEO)` and `(TES-1, QA)` are distinct rows with distinct `acp_session_id`s and CEO's working notes do not appear in QA's chat embed.
- **GIVEN** the agent finishes a turn, **WHEN** turn-complete fires for an office session, **THEN** the state flips RUNNING → IDLE BEFORE the workflow handler runs, the agent process exits, the executor backend tears down, and the topbar spinner disappears without a refresh.
- **GIVEN** a reviewer approves, **WHEN** the decision is recorded, **THEN** the reviewer's session goes RUNNING → IDLE (not COMPLETED) and the row keeps its `acp_session_id` for the next review cycle.
- **GIVEN** the task assignee is reassigned, **WHEN** the change is applied, **THEN** the prior assignee's session goes COMPLETED.
- **GIVEN** a reviewer is removed via the picker, **WHEN** `DELETE /tasks/:id/reviewers/:agentId` runs, **THEN** that agent's session for the task goes COMPLETED.
- **GIVEN** an office session is in IDLE, **WHEN** `lifecycle.Manager` is queried, **THEN** it reports zero in-memory executions for the (task, agent) pair until the next wakeup.
- **GIVEN** the stored `acp_session_id` is rejected by the agent CLI, **WHEN** ACP init runs, **THEN** the runtime falls back to `session/new`, the stored token is overwritten, the conversation is treated as fresh, but the kandev session row identity persists.
- **GIVEN** a kanban or quick-chat task, **WHEN** any wakeup or turn runs, **THEN** the per-launch + `is_primary` + WAITING_FOR_INPUT + warm-executor model applies unchanged.

**Advanced mode:**

- **GIVEN** an office task whose agent ran and completed (execution torn down), **WHEN** the user enters advanced mode, **THEN** `session.ensure` is called with `ensure_execution: true`, the backend resumes the executor for the resolved session, and files / terminal / changes panels load.
- **GIVEN** the user leaves and re-enters advanced mode, **WHEN** `session.ensure` runs again, **THEN** the call is idempotent and no duplicate execution is created.
- **GIVEN** the backend restarts while the user is in advanced mode, **WHEN** the next workspace-oriented call is made, **THEN** `GetOrEnsureExecution` recovers the execution on-demand.
- **GIVEN** a task with no prior session, **WHEN** the user enters advanced mode, **THEN** `EnsureSession` creates a new session for the assignee and the execution starts.
- **GIVEN** executor resume fails, **WHEN** `session.ensure { ensure_execution: true }` runs, **THEN** the session is still returned and panels show their existing "not available" states.

**Task chat:**

- **GIVEN** the CEO agent completes a turn on a "present yourself" task, **WHEN** the user opens the task detail page, **THEN** the Chat tab shows the agent's final text as a comment with the agent's name, timestamp, and a "via session" indicator.
- **GIVEN** a task with 2 agent comments and 1 user comment, **WHEN** the user views Chat, **THEN** all 3 entries appear chronologically with distinct styling for agent vs user.
- **GIVEN** a task in REVIEW, **WHEN** the user types a reply and sends, **THEN** a comment is created, the assignee is woken with `task_comment`, and the comment appears in the chat immediately.
- **GIVEN** a task that transitioned TODO → IN_PROGRESS → REVIEW, **WHEN** the user views Chat, **THEN** timeline events for each status change appear inline between comments.
- **GIVEN** a workspace with 5 tasks, 3 skills, 2 routines, **WHEN** the user views the office sidebar, **THEN** count badges show 5 / 3 / 2 next to Tasks / Skills / Routines.
- **GIVEN** an office task where an agent completed a turn, **WHEN** the user views Chat, **THEN** they see a collapsible "Agent worked for 4s" entry next to the auto-bridged comment; expanding it shows the full session transcript with tool calls.
- **GIVEN** an agent currently running on a task, **WHEN** the user views Chat, **THEN** they see "Agent working..." with a spinner, auto-expanded, showing the live session transcript.
- **GIVEN** a task with assignee + 2 reviewers each having had at least one turn, **WHEN** the user views Chat, **THEN** they see up to 3 collapsible agent-session entries, one per (task, agent) pair, ordered by most-recent activity.
- **GIVEN** a task that transitioned CREATED → REVIEW, **WHEN** the user views the Activity tab, **THEN** entries for each status change appear with timestamps from `office_activity_log`.

## Out of scope

- Conditional / quorum approval ("any 1 of 3 approvers"). v1 is unanimous.
- Approval delegation ("Eng Lead is OOO; let X approve in their place").
- Bulk approve / bulk edit across many tasks at once.
- External (non-workspace) approvers - v1 only supports workspace agents and the single human user.
- Approval expiry / TTL ("approval valid for 7 days"). Rework still clears decisions.
- Reviewer role gating completion - reviewers stay advisory. Use approvers for a hard gate.
- Preventing parallel editing of the same dirty workspace; workspace locks; active-holder recovery.
- Changing workspace-mode defaults, repository seeding, base-branch selection, generated task-branch naming, or whether uncommitted parent changes enter an isolated workspace.
- Automatic guessing between task documents and repo files.
- Replacing the existing task documents or task plans implementations.
- Detecting blocker cycles on parent / child relationships - that's a separate domain.
- A background job to scan existing data for pre-existing blocker cycles.
- Polling fallbacks for the wakeup queue.
- Webhook / external notifications (Slack, email) on reactivity events.
- Backfilling decisions for tasks closed before this lands.
- Cross-task session sharing (CEO on TES-1 is independent from CEO on TES-2).
- ACP session expiry / GC; a "GC stale IDLE sessions older than N days" sweep is a future spec.
- Conversation export / import.
- Auto-starting the agent (sending a prompt) on advanced-mode entry - "ensure execution" means agentctl is running, the agent is idle.
- Creating new sessions on advanced-mode entry beyond the on-demand assignee case - generally `ensure_execution` only resumes executions for existing sessions.
- Changing how the kanban task details page works - `ensure_execution` is opt-in.
- A "no permission" UI state for fields the user cannot edit. Defer until a permission model exists.
- Drag-and-drop reordering of sub-tasks; this spec covers scalar properties only.
- Bulk-editing across multiple tasks.
- Editing reviewers / approvers when those roles aren't yet wired end-to-end at the backend.
- Provider / model fallback - covered by `../office-provider-routing/spec.md`.
- Live streaming of agent responses in the office chat (exists in the kanban task detail page).
- Related work tab (inbound / outbound task references).
- Thread interactions (suggest tasks, request confirmation).
- File attachments on comments.
- Multiple-session display in the v2 chat embed beyond one-per-(task, agent) entry; custom transcript parsing (reuse existing `MessageRenderer`).
- Run-level cost tracking per session.

## Open questions

- Should each property edit fire its own focused WS event (`office.task.priority_changed`, etc.) or piggyback on a generic `office.task.updated`? Current direction: generic event, since `office.task.updated` already triggers a refetch.
- For `mentioned` wakeups: do we resolve `@name` against agent names, agent slugs, or both? The implementation plan picks one and documents the matching rule.
- Decision records on policy transitions: one row per stage entry or one per stage exit (with verdict)? The implementation plan picks one.
- Use a new `office.task.cancelled` event subject, or piggyback on `office.task.status_changed` with the new status? The implementation plan picks one.
