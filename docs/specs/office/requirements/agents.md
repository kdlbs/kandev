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

## System design

The migrated technical source is split into [part 1](../system-design/agents-01.md), [part 2](../system-design/agents-02.md), [part 3](../system-design/agents-03.md).
