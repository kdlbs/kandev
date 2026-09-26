---
id: coordinator-coordinators
title: Coordinators in a workspace
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-26
last_updated: 2026-09-26
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
  shall be reported per `AC-COORDINATOR-COORDINATORS-005.1`-`005.3` rather
  than the conversation continuing on the old, now-archived session. Sending
  a profile id back unchanged shall keep the conversation.

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
  Needs you and a **Configure** action to its page, and a note that Watches,
  May do and Standing orders arrive in a later phase.
- **AC-COORDINATOR-COORDINATORS-004.3:** When a manager opens **+ Add
  coordinator**, the page shall show Name, Agent profile, Executor and Context,
  shall list CLI-passthrough profiles disabled with their reason, and shall
  enable **Add coordinator** only while the name is valid and both profiles are
  chosen; after adding, the new coordinator's page shall open.
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
- **AC-COORDINATOR-COORDINATORS-004.7:** At a 390px-wide viewport the list and
  the coordinator's page shall stack in one column with no horizontal scroll and
  touch targets of at least 44px.

### REQ-COORDINATOR-COORDINATORS-005: Missing profile

**Intent:** A coordinator whose agent or executor profile was deleted, or
whose agent profile became CLI passthrough, says so instead of failing
silently.

Mockup:

- [`docs/plans/workspace-coordinator/assets/p1-06-settings-coordinator-editor.png`](../../../plans/workspace-coordinator/assets/p1-06-settings-coordinator-editor.png): the Agent profile field the missing-profile warning attaches to (the warning itself is not in the mockup).

#### Acceptance criteria

- **AC-COORDINATOR-COORDINATORS-005.1:** When a coordinator's agent profile no
  longer exists, its settings page and its copilot shall say the profile was
  removed and ask for another, and the conversation route shall return 409
  until a manager saves an existing, non-passthrough profile.
- **AC-COORDINATOR-COORDINATORS-005.2:** When a coordinator's executor profile
  no longer exists, its settings page and its copilot shall say the executor
  was removed and ask for another, and the conversation route shall return 409
  until a manager saves an existing executor profile. When both profiles are
  missing, both messages shall show: on the settings page each under its own
  field, and in the copilot stacked in place of the composer, the agent
  profile message first.
- **AC-COORDINATOR-COORDINATORS-005.3:** When a coordinator's agent profile
  exists but was switched to CLI passthrough after it was saved, its settings
  page and its copilot shall say the profile uses CLI passthrough, which a
  coordinator cannot use, and ask for another (not that it was removed), and
  the conversation route shall return 409 until a manager saves an existing,
  non-passthrough profile. With a missing executor profile as well, both
  messages shall show as `AC-COORDINATOR-COORDINATORS-005.2` says.

### REQ-COORDINATOR-COORDINATORS-006: Workspace deletion

**Intent:** Coordinator data never outlives its workspace.

#### Acceptance criteria

- **AC-COORDINATOR-COORDINATORS-006.1:** When a workspace is deleted, the system
  shall delete its coordinators, proposals and stall records in one
  transaction, and its conversation tasks shall be deleted with the workspace's
  other tasks; a repeated deletion event for the same workspace shall change
  nothing and report no error.

## Out of scope

- Watches, May do, Standing orders and guided setup (phase 2, gate G2).
- Per-coordinator action tiers (decision D16, gate G2).
- Scoping a coordinator to a subset of workflows (phase 2 Watches).
- Unique coordinator names: duplicates are allowed; the id identifies a
  coordinator everywhere.
- Coordinators for Office workspaces: Office keeps its own coordinator role.
