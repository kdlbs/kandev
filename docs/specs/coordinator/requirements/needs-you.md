---
id: coordinator-needs-you
title: Needs you and Queue
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-26
last_updated: 2026-09-26
---

# Needs you and Queue Requirements

## Overview

Each coordinator has two screens: **Needs you**, the items that need a person
and why, and **Queue**, every other open task grouped by position. Both are
derived from task facts Kandev already publishes, the coordinator's stall
records and its pending proposals, and both change as those facts change. The
screens only read; the only write they offer is deciding a proposal
([proposals](proposals.md)).

Kandev's Needs-you Inbox is unchanged: its row, count and tabs stay as they are
wherever `features.needsYouInbox` is on.

## Terminology

- **Item:** one entry on Needs you. A task produces at most one item; a
  proposal produces one item.
- **Reference time:** the timestamp an item's age is measured from, defined in
  `AC-COORDINATOR-NEEDS-YOU-002.3`.
- **Open task:** a task of the workspace that is neither archived nor
  ephemeral. A coordinator's conversation task is ephemeral and never appears.
- Other terms are defined in the [system README](../README.md#terms).

## Mockup

The phase 1 mockup screenshots are the visual reference for this document's
user-facing criteria; each requirement below cites the ones it covers. Where
a screenshot and an acceptance criterion differ, the criterion governs. The
prototype banner, the demo controls and the `P1` and `WC-` labels are mockup
chrome, not product; the data is seeded fiction.

- [`docs/plans/workspace-coordinator/assets/p1-01-needs-you.png`](../../../plans/workspace-coordinator/assets/p1-01-needs-you.png)
- [`docs/plans/workspace-coordinator/assets/p1-03-queue.png`](../../../plans/workspace-coordinator/assets/p1-03-queue.png)

## Requirements

### REQ-COORDINATOR-NEEDS-YOU-001: Classification

**Intent:** Every open task has exactly one position, so the counts and the
lists agree.

#### Acceptance criteria

- **AC-COORDINATOR-NEEDS-YOU-001.1:** The system shall place each open
  proposal (see [proposals](proposals.md)) of the coordinator on Needs you as a proposal item.
- **AC-COORDINATOR-NEEDS-YOU-001.2:** The system shall place each open task in
  the first group, in this order, whose rule it matches: (1) question or
  permission: its pending action is a clarification or a permission request;
  (2) stall: it has a stall record whose detection time is not earlier than
  the task's last activity; (3) error: it has an active session error or a
  task error; (4) Ready to merge: its pull request aggregate state is `ready`;
  (5) In review: its pull request aggregate state is `awaiting_review`;
  (6) Working: its primary session state is `RUNNING` or `STARTING`;
  (7) Done: the task state is `COMPLETED`; (8) Other: anything else.
- **AC-COORDINATOR-NEEDS-YOU-001.3:** Groups 1 to 3 shall be shown on Needs you;
  groups 4 to 8 shall be shown on Queue.
- **AC-COORDINATOR-NEEDS-YOU-001.4:** When a task has no last activity time,
  including a task whose status summary is absent, a stall record for it shall
  count as current, and the task shall be a stall item.
- **AC-COORDINATOR-NEEDS-YOU-001.5:** When a stall record's detection time
  equals the task's last activity time, the stall shall still be shown.
- **AC-COORDINATOR-NEEDS-YOU-001.6:** Archived and ephemeral tasks shall never
  appear on either screen or in any count, and a stall record for such a task
  shall be ignored.

### REQ-COORDINATOR-NEEDS-YOU-002: Needs you

**Intent:** A manager sees what needs them, why, and what clears it.

**User story:** As a workspace manager, I want one list of what needs me, each
with why it is there and what clears it, so that I do not read every board.

Mockup:

- [`docs/plans/workspace-coordinator/assets/p1-01-needs-you.png`](../../../plans/workspace-coordinator/assets/p1-01-needs-you.png): Needs you with question, stall and error items, each with why it is here, what clears it and its actions.

#### Acceptance criteria

- **AC-COORDINATOR-NEEDS-YOU-002.1:** Each item shall show the card identifier
  (or "New task" for a proposal whose source task is absent, archived or
  deleted), that task's current step (none for "New task"), a severity pill (**Decide now** for questions, permissions, stalls and
  errors; **Review** for proposals), its age, a one-line reason, **Why it is
  here**, **What clears it** and **Ask about this**.
- **AC-COORDINATOR-NEEDS-YOU-002.2:** The why and clears texts shall be, by
  kind: proposal: the coordinator's rationale / "Approve, edit or reject";
  question or permission: "The agent is waiting for your answer" / "Your answer,
  on the task"; stall: "No activity for <stalled for>, and no agent is running"
  / "Resuming or restarting the task"; error: "The agent reported an error:
  <preview>" when an active session error exists (the preview is the active
  error's), else "The task failed" with no preview, including when the task
  error carries a preview / "Retrying the task, or fixing the cause".
