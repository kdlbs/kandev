---
status: approved
created: 2026-07-30
owner: Kandev
---

# Session MCP Attachment Observability

Decision:
[ADR-2026-07-30-session-owned-mcp-observability](../../decisions/2026-07-30-session-owned-mcp-observability.md)

## Why

The chat toolbar currently derives its MCP list from agent-profile
configuration. That answers what Kandev intended to expose, but not what the
specific agent execution received, connected to, or loaded. A user can
therefore see `kandev` in the toolbar while the agent has no Kandev tools.

This is especially difficult to diagnose in release mode. Raw ACP frame
logging is intentionally disabled because frames can contain prompts, files,
tool arguments, credentials, and other sensitive data. Backend logs alone also
cannot prove a failed outbound connection: if the agent never reaches an MCP
server, that server observes no request.

## Evidence model

Kandev reports the strongest evidence it has for each MCP server without
turning absence into failure:

| Evidence | Meaning |
|---|---|
| Configured | The server exists in the selected profile or is Kandev's built-in task server. |
| Filtered | Kandev deliberately omitted the server because the agent, executor policy, transport, or passthrough strategy could not expose it. |
| Delivered | The server was included in ACP `session/new`, `session/load`, or `session/reset`, or was materialized into a passthrough CLI's effective config. |
| Connected | Kandev's in-session MCP endpoint observed that client connection initialize successfully. |
| Tools loaded | Kandev's in-session endpoint served `tools/list` successfully to that connection. |
| Used | Kandev's in-session endpoint observed at least one tool call on that connection. |
| Failed | Kandev received an explicit server-specific attachment error. |

ACP does not standardize a response that lists connected MCP servers, and
third-party profile MCP servers normally connect directly to the agent. Those
servers therefore remain **Delivered · connection unverified** unless the
agent exposes a specific error or Kandev later gains an observable proxy or
provider status contract.

`Connected` and `Tools loaded` are observable automatically for Kandev's
built-in task server because agentctl hosts that endpoint. They are not inferred
from profile configuration, agent capability flags, a successful ACP
`session/new`, or a separate endpoint test.

## Session, execution, and attempt ownership

- Every attachment report belongs to one Kandev task session and one agent
  execution generation. Within that execution, every `session/new`,
  `session/load`, `session/reset`, or passthrough process start creates a new
  backend-owned attachment attempt ID.
- The report carries the backend-owned `task_id`, `session_id`, `execution_id`,
  `attachment_attempt_id`, `agent_id`, and `agent_profile_id`. It also records
  the provider's `acp_session_id` when available.
- Each observed MCP transport client receives a connection ID. Connection
  evidence is attributed to the agentctl instance's backend-owned task and
  session identity, never to IDs supplied by the agent.
- Multiple agents inside one task remain distinct because they have distinct
  Kandev session IDs. Restarting one session creates a new execution report;
  evidence from the superseded execution cannot keep the current execution
  green. Resetting or loading inside one execution creates a new attachment
  attempt with the same protection.
- If an agent internally runs subagents over one shared MCP connection, Kandev
  reports that shared connection. It does not claim per-subagent attribution
  unless the agent opens separately observable connections and provides a
  trustworthy identity contract.

## Release-safe attachment report

Release mode always retains a small structured report for the current
attachment attempt and the two immediately previous attempts of a session.
Each attempt timeline is bounded and can contain these events:

- server resolved;
- server filtered, with a stable reason code;
- server delivered;
- agent session accepted;
- MCP initialize observed;
- tools list observed, including tool count;
- tool call observed, without tool arguments or result;
- explicit attachment error;
- connection closed;
- attachment attempt superseded.

The durable report excludes prompts, files, tool arguments and results, header
values, environment values, credentials, full sensitive URLs, and raw ACP
frames. A network target is reduced to scheme and host with optional port; it
contains no user info, path, query, or fragment. A stdio target contains only
the executable basename, never arguments or environment. Error details use a
stable reason code plus a bounded sanitized summary.

Raw ACP JSONL logging remains a development-only diagnostic and is not enabled
by this feature.

## User experience

The MCP toolbar icon remains neutral by default. It does not become a row of
colored indicators.

On precise-pointer desktop:

- hover or keyboard focus opens a compact MCP status popover;
- clicking the trigger pins the popover so diagnostic actions can be used;
- each server row shows its own status color and plain-language label.

On touch and coarse-pointer devices:

- tapping a minimum 44px toolbar target opens an inset, safe-area-aware bottom
  drawer;
- the drawer uses the same server rows, evidence checklist, and actions as the
  desktop popover;
- no capability is hidden behind hover.

The list uses these display states:

- **Active** in green when `tools/list` succeeded for the current execution;
- **Connected** in amber when initialize succeeded but tools have not been
  listed;
