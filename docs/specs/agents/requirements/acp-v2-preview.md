---
status: active
system: agents
created: 2026-11-23
owners:
  - kandev
---

# ACP v2 preview requirements

## Overview

The Agent system owns the contract between Kandev and ACP agents.
An operator can try ACP v2 Preview with qualified agents in local Kandev
sessions and switch back to v1.

## Requirements

### REQ-AGENTS-ACP-V2-001: Opt in to ACP v2

- **AC-AGENTS-ACP-V2-001.1:** The ACP v2 preview toggle starts off in all shipped profiles. An admin can enable or disable it in Feature Toggles. A restart applies the change to subsequent agent launches.
- **AC-AGENTS-ACP-V2-001.2:** With the toggle on, supported local Claude ACP and Codex ACP sessions can use v2. The launch uses v1 when the toggle is off or the agent or executor falls outside the pilot.
- **AC-AGENTS-ACP-V2-001.3:** If a candidate selects v1 or returns a recognizable legacy initialization response, Kandev opens a fresh v1 connection. An ambiguous response displays a launch error. Kandev attempts this fallback before session operations.

### REQ-AGENTS-ACP-V2-002: Use a preview session in Kandev

- **AC-AGENTS-ACP-V2-002.1:** A user can start a qualified session, choose an advertised model or mode, and send a prompt. Chat shows the answer and finishes the turn when the agent reports the foreground outcome.
- **AC-AGENTS-ACP-V2-002.2:** A user can cancel the foreground turn and answer supported permission requests and questions through Kandev. Replies go only to the originating request and follow the session's existing approval policy.
- **AC-AGENTS-ACP-V2-002.3:** When the agent accepts a prompt and later reports a failure, Kandev shows the failure on that turn. If acceptance is uncertain, Kandev reports that state and avoids an automatic duplicate submission.
- **AC-AGENTS-ACP-V2-002.4:** Kandev attaches configured MCP servers that use an advertised transport. The session reports attachment evidence through the existing MCP diagnostics.

### REQ-AGENTS-ACP-V2-003: Restore the same conversation

- **AC-AGENTS-ACP-V2-003.1:** After reconnecting or restarting an agent, Kandev restores the saved native conversation and avoids duplicate messages and turn completion. If restoration fails, Kandev reports the failure and preserves the saved conversation ID.
- **AC-AGENTS-ACP-V2-003.2:** When the toggle is off, a new connection uses v1 to restore an eligible saved conversation. Kandev reports a restore error if the agent cannot open that conversation over v1.
- **AC-AGENTS-ACP-V2-003.3:** If an eligible live v2 process survives a restart that turns the toggle off, Kandev keeps its history, pending interactions, and stop controls available. It holds new prompts until the user reconnects through v1.

### REQ-AGENTS-ACP-V2-004: Verify the wire version

- **AC-AGENTS-ACP-V2-004.1:** A developer can select v1 or v2 in `acpdbg` and see the requested version, selected version, and the result of session setup in the capture.
- **AC-AGENTS-ACP-V2-004.2:** A v2 capture can show a prompt acknowledgement, foreground completion, and the replay boundary for a saved conversation. Agentctl records its selected version and any compatibility fallback without credentials or prompt content.

## Pilot scope

The pilot covers structured chat on the local process executor for qualified
Claude ACP and Codex ACP launches with an existing local login or API key.
Follow-up milestones can expand qualification to remote and container executors,
provider gateways, other agents, and new controls for individual background items.

## Implementation plans

[ACP v2 preview plan](../../../plans/acp-v2-preview/plan.md)