- **AC-COORDINATOR-NEEDS-YOU-002.3:** An item's reference time shall be: for a
  proposal, its `created_at`; for a stall, the stall's last event time; for
  any other item, the task's last activity time, or its `updated_at` when it
  has none. The age shall be the current time minus the reference time and
  shall increase while the screen is open.
- **AC-COORDINATOR-NEEDS-YOU-002.4:** Items shall be ordered by reference time
  ascending (oldest first), then by kind in the order proposal, question or
  permission, stall, error, then by item id ascending (proposal id or task id,
  compared bytewise).
- **AC-COORDINATOR-NEEDS-YOU-002.5:** A question or permission item shall say
  "The agent is waiting for your answer. Answer it on the task." and offer
  **Open task**; it shall not offer an answer control.
- **AC-COORDINATOR-NEEDS-YOU-002.6:** A stall item shall offer **Open task** and
  **Show the evidence**, which reveals stalled for, last event at, detected at,
  and whether an agent is running now from the task's primary session state.
- **AC-COORDINATOR-NEEDS-YOU-002.7:** An error item shall offer **Open task**.
- **AC-COORDINATOR-NEEDS-YOU-002.8:** A proposal item shall show the title, the
  description, the target workflow and step, "Proposed by <coordinator name>",
  "Policy: Create a card is propose-only" and the actions defined by
  [proposals](proposals.md); a reader sees no decision actions.
- **AC-COORDINATOR-NEEDS-YOU-002.9:** When the coordinator has a proposal whose
  source task is also an item, both items shall be shown.

### REQ-COORDINATOR-NEEDS-YOU-003: Count strip

**Intent:** The manager sees the shape of the workspace at a glance.

Mockup:

- [`docs/plans/workspace-coordinator/assets/p1-01-needs-you.png`](../../../plans/workspace-coordinator/assets/p1-01-needs-you.png): the count strip above the list.
- [`docs/plans/workspace-coordinator/assets/p1-03-queue.png`](../../../plans/workspace-coordinator/assets/p1-03-queue.png): the same strip on Queue.

#### Acceptance criteria

- **AC-COORDINATOR-NEEDS-YOU-003.1:** Both screens shall show a strip with the
  counts Needs you (items), Working, In review and Ready to merge, computed from
  the same classification as the lists, outside the scrolling region.
- **AC-COORDINATOR-NEEDS-YOU-003.2:** The Needs you count shall link to Needs
  you and the other three counts shall link to their Queue group.
- **AC-COORDINATOR-NEEDS-YOU-003.3:** When a task's facts change, the strip and
  the lists shall update without reload and without a recompute action, and a
  line beside the strip shall say "Positions derived from session, PR, CI and
  review facts, as they change".

### REQ-COORDINATOR-NEEDS-YOU-004: Queue

**Intent:** The manager can see everything else without leaving the coordinator.

Mockup:

- [`docs/plans/workspace-coordinator/assets/p1-03-queue.png`](../../../plans/workspace-coordinator/assets/p1-03-queue.png): Queue groups and rows.

#### Acceptance criteria

- **AC-COORDINATOR-NEEDS-YOU-004.1:** Queue shall show Working, In review and
  Ready to merge expanded, then Done and Other collapsed, each with its count.
- **AC-COORDINATOR-NEEDS-YOU-004.2:** Within a group, rows shall be ordered by
  last activity time descending, rows without one last, then by task id
  ascending.
