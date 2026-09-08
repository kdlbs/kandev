---
status: active
system: tasks
created: 2026-08-26
owners:
  - kandev
---

# Quick Chat Agent Titles Requirements

## Overview

An ordinary Quick Chat starts before the user sends a request. Its provisional label identifies the
agent, but it does not describe the conversation. The existing task-title agent can give each chat a
useful name without another generation request.

The task system owns this behavior because each Quick Chat tab uses a task title. The UI shows task
updates but does not own title state or MCP authority.

## Requirements

### REQ-TASKS-QUICK-CHAT-AGENT-TITLES-001: Quick Chat Agent Titles

**Intent:** An ordinary Quick Chat agent gives its tab a concise title that describes the user's first request.

**User story:** As a Quick Chat user, I want the agent to name the chat, so that I can find the conversation again.

#### Acceptance criteria

- **AC-TASKS-QUICK-CHAT-AGENT-TITLES-001.1:** When agent-generated task titles are enabled, a new ordinary Quick Chat starts with a usable provisional tab title.
- **AC-TASKS-QUICK-CHAT-AGENT-TITLES-001.2:** Before the owner agent processes the first user request, its Kandev context instructs it to set a concise task title first. Its MCP catalog contains `set_task_title_kandev`.
- **AC-TASKS-QUICK-CHAT-AGENT-TITLES-001.3:** When the owner agent sets a valid title, the existing Quick Chat tab shows that title without a reload or a new session.
- **AC-TASKS-QUICK-CHAT-AGENT-TITLES-001.4:** When the user disables agent-generated task titles, a new Quick Chat keeps its provisional title. Its agent receives no title instruction or title tool.
- **AC-TASKS-QUICK-CHAT-AGENT-TITLES-001.5:** When the user renames a pending Quick Chat first, the user title remains authoritative. A later agent title call cannot replace it.
- **AC-TASKS-QUICK-CHAT-AGENT-TITLES-001.6:** When the owner agent ignores the instruction or a title call fails, the provisional title remains usable after reload. Another session does not inherit title ownership.
- **AC-TASKS-QUICK-CHAT-AGENT-TITLES-001.7:** Structured and passthrough Quick Chat agents receive the title instruction through their existing first-turn context treatment. Configuration Chat and Quick Terminal do not receive this capability.

## Out of scope

- Agent-generated titles for Configuration Chat or Quick Terminal.
- A separate title-generation request or agent.
- Changes to normal task-title validation, ownership reassignment, or branch rules.
- Removal of manual Quick Chat rename controls.

## System design

[Quick Chat Agent Titles](../system-design/quick-chat-agent-titles.md)
