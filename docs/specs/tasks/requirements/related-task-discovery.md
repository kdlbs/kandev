---
status: draft
system: tasks
created: 2026-10-28
owners:
  - kandev
---

# Related task discovery requirements

## Overview

Task agents use `list_related_tasks_kandev` to inspect task relationships.
Its advertised access rules must match the supported task boundary.
Tasks owns this contract because it owns task identity, hierarchy, and dependency edges.
Agents owns the runtime identities that consume it.

This document records the existing relation restriction and clarifies its discovery contract.
It does not authorize workspace-wide relation reads.

## Terminology

- **Caller:** The task bound to the agent's MCP server.
- **Target:** The task whose relationships the agent requests.
- **Sibling:** A task with the same non-empty parent as the caller.
- **Blocker:** A task that the caller directly depends on through a stored blocker edge.

## Requirements

### REQ-TASKS-RELATED-DISCOVERY-001: Accurate relation discovery contract

**Intent:** An agent can determine which targets it can inspect without mistaking output options for access permissions.

#### Acceptance criteria

- **AC-TASKS-RELATED-DISCOVERY-001.1:** The tool description and `task_id` help shall identify the current task as the default target.
  They shall state that other targets require a permitted relation in the same workspace.
  The tool description shall identify ancestors, descendants, siblings with a shared non-empty parent, and blockers of the caller as permitted targets.
- **AC-TASKS-RELATED-DISCOVERY-001.2:** For an unrelated target, the task-bound tool shall return the existing forbidden response without relation data.
  Shared workspace membership alone shall not grant access.
  Two independent root tasks shall remain unrelated.
  A cross-workspace target shall remain forbidden, including across a stored blocker edge.
- **AC-TASKS-RELATED-DISCOVERY-001.3:** An omitted or false `verbose` shall omit descriptions from an authorized result.
  A true `verbose` shall retain descriptions in an authorized result.
  Neither value shall change target eligibility or convert a forbidden response into an empty successful result.
- **AC-TASKS-RELATED-DISCOVERY-001.4:** The tool shall use its bound caller identity for every target.
  Tool arguments shall not replace that identity.
  Omitted `task_id` and `task_id: "self"` shall select the bound task.
- **AC-TASKS-RELATED-DISCOVERY-001.5:** The tool shall retain its name, arguments, available modes, relationship groups, and authorized response fields.
  Its description and public coordination guidance shall describe the same access restriction.
  The guidance shall state that compact output is a presentation option, not a separate metadata permission.

## Out of scope

- Delegation grants, workspace-wide metadata discovery, and new configuration controls.
- Changes to document permissions, write permissions, task hierarchy, or blocker admission.
- New pagination, graph-completeness guarantees, or PR enrichment behavior.
- A rendered UI change or changes to conversation and messaging permissions.

An explicitly delegated metadata reader needs a separate requirements and authorization design.
This contract correction leaves that coordination use case unresolved.