- **AC-COORDINATOR-NEEDS-YOU-004.3:** A Working row shall show the card, step,
  agent state and last activity. When a task's session is unreadable (its
  status summary is absent, or it has a primary session with no state), the
  Working, question/permission, error and pull-request rules shall be unable
  to match it, since each reads a status-summary field, but the stall rule (it
  can, per `AC-COORDINATOR-NEEDS-YOU-001.4`) and the Done rule (it reads the
  task's own state, not the status summary) shall still be able to. Only when
  none of those places it shall the task be in Other, with its row showing
  "position underivable: session unreadable" instead of the agent state; a
  task with no session shall show "No session".
- **AC-COORDINATOR-NEEDS-YOU-004.4:** In review and Ready to merge rows shall
  show the pull request state, unresolved review thread count and CI state of
  the task's primary GitHub pull request; when that pull request's detail is
  not available the row shall show the pull request state and "PR detail
  unavailable".
- **AC-COORDINATOR-NEEDS-YOU-004.5:** Queue rows shall have no action other than
  opening the task.

### REQ-COORDINATOR-NEEDS-YOU-005: Stall records

**Intent:** Stalls Kandev detects become durable cards for coordinated
workspaces.

#### Acceptance criteria

- **AC-COORDINATOR-NEEDS-YOU-005.1:** When `task.stalled` is published for a
  workspace with at least one coordinator, the system shall store one stall
  record per task with its stalled-for duration, last event time and detection
  time. An event for the same task whose last event time is later than the
  stored one shall replace those values; an event whose last event time is
  equal to or earlier than the stored one (a redelivery, or events handled out
  of order) shall change nothing and emit nothing.
- **AC-COORDINATOR-NEEDS-YOU-005.2:** When `task.stalled` is published for a
  workspace with no coordinator, the system shall store nothing, and a later
  first coordinator shall not see that stall.
- **AC-COORDINATOR-NEEDS-YOU-005.3:** At startup the system shall delete stall
  records whose task no longer exists or is archived and records detected more
  than 30 days earlier.
- **AC-COORDINATOR-NEEDS-YOU-005.4:** Stall records shall be shared by every
  coordinator of the workspace.
- **AC-COORDINATOR-NEEDS-YOU-005.5:** Recording a stall shall never send a
  message to, or start, any session.

### REQ-COORDINATOR-NEEDS-YOU-006: Entry, routes and sidebar

**Intent:** A manager reaches each coordinator from the sidebar.

Mockup:

- [`docs/plans/workspace-coordinator/assets/p1-01-needs-you.png`](../../../plans/workspace-coordinator/assets/p1-01-needs-you.png): the Planner sidebar entry with its open-proposal badge after Inbox and before New Task, and the header with Configure.

#### Acceptance criteria

- **AC-COORDINATOR-NEEDS-YOU-006.1:** When the flag is on, the sidebar shall
  show one entry per coordinator, by name and in the order of
  `AC-COORDINATOR-COORDINATORS-003.1`, after the Inbox row and before the New
  Task entry, and the same entries in the phone navigation rows.
- **AC-COORDINATOR-NEEDS-YOU-006.2:** A coordinator's entry shall show a badge
  with its open proposal count when that count is above zero and no badge
  at zero, and the badge shall update from `coordinator.updated` without reload.
- **AC-COORDINATOR-NEEDS-YOU-006.3:** When the workspace has no coordinator, the
  sidebar shall show one "Coordinator" entry with no badge, and the Inbox row
  shall be unchanged.
- **AC-COORDINATOR-NEEDS-YOU-006.4:** `/workspaces/:id/coordinator/:coordinatorId`
  shall open Needs you and `/workspaces/:id/coordinator/:coordinatorId/queue`
  Queue; `/workspaces/:id/coordinator` shall open the first coordinator's Needs
  you, or the no-coordinator state when there is none; an unknown coordinator
  id shall show the no-coordinator state with a link to the list.
- **AC-COORDINATOR-NEEDS-YOU-006.5:** The header shall show the coordinator's
  name, or a selector of the workspace's coordinators when there are several,
  and a **Configure** action to its settings page for managers.

### REQ-COORDINATOR-NEEDS-YOU-007: Empty, missing and error states

**Intent:** An empty list reads as success and a failure never hides data.

#### Acceptance criteria

- **AC-COORDINATOR-NEEDS-YOU-007.1:** When Needs you has no item, it shall say
  "Nothing needs you. That is the working state." with the Working count and a
  **See what is running** link to Queue.
- **AC-COORDINATOR-NEEDS-YOU-007.2:** When the workspace has no coordinator, the
  screen shall say "No coordinator in this workspace yet. Questions from your
  agents still wait on their tasks." with no count strip, and managers shall
  see **Add a coordinator**, which opens the add page.
- **AC-COORDINATOR-NEEDS-YOU-007.3:** When loading tasks, stall records or
  proposals fails after an earlier successful load, the screens shall keep
  what was loaded, say "Could not load this workspace's tasks. Showing what
  was loaded at <time>." and offer **Try again**, which retries the failed
  read only. Tasks load per workflow: when any workflow's tasks fail to load,
  the tasks input has failed, the lists and counts shall be built from the
  tasks of every workflow that has loaded, and **Try again** shall retry only
  the workflows that failed.
- **AC-COORDINATOR-NEEDS-YOU-007.5:** When a first load fails (nothing was
  loaded yet for that input), the screens shall say "Could not load this
  workspace's tasks." with no time and offer **Try again**; when no workflow's
  tasks have ever loaded, no list and no count strip shall be shown.
- **AC-COORDINATOR-NEEDS-YOU-007.4:** When the coordinator's session cannot
  start or resume, both lists shall keep working.

### REQ-COORDINATOR-NEEDS-YOU-008: Phone and accessibility

**Intent:** Both screens keep full capability on a phone and with assistive
technology.

#### Acceptance criteria

- **AC-COORDINATOR-NEEDS-YOU-008.1:** At a 390px-wide viewport, item cards shall
  stack in one column, touch targets shall be at least 44px, the page shall not
  scroll horizontally and the count strip shall stay in view.
- **AC-COORDINATOR-NEEDS-YOU-008.2:** An automated accessibility scan of each
  screen shall report no critical violation.

## Out of scope

- Answering a question or permission on the card (phase 4, gate G4).
- Resume on a stall card, merge prompts, the "What it did" log and "Came in"
  (phases 2 and 4).
- A workspace-wide Needs you across coordinators: each coordinator has its own
  list; the stall records are shared.
- Stall classes beyond `task.stalled`: one class is supported by current data.
- Changing the Needs-you Inbox, its count or its single-source rule.
- Server-side computation of the groups: the classification is a projection of
  task facts the client already holds.
