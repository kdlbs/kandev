---
status: draft
created: 2026-04-25
owner: cfl
---

# Orchestrate: Inbox, Approvals & Activity Log

## Why

When agents work autonomously, they will encounter situations that require human judgment: hiring a new agent, exceeding a budget, completing a task that needs review, or hitting an error they cannot resolve. Without a centralized inbox, users must poll individual agent statuses and task lists to discover what needs attention. Without an approval system, agents cannot request permission for high-impact actions. Without an activity log, there is no audit trail of what happened and who did it.

Orchestrate adds an inbox that aggregates all items requiring human attention, an approval system for gating agent actions, and a full activity log for audit and debugging.

## What

### Inbox

- The inbox at `/orchestrate/inbox` is the user's single view of everything that needs their attention.
- Inbox items are not a separate table -- the inbox is a computed view over:
  - **Pending approvals**: hire requests, budget increase requests, board approval requests.
  - **Budget alerts**: agents approaching or exceeding their budget limits.
  - **Agent errors**: sessions that failed with unrecoverable errors.
  - **Review requests**: tasks in `in_review` status that need human sign-off.
  - **Clarification requests**: agents that asked a question and are waiting for a response.
- Each item shows: type icon, summary text, which agent/task it relates to, timestamp, and action buttons (approve/reject, view task, dismiss).
- Items are ordered by recency, with unresolved items first.
- A badge count on the sidebar "Inbox" link shows the number of unresolved items.
- Resolved items (approved/rejected/dismissed) move to an archive view accessible from the inbox page.

### Notifications

- New inbox items trigger notifications via the existing notification providers (Local/WebSocket, System/OS, Apprise).
- A new event type `orchestrate.inbox_item` is added alongside the existing `session.waiting_for_input` event.
- Users subscribe to this event per provider in the notification settings (the same UI at `/settings/general`).
- Default behavior: Local (browser) and System (OS) providers are auto-subscribed to `orchestrate.inbox_item` when Orchestrate is enabled.
- Notification content includes: item type, summary, and a deep link to `/orchestrate/inbox`.

### Approvals

- An approval is a structured request from an agent (or the system) that requires human decision.
- Approval fields:
  - `id`: unique identifier.
  - `workspace_id`: scoped to workspace.
  - `type`: discriminator for the approval kind:
    - `hire_agent`: agent requests to create a new agent instance.
    - `budget_increase`: agent requests a higher budget for itself or a sub-agent.
    - `board_approval`: agent requests permission for a specific action (custom payload).
    - `task_review`: agent marks a task as needing human review before completion.
    - `skill_creation`: agent requests to create or edit a skill in the registry (only when `require_approval_for_skill_changes=true`).
  - `requested_by_agent_instance_id`: which agent submitted the request (null for system-generated).
  - `status`: `pending`, `approved`, `rejected`.
  - `payload`: JSON with type-specific data:
    - For `hire_agent`: proposed name, role, profile, skills, budget.
    - For `budget_increase`: current budget, requested budget, reason.
    - For `board_approval`: action description, context, impact.
    - For `task_review`: task ID, completion summary, deliverables.
    - For `skill_creation`: skill name, slug, SKILL.md content preview, requesting agent.
  - `decision_note`: optional text from the user explaining their decision.
  - `decided_by`: user ID of the reviewer.
  - `decided_at`: timestamp of the decision.
  - `created_at`: when the request was submitted.

### Approval flow

1. Agent submits an approval request (via tool call during a session, or via the scheduler for system events like budget alerts).
2. The approval is created with `status=pending` and appears in the user's inbox.
3. The requesting agent's session completes (agents do not block waiting for approval -- they exit and are woken later).
4. The user reviews the request in the inbox and approves or rejects with an optional note.
5. On resolution, an `approval_resolved` wakeup is queued for the requesting agent instance.
6. The agent's next session receives the approval result and acts accordingly:
   - `hire_agent` approved: the CEO assigns tasks to the new agent.
   - `hire_agent` rejected: the CEO finds an alternative approach.
   - `budget_increase` approved: the budget is updated and the agent resumes.
   - `task_review` approved: the task status moves to `done`.
   - `task_review` rejected: the task returns to `in_progress` with the reviewer's feedback.

