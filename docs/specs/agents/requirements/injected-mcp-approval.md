---
status: active
system: agents
created: 2026-09-16
owners:
  - kandev
---

# Injected MCP approval requirements

## Overview

Claude ACP sessions require repeated approval for Kandev task and plan tools.
Codex already receives automatic approval for the injected Kandev server.
The agent system owns this provider permission contract.

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
- **AC-AGENTS-INJECTED-MCP-APPROVAL-001.6:** Without an offered allow choice, the request shall retain its normal permission behavior. A request without choices shall cancel.
- **AC-AGENTS-INJECTED-MCP-APPROVAL-001.7:** Fresh and resumed task sessions, including Quick Chat, shall use the same policy without a saved approval decision.
- **AC-AGENTS-INJECTED-MCP-APPROVAL-001.8:** Existing Codex approval and blanket approval behavior shall remain unchanged. Unknown provider naming formats shall retain normal permission behavior.

## Out of scope

- Selective tool lists, persisted approval decisions, and new profile or workspace settings.
- Claude terminal passthrough, which does not use ACP permission frames.
- Automatic support claims for unverified provider naming formats.
- Changes to question barriers, workflow gates, or server-side task authorization.

## Implementation plans

- [Issue 3729 fix package](../../../plans/claude-injected-mcp-approval/plan.md)
