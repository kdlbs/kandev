---
status: active
system: platform
created: 2026-07-30
updated: 2026-10-06
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
- **AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.4:** **GIVEN** Kandev's in-session MCP endpoint accepts the client's protocol and serves `tools/list`, **WHEN** the status surface is opened, **THEN** the Kandev row is green, shows the tool count, and identifies the current execution and attachment attempt.
- **AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.5:** **GIVEN** an agent reports a server-specific connection refusal, **WHEN** the status surface is opened, **THEN** that server is red with a sanitized reason and the other servers keep their own evidence states.
- **AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.6:** **GIVEN** a release-mode user has an unverified attachment, **WHEN** they run Test endpoint and copy diagnostics, **THEN** the test runs from the session's executor, its result is distinguished from agent attachment, and the copied report contains no secrets or raw agent output.
- **AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.7:** **GIVEN** an Active Kandev server, **WHEN** a precise-pointer user hovers or focuses the MCP trigger, **THEN** the tooltip shows a green Kandev row and its Active status.
- **AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.8:** **GIVEN** a precise-pointer user, **WHEN** they click the MCP trigger, **THEN** a wide dialog lists the active session's MCP servers.
- **AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.9:** **GIVEN** a modern stateless MCP request, **WHEN** Kandev records protocol evidence, **THEN** the evidence belongs to the current attachment attempt without a fabricated connection ID or a connection-closed event at the end of the HTTP request.
- **AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.10:** **GIVEN** a session list contains valid version-1 attachment history, **WHEN** the client loads the task, **THEN** the status surface restores it without a live event.
- **AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.11:** **GIVEN** the client holds newer live evidence, **WHEN** an equal or older session snapshot arrives, **THEN** the status surface retains the live evidence.
- **AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.12:** **GIVEN** session-list attachment history is malformed or unsupported, **WHEN** the client loads the task, **THEN** the client ignores it and retains other session statuses.
- **AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.13:** When a server is excluded from delivery, its hover/focus summary and explorer detail shall say that it is not available to this agent and explain the recorded transport, executor-policy, or command-resolution reason in the active locale. A stable reason code shall not be the only explanation.
- **AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.14:** When a saved session selection is pending, deferred, or failed, the plug summary shall separate saved changes from current agent evidence, state whether application is waiting, requires the next start, or failed, and explain that the rows describe the current agent setup. Saved or applied configuration shall not imply successful server connections. An applied selection shall not display a pending-change notice.
- **AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.15:** A phone or coarse-pointer user shall receive the same saved-change explanation and server-specific exclusion reasons through the existing plug drawer without hover. Reasons shall wrap within the surface, remain keyboard/screen-reader accessible, and preserve the existing navigation and one scroll owner.
- **AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.16:** When connection observation is absent, the summary shall distinguish delivered configuration from verified tools and use an explanatory fallback for an unknown exclusion reason. Delivered configuration and unobserved connections shall use neutral styling independently of saved-selection application state. A configured name without current server-specific evidence shall remain unknown, including beside observed servers. Reasons from a superseded or weaker evidence state shall not contradict the current stronger state. An unreachable endpoint or missing executable shall not be claimed as an observed agent failure merely because it is configured, and a pending notice shall not invent a per-server change count without an authoritative comparison.
- **AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.17:** The customization application summary shall use a distinct icon, restrained semantic color, and plain-language headline for up-to-date configuration, waiting changes, next-start changes, and application failure. Its supporting sentence shall explain what the current agent keeps and what happens next. Color alone shall not convey state, an applied configuration shall not imply verified tools, and a pending summary shall not claim an active turn exists without current activity evidence.

## System design

The migrated technical source is split into [part 1](../system-design/mcp-session-observability.md).
