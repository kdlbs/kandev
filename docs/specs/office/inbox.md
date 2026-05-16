---
status: draft
created: 2026-04-25
owner: cfl
needs-upgrade: [persistence-guarantees]
---

# Office: Inbox, Approvals & Activity Log

## Why

When agents work autonomously, they will encounter situations that require human judgment: hiring a new agent, exceeding a budget, completing a task that needs review, or hitting an error they cannot resolve. Without a centralized inbox, users must poll individual agent statuses and task lists to discover what needs attention. Without an approval system, agents cannot request permission for high-impact actions. Without an activity log, there is no audit trail of what happened and who did it.

Office adds an inbox that aggregates all items requiring human attention, an approval system for gating agent actions, and a full activity log for audit and debugging.

## What

- The inbox at `/office/inbox` is the user's single view of everything that needs attention.
- The inbox is a **computed view** over pending approvals, budget alerts, agent errors, review requests, and clarification requests - not a separate table.
- Each inbox item shows: type icon, summary text, related agent/task, timestamp, action buttons (approve/reject, view task, dismiss).
- A badge count on the sidebar "Inbox" link shows unresolved items; resolved items move to an archive view.
- New inbox items trigger notifications via existing providers (Local/WebSocket, System/OS, Apprise).
- Approvals SHALL gate high-impact agent actions (hire, budget increase, board approval, task review, skill creation).
- Tasks SHALL support an `execution_policy` defining ordered review and approval stages.
- An activity log SHALL record every significant action with actor, target, and details for audit.

## Data model

### Inbox (computed)

Inbox is not a table. It is a read-side aggregation over:
- **Pending approvals**: hire requests, budget increase requests, board approval requests.
- **Budget alerts**: agents approaching or exceeding limits.
- **Agent errors**: sessions that failed with unrecoverable errors.
- **Review requests**: tasks in `in_review` status.
- **Clarification requests**: agents waiting on a question response.

Items ordered by recency, unresolved first.

### `office_approvals`

| Field | Type | Notes |
|---|---|---|
| `id` | string | PK |
| `workspace_id` | string | FK |
| `type` | enum | `hire_agent` \| `budget_increase` \| `board_approval` \| `task_review` \| `skill_creation` |
| `requested_by_agent_instance_id` | string | null for system-generated |
| `status` | enum | `pending` \| `approved` \| `rejected` |
| `payload` | JSON | type-specific data (see below) |
| `decision_note` | text | optional, from reviewer |
| `decided_by` | string | user ID |
| `decided_at` | timestamp | nullable until resolved |
| `created_at` | timestamp | |

Payload shapes:
- `hire_agent`: proposed name, role, profile, skills, budget.
- `budget_increase`: current budget, requested budget, reason.
- `board_approval`: action description, context, impact.
- `task_review`: task ID, completion summary, deliverables.
- `skill_creation`: skill name, slug, SKILL.md content preview, requesting agent.

### Task `execution_policy` (JSON)

Optional field on each task defining ordered review/approval stages.

```
execution_policy: {
  stages: [
    { type: "review", participants: [{type: "agent", id: "security-agent"}, {type: "agent", id: "qa-agent"}], approvals_needed: 2 },
    { type: "approval", participants: [{type: "user", id: "cfl"}], approvals_needed: 1 }
  ]
}
```

- **Reviewers** (`type=review`): N agents/users review output in parallel; each provides feedback.
- **Approvers** (`type=approval`): M agents/users must approve before `done`; requires `approvals_needed`.
- Stages run sequentially. Within a stage, participants run in parallel. Sequential reviews use multiple single-participant review stages.

A sibling `execution_state` JSON field tracks current stage, which participants have responded, and the outcome.

### `office_activity_log`

| Field | Type | Notes |
|---|---|---|
| `id` | string | PK |
| `workspace_id` | string | FK |
| `actor_type` | enum | `user` \| `agent` \| `system` |
| `actor_id` | string | user ID, agent instance ID, or `"system"` |
| `action` | string | verb (see table below) |
| `target_type` | enum | `task` \| `agent_instance` \| `routine` \| `project` \| `approval` \| `skill` \| `budget_policy` |
| `target_id` | string | entity ID |
| `details` | JSON | action-specific context |
| `created_at` | timestamp | |

