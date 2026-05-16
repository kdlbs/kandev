---
status: draft
created: 2026-04-28
owner: cfl
---

# Blocked Task Escalation

## Why

When an agent hits a decision point it cannot resolve on its own — a missing credential, an ambiguous requirement, a design choice only the human can make — it currently has two options: post a comment and exit, or spin indefinitely. Neither gives the human a clean, actionable queue. Comments get buried in the task thread and there is no way to filter for "things that need my attention right now." The result is that blocked agents sit idle while the human has no structured signal that their input is required.

## What

- When an agent is blocked and needs human input (design decision, access credentials, external approval, architectural choice), it creates a new task titled `Decision needed: <question>` and assigns it to the human user.
- The new task is linked as a blocker of the agent's current task: the agent task's `blockedBy` list includes the human task's ID.
- The agent task moves to `Blocked` status. The agent exits cleanly after creating the blocker task and posting a comment.
- The human sees a dedicated, filterable card in their kanban view. When the human resolves the decision task (marks it done), the orchestrator detects that all blockers for the agent task have cleared and sends a `task_blockers_resolved` wakeup to the agent.
- The agent resumes with the resolution context included in its wake payload.
- This behavior is taught to agents via a new bundled skill (`kandev-escalation`) rather than being hardcoded in the backend. The skill provides the pattern and CLI commands; the underlying infrastructure (`task create --blocked-by`, `task_blockers_resolved` wakeup) already exists.

## Scenarios

- **GIVEN** an agent working on "Implement OAuth login" encounters an ambiguous requirement about which OAuth provider to support, **WHEN** it creates a human task `Decision needed: Which OAuth providers should we support?` blocked-by the agent task and sets the agent task to blocked, **THEN** a new unassigned "Decision needed" card appears on the kanban board and the agent's task shows a blocker badge pointing to that card.

- **GIVEN** the human sees the "Decision needed" card, reads the question in the description, and marks it done, **WHEN** the orchestrator processes the task-completion event, **THEN** it detects the blocked agent task has no remaining open blockers and queues a `task_blockers_resolved` wakeup for the assigned agent.

- **GIVEN** the `task_blockers_resolved` wakeup fires, **WHEN** the agent wakes, **THEN** `KANDEV_WAKE_REASON` is `task_blockers_resolved` and the wake payload includes the resolved blocker title so the agent can reference the decision in its next message.

- **GIVEN** multiple blockers exist (e.g., the agent escalated two separate questions), **WHEN** the human resolves each human task one by one, **THEN** the wakeup fires only after the last open blocker is resolved (existing coalescing/idempotency behavior in `queueBlockersResolvedWakeups` handles this correctly).

## Out of scope

- Automatically assigning the human task to a specific user account (kandev currently has no user account model; "assigned to the human" means unassigned or assigned by a workspace `human_user_id` setting if one is configured).
- Backend changes to the orchestrator or scheduler — the existing `task_blockers_resolved` event chain is sufficient.
- A dedicated UI widget for "awaiting my decision" items; the standard kanban filter covers this.
- Automatic escalation timeouts (agent wakes after N hours if human hasn't responded).
- Any changes to the clarification/permission system — this is a task-creation pattern, not a request/response channel.

## Open questions

- Should the human task be left unassigned (relies on the human scanning the board) or should a `KANDEV_HUMAN_USER_ID` workspace setting be introduced so the task is explicitly assigned? The unassigned approach works today; explicit assignment requires adding the setting.
