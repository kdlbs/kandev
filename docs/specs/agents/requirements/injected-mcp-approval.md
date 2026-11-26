---
status: active
system: agents
created: 2026-09-16
owners:
  - kandev
---

# Injected MCP approval requirements

## Overview

Users can run tools from the injected Kandev MCP server with automatic approval.
Claude and Antigravity use provider-specific ACP tool names.
The agent system owns the translation from those names to the approval policy.

This package adopts the minimum server-wide parity option in
[issue #3729](https://github.com/kdlbs/kandev/issues/3729).
It does not adopt the issue's separate proposal for selective approval and a new setting.

## Requirements

### REQ-AGENTS-INJECTED-MCP-APPROVAL-001: Approval for the injected Kandev server

**Intent:** Users can use Kandev tools through Claude ACP without enabling blanket approval for the agent.

#### Acceptance criteria

- **AC-AGENTS-INJECTED-MCP-APPROVAL-001.1:** With blanket approval disabled, identifiable calls to the injected Kandev server shall proceed without permission prompts in Claude ACP sessions.
- **AC-AGENTS-INJECTED-MCP-APPROVAL-001.2:** This policy shall apply to every tool on that server, including task creation and deletion. Tool authorization shall remain enforced.
- **AC-AGENTS-INJECTED-MCP-APPROVAL-001.3:** Shell commands, file operations, third-party tools, and ambiguous requests shall retain their existing permission behavior.
- **AC-AGENTS-INJECTED-MCP-APPROVAL-001.4:** A user-configured server with a similar or identical name shall not receive approval through this policy.
- **AC-AGENTS-INJECTED-MCP-APPROVAL-001.5:** Automatic approval shall select an offered allow-once choice. If none exists, it shall use an offered allow-always choice.
- **AC-AGENTS-INJECTED-MCP-APPROVAL-001.6:** Without an offered allow choice, the request shall follow the ordinary permission policy.
- **AC-AGENTS-INJECTED-MCP-APPROVAL-001.7:** Fresh and resumed task sessions, including Quick Chat, shall use the same policy without a saved approval decision.
- **AC-AGENTS-INJECTED-MCP-APPROVAL-001.8:** Existing Codex approval and blanket approval behavior shall remain unchanged. Unknown provider naming formats shall retain normal permission behavior.
- **AC-AGENTS-INJECTED-MCP-APPROVAL-001.9:** With blanket approval disabled, identifiable calls to the injected Kandev Core server shall proceed without permission prompts in supported Antigravity CLI ACP sessions.
  A tool identity that could belong to another server shall follow the ordinary permission policy.
- **AC-AGENTS-INJECTED-MCP-APPROVAL-001.10:** If the Antigravity CLI's global MCP source, workspace customization roots through the nearest repository root, or plugin MCP sources cannot be inventoried, the request shall follow the ordinary permission policy. A global or effective workspace `plugins.json` discovery manifest shall make the inventory incomplete until its external plugin registrations can be verified. A source that can produce the same flattened server alias shall also follow the ordinary permission policy.

## Out of scope

- Selective tool lists, persisted approval decisions, and new profile or workspace settings.
- Claude terminal passthrough, which does not use ACP permission frames.
- Automatic support claims for unverified provider naming formats.
- Changes to question barriers, workflow gates, or server-side task authorization.

## Implementation plans

- [Issue 3729 fix package](../../../plans/claude-injected-mcp-approval/plan.md)
- [Antigravity approval and logo fix](../../../plans/antigravity-approval-and-logo/plan.md)
