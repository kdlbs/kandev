---
status: active
system: executors
created: 2026-10-15
owners:
  - kandev
---

# Per-user executor ownership requirements

## Overview

[Issue #4512](https://github.com/kdlbs/kandev/issues/4512) requests private
compute and profile-seeded credentials within a shared Kandev control plane.
The executor system owns this capability because it owns executor profiles and
admission to their compute. Identity, organization reach, and existing scopes
remain owned by [auth](../../auth/README.md).

## Terminology

- **Owned resource:** An executor or executor profile with an assigned user.
- **Shared resource:** A resource without an assigned user.
- **Configuration manager:** A caller with `org.config.manage` within the
  resource's existing authorization boundary.
- **Launch actor:** The authenticated user accountable for an interactive launch,
  or the recorded Kandev creator of an autonomous launch source.

## Requirements

### REQ-EXECUTORS-OWNERSHIP-001: Compatible ownership

**Intent:** Add private resources without changing existing installations.

#### Acceptance criteria

- **AC-EXECUTORS-OWNERSHIP-001.1:** Existing executors and profiles shall remain
  shared after upgrade. Omitting ownership when creating a resource through an
  existing client shall retain shared behavior, subject to existing permissions.
- **AC-EXECUTORS-OWNERSHIP-001.2:** A user shall be eligible for an owned
  executor/profile only when they own it or are a configuration manager. When
  both parent executor and profile have owners, both checks shall pass.
- **AC-EXECUTORS-OWNERSHIP-001.3:** Ownership shall survive restart and
  authentication being disabled and re-enabled. Authentication-disabled mode
  shall retain the existing single-user access behavior.
- **AC-EXECUTORS-OWNERSHIP-001.4:** Deleting or disabling an owner shall not make
  their resources shared. Configuration managers shall retain recovery and
  reassignment capability within existing resource boundaries.

### REQ-EXECUTORS-OWNERSHIP-002: Ownership management

**Intent:** Let engineers maintain personal profiles while administrators retain
configuration control.

#### Acceptance criteria

- **AC-EXECUTORS-OWNERSHIP-002.1:** A caller permitted to create a resource shall
  be able to assign themselves as owner. An owner shall be able to clear their
  ownership; a configuration manager shall be able to assign, clear, or reassign
  ownership. Non-managers shall not claim an existing shared resource or assign
  someone else as owner.
- **AC-EXECUTORS-OWNERSHIP-002.2:** A non-owner shall not modify, delete, test a
  connection for, clone, or otherwise obtain protected configuration from an
  owned resource. Ownership shall not remove existing provider-specific
  restrictions on configuration changes.
- **AC-EXECUTORS-OWNERSHIP-002.3:** Invalid owners and incompatible parent/profile
  assignments shall be rejected without partial writes or successful change
  notifications. A non-manager shall not use a stale save to undo reassignment.
- **AC-EXECUTORS-OWNERSHIP-002.4:** Desktop and phone Settings shall display
  shared or owned status and support permitted ownership changes, save, discard,
  reload, and denied-save recovery. Controls shall be accessible by keyboard and
  touch, and new copy shall be localized in all supported languages.

### REQ-EXECUTORS-OWNERSHIP-003: Private catalogue projections

**Intent:** Private configuration stays out of other members' selectable data.

#### Acceptance criteria

- **AC-EXECUTORS-OWNERSHIP-003.1:** Settings, task creation, defaults, workflow
  configuration, automation/watch pickers, and other catalogue consumers shall
  contain only shared resources and owned resources available to the caller.
  Configuration managers shall see all resources within their existing reach.
- **AC-EXECUTORS-OWNERSHIP-003.2:** HTTP, WebSocket, MCP, settings operations,
  initial application state, and live notifications shall enforce the same
  visibility. Foreign owned resources addressed directly shall return not found,
  without disclosing their configuration, owner identity, or secrets.
- **AC-EXECUTORS-OWNERSHIP-003.3:** After ownership changes, connected clients
  shall remove resources they can no longer access and obtain newly accessible
  resources. A previously selected inaccessible default shall require an
  eligible explicit selection before starting work; it shall not silently select
  a different executor or expose the inaccessible resource's details.
- **AC-EXECUTORS-OWNERSHIP-003.4:** Phone task creation and Settings shall provide
  the same eligible choices and ownership actions as desktop, with no document
  horizontal overflow, discoverable actions, and touch targets at least 44px.

### REQ-EXECUTORS-OWNERSHIP-004: Authorized compute admission

**Intent:** Enforce ownership before using compute or profile credentials.

#### Acceptance criteria

- **AC-EXECUTORS-OWNERSHIP-004.1:** Interactive task/session start, resume,
  runner switch, inherited environment selection, retry, and MCP launches shall
  verify the requesting user's eligibility independently of picker contents.
  Denial shall occur before compute provisioning, credential resolution or
  seeding, scripts, or agent prompt delivery.
- **AC-EXECUTORS-OWNERSHIP-004.2:** Shared-task access alone shall not authorize
  prompting an agent, starting a terminal, writing files, or otherwise executing
  commands on another user's owned compute. Existing transcript and board read
  permissions shall remain unchanged; ownership shall not grant workspace reach.
- **AC-EXECUTORS-OWNERSHIP-004.3:** A queued launch shall be rechecked at
  admission using current ownership and account eligibility. Missing identity,
  account lookup failures, and inaccessible explicit profiles shall fail closed
  for owned compute, with no fallback to another profile or workspace owner.
- **AC-EXECUTORS-OWNERSHIP-004.4:** Ownership changes shall apply to subsequent
  admissions and interactive execution requests without terminating work already
  admitted. Read-only recovery of an existing execution shall not authorize new
  prompts or provision new compute without a valid launch actor.

### REQ-EXECUTORS-OWNERSHIP-005: Accountable autonomous launches

**Intent:** Background services must not bypass private-compute ownership.

#### Acceptance criteria

- **AC-EXECUTORS-OWNERSHIP-005.1:** Autonomous provider-watch, automation, and
  workflow launches shall check owned resources as the source's recorded Kandev
  creator, using that account's current permissions and reach. Provider-side
  issue-author filters and the scheduler's service identity shall not establish
  that authority.
- **AC-EXECUTORS-OWNERSHIP-005.2:** New autonomous sources shall retain their
  authenticated creator across edits, retries, and restarts. Exported
  definitions shall not carry trusted execution authority. Import shall establish the importing user as the new source's creator rather
  than trusting exported creator identity. Unknown legacy creators may continue
  to use shared resources but shall not launch owned resources.
- **AC-EXECUTORS-OWNERSHIP-005.3:** A disabled, missing, or ineligible creator
  shall cause an owned-resource run to fail with an actionable authorization
  result and no compute or credential side effects. Creator changes shall not
  be inferred from workspace ownership or the last editor.
- **AC-EXECUTORS-OWNERSHIP-005.4:** An editor shall not turn another user's
  private-executor source into authority for arbitrary work. Updating or
  triggering execution through an interactive path shall check that caller's
  eligibility as well as any autonomous creator authority used for execution.

## Out of scope

Allowed-users lists, per-workspace executor grants, a new `executor.use` scope,
ownership of other shared configuration, OS credential isolation for Local or
Worktree execution, and stopping existing executions on ownership transfer.
Read access to shared boards does not promise shell access to private compute.

## Delivery

[Implementation plan](../../../plans/executor-ownership/plan.md).
