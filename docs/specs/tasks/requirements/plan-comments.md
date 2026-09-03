---
status: active
system: tasks
created: 2026-09-02
owners:
  - kandev
---

# Task Plan Comments Requirements

## Overview

Pending comments on a task plan are feedback the user has prepared but has not
yet delivered to an agent. They belong to the task's current plan, not to the
Agent session that happened to be selected when they were written. Kandev
shows one shared pending set on the plan and at every session composer, then
chooses a destination only when the user sends a message or selects **Run**.

## Terminology

- **Pending plan comment:** An unsent comment anchored to selected text in the
  task's current plan.
- **Selected session:** The Agent session whose composer the user is currently
  using.
- **Primary session:** The task session currently designated as primary.
- **Accepted delivery:** A direct user message durably persisted for dispatch,
  or a queued prompt durably admitted for a session.
- **Run:** The action beside one plan comment that delivers that comment
  without requiring composer text.

## Requirements

### REQ-TASKS-PLAN-COMMENTS-001: Task-owned pending feedback

**Intent:** A pending plan comment remains attached to the plan while the user
navigates between sessions, devices, and browser lifecycles.

**User story:** As a user reviewing a task plan, I want one durable comment set
for the task, so that choosing another Agent session cannot hide or destroy my
unsent feedback.

#### Acceptance criteria

- **AC-TASKS-PLAN-COMMENTS-001.1:** When an authorized user adds a plan
  comment, Kandev shall persist it against the task's current plan and show the
  same highlight, badge, and comment text in every view of that plan.
- **AC-TASKS-PLAN-COMMENTS-001.2:** Selecting another session, changing the
  primary session, creating a session, or deleting a session shall not hide,
  move, duplicate, or delete a pending plan comment.
- **AC-TASKS-PLAN-COMMENTS-001.3:** After reload, reconnect, backend restart, or
  opening the task from another authorized browser, Kandev shall restore the
  current pending comment set from backend state.
- **AC-TASKS-PLAN-COMMENTS-001.4:** A user shall be able to add, edit, and
  delete pending comments whenever the task has a current plan, even when the
  task has no session that can accept input.
- **AC-TASKS-PLAN-COMMENTS-001.5:** Updating or reverting the current plan shall
  retain comments whose selected text can still be anchored. An explicit
  comment delete or a destructive plan edit that removes the marked range
  shall delete that pending comment for the task.
- **AC-TASKS-PLAN-COMMENTS-001.6:** Deleting the current plan or deleting the
  task shall delete its pending plan comments. Deleting and later recreating a
  plan shall not attach comments from the deleted plan to the new plan.
- **AC-TASKS-PLAN-COMMENTS-001.7:** Desktop and mobile task surfaces shall
  expose the same pending comment set and lifecycle.

### REQ-TASKS-PLAN-COMMENTS-002: Selected-session composer delivery

**Intent:** Pending plan feedback can accompany a message to whichever session
the user deliberately addresses.

**User story:** As a user working with several Agent sessions, I want the plan
comments available at each composer, so that I can send my message and its plan
context to a non-primary session when I choose.

#### Acceptance criteria

- **AC-TASKS-PLAN-COMMENTS-002.1:** Every rendered composer for a task session
  shall show a plan-comment context item above its text area when the task has
  pending plan comments. The item shall represent the same shared set shown on
  the plan.
- **AC-TASKS-PLAN-COMMENTS-002.2:** When the user selects ordinary **Send** from
  a session composer, Kandev shall include the visible pending plan-comment
  snapshot with the message and address the delivery to that selected session,
  even when another session is primary.
- **AC-TASKS-PLAN-COMMENTS-002.3:** A pending plan comment shall not be injected
  into an agent's context merely because a session or plan is opened. It shall
  become agent context only through an explicit **Send** or **Run** action.
- **AC-TASKS-PLAN-COMMENTS-002.4:** A composer shall allow submission with no
  typed text when at least one pending plan comment is included.
- **AC-TASKS-PLAN-COMMENTS-002.5:** After an accepted direct or queued delivery,
  Kandev shall consume exactly the included comments and remove them from the
  plan and every session composer. Comments created after the submitted
  snapshot shall remain pending.
- **AC-TASKS-PLAN-COMMENTS-002.6:** If delivery is rejected or Kandev cannot
  establish that it was accepted, the included comments and typed composer
  text shall remain available for retry.
- **AC-TASKS-PLAN-COMMENTS-002.7:** If two delivery attempts include the same
  pending comment, no more than one attempt shall be accepted with that
  comment. A competing attempt shall fail without silently sending its typed
  text without the requested plan context.

### REQ-TASKS-PLAN-COMMENTS-003: Primary-session Run routing

**Intent:** **Run** is a task-level shortcut with one predictable destination,
independent of the currently selected Agent tab.

#### Acceptance criteria

- **AC-TASKS-PLAN-COMMENTS-003.1:** Selecting **Run** for a plan comment shall
  submit that comment, and no other pending plan comment, to the task's current
  primary session.
- **AC-TASKS-PLAN-COMMENTS-003.2:** When a non-primary session is selected,
  **Run** shall still target the primary session and shall not add the comment
  to the selected session's transcript or queue.
- **AC-TASKS-PLAN-COMMENTS-003.3:** When the primary session can accept a
  prompt, **Run** shall create a direct plan-mode user message. When the primary
  session is busy but accepts queued prompts, **Run** shall create a distinct
  queued plan-mode prompt.
- **AC-TASKS-PLAN-COMMENTS-003.4:** Kandev shall validate primary ownership at
  delivery acceptance. If the primary changes during the action, Kandev shall
  not deliver to the stale target and shall preserve the comment for retry.
- **AC-TASKS-PLAN-COMMENTS-003.5:** When there is no eligible primary session,
  **Run** shall be unavailable with an actionable reason. Adding and editing
  comments shall remain available.
- **AC-TASKS-PLAN-COMMENTS-003.6:** An accepted **Run** shall consume the run
  comment task-wide. A rejected or failed **Run** shall preserve it.

### REQ-TASKS-PLAN-COMMENTS-004: Lossless legacy-draft migration

**Intent:** The session-scoped browser drafts shipped by the earlier repair
must move to task ownership without another comment-loss window.

#### Acceptance criteria

- **AC-TASKS-PLAN-COMMENTS-004.1:** When a browser contains session-scoped
  pending plan comments for sessions belonging to the open task, Kandev shall
  migrate those comments to the task's current plan before enabling a delivery
  that could omit them.
- **AC-TASKS-PLAN-COMMENTS-004.2:** Migration shall retain each comment's
  client-generated identifier. Retrying the same identifier and content shall
  be idempotent; distinct identifiers shall remain distinct even when their
  selected text and comment text match.
- **AC-TASKS-PLAN-COMMENTS-004.3:** Kandev shall remove a legacy plan-comment
  record from browser storage only after backend persistence is acknowledged.
  Failed plan comments and every non-plan comment in the same session record
  shall remain unchanged.
- **AC-TASKS-PLAN-COMMENTS-004.4:** A migration failure shall be visible and
  retryable. It shall not silently discard a draft or send a message without
  the legacy plan comments that the composer represented.

## Out of scope

- Changing the task-session ownership of diff, file, pull-request,
  walkthrough, or agent-message comments.
- Keeping a separate sent-comment history. Accepted message and queue content
  remain the durable delivery record.
- Adding comments to historical plan revisions.
- Broadcasting one comment prompt to every session.
- Automatically exposing pending comments to an agent before the user sends
  them.