### Activity actions

| Action | When logged |
|--------|------------|
| `task.created` | A task is created (by user, agent, or routine) |
| `task.assigned` | A task's assignee changes |
| `task.status_changed` | A task's status changes (including completion) |
| `task.commented` | A comment is posted on a task |
| `agent.created` | A new agent instance is created |
| `agent.hired` | A hire approval is approved and agent activates |
| `agent.paused` | An agent is paused (manually or budget) |
| `agent.resumed` | A paused agent resumes |
| `agent.stopped` | An agent is deactivated |
| `agent.error` | An agent session fails with an error |
| `approval.created` | An approval request is submitted |
| `approval.resolved` | An approval is approved or rejected |
| `routine.triggered` | A routine fires and creates a task |
| `routine.skipped` | A routine fires but skips due to concurrency policy |
| `budget.alert` | A budget threshold is crossed |
| `budget.exceeded` | A budget limit is exceeded |
| `budget.reset` | Monthly budget counters reset |
| `skill.created` | A skill is added to the registry |
| `skill.updated` | A skill is modified |
| `cost.recorded` | A cost event is recorded (summarized, not per-token) |
| `wakeup.processed` | A wakeup is claimed and processed |

## API surface

### Notifications

A new event type `office.inbox_item` is added alongside the existing `session.waiting_for_input` event. Users subscribe per provider at `/settings/general`. Defaults: Local (browser) and System (OS) auto-subscribed when Office is enabled. Notification content: item type, summary, deep link to `/office/inbox`.

### Approval flow

1. Agent submits an approval request via tool call during a session, or scheduler submits for system events (budget alerts).
2. Approval created with `status=pending`; appears in inbox.
3. Requesting agent's session completes (agents do not block - they exit and are woken later).
4. User reviews and approves/rejects with optional note.
5. On resolution, `approval_resolved` wakeup queued for the requesting agent instance.
6. Agent's next session receives the result:
   - `hire_agent` approved: CEO assigns tasks to new agent.
   - `hire_agent` rejected: CEO finds an alternative.
   - `budget_increase` approved: budget updated, agent resumes.
   - `task_review` approved: task -> `done`.
   - `task_review` rejected: task -> `in_progress` with feedback.

### Review/approval flow (task `execution_policy`)

When an assignee agent marks a task done:
1. If review stage exists: status -> `in_review`. All reviewer participants woken in parallel. Each reviews independently and posts comments with approve/reject verdict.
2. Task stays `in_review` until all `approvals_needed` responses collected. No action on individual responses.
3. All reviewers approve: advance to next stage (approval, or `done` if none).
4. Any reviewer rejects: wait for all remaining reviewers; then return to `in_progress`. Assignee receives a single wakeup with ALL review comments aggregated, avoiding thrash on individual reviews.
5. Approval stage: inbox item per approver. When `approvals_needed` met, task -> `done`.
6. Any approver rejects: same pattern - wait for all, then `in_progress` with all feedback.
7. No policy: task -> `done`.

Downstream blockers resolve only when task reaches `done` (after all stages pass).

### Per-task and workspace defaults

- Users set reviewers/approvers when creating a task (properties panel or new task dialog).
- Agents can set reviewers/approvers during subtask creation (e.g. CEO creates "build" subtask with QA agent as reviewer).
- Workspace setting `require_approval_for_task_completion` (default `false`): all office tasks default to having the user as an approver. Overridable per task.
- The blocker system handles sequencing: subtask 2 `blocked_by: [subtask 1]`. When subtask 1 reaches `done` (after all stages), blocker resolves and `task_blockers_resolved` wakeup fires for subtask 2's agent.

### Approval configuration (workspace settings)

- `require_approval_for_new_agents`: default `true`. If false, hire requests auto-approve.
- `require_approval_for_task_completion`: default `false`.
- `auto_approve_budget_under_cents`: threshold below which budget increase requests auto-approve (default 0 = all require approval).
- `require_approval_for_skill_changes`: default `true`. If false, agent-created skills bypass approval.

### UI routes

