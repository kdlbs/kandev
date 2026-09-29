---
id: coordinator-permissions
title: Coordinator permissions and Watches
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
---

# Coordinator permissions and Watches Requirements

## Overview

Phase 2 builds the coordinator permission model that phase 1 defined and
hardcoded (decision D17). Each coordinator has a setting per action (May do)
and a scope of workflows (Watches). A manager edits both in settings. The
coordinator's tools are the output of that policy, and the coordinator can
never read or change it. No write action is `automatic` in phase 2 (D13).

## Terminology

- **Action:** one of `create_task`, `start_agent`, `message`, `move`, `resume`,
  `stop`. Reading is not an action: the read tools are always available.
  Merging a pull request and moving a task to Done are never actions.
- **Setting:** `denied`, `requires_approval` or `automatic`.
- **Phase-1 policy:** `create_task` `requires_approval`, every other action
  `denied`.
- **Watches:** `all` (every workflow of the workspace, including workflows
  created later) or `selected` (a list of the workspace's workflows).
- **Watched task:** a task whose workflow is watched. A **watched workflow** is
  one in scope under Watches.
- **Agent-starting step:** a workflow step that is not an eligible step as
  [proposals](proposals.md#terminology) defines it.
- **Tool profile:** the Kandev tools on a coordinator session, derived from
  its policy when the conversation opens and bound to it.
- **Phase-2 flag:** `features.coordinatorPhase2`
  ([coordinators](coordinators.md#req-coordinator-coordinators-007-phase-2-release-toggle)).
- Other terms are defined in the [system README](../README.md#terms).

## Mockup

The phase 2 screenshots are the visual reference; where a screenshot and an
acceptance criterion differ, the criterion governs. The prototype banner,
demo controls and `P1` to `P4` and `WC-` labels are mockup chrome.

- [`docs/plans/workspace-coordinator-p2/assets/p2-02-settings-coordinator-sections.png`](../../../plans/workspace-coordinator-p2/assets/p2-02-settings-coordinator-sections.png)

## Requirements

### REQ-COORDINATOR-PERMISSIONS-001: May do settings

**Intent:** A manager decides, per action, what the coordinator may ask for.

**User story:** As a workspace manager, I want to choose which actions my
coordinator may propose, so that it asks only for what I am willing to decide.

Mockup:

- [`docs/plans/workspace-coordinator-p2/assets/p2-02-settings-coordinator-sections.png`](../../../plans/workspace-coordinator-p2/assets/p2-02-settings-coordinator-sections.png): the Sections row with May do.

#### Acceptance criteria

- **AC-COORDINATOR-PERMISSIONS-001.1:** While the phase-2 flag is on, the
  system shall return every coordinator with one setting for each of the six
  actions. A coordinator that existed before phase 2, and one created without
  settings, shall have the phase-1 policy.
- **AC-COORDINATOR-PERMISSIONS-001.2:** When a manager saves settings that
  differ from the stored ones, the system shall store them, increase
  `policy_revision` by one and return the coordinator with its settings and
  new revision. A save equal to the stored settings shall change nothing,
  including the revision.
- **AC-COORDINATOR-PERMISSIONS-001.3:** The system shall refuse a settings save
  with 400, storing nothing, when it names an action outside the six, gives a
  value outside the three settings, sets `stop` to anything but `denied`, or
  sets any action to `automatic`; the last case shall use the error code
  `automatic_not_available`. The error shall name the action.
- **AC-COORDINATOR-PERMISSIONS-001.4:** The system shall have no setting, tool,
  proposal kind or approval path that merges a pull request or moves a task
  into a step that completes the task on entry.
- **AC-COORDINATOR-PERMISSIONS-001.5:** When a reader sends a settings save,
  the system shall refuse it with 403 and change nothing.
- **AC-COORDINATOR-PERMISSIONS-001.6:** The May do section shall list the six
  actions in this order: Create a task, Start an agent, Message a task, Move a
  task, Resume a task, Stop a task. Each row shall have a Denied, Requires
  approval and Automatic choice. Automatic shall be shown disabled with the
  note that it arrives when the coordinator can act on its own. Stop shall
  offer Denied only, with the note that stopping is not available yet.
- **AC-COORDINATOR-PERMISSIONS-001.7:** Below the six rows, the May do section
  shall show two locked rows, Merge a pull request and Move a task to Done,
  each marked "Always human", with no control.
- **AC-COORDINATOR-PERMISSIONS-001.8:** Each action row shall show its last 30
  days from the activity summary
  ([activity log](activity-log.md#req-coordinator-activity-log-005-retention-and-summary)):
  approved and rejected counts, or "Nothing yet" when both are zero. A
  **Review the last 30 days** link shall open What it did filtered to that
  action class.
- **AC-COORDINATOR-PERMISSIONS-001.9:** When Start an agent is not `denied`,
  the May do section shall say that create and move proposals may then target
  steps that start an agent, and that such a card says so before approval.
- **AC-COORDINATOR-PERMISSIONS-001.10:** While the activity summary is loading,
  each May do action row shall show a placeholder; when reading it has failed,
  each row shall say that the last 30 days could not be loaded and offer Try
  again, and shall not show "Nothing yet" or a count.

### REQ-COORDINATOR-PERMISSIONS-002: Enforcement

**Intent:** The policy, not the coordinator's instructions, decides what can
run.

#### Acceptance criteria

- **AC-COORDINATOR-PERMISSIONS-002.1:** When a coordinator conversation opens
  while the phase-2 flag is on, the system shall derive its tool profile from
  the stored policy and bind it with the policy revision to the conversation.
  The profile shall hold the phase-1 read tools and
  `list_coordinator_activity_kandev`, plus `propose_task_kandev` when
  `create_task`, `propose_message_kandev` when `message`,
  `propose_move_kandev` when `move`, and `propose_resume_kandev` when `resume`
  is not `denied`. `start_agent` and `stop` shall add no tool.
- **AC-COORDINATOR-PERMISSIONS-002.2:** When a coordinator session's call
  reaches the backend guard for a Kandev action that is not in its bound
  tool profile, or for a propose action whose action the currently stored
  policy sets to `denied`, or while its binding is invalid, the system shall
  refuse it with the phase-1 unknown-action error, change nothing, and record
  a refused row in the activity log. A tightened setting shall take effect on
  the next call, even within a running conversation. A tool name that was
  never registered for the session is rejected by the agent's MCP client
  before any Kandev action runs; it reaches no guard and writes no row.
- **AC-COORDINATOR-PERMISSIONS-002.3:** When a coordinator conversation opened
  before phase 2 has no bound tool profile, the system shall treat it as the
  phase-1 tool profile. When a bound tool profile cannot be parsed or fails
  validation, the system shall refuse every Kandev action of that session.
- **AC-COORDINATOR-PERMISSIONS-002.4:** Kandev shall auto-approve, for a
  coordinator session, exactly the tool names of its bound tool profile and
  no other tool.
- **AC-COORDINATOR-PERMISSIONS-002.5:** The coordinator's tool surface shall
  have no tool or action that reads or writes its settings, Watches, standing
  orders or goal. These shall change only through the coordinator settings
  routes, which require `workspace.manage`.
- **AC-COORDINATOR-PERMISSIONS-002.6:** When a manager approves a proposal whose
  action the stored policy now sets to `denied`, the system shall refuse the
  approval with 409 and the code `policy_denied`, leave the proposal in its
  status, and the card shall say that its May do settings no longer allow
  this and offer Reject. Rejecting it shall still succeed.

### REQ-COORDINATOR-PERMISSIONS-003: Watches

**Intent:** A manager narrows what the coordinator reads and proposes about.

**User story:** As a workspace manager, I want a coordinator to watch only
some boards, so that two coordinators of one workspace can each own their
part.

Mockup:

- [`docs/plans/workspace-coordinator-p2/assets/p2-02-settings-coordinator-sections.png`](../../../plans/workspace-coordinator-p2/assets/p2-02-settings-coordinator-sections.png): the Sections row with Watches.

#### Acceptance criteria

- **AC-COORDINATOR-PERMISSIONS-003.1:** While the phase-2 flag is on, the
  system shall return every coordinator with its Watches. A coordinator that
  existed before phase 2, and one created without Watches, shall watch `all`.
- **AC-COORDINATOR-PERMISSIONS-003.2:** The system shall refuse a Watches save
  with 400 naming `watches`, storing nothing, when it is `selected` with no
  workflow, with more than 50, with a duplicate, or with a workflow that is
  not in the coordinator's workspace. A save that leaves Watches as stored,
  including an empty list left by a deleted workflow, shall not be refused
  for it.
- **AC-COORDINATOR-PERMISSIONS-003.3:** While a coordinator watches `selected`,
  `list_workflows_kandev` shall return only its watched workflows;
  `list_tasks_kandev` and `list_workflow_steps_kandev` for an unwatched
  workflow, and `get_task_conversation_kandev` and
  `get_coordinator_item_kandev` for a task that is not watched, shall return
  the phase-1 not-found error; and every propose tool shall refuse a target
  task, workflow or step that is not watched, naming the field.
- **AC-COORDINATOR-PERMISSIONS-003.4:** Needs you, Queue and the count strip
  shall count and show only watched tasks and stalls of watched tasks. The
  coordinator's own proposals shall always show.
- **AC-COORDINATOR-PERMISSIONS-003.5:** When a watched workflow is deleted, the
  system shall remove it from every coordinator's list. When a `selected`
  list becomes empty, the coordinator shall watch nothing: its reads return
  empty lists, its Needs you shows only its proposals, and Configure and
  Needs you shall say that it watches no board, with a link to Watches for
  managers. The system shall never switch it to `all`.
- **AC-COORDINATOR-PERMISSIONS-003.6:** The Watches section shall show a
  "Watch every board, including new ones" switch for `all`. When it is off,
  the section shall list every workflow of the workspace in the workspace's
  workflow order, each marked In scope or Out with **Put this board in
  scope** or **Take this board out of scope**; taking the last board out of
  scope shall be refused in the form with a message. The switch shall not be
  turned off while the board list has not loaded or the workspace has no
  board, at most 50 boards shall be put in scope, and a draft left with no
  board in scope shall disable Save and say why.
- **AC-COORDINATOR-PERMISSIONS-003.7:** While the phase-2 flag is on and the
  coordinator's watch set has not loaded, or its read failed with nothing
  loaded, Needs you, Queue and the count strip shall show no task, stall or
  count derived from them, the coordinator's own proposals shall show, and a
  banner line shall say that the boards it watches could not be loaded when the
  read failed.

### REQ-COORDINATOR-PERMISSIONS-004: Saving and the conversation

**Intent:** A policy change applies to the next conversation, and a tighter
one applies at once.

#### Acceptance criteria

- **AC-COORDINATOR-PERMISSIONS-004.1:** When a manager saves a change to May do
  or Watches, the system shall archive the coordinator's current conversation
  task and clear its reference, as a context change does
  (`AC-COORDINATOR-COORDINATORS-002.7`), so the next conversation opens with
  the new tool profile. A save that changes neither shall keep the
  conversation.
- **AC-COORDINATOR-PERMISSIONS-004.2:** When two managers save settings of the
  same coordinator, the system shall apply each accepted save in commit
  order; the last committed save shall determine every field it sets, and
  each save that changes something shall increase `policy_revision` exactly
  once.
- **AC-COORDINATOR-PERMISSIONS-004.3:** The May do and Watches sections shall
  save through the settings save bar with the coordinator's other fields, and
  shall say that saving a change starts the next conversation fresh. One save
  shall send one request for these two sections, carrying only the member
  (May do, Watches or both) that differs from the stored value, whichever
  section the manager last visited; the Identity fields, when also changed,
  are saved by their own request.

## Out of scope

- Setting any action to `automatic`, and choosing which is first (phase 3,
  decision D13).
- A stop proposal kind; `stop` stays `denied` (ADR D25).
- Watching part of a workflow (by step, label or repository).
- A warning when a watched workflow has no repository; the mockup's
  repository-resolution warning belongs to phase 4 intake.
- Workspace-wide maxima set above the per-coordinator settings.
