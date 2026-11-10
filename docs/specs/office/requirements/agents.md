---
status: draft
system: office
created: 2026-04-25
owners:
  - cfl
---
# Office: Agents Requirements

## Overview

Kandev has execution profiles (configuration templates for a concrete CLI, account, model, flags, environment, and MCP setup) but also needs persistent, stateful Office agents. Without a stable Office identity, switching a provider also risks switching or copying the agent's role, instructions, skills, permissions, budget, and history.

## Requirements

### REQ-OFFICE-AGENTS-001: Office: Agents

**Intent:** Kandev has execution profiles (configuration templates for a concrete CLI, account, model, flags, environment, and MCP setup) but also needs persistent, stateful Office agents. Without a stable Office identity, switching a provider also risks switching or copying the agent's role, instructions, skills, permissions, budget, and history.

#### Acceptance criteria

- **AC-OFFICE-AGENTS-001.1:** An Office agent is a persistent `agent_profiles` row scoped by `workspace_id`; its row ID is the logical `agent_profile_id` referenced by assignments, instructions, skills, budgets, permissions, and Office history.
- **AC-OFFICE-AGENTS-001.2:** An Office agent selects an execution agent profile. A concrete selection launches directly. A dynamic selection resolves an `execution_profile_id` from the dynamic profile; the resolved concrete profile owns the CLI runtime configuration.
- **AC-OFFICE-AGENTS-001.3:** The Office identity owns:
- **AC-OFFICE-AGENTS-001.4:** **Name**: human-readable label ("CEO", "Frontend Worker", "QA Bot").
- **AC-OFFICE-AGENTS-001.5:** **Role**: `ceo`, `worker`, `specialist`, `assistant`, or `reviewer`. Determines default permissions and UI treatment.
- **AC-OFFICE-AGENTS-001.6:** **Status**: `idle`, `working`, `paused`, `stopped`, plus transitional `pending_approval`.
- **AC-OFFICE-AGENTS-001.7:** **Permissions**: JSON object controlling what the instance can do.
- **AC-OFFICE-AGENTS-001.8:** **Budget**: remaining spend allowance (see [costs](costs.md)).

### REQ-OFFICE-AGENT-NAMES-001: Workspace agent name admission

**Intent:** An Office identity must remain distinguishable within its workspace
when users create or rename it. A refused name must leave the saved identity
and its exportable configuration intact. Office owns this contract because it
owns the persistent workspace identities; execution profiles remain a separate
namespace even though both concepts share storage.

An occupied name is the exact name of any other non-deleted Office identity in
the same workspace. The agent being edited does not occupy its own name for
this purpose. Agent status does not release a name. Names remain case-sensitive
and retain whitespace; this contract adds no normalization.

#### Acceptance criteria

- **AC-OFFICE-AGENT-NAMES-001.1:** When another non-deleted Office agent in the
  same workspace already has a requested exact name, creating an agent or
  renaming an existing agent to that name shall be refused.
- **AC-OFFICE-AGENT-NAMES-001.2:** A name conflict on the Office agent create or
  update HTTP endpoint shall return `400 Bad Request`, a non-empty `error`, and
  `code: agent_name_already_exists`. A refused rename shall leave both saved
  agents, their timestamps, and any other fields submitted with that rename
  unchanged.
- **AC-OFFICE-AGENT-NAMES-001.3:** Saving an agent with its unchanged name, or
  omitting the name while changing another valid field, shall succeed when no
  other agent in that workspace occupies the name. The accepted other field
  shall be saved on the same identity.
- **AC-OFFICE-AGENT-NAMES-001.4:** An agent name used only in another workspace
  shall remain available for creation or rename in the target workspace.
  Both workspaces shall retain distinct identities and valid configuration
  exports.
- **AC-OFFICE-AGENT-NAMES-001.5:** An otherwise valid rename to an unoccupied
  name shall succeed and preserve the agent's identity. Names differing only
  by case or retained whitespace shall continue to be distinct; a deleted
  Office identity or a global execution profile shall not reserve a name in
  the workspace.
- **AC-OFFICE-AGENT-NAMES-001.6:** Given a workspace whose configuration export
  succeeds before a refused rename, its export shall still succeed afterward
  with the original distinct agent configurations and the same exported
  contents and revision.

## Name admission scope

This requirement covers admission against names already present when an
ordinary Office create or update is validated. Concurrent write serialization,
repair of existing duplicates, import reconciliation, filename normalization,
and execution-profile naming rules are outside this change. No new desktop or
phone interaction is required: both consume the same accepted or refused save
outcome through the existing API.

## System design

The migrated technical source is split into [part 1](../system-design/agents-01.md), [part 2](../system-design/agents-02.md), [part 3](../system-design/agents-03.md).
Workspace name admission is defined in [part 1](../system-design/agents-01.md#workspace-agent-name-admission).

## Implementation plans

- [Office agent rename uniqueness](../../../plans/office-agent-rename-uniqueness/plan.md)
