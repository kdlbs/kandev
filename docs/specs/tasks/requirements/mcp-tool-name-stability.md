---
status: active
system: tasks
created: 2026-08-31
owners:
  - kandev
---

# MCP Tool Name Stability Requirements

## Overview

Kandev gives agents built-in MCP tools whose canonical names end in
`_kandev`. The task system owns the model-facing names used by its system
prompts and session tool surface. The agent system owns the capability data
that describes how a particular client presents those names.

An agent client can add the MCP server name to every tool before presenting the
tool to its model. Kandev must compensate for that behavior without changing
the canonical tool names used by clients that present server definitions
unchanged.

## Terminology

- **Canonical tool name:** The registered Kandev tool name, including its
  trailing `_kandev` suffix.
- **Server-namespacing agent:** An agent client that presents an MCP tool as
  `<server tool name>_<server name>`. The client removes that namespace before
  it sends `tools/call` to the server.
- **Transport-facing tool name:** The name returned by the per-session Kandev
  MCP endpoint before an agent client applies its own presentation rules.

## Requirements

### REQ-TASKS-MCP-TOOL-NAMES-001: Stable model-facing Kandev tool names

**Intent:** Let every supported agent model discover and call the canonical
Kandev tool name on its first attempt. Client-side MCP namespacing must not
change that name.

#### Acceptance criteria

- **AC-TASKS-MCP-TOOL-NAMES-001.1:** When a server-namespacing agent receives a
  built-in canonical name, the system shall present exactly one `_kandev`
  suffix to the model.
- **AC-TASKS-MCP-TOOL-NAMES-001.2:** When an agent does not namespace MCP tools
  by server name, the system shall present the unchanged canonical tool name to
  the model.
- **AC-TASKS-MCP-TOOL-NAMES-001.3:** For every built-in Kandev tool in every MCP
  profile, the name described by the Kandev system context shall match the name
  presented to the model.
- **AC-TASKS-MCP-TOOL-NAMES-001.4:** When a server-namespacing agent calls the
  model-facing name, the MCP server shall use the canonical registered name.
  The existing handler contract shall receive the request.
- **AC-TASKS-MCP-TOOL-NAMES-001.5:** New sessions, loaded sessions, resumed
  sessions, and reset sessions shall apply the same naming behavior for the
  selected agent.

## Compatibility

- `ask_user_question_kandev`, `get_task_plan_kandev`, and all other built-in
  canonical names retain their existing spelling.
- The injected MCP server retains the name `kandev`.
- Agents that do not declare server namespacing retain the current tool list
  and call behavior.
- Tool descriptions and Kandev system prompts continue to use canonical names.

## Out of scope

- Changing the behavior of the external Auggie CLI or any other agent client.
- Globally removing the `_kandev` suffix from registered tools.
- Renaming the injected Kandev MCP server.
- Changing plugin-contributed tool names that do not use the trailing
  `_kandev` convention.
- Changing tool schemas, permissions, MCP profiles, or handler behavior.