### Task execution policy (reviewers and approvers)

- Each task has an optional `execution_policy` JSON field defining review and approval stages.
- The policy contains ordered stages, each with a type, participants, and an approvals-needed count:
  ```
  execution_policy: {
    stages: [
      { type: "review", participants: [{type: "agent", id: "security-agent"}, {type: "agent", id: "qa-agent"}], approvals_needed: 2 },
      { type: "approval", participants: [{type: "user", id: "cfl"}], approvals_needed: 1 }
    ]
  }
  ```
- **Reviewers** (type=review): N agents/users who review the task output in parallel. Each reviewer runs independently and provides feedback. Useful for parallel review by specialized agents (security, performance, QA).
- **Approvers** (type=approval): M agents/users who approve before the task can move to done. Requires `approvals_needed` count to be met.
- An `execution_state` JSON field tracks the current stage, which participants have responded, and the outcome.
- Stages run sequentially (stage 2 waits for stage 1 to complete). Within a stage, participants run in parallel. To get sequential reviews, use multiple review stages with one participant each:
  ```
  stages: [
    { type: "review", participants: [{id: "security-agent"}], approvals_needed: 1 },
    { type: "review", participants: [{id: "qa-agent"}], approvals_needed: 1 },
    { type: "approval", participants: [{id: "user"}], approvals_needed: 1 }
  ]
  ```

### Review/approval flow

- When an agent marks a task as done:
  1. If the task has a review stage: status moves to `in_review`. All reviewer participants are woken in parallel. Each reviewer runs independently, reviews the task's output, and posts comments with an approve or reject verdict.
  2. The task stays in `in_review` until all `approvals_needed` responses are collected. No action is taken on individual responses -- the system waits for the full set.
  3. If all reviewers approve: the task advances to the next stage (approval, or `done` if no approval stage).
  4. If any reviewer rejects: the task waits for all remaining reviewers to finish, then returns to `in_progress`. The assignee agent receives a single wakeup with ALL review comments aggregated, so it can address all feedback at once rather than thrashing on individual reviews.
  5. If the task has an approval stage: an approval inbox item is created for each approver. When `approvals_needed` approvals are met, the task moves to `done`.
  6. If any approver rejects: same pattern -- wait for all, then return to `in_progress` with all feedback.
  7. If no execution policy: task moves directly to `done`.
- Downstream blockers resolve only when the task reaches `done` (after all stages pass).

### Per-task and workspace defaults

- Users set reviewers/approvers when creating a task (via the properties panel or new issue dialog).
- Agents can set reviewers/approvers during subtask creation (e.g. the CEO creates a "build" subtask with the QA agent as reviewer).
- Workspace setting `require_approval_for_task_completion` (default `false`): if true, all orchestrate tasks default to having the user as an approver. Can be overridden per task.
- The blocker system handles sequencing: subtask 2 is `blocked_by: [subtask 1]`. When subtask 1 reaches `done` (after all review/approval stages), the blocker resolves and a `task_blockers_resolved` wakeup fires for subtask 2's agent.

### Approval configuration

- Workspace-level settings control defaults:
  - `require_approval_for_new_agents`: default `true`. If false, hire requests auto-approve.
  - `require_approval_for_task_completion`: default `false`. If true, all orchestrate tasks default to `requires_approval=true`. Users and agents can override per task.
  - `auto_approve_budget_under_cents`: threshold below which budget increase requests auto-approve (default 0 = all require approval).
  - `require_approval_for_skill_changes`: default `true`. If false, agent-created skills bypass approval and are immediately added to the registry.

