---
status: draft
system: agents
created: 2026-09-28
owners:
  - kandev
---

# Agent MCP preparation requirements

## Overview

Agents owns profile-backed selection and preparation of agent-native MCP servers.
Task workspace preparation exposes the progress, while agent adapters implement
native discovery, approval and authentication. Cursor is the first supported
adapter; other agents retain their current behavior until explicitly supported.
This capability extends the Cursor import and credential bridge contracts; it
owns profile selection, automatic server approval, readiness and recovery.

## Requirements

### REQ-AGENTS-MCP-PREP-001: Profile selection

A user can choose which discovered MCP servers Kandev prepares for new task
launches without hand-editing agent configuration or running terminal commands.

- **AC-AGENTS-MCP-PREP-001.1:** Supported profiles shall offer inherit-enabled and selected-only modes. Existing profiles default to inherit-enabled; selected-only with an empty selection imports none. Changing unrelated settings shall preserve the choice and selected identities across save, reload, duplication and launch.
- **AC-AGENTS-MCP-PREP-001.2:** Discovery shall show individual server identities grouped by plugin where applicable, their source and whether reusable credentials are available, without exposing credential values, commands, environment values or private endpoint details. Refresh failure shall preserve the saved selection and distinguish unavailable discovery from an empty inventory.
- **AC-AGENTS-MCP-PREP-001.3:** Profile selection shall only narrow eligible imports. Agent enablement, executor policy and disables in the task's primary source repository and task workspace shall still apply. Unrelated workspaces shall not supply disables. Explicit user-owned project/profile configuration remains governed by its existing contract.
- **AC-AGENTS-MCP-PREP-001.4:** Import and reuse-existing-credentials preferences shall remain independent. Profiles without a compatible preparation adapter shall not advertise unsupported controls or perform host discovery.
- **AC-AGENTS-MCP-PREP-001.5:** Desktop and phone users shall be able to refresh, select, save and discard the same settings. Phone controls shall have at least 44px touch targets, visible labels, one page scroll owner and no document horizontal overflow. Pending, empty and failed discovery shall have localized accessible feedback; late responses shall not overwrite edits or a different profile's state.

### REQ-AGENTS-MCP-PREP-002: Seamless launch preparation

The import preference authorizes preparing the selected imported server
connections. Server approval is separate from permission to invoke tools.

- **AC-AGENTS-MCP-PREP-002.1:** Eligible local tasks with working reusable credentials shall load their selected MCP tools in ACP and terminal modes without manual server enable/login commands. Preparation shall finish before the conversational agent starts and shall create no chat turns.
- **AC-AGENTS-MCP-PREP-002.2:** Automatic approval shall cover only the final eligible Kandev-imported definitions after selection, policy, precedence and disable filtering. It shall not approve unrelated user-owned servers, re-enable disabled servers or bypass individual tool permissions.
- **AC-AGENTS-MCP-PREP-002.3:** The existing workspace preparation surface shall expose agent discovery, selection/configuration, credential reuse, server approval and connection verification with running/completed/skipped/failed outcomes. Results shall survive reload, append to existing environment steps and not report overall success before the agent preparation finishes.
- **AC-AGENTS-MCP-PREP-002.4:** Readiness shall require a native connection/tool-discovery check rather than configuration or token presence alone. Verification shall not invoke business tools. Authentication-required, approval-failed, unavailable and connection-failed outcomes shall remain distinct; raw process output or secrets shall not enter chat, logs or persisted preparation output.
- **AC-AGENTS-MCP-PREP-002.5:** Fresh launches, resumes and workspace promotion shall apply the same selected profile and trusted source context. Remote/container/unknown executors and different runtime homes shall remain isolated. Cancellation, stale concurrent preparation and unsafe filesystem changes shall not approve or publish a successor's or unrelated definition.

### REQ-AGENTS-MCP-PREP-003: Authentication recovery

Users complete genuinely required provider consent through Kandev rather than
being instructed to construct and run commands themselves. The initial Cursor
adapter supports native login terminals on POSIX hosts and automatic
conversation-preserving recovery for ACP sessions. Terminal-mode sessions report
automatic reload as unavailable until their exact native chat identity can be
retained; native Windows login terminals are outside this implementation.

- **AC-AGENTS-MCP-PREP-003.1:** When an eligible selected server requires authentication, task preparation shall expose an Authenticate action with the server identity. The action shall invoke the provider-native login flow in the correct task context without an agent prompt or blanket approval.
- **AC-AGENTS-MCP-PREP-003.2:** Users shall receive the native browser/terminal login flow, and shall be able to retry preparation and resume the same conversation through Kandev after completing consent. Recovery must not replay a completed user request or create a new task/conversation.
- **AC-AGENTS-MCP-PREP-003.3:** Recovery shall validate current session, runtime eligibility and selected server identity; unsupported or stale requests shall fail without launching arbitrary commands. Authentication actions shall be available on desktop and phone, with explicit busy/failure feedback.
- **AC-AGENTS-MCP-PREP-003.4:** A successful native refresh/login shall not be overwritten by older discovered credential data on the next preparation. Removed source credentials shall not be resurrected indefinitely from an unmanaged stale snapshot.

### Setup warning amendment (2026-10-02, draft)

- **AC-AGENTS-MCP-PREP-002.6:** When imported plugin connection verification requires authentication but environment preparation succeeds, the preparation summary, server row, and authentication message shall show a warning rather than an error. The server shall remain explicitly unavailable until authentication and verification succeed; the agent can continue without those tools.
- **AC-AGENTS-MCP-PREP-002.7:** When an authentication warning coexists with a genuine preparation failure, the failure shall retain error priority. Live updates and persisted preparation results shall produce the same severity, including older authentication-required rows recorded as failed.
- **AC-AGENTS-MCP-PREP-003.5:** Authentication warnings shall retain Authenticate and Retry connection actions on desktop and phone. Successful verification shall clear the current warning without erasing other servers' failures or warnings.

Delivery: [Setup recovery UX](../../../plans/setup-recovery-ux/plan.md).

## Out of scope

Implementing adapters for other agents, remote credential transfer, plugin
installation, account merging, per-server account picking, blanket tool approval,
provider token refresh implemented by Kandev, and executing real provider
business operations during readiness checks are excluded.

## Implementation plans

- [Agent MCP preparation](../../../plans/agent-mcp-preparation/plan.md)