- **Delivered · connection unverified** in amber when configuration reached
  the agent but Kandev has no connection observation;
- **Failed** in red only for an explicit server-specific attachment error;
- **Filtered** or **Unavailable** in gray with the reason.

An unscoped ACP session error appears as a report-level diagnostic and does not
incorrectly mark every delivered server as failed. Tool use is shown as detail
under an Active server rather than as another toolbar color.

The popover or drawer provides:

- a per-server attachment checklist with timestamps for the current execution;
- **Test endpoint**, which performs a bounded initialize and `tools/list` from
  the same executor using the stored effective server configuration;
- **View recent agent output**, fetched on demand and limited to a bounded
  excerpt with a warning that agent output may contain sensitive data;
- **Copy sanitized diagnostics**, which never includes the on-demand agent
  output;
- **Open MCP settings**;
- the existing reset/restart recovery only where supported, with its existing
  confirmation and clear context-loss consequences.

An endpoint test reports reachability separately from attachment evidence. A
successful test does not turn an unverified agent attachment into Active, and a
failed test is labelled as a test failure rather than proof of what happened
earlier inside the agent.

## Developer probe

`acpdbg` can inject a built-in sentinel MCP server into a real ACP
`session/new` request and report whether the agent initialized it, listed its
tools, and optionally called its sentinel tool. The probe uses the same
registered agent command, environment handling, and effective working
directory as the ordinary ACP probe. Its JSONL retains raw frames only because
the developer explicitly invoked the debug tool.

The probe also reports the agent's advertised MCP transport capabilities, but
capabilities are not treated as connection evidence.

## Failure modes

- If `session/new`, `session/load`, or passthrough materialization fails before
  delivery, Kandev records the explicit sanitized failure and keeps the server
  out of Active state.
- If the agent accepts the session but never reaches Kandev's MCP endpoint, the
  report remains Delivered or connection unverified. It does not invent a
  timeout failure.
- If an MCP connection closes, the current connection is marked disconnected;
  a later connection receives a new connection ID and can restore current
  evidence.
- If persistence fails, live status may still update, the failure is logged,
  and the agent session continues unchanged.
- If the user tests a server that is no longer part of the session's effective
  configuration, Kandev rejects the request rather than testing an arbitrary
  URL or command.
- If recent agent output is unavailable because the execution ended or the
  stream disconnected, the UI explains that limitation without changing MCP
  attachment status.

## Scenarios

- **GIVEN** two agents are running inside the same task, **WHEN** only one
  reaches `tools/list` on its Kandev MCP endpoint, **THEN** only that session's
  toolbar lists Kandev as Active.
- **GIVEN** a session is restarted or reset after an Active connection,
  **WHEN** the new attachment attempt has not contacted MCP, **THEN** the
  toolbar shows the new attempt as Delivered or unverified and retains the
  previous evidence only as historical diagnostics.
- **GIVEN** an ACP agent accepts `session/new` with a third-party MCP server,
  **WHEN** Kandev cannot observe the direct connection, **THEN** the row says
  Delivered · connection unverified instead of Active or Failed.
- **GIVEN** Kandev's in-session MCP endpoint receives initialize and
  `tools/list`, **WHEN** the status surface is opened, **THEN** the Kandev row
  is green, shows the tool count, and identifies the current execution and
  connection.
- **GIVEN** an agent reports a server-specific connection refusal, **WHEN** the
  status surface is opened, **THEN** that server is red with a sanitized
  reason and the other servers keep their own evidence states.
- **GIVEN** a release-mode user has an unverified attachment, **WHEN** they run
  Test endpoint and copy diagnostics, **THEN** the test runs from the session's
  executor, its result is distinguished from agent attachment, and the copied
  report contains no secrets or raw agent output.
- **GIVEN** a precise-pointer user, **WHEN** they hover or focus the neutral MCP
  trigger, **THEN** a compact status list appears and can be pinned by click.
- **GIVEN** a phone or coarse-pointer user, **WHEN** they tap the MCP trigger,
  **THEN** the same status and diagnostic actions appear in a bottom drawer
  without horizontal overflow.
- **GIVEN** Auggie or another ACP agent is under investigation, **WHEN** a
  developer runs the sentinel MCP probe, **THEN** the JSONL and summary
  distinguish advertised capability, configuration delivery, initialize,
  tools list, and tool use.

## Out of scope

- A new ACP extension that requires every agent vendor to return connected MCP
  server status.
- Claiming automatic connection status for direct third-party MCP servers that
  Kandev cannot observe.
- Enabling raw ACP frame logs or persistent raw stderr in release mode.
- Persisting prompts, tool arguments, credentials, header values, environment
  values, or full endpoint URLs in attachment diagnostics.
- Attributing a shared MCP connection to opaque internal subagents.
- Automatically restarting an agent, resetting context, or changing session
  state because attachment evidence is absent.
