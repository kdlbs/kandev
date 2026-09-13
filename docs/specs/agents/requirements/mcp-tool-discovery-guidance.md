---
status: draft
system: agents
created: 2026-09-08
owners:
  - kandev
---

# MCP tool discovery guidance requirements

## Overview

Task agents need to discover Kandev operations without an exhaustive tool list
in every prompt. The Agents system owns this instruction contract across agent
clients. Canvases owns its authoring workflow and task creation preset.

## Requirements

### REQ-AGENTS-MCP-DISCOVERY-001: Discoverable Kandev operations

**Intent:** Agents receive enough guidance to find operations that the prompt
does not list.

#### Acceptance criteria

- **AC-AGENTS-MCP-DISCOVERY-001.1:** Task instructions shall identify the
  injected tool list as selected guidance, not the complete available catalog.
- **AC-AGENTS-MCP-DISCOVERY-001.2:** When a needed Kandev tool is not callable,
  instructions shall direct agents to native tool search when available.
- **AC-AGENTS-MCP-DISCOVERY-001.3:** Search guidance shall include the Kandev
  server name and the requested operation or known canonical tool name.
- **AC-AGENTS-MCP-DISCOVERY-001.4:** When native search is absent, instructions
  shall direct agents to the available MCP catalog. They shall not assume a
  client-specific search API.
- **AC-AGENTS-MCP-DISCOVERY-001.5:** Instructions shall require the callable
  name and schema returned by the client. An omitted prompt entry shall not
  imply an unavailable tool.
- **AC-AGENTS-MCP-DISCOVERY-001.6:** When discovery cannot find the required
  operation, instructions shall require an accurate limitation report. They
  shall not direct agents to claim an equivalent platform result from files.

### REQ-AGENTS-MCP-DISCOVERY-002: Compact capability-aware guidance

**Intent:** Shorter prompts preserve mandatory workflow behavior and capability
boundaries.

#### Acceptance criteria

- **AC-AGENTS-MCP-DISCOVERY-002.1:** The default task instructions shall omit
  routine inventory and CRUD descriptions that agents can discover on demand.
- **AC-AGENTS-MCP-DISCOVERY-002.2:** The instructions shall preserve question
  barriers, title ownership, completion conditions, autopilot rules, and
  delegation authorization.
- **AC-AGENTS-MCP-DISCOVERY-002.3:** Optional operation guidance shall match the
  resolved session capabilities on task launch and context reset.
- **AC-AGENTS-MCP-DISCOVERY-002.4:** Recorded and dispatched task instructions
  shall agree about discovery and optional capabilities.
- **AC-AGENTS-MCP-DISCOVERY-002.5:** Shorter instructions shall retain the use
  of Kandev plans and native rich output when the user requests those results.

## Out of scope

- New MCP tools, client search implementations, or tool registration changes.
- Exhaustive catalog redesign for Office or configuration sessions.
- Changes to canvas permissions, promotion, or package validation.
- Automatic repair of existing task files or conversations.

Canvas-specific outcomes remain in the
[canvas requirements](../../canvases/requirements/agent-authored-web-apps.md).