- `/office/inbox` - inbox with type icons, summaries, action buttons; archive view for resolved items.
- `/office/company/activity` - chronological feed; filter by actor type, action, target type, time range; click target to navigate.
- `/office` dashboard - "Recent Activity" with last ~10 entries.

## State machine

Approval states: `pending` -> `approved` | `rejected`. Once resolved, terminal.

Task states relevant to inbox: `in_progress` -> `in_review` -> (back to `in_progress` on rejection, or forward to `done` after all stages pass). Stages run sequentially; participants within a stage run in parallel.

## Permissions

- Workspace users can approve/reject items in the inbox.
- Agents submit approval requests via tool calls; they cannot resolve their own requests.
- The CEO receives `budget_alert` wakeups for agent-scoped budget alerts.
- See `agents/` spec for the `can_manage_own_skills` permission gating `skill_creation` approvals.

## Failure modes

- Agent submits approval but kandev restarts: pending approval persists in DB; inbox item reappears on next read.
- All reviewers approve but one rejection arrives late: rejection wins only if it arrives before the count is met; otherwise the task has already advanced and the late rejection is ignored.
- Assignee agent missing when blocker resolves: `task_blockers_resolved` wakeup remains queued until the assignee comes online or the task is reassigned.

## Scenarios

- **GIVEN** the CEO agent submits a hire request for a new "QA Bot" worker, **WHEN** the approval is created, **THEN** it appears in the inbox with type `hire_agent`, showing the proposed name, role, profile, skills, and budget. The inbox badge increments.

- **GIVEN** a pending hire approval in the inbox, **WHEN** the user clicks "Approve" with the note "Looks good, start with the login tests", **THEN** the approval status moves to `approved`, the agent instance activates, an `approval_resolved` wakeup is queued for the CEO, and activity entries are logged for both the approval resolution and the agent hire.

- **GIVEN** a worker agent's budget crosses 80%, **WHEN** the budget check fires, **THEN** a budget alert appears in the inbox with the agent name, current spend, and limit. The CEO also receives a `budget_alert` wakeup.

- **GIVEN** a user on the activity log page, **WHEN** they filter by `actor_type=agent` and `action=task.created`, **THEN** they see only tasks created by agent instances, with links to each task.

- **GIVEN** the workspace setting `require_approval_for_new_agents=false`, **WHEN** the CEO submits a hire request, **THEN** the agent is auto-approved and activates immediately. An activity entry is still logged.

- **GIVEN** an agent session that fails with an error, **WHEN** the error is detected, **THEN** an `agent.error` inbox item appears with the error message and a link to the failed session.

- **GIVEN** a task with reviewers [security-agent, qa-agent] (approvals_needed=2) and approver [user], **WHEN** the assignee agent completes the task, **THEN** the task moves to `in_review`. Both reviewer agents are woken in parallel. Each reviews the changes and posts comments. When both approve, the task advances to the approval stage and an inbox item is created for the user.

- **GIVEN** a task in review stage where the security-agent rejects, **WHEN** the rejection is recorded, **THEN** the task returns to `in_progress`. The assignee agent receives a wakeup with the security agent's feedback. The QA agent's pending review is cancelled.

- **GIVEN** a parent task "Add auth" with subtasks Spec (requires_approval=true) -> Build (blocked_by Spec) -> Review -> Ship, **WHEN** the spec agent completes the Spec subtask, **THEN** Spec moves to `in_review` and an inbox item appears. **WHEN** the user approves, **THEN** Spec moves to `done`, the blocker on Build resolves, and the build agent receives a `task_blockers_resolved` wakeup.

- **GIVEN** a new inbox item (any type), **WHEN** the item is created, **THEN** a browser notification is shown (if Local notifications enabled) and an OS notification fires (if System provider enabled).

- **GIVEN** a task with `requires_approval=false` created by an agent, **WHEN** the agent marks it done, **THEN** the task moves directly to `done` without creating an inbox item. Downstream blockers resolve immediately.

## Out of scope

- Approval workflows with multiple reviewers or escalation chains beyond the staged execution policy.
- Activity log retention policies or archival.
- Batch approval (approve/reject multiple items at once).
- Custom approval types defined by users.
