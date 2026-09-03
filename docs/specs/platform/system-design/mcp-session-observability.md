---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-MCP-SESSION-OBSERVABILITY-001
created: 2026-07-30
updated: 2026-08-30
owners:
  - Kandev
---
# Session MCP Attachment Observability System Design

## Purpose and boundaries

This design preserves the technical source detail for `REQ-PLATFORM-MCP-SESSION-OBSERVABILITY-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-MCP-SESSION-OBSERVABILITY-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

Decision:
[ADR-2026-07-30-session-owned-mcp-observability](../../../decisions/2026-07-30-session-owned-mcp-observability.md)

Tool definition decision:
[ADR-2026-08-18-session-mcp-tool-definition-details](../../../decisions/2026-08-18-session-mcp-tool-definition-details.md)

Implementation plans:

- [Session MCP attachment observability](../../../plans/mcp-session-observability/plan.md)
- [MCP server explorer](../../../plans/mcp-server-explorer/plan.md)

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

| Evidence     | Meaning                                                                                                                                          |
| ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| Configured   | The server exists in the selected profile or is Kandev's built-in task server.                                                                   |
| Filtered     | Kandev deliberately omitted the server because the agent, executor policy, transport, or passthrough strategy could not expose it.               |
| Delivered    | The server was included in ACP `session/new`, `session/load`, or `session/reset`, or was materialized into a passthrough CLI's effective config. |
| Connected    | Kandev's in-session MCP endpoint accepted the client's protocol. For a legacy client, initialize supplies this evidence.                         |
| Tools loaded | Kandev's in-session endpoint served `tools/list` successfully to that connection.                                                                |
| Used         | Kandev's in-session endpoint observed at least one tool call on that connection.                                                                 |
| Failed       | Kandev received an explicit server-specific attachment error.                                                                                    |

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
- Each observed legacy MCP transport client receives a connection ID.
  Connection evidence is attributed to the agentctl instance's backend-owned
  task and session identity, never to IDs supplied by the agent.
- Modern MCP requests are stateless. Their protocol, tool-list, and tool-call
  evidence belongs to the current attachment attempt. Kandev does not invent a
  connection ID and does not treat the end of an HTTP request as a disconnect.
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
- MCP protocol accepted, including legacy initialize where applicable;
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

## Kandev tool catalog

After Kandev serves `tools/list`, the current attachment report includes a
safe catalog for the built-in `kandev` server. Each entry contains the tool
name, description, input schema, and token estimate from that response.

The catalog has these limits:

- it contains at most 128 tools, sorted by name.
- each description contains at most 1,024 UTF-8 bytes.
- each stored input schema contains at most 64 KiB.
- all stored input schemas contain at most 512 KiB in one catalog.
- `tool_count` remains the total count from `tools/list`.
- a truncation marker tells the UI when the total count exceeds the stored
  catalog.
- a schema truncation marker tells the UI when one input schema is unavailable.
- superseded attempts keep the total count but do not keep catalog entries.

The catalog excludes output schemas, annotations, `_meta`, invocation
arguments, results, prompts, credentials, and endpoint configuration.
Descriptions and schemas render as plain text or structured data, never HTML.

Kandev calculates each token estimate from the compact JSON for the complete
MCP tool definition. The estimate uses `o200k_base` and the method identifier
`o200k_base:mcp-tool-json-v1`. It excludes the enclosing response and any
provider-specific wrapper.

The UI labels this value as `~N tokens`. It explains that the selected agent
can use a different tokenizer or tool-loading format. Kandev does not show a
character-based fallback when tokenization is unavailable.

Kandev does not collect a catalog for a third-party profile server. Those
servers connect directly to the agent, so Kandev cannot observe their
`tools/list` result. The UI explains this limit and continues to show the safe
status metadata that Kandev owns.

## User experience

The MCP toolbar icon remains one compact neutral control. The tooltip and
explorer use status colors for individual servers.

On precise-pointer desktop:

- hover or keyboard focus shows the current server names and status colors.
- the tooltip includes the green Kandev row when Kandev is Active.
- clicking the trigger opens a wide MCP server dialog.
- the left pane lists servers for the active Kandev session.
- the right pane gives most of its height to the selected server's tool list.
- server metadata appears in a compact summary with optional connection details.
- selecting `kandev` shows a scrollable list of enabled tool names and token
  estimates after `tools/list` succeeds.
- selecting a tool opens a focused tool page with its description and input
  schema.
- a visible Back control returns to the same tool-list position and focus.
- selecting a third-party server explains why its tool catalog is unavailable.
- the dialog has one close control.

On touch and coarse-pointer devices:

- tapping a minimum 44px toolbar target opens a safe-area-aware drawer.
- phones use a full-height drawer with one internal scroll area.
- tablets use a bounded drawer with the same server and tool data.
- the first view lists servers, and a server tap opens its tool list.
- a tool tap opens its focused tool page.
- Back returns from tool to tools, then from tools to servers.
- no capability is hidden behind hover.

The dialog and drawer select `kandev` first when it is present. Otherwise, they
select the first server. If live status removes the selected server, the
surface selects the same deterministic fallback.

Before Kandev observes `tools/list`, its detail view explains that the catalog
is not loaded. If the stored catalog is truncated, the list states how many
tools are shown and the total tool count.

The explorer header and server summary do not scroll. The active page body is
the only vertical scroll owner. The tool list and tool page use the available
height without increasing the dialog beyond the viewport.

The tool page presents common object properties as argument rows. Each row can
show its name, type, requirement state, and description. The page also provides
a plain JSON view for nested or composed JSON Schema structures.

If a schema exceeds a storage limit, the tool page states that the schema is
too large to show. The page still shows the description and token estimate.

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
- **GIVEN** an Active Kandev server, **WHEN** a precise-pointer user hovers or
  focuses the MCP trigger, **THEN** the tooltip shows a green Kandev row and its
  Active status.
- **GIVEN** a precise-pointer user, **WHEN** they click the MCP trigger,
  **THEN** a wide dialog lists the active session's MCP servers.
- **GIVEN** Kandev served `tools/list` for the current attempt, **WHEN** the user
  selects `kandev`, **THEN** the detail pane lists enabled tool names and token
  estimates in a scrollable list.
- **GIVEN** a loaded Kandev tool, **WHEN** the user selects that tool, **THEN**
  a focused page shows its description and input schema.
- **GIVEN** a user returns from a tool page, **WHEN** the tool list appears,
  **THEN** the list restores its prior scroll position and focus.
- **GIVEN** a loaded Kandev tool, **WHEN** the explorer shows its token size,
  **THEN** the value uses the recorded estimate method and has an estimate label.
- **GIVEN** the shared dialog adds a close control, **WHEN** the explorer opens,
  **THEN** only one close control is visible and available to assistive technology.
- **GIVEN** the tool list exceeds the available height, **WHEN** the user
  scrolls, **THEN** the list scrolls inside the explorer and the header remains
  visible.
- **GIVEN** Kandev has not served `tools/list` for the current attempt, **WHEN**
  the user selects `kandev`, **THEN** the detail pane says that the tool catalog
  is not loaded.
- **GIVEN** a third-party server is present, **WHEN** the user selects it,
  **THEN** the detail pane shows safe status metadata and explains that tool
  details are unavailable.
- **GIVEN** a catalog exceeds the storage limit, **WHEN** the user opens the
  Kandev detail pane, **THEN** the UI shows the stored entries and the full tool
  count with a truncation notice.
- **GIVEN** a phone user, **WHEN** they select a server and a tool, **THEN** a
  full-height drawer provides Back navigation and no horizontal overflow.
- **GIVEN** Auggie or another ACP agent is under investigation, **WHEN** a
  developer runs the sentinel MCP probe, **THEN** the JSONL and summary
  distinguish advertised capability, configuration delivery, initialize,
  tools list, and tool use.

## Out of scope

- A new ACP extension that requires every agent vendor to return connected MCP
  server status.
- Claiming automatic connection status for direct third-party MCP servers that
  Kandev cannot observe.
- Connecting to or proxying third-party MCP servers to collect their tool
  catalogs.
- Persisting third-party tool names or descriptions.
- Showing MCP output schemas, invocation arguments, or tool results in the
  explorer.
- Claiming that a tool estimate is an exact provider or billing token count.
- Enabling raw ACP frame logs or persistent raw stderr in release mode.
- Persisting prompts, tool arguments, credentials, header values, environment
  values, or full endpoint URLs in attachment diagnostics.
- Attributing a shared MCP connection to opaque internal subagents.
- Automatically restarting an agent, resetting context, or changing session
  state because attachment evidence is absent.
