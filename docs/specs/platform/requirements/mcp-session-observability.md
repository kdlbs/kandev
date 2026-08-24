---
status: active
system: platform
created: 2026-07-30
updated: 2026-08-18
owners:
  - Kandev
---
# Session MCP Attachment Observability Requirements

## Overview

The chat toolbar currently derives its MCP list from agent-profile configuration. That answers what Kandev intended to expose, but not what the specific agent execution received, connected to, or loaded. A user can therefore see `kandev` in the toolbar while the agent has no Kandev tools.

## Requirements

### REQ-PLATFORM-MCP-SESSION-OBSERVABILITY-001: Session MCP Attachment Observability

**Intent:** The chat toolbar currently derives its MCP list from agent-profile configuration. That answers what Kandev intended to expose, but not what the specific agent execution received, connected to, or loaded. A user can therefore see `kandev` in the toolbar while the agent has no Kandev tools.

#### Acceptance criteria

- **AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.1:** **GIVEN** two agents are running inside the same task, **WHEN** only one reaches `tools/list` on its Kandev MCP endpoint, **THEN** only that session's toolbar lists Kandev as Active.
- **AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.2:** **GIVEN** a session is restarted or reset after an Active connection, **WHEN** the new attachment attempt has not contacted MCP, **THEN** the toolbar shows the new attempt as Delivered or unverified and retains the previous evidence only as historical diagnostics.
- **AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.3:** **GIVEN** an ACP agent accepts `session/new` with a third-party MCP server, **WHEN** Kandev cannot observe the direct connection, **THEN** the row says Delivered · connection unverified instead of Active or Failed.
- **AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.4:** **GIVEN** Kandev's in-session MCP endpoint receives initialize and `tools/list`, **WHEN** the status surface is opened, **THEN** the Kandev row is green, shows the tool count, and identifies the current execution and connection.
- **AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.5:** **GIVEN** an agent reports a server-specific connection refusal, **WHEN** the status surface is opened, **THEN** that server is red with a sanitized reason and the other servers keep their own evidence states.
- **AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.6:** **GIVEN** a release-mode user has an unverified attachment, **WHEN** they run Test endpoint and copy diagnostics, **THEN** the test runs from the session's executor, its result is distinguished from agent attachment, and the copied report contains no secrets or raw agent output.
- **AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.7:** **GIVEN** an Active Kandev server, **WHEN** a precise-pointer user hovers or focuses the MCP trigger, **THEN** the tooltip shows a green Kandev row and its Active status.
- **AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.8:** **GIVEN** a precise-pointer user, **WHEN** they click the MCP trigger, **THEN** a wide dialog lists the active session's MCP servers.

## System design

The migrated technical source is split into [part 1](../system-design/mcp-session-observability.md).
