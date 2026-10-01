---
id: coordinator-coordinators
title: Coordinators in a workspace
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-26
last_updated: 2026-10-01
---

# Coordinators in a workspace Requirements

## Overview

A workspace manager adds one or more named coordinators to a Kanban workspace,
chooses the agent profile and executor each runs on, and gives each a short
context text. Each coordinator reads the whole workspace in phase 1. This
system owns coordinator identity because the coordinator's conversation,
proposals and attention count are keyed by it.

Everything in this document exists only while `features.coordinator` is on.

## Terminology

- **Manager:** a person holding `workspace.manage` on the workspace. With
  `features.auth` off, the local user is a manager of every workspace.
- **Reader:** a person holding `workspace.read` but not `workspace.manage`.
- **CLI-passthrough profile:** an agent profile that runs the agent CLI without
  Kandev's MCP wrapping.
- Other terms are defined in the [system README](../README.md#terms).

## Mockup

The phase 1 mockup screenshots are the visual reference for this document's
user-facing criteria; each requirement below cites the ones it covers. Where
a screenshot and an acceptance criterion differ, the criterion governs. The
prototype banner, the demo controls and the `P1` and `WC-` labels are mockup
chrome, not product; the data is seeded fiction.

- [`docs/plans/workspace-coordinator/assets/p1-04-settings-coordinators-list.png`](../../../plans/workspace-coordinator/assets/p1-04-settings-coordinators-list.png)
- [`docs/plans/workspace-coordinator/assets/p1-06-settings-coordinator-editor.png`](../../../plans/workspace-coordinator/assets/p1-06-settings-coordinator-editor.png)

## Requirements

### REQ-COORDINATOR-COORDINATORS-001: Release toggle

**Intent:** The coordinator ships dark and can be enabled per install.

#### Acceptance criteria

- **AC-COORDINATOR-COORDINATORS-001.1:** The runtime flag `features.coordinator`
  shall default to `"false"` in the `prod` and `dev` profiles and to `"true"` in
  the `e2e` profile, and shall be restart-required.
- **AC-COORDINATOR-COORDINATORS-001.2:** When the flag is off, every
  `/api/v1/workspaces/:id/coordinators*` and `/api/v1/workspaces/:id/coordinator-stalls`
  route shall return 404, and no coordinator settings tab, sidebar entry, route
  or copilot shall render.
- **AC-COORDINATOR-COORDINATORS-001.3:** When the flag is off, coordinator data
  already stored shall be kept unchanged, and turning the flag on again after a
  restart shall show the same coordinators and proposals.

### REQ-COORDINATOR-COORDINATORS-002: Add, edit and delete coordinators

**Intent:** A manager configures who the coordinator is and what it runs on.

**User story:** As a workspace manager, I want to add a coordinator with a name,
an agent profile, an executor and a context, so that I can ask it about my
workspace.

Mockup:

- [`docs/plans/workspace-coordinator/assets/p1-06-settings-coordinator-editor.png`](../../../plans/workspace-coordinator/assets/p1-06-settings-coordinator-editor.png): the coordinator page: name, agent profile, executor, context with its help text, and Delete coordinator.

#### Acceptance criteria

- **AC-COORDINATOR-COORDINATORS-002.1:** When a manager creates a coordinator
  with a name, an agent profile of the workspace's agents, an executor profile
  and an optional context, the system shall store it and return it with a new
  id, `created_at` and `updated_at`.
- **AC-COORDINATOR-COORDINATORS-002.2:** When the name, after trimming leading
  and trailing whitespace, is empty or longer than 60 characters, the system
  shall refuse the create or edit with a 400 error naming the field. Names are
  not required to be unique in a workspace.
- **AC-COORDINATOR-COORDINATORS-002.3:** When the context is longer than 4,000
  characters, the system shall refuse the create or edit with a 400 error. An
  absent context is stored as the empty string.
- **AC-COORDINATOR-COORDINATORS-002.4:** When the chosen agent profile is a
  CLI-passthrough profile, the system shall refuse the create or edit with a
  400 error that says the coordinator needs Kandev's MCP tools.
- **AC-COORDINATOR-COORDINATORS-002.5:** When the agent profile or the executor
  profile does not exist, the system shall refuse the create or edit with a
  400 error naming the field.
- **AC-COORDINATOR-COORDINATORS-002.6:** When two managers edit the same
  coordinator, the system shall apply each accepted edit in commit order, and
  the last committed edit shall determine every field it sets.
- **AC-COORDINATOR-COORDINATORS-002.7:** When a manager saves a coordinator whose
  context, trimmed of leading and trailing whitespace, differs from its stored
  (trimmed) context, the system shall archive the current conversation task and
  clear the coordinator's reference to it, so that the next conversation starts
  with the new context (see
  `AC-COORDINATOR-COPILOT-001.4`). An edit that leaves the trimmed context
  unchanged, including one that only adds leading or trailing whitespace,
  shall keep the conversation.
- **AC-COORDINATOR-COORDINATORS-002.8:** When a manager deletes a coordinator,
  the system shall delete the coordinator, all of its proposals in every status
  and its conversation tasks (the current one and any archived by a context
  change), and shall leave every task created by approving its proposals on its
  board.
- **AC-COORDINATOR-COORDINATORS-002.9:** When a request edits or deletes a
  coordinator id that does not exist in the workspace, the system shall return
  404. A repeated delete of the same id shall return 404 and change nothing.
- **AC-COORDINATOR-COORDINATORS-002.10:** When a manager saves a coordinator
  whose `agent_profile_id` or `executor_profile_id` differs from its stored
  value, the system shall archive the current conversation task and clear the
  coordinator's reference to it, the same as a context change
  (`AC-COORDINATOR-COORDINATORS-002.7`), because a running session cannot be
  moved onto a different profile pair mid-session. The next conversation
  opened after the save shall create its task and session from the newly
  saved profiles, and any missing or passthrough profile of that new pair
  shall be reported per `AC-COORDINATOR-COORDINATORS-005.1` rather than the
  conversation continuing on the old, now-archived session. Sending a profile
  id back unchanged shall keep the conversation.
### REQ-COORDINATOR-COORDINATORS-003: Listing and permissions

**Intent:** Readers see coordinators; only managers change them.

#### Acceptance criteria

- **AC-COORDINATOR-COORDINATORS-003.1:** When any caller with `workspace.read`
  lists coordinators, the system shall return every coordinator of the
  workspace ordered by `created_at` ascending, then `id` ascending, and shall
  return an empty list when there is none.
- **AC-COORDINATOR-COORDINATORS-003.2:** When a reader sends a create, edit or
  delete, the system shall refuse it with 403 and change nothing.
- **AC-COORDINATOR-COORDINATORS-003.3:** When a coordinator id belongs to another
  workspace, every coordinator route addressed through this workspace shall
  return 404.

### REQ-COORDINATOR-COORDINATORS-004: Coordinators settings tab

**Intent:** Coordinators are configured where other workspace settings are.

Mockup:

- [`docs/plans/workspace-coordinator/assets/p1-04-settings-coordinators-list.png`](../../../plans/workspace-coordinator/assets/p1-04-settings-coordinators-list.png): the Coordinators settings tab with the list.

#### Acceptance criteria

- **AC-COORDINATOR-COORDINATORS-004.1:** When the flag is on, the workspace
  settings shall show a **Coordinators** tab after **Secrets**, listed under the
  workspace in the settings tree and found by settings search.
- **AC-COORDINATOR-COORDINATORS-004.2:** The Coordinators list shall show one
  card per coordinator, in the order of `AC-COORDINATOR-COORDINATORS-003.1`,
  with its name, agent profile, executor and context, an **Open** action to its
  Needs you and a **Configure** action to its page, and, while the phase-2
  flag is off, a note that Watches, May do and Standing orders arrive in a
  later phase.
- **AC-COORDINATOR-COORDINATORS-004.3:** When a manager opens **+ Add
  coordinator**, the page shall show Name, Agent profile, Executor and Context,
  shall list CLI-passthrough profiles disabled with their reason, and shall
  enable **Add coordinator** only while the name is valid, both profiles are
  chosen and the Agent for created tasks pair of
  [`REQ-COORDINATOR-CREATED-TASK-AGENT-001`](created-task-agent.md) is chosen; after adding, the new
  coordinator's page shall open.
- **AC-COORDINATOR-COORDINATORS-004.4:** On an existing coordinator's page, a
  change shall be saved through the settings save bar (Discard and Save appear
  only while something changed), the page shall say that the profile's
  auto-approve setting is ignored for coordinators and that saving a changed
  context starts the next conversation fresh, and an **All coordinators** link
  shall return to the list.
- **AC-COORDINATOR-COORDINATORS-004.5:** When a manager chooses **Delete
  coordinator**, a confirmation dialog shall say the action cannot be undone,
  that pending proposals and the conversation are deleted and that created
  tasks stay; only confirming deletes.
- **AC-COORDINATOR-COORDINATORS-004.6:** When the viewer is a reader, the list
  shall have no **+ Add coordinator** and the coordinator's page shall show its
  fields disabled with no Save or Delete.
- **AC-COORDINATOR-COORDINATORS-004.7:** The Coordinators list shall be
  introduced by "Coordinators read this workspace's boards, tell you what
  needs you and why, and propose work that waits for your decision." It
  shall not say or imply that a manager approves every action a coordinator
  takes, because the MCP guard enforces only the Kandev surface
  (`AC-COORDINATOR-PROPOSALS-002.15`).

### REQ-COORDINATOR-COORDINATORS-005: Missing profile

**Intent:** A coordinator whose agent or executor profile was deleted, or
whose agent profile became CLI passthrough, says so instead of failing
silently.

Mockup:

- [`docs/plans/workspace-coordinator/assets/p1-06-settings-coordinator-editor.png`](../../../plans/workspace-coordinator/assets/p1-06-settings-coordinator-editor.png): the Agent profile field the missing-profile warning attaches to (the warning itself is not in the mockup).

#### Acceptance criteria

- **AC-COORDINATOR-COORDINATORS-005.1:** When a coordinator's `profileStatus`
  reports `agent_profile_status` or `executor_profile_status` as anything
  other than `ok`, its settings page and its copilot shall show that field's
  message and ask for another: agent `missing` says the profile was removed;
  agent `passthrough` says the profile uses CLI passthrough, which a
  coordinator cannot use (not that it was removed); executor `missing` says
  the executor was removed. The conversation route shall return 409 until a
  manager saves profiles that are both `ok`. When both fields are not `ok`,
  both messages shall show, each under its own field on the settings page and
  both in the copilot in place of the composer.

### REQ-COORDINATOR-COORDINATORS-006: Workspace deletion

**Intent:** Coordinator data never outlives its workspace.

#### Acceptance criteria

- **AC-COORDINATOR-COORDINATORS-006.1:** When a workspace is deleted, the system
  shall delete its coordinators, proposals and stall records in one
  transaction, and its conversation tasks shall be deleted with the workspace's
  other tasks; a repeated deletion event for the same workspace shall change
  nothing and report no error.

### REQ-COORDINATOR-COORDINATORS-007: Phase 2 release toggle

**Intent:** Phase 2 ships dark on top of phase 1 and can be enabled per
install.

#### Acceptance criteria

- **AC-COORDINATOR-COORDINATORS-007.1:** The runtime flag
  `features.coordinatorPhase2` (`KANDEV_FEATURES_COORDINATOR_PHASE2`) shall
  default to `"false"` in the `prod` and `dev` profiles and to `"true"` in the
  `e2e` profile, shall be restart-required, and shall take effect only while
  `features.coordinator` is also on.
- **AC-COORDINATOR-COORDINATORS-007.2:** While the phase-2 flag is off, the
  coordinator shall behave exactly as phase 1 specifies: the phase-2 routes
  shall return 404, no phase-2 section, launcher, card kind, action or note
  shall render, conversations shall open with the phase-1 tool profile and
  instructions, and the phase-2 propose tools shall not be registered.
- **AC-COORDINATOR-COORDINATORS-007.3:** While the phase-2 flag is off, stored
  phase-2 data (settings, Watches, standing orders, goals, activity rows and
  non-create proposals) shall be kept unchanged; open non-create proposals
  shall not show and shall not count toward Needs you; reading, approving or
  rejecting a non-create proposal by its id shall return 404 as for an
  unknown proposal, change nothing and run nothing; turning the flag on again
  after a restart shall show the same data.
- **AC-COORDINATOR-COORDINATORS-007.4:** While the phase-2 flag is off, the
  guard shall enforce the phase-1 policy for every coordinator, whatever its
  stored settings.

### REQ-COORDINATOR-COORDINATORS-008: Guided setup

**Intent:** A manager adds a coordinator by answering a few questions in
order.

**User story:** As a workspace manager, I want to be walked through adding a
coordinator, so that it starts with a scope, a purpose and the permissions I
chose.

#### Acceptance criteria

- **AC-COORDINATOR-COORDINATORS-008.1:** While the phase-2 flag is on, **+ Add
  coordinator** shall open a setup, in place of the add form of `004.3`, with
  the steps Who runs it, What it watches, What it is for, What it knows, What
  it may do and Review. On a wide screen the step list shall be visible with
  the current step marked by more than colour; on a phone the page shall show
  "Step N of 6" and the step name instead. Steps 1
  to 5 shall have **Next** and, from step 2, **Back**; Review shall have
  **Back** and **Finish** and no **Next**.
- **AC-COORDINATOR-COORDINATORS-008.2:** Who runs it shall ask for the name,
  agent profile and executor under the phase-1 rules, and for Agent for
  created tasks under `REQ-COORDINATOR-CREATED-TASK-AGENT-001`; What it watches shall
  offer the Watches choice of
  [permissions](permissions.md#req-coordinator-permissions-003-watches) with
  `all` chosen; What it is for shall offer the goal form of
  [goals](goals.md#req-coordinator-goals-001-setting-a-goal) and **Skip this
  step**; What it knows shall offer the context and **Skip this step**; What
  it may do shall show the May do rows with `create_task`, `message`, `move`
  and `resume` set to Requires approval and `start_agent` and `stop` Denied.
- **AC-COORDINATOR-COORDINATORS-008.3:** Review shall show a "What it wrote"
  table with the columns Setting, Value and Owned from now on by, thirteen rows
  (Name, Agent profile, Executor, Agent for created tasks, Watches, Goal,
  Context and one per May do action), each naming the Configure section that owns it and showing "Not
  set" for a skipped Goal or Context; **Change**
  on a row shall return to its step with the values kept.
- **AC-COORDINATOR-COORDINATORS-008.4:** **Finish** shall be enabled only while
  the name is valid, both profiles and the Agent for created tasks pair are
  chosen and Watches is `all` or has at
  least one workflow; the system shall create the coordinator, its settings,
  its Watches and its goal in one transaction, or none of them, and open the
  new coordinator's Configure page.
- **AC-COORDINATOR-COORDINATORS-008.5:** When a manager leaves the setup before
  Finish, nothing shall be created. The setup shall be available only to
  managers.
- **AC-COORDINATOR-COORDINATORS-008.6:** If the setup request is refused
  because a chosen value is invalid, the system shall create nothing and
  the page shall return to the step that holds the value, with the value
  kept and the error shown beside it; if the server answers with any other
  failure, the page shall stay on Review with every value kept and show that
  nothing was created; if no answer arrives, the page shall stay on Review
  with every value kept and show that it could not confirm whether the
  coordinator was created and that the list should be checked before trying
  again.

### REQ-COORDINATOR-COORDINATORS-009: Configure sections and list summary

**Intent:** Each coordinator's page has one section per kind of setting.

Mockup:

- [`docs/plans/workspace-coordinator-p2/assets/p2-02-settings-coordinator-sections.png`](../../../plans/workspace-coordinator-p2/assets/p2-02-settings-coordinator-sections.png): the Sections row.

#### Acceptance criteria

- **AC-COORDINATOR-COORDINATORS-009.1:** While the phase-2 flag is on, a
  coordinator's page shall show a Sections row with Identity, Watches, May
  do, Standing orders and Goal, each with a one-line help text; Identity
  shall hold the phase-1 fields, and the chosen section shall be kept in the
  page address.
- **AC-COORDINATOR-COORDINATORS-009.2:** While the phase-2 flag is on, each
  list card shall replace the later-phase note with a summary line: the
  number of watched boards or "Every board", the number of actions that
  require approval, and the number of active standing orders.

## Out of scope

- Watches, May do, Standing orders and guided setup in phase 1; phase 2
  specifies them in [permissions](permissions.md),
  [standing orders](standing-orders.md) and
  `REQ-COORDINATOR-COORDINATORS-008`.
- The coordinator permission model (decision D17), defined now and built in a
  later phase (gate G2). Each coordinator has a scope and a setting per
  action, and each action is one of `denied`, `requires approval` or
  `automatic`. Phase 1 has no settings: it hardcodes one policy for every
  coordinator. The six read tools of the phase-1 tool profile
  ([copilot](copilot.md#req-coordinator-copilot-003-kandev-tool-surface)) are
  `automatic`, `propose_task_kandev` is `requires approval` (a proposal
  becomes a task only when a manager approves it), and every other action is
  `denied`. Approving one proposal grants no permission for any later action.
  A coordinator's runtime session can never change or raise its own
  permissions; only a manager edits them. Decision D13 still governs when a
  write action may first be set to `automatic`.
- Scoping a coordinator to a subset of workflows (phase 2 Watches).
- Unique coordinator names: duplicates are allowed; the id identifies a
  coordinator everywhere.
- Coordinators for Office workspaces: Office keeps its own coordinator role.