### Activity log

- The activity log records every significant action in the Orchestrate system.
- Activity entry fields:
  - `id`: unique identifier.
  - `workspace_id`: scoped to workspace.
  - `actor_type`: `user`, `agent`, or `system`.
  - `actor_id`: the entity that performed the action (user ID, agent instance ID, or "system").
  - `action`: verb describing what happened (see table below).
  - `target_type`: what was acted upon (`task`, `agent_instance`, `routine`, `project`, `approval`, `skill`, `budget_policy`).
  - `target_id`: the entity ID.
  - `details`: JSON with action-specific context.
  - `created_at`: timestamp.

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

### Activity log UI

- `/orchestrate/company/activity` page shows:
  - Chronological feed of activity entries with actor, action, target, and timestamp.
  - Filter by: actor type, action, target type, time range.
  - Click a target to navigate to its detail page.
- The Orchestrate dashboard at `/orchestrate` shows a "Recent Activity" section with the last ~10 entries.

## Scenarios

- **GIVEN** the CEO agent submits a hire request for a new "QA Bot" worker, **WHEN** the approval is created, **THEN** it appears in the inbox with type `hire_agent`, showing the proposed name, role, profile, skills, and budget. The inbox badge increments.

- **GIVEN** a pending hire approval in the inbox, **WHEN** the user clicks "Approve" with the note "Looks good, start with the login tests", **THEN** the approval status moves to `approved`, the agent instance activates, an `approval_resolved` wakeup is queued for the CEO, and activity entries are logged for both the approval resolution and the agent hire.

- **GIVEN** a worker agent's budget crosses 80%, **WHEN** the budget check fires, **THEN** a budget alert appears in the inbox with the agent name, current spend, and limit. The CEO also receives a `budget_alert` wakeup.

- **GIVEN** a user on the activity log page, **WHEN** they filter by `actor_type=agent` and `action=task.created`, **THEN** they see only tasks created by agent instances, with links to each task.

- **GIVEN** the workspace setting `require_approval_for_new_agents=false`, **WHEN** the CEO submits a hire request, **THEN** the agent is auto-approved and activates immediately. An activity entry is still logged.

- **GIVEN** an agent session that fails with an error, **WHEN** the error is detected, **THEN** an `agent.error` inbox item appears with the error message and a link to the failed session.

- **GIVEN** a task with reviewers [security-agent, qa-agent] (approvals_needed=2) and approver [user], **WHEN** the assignee agent completes the task, **THEN** the task moves to `in_review`. Both reviewer agents are woken in parallel. Each reviews the changes and posts comments. When both approve, the task advances to the approval stage and an inbox item is created for the user.

- **GIVEN** a task in review stage where the security-agent rejects, **WHEN** the rejection is recorded, **THEN** the task returns to `in_progress`. The assignee agent receives a wakeup with the security agent's feedback. The QA agent's pending review is cancelled.

- **GIVEN** a parent task "Add auth" with subtasks Spec (requires_approval=true) -> Build (blocked_by Spec) -> Review -> Ship, **WHEN** the spec agent completes the Spec subtask, **THEN** the Spec task moves to `in_review` and an inbox item appears. **WHEN** the user approves, **THEN** Spec moves to `done`, the blocker on Build resolves, and the build agent receives a `task_blockers_resolved` wakeup.

- **GIVEN** a new inbox item (any type), **WHEN** the item is created, **THEN** a browser notification is shown (if the user has Local notifications enabled) and an OS notification fires (if System provider is enabled).

- **GIVEN** a task with `requires_approval=false` created by an agent, **WHEN** the agent marks it done, **THEN** the task moves directly to `done` without creating an inbox item. Downstream blockers resolve immediately.

## Out of scope

- Approval workflows with multiple reviewers or escalation chains.
- Activity log retention policies or archival.
- Batch approval (approve/reject multiple items at once).
- Custom approval types defined by users.
