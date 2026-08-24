# ADR-2026-07-30-session-owned-mcp-observability: Keep MCP Attachment Evidence Session Owned

**Status:** accepted
**Date:** 2026-07-30
**Area:** backend, frontend, protocol, security

## Context

Kandev configures MCP at several boundaries: agent profiles resolve servers,
executor policy can filter them, ACP adapters deliver supported transports,
passthrough strategies materialize CLI-specific configuration, and agentctl
hosts Kandev's task-aware MCP endpoint. The current toolbar collapses those
steps into a list derived from the profile, so “configured” is presented as if
it meant “attached.”

ACP `initialize` advertises transport capabilities and `session/new` accepts
`mcpServers`, but the protocol does not return a standard list of MCP servers
that connected successfully. A connection failure can also happen before the
target server observes any request. Raw ACP and stderr capture would improve
diagnosis but can contain prompts, files, tool arguments, URLs, headers,
environment values, and credentials, making unconditional release logging an
unacceptable privacy boundary.

Concurrent agents make global status invalid. A task can own several Kandev
sessions, and a single session can restart into several execution generations
or reset its provider session inside one execution. An old successful
connection must not make a new attachment attempt appear healthy.

## Decision

MCP attachment observability is owned by the Kandev session and execution:

- Every report is keyed by backend-owned `task_id`, `session_id`,
  `execution_id`, and `attachment_attempt_id`, enriched with agent/profile
  identity and provider ACP session identity when available. Every
  `session/new`, `session/load`, `session/reset`, or passthrough process start
  creates a new attempt before delivery.
- Agentctl's instance identity is the authoritative execution generation for
  local MCP observations. The server injects task and session ownership from
  that instance; agent-supplied IDs never select the report.
- Each MCP transport client gets an opaque connection ID. Shared connections
  remain shared; Kandev does not infer internal subagent identity.
- Configuration resolution, filtering, delivery, session acceptance,
  initialize, `tools/list`, tool use, explicit errors, disconnect, and
  supersession are separate evidence events. The UI displays the strongest
  observed evidence and never treats missing evidence as a connection failure.
- Kandev's in-agentctl MCP server uses transport/session hooks to observe
  initialize, `tools/list`, calls, and disconnects. Third-party direct MCP
  servers remain delivery-unverified unless an explicit agent error or future
  observable proxy/provider contract exists.
- Release mode persists a bounded structured timeline for the current and two
  previous attachment attempts. It contains stable reason codes and sanitized
  summaries, not raw protocol frames or persistent stderr.
- Sanitized network targets retain only scheme and host with optional port.
  Stdio targets retain only the executable basename. Headers, environment
  values, arguments, credentials, prompts, files, tool inputs/results, full
  URLs, and raw agent output are excluded.
- Recent agent stderr remains an explicit, on-demand, bounded, ephemeral
  diagnostic. It is warned as potentially sensitive and is never included in
  the default copied report.
- A user-initiated endpoint test runs only against a server already present in
  the session's effective configuration and from the same executor. Its result
  is presented as reachability evidence, not proof that the agent attached.
- The toolbar trigger stays visually neutral. Per-server colors and diagnostic
  actions appear in a desktop hover/focus popover that can be pinned, or an
  inset mobile drawer opened by a 44px touch target.
- `acpdbg` gains an opt-in sentinel MCP probe so agent integrations can be
  tested against the real ACP handshake. Advertised capabilities, delivery,
  initialize, tool listing, and tool use remain separate findings.

The observable behavior is specified in
[`docs/specs/platform/requirements/mcp-session-observability.md`](../specs/platform/requirements/mcp-session-observability.md).

## Consequences

Users can distinguish “Kandev sent this configuration” from “the agent loaded
these tools” for each concurrent session, including after restarts. Release
diagnostics remain useful without enabling high-risk raw frame logging.

The report is deliberately asymmetric: Kandev can automatically prove more
about its own in-session endpoint than about third-party servers. Some rows
will remain unverified even when the agent is working correctly. The UI and
public documentation must explain that limitation.

Lifecycle, agentctl, orchestrator persistence, boot hydration, WebSocket state,
and the chat toolbar must share one versioned report contract. Each new
attachment attempt supersedes current display evidence before it can deliver
configuration, including resets that reuse an execution. Passthrough agents
must emit honest resolution/materialization evidence; a missing passthrough
strategy is Filtered or Unavailable, not Active.

Endpoint tests and stderr reads become new session-keyed diagnostic entry
points. They must use the existing authorization guards, accept only stored
server names, remain bounded, and avoid turning arbitrary user input into
network or process execution.

## Alternatives Considered

- **Keep deriving status from profile configuration.** Rejected because it
  creates false positives and cannot distinguish concurrent executions.
- **Parse a connected-server list from ACP.** Rejected because ACP defines no
  such portable response; provider-specific metadata can be supplemental but
  cannot be the product contract.
- **Mark a server failed when no connection appears after a timeout.** Rejected
  because third-party connections may be unobservable or lazy, and absence is
  not failure evidence.
- **Proxy every MCP server through Kandev.** Deferred because it changes
  transport ownership, credentials, performance, and failure domains far
  beyond an observability feature.
- **Enable raw ACP and stderr logging in release mode.** Rejected because the
  diagnostic value does not justify persistent capture of user and credential
  data.
- **Show status colors directly on every toolbar icon.** Rejected because many
  concurrent sessions would create visual noise; the neutral trigger with
  details on disclosure preserves density.
- **Key status only by task or ACP session ID.** Rejected because tasks can run
  several agents and ACP IDs can rotate or collide across providers. Kandev's
  session, execution, and attachment-attempt identities form the stable
  ownership boundary.
