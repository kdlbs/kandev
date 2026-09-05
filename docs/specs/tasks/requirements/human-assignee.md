---
status: draft
system: tasks
created: 2026-08-24
owners:
  - tbd
---

# Human Assignee and Actor Attribution Requirements

## Overview

When several people share a board, two questions appear that a single-user
instance never had to answer: who owns this task, and who did this. A task
already names the agent that runs it, which is not a person, and human actions
were previously recorded without an actor because there was only ever one.

The task system owns both contracts because both are properties of a task and
its session, not of the workspace that contains them. Which workspaces a person
can reach is owned by
[organization units](../../workspaces/requirements/org-units.md).

## Terminology

- **Human assignee:** The person who owns a task. Distinct from the agent
  assignee, which names an agent profile.
- **Takeover:** One person becoming the human assignee of a task another person
  held.
- **Actor:** The authenticated user whose request caused a recorded change.

## Requirements

### REQ-TASKS-HUMAN-ASSIGNEE-001: Human assignee and takeover

**Intent:** A shared board needs to say who owns each task, and handing a task
over must not disturb work already in flight.

**User story:** As a team member, I want to take a colleague's task over, so
that I can continue it without waiting for them to hand it off.

#### Acceptance criteria

- **AC-TASKS-HUMAN-ASSIGNEE-001.1:** The system shall record a task's human
  assignee independently of its agent assignee, and shall allow a task to carry
  both at once.
- **AC-TASKS-HUMAN-ASSIGNEE-001.2:** When either assignee is set, the system
  shall leave the other unchanged.
- **AC-TASKS-HUMAN-ASSIGNEE-001.3:** When a caller holding `task.write` assigns
  a task, the system shall accept the assignment, including assigning to
  themselves, without additional confirmation.
- **AC-TASKS-HUMAN-ASSIGNEE-001.4:** When a caller assigns a task to a user who
  cannot reach the task's workspace, the system shall refuse the assignment and
  return a reason suitable for display.
- **AC-TASKS-HUMAN-ASSIGNEE-001.5:** When a task is reassigned, the system shall
  leave its session, worktree, executor, and running agent turn untouched, and
  shall take no lock.
- **AC-TASKS-HUMAN-ASSIGNEE-001.6:** When a caller lacking `task.write` requests
  an assignment, the system shall refuse it and leave the assignee unchanged.
- **AC-TASKS-HUMAN-ASSIGNEE-001.7:** When the assignee changes, the system shall
  deliver the new value to connected clients without requiring a reload,
  including when the assignee is cleared.
- **AC-TASKS-HUMAN-ASSIGNEE-001.8:** The system shall treat the human assignee
  as advisory, granting and withholding no permission on the basis of it.

### REQ-TASKS-HUMAN-ASSIGNEE-002: Actor attribution

**Intent:** The audit trail a shared login cannot provide is the reason to have
per-person accounts at all, so every human action has to carry its actor.

#### Acceptance criteria

- **AC-TASKS-HUMAN-ASSIGNEE-002.1:** When a user sends a session message, queues
  a message, moves a task through a workflow step, changes a task's state, or
  stops or cancels an agent, the system shall record the acting user against
  that change.
- **AC-TASKS-HUMAN-ASSIGNEE-002.2:** The system shall attribute recorded human
  actions in the interface to the user who performed them.
- **AC-TASKS-HUMAN-ASSIGNEE-002.3:** When an action has no human actor, the
  system shall record the absence of one, and shall not substitute the workspace
  owner or any other user.
- **AC-TASKS-HUMAN-ASSIGNEE-002.4:** When a user's access is later withdrawn or
  their account is disabled, the system shall retain their existing attribution
  unchanged.
- **AC-TASKS-HUMAN-ASSIGNEE-002.5:** When two users prompt one session at the
  same time, the system shall queue both messages in arrival order, attribute
  each to its sender, and drop neither.

## Out of scope

- **Reach.** Which workspaces, and therefore which tasks, a person can see is
  owned by [organization units](../../workspaces/requirements/org-units.md).
- **Assignment as permission.** The human assignee never gates an action.
- **Per-person execution resources.** Sessions, worktrees, and previews stay
  shared per workspace, which is what allows takeover to continue work in place.
- **Notification on assignment.** Alerting an assignee is not part of this
  capability.
