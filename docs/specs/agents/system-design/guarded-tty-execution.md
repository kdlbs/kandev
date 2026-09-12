---
status: draft
system: agents
requirements:
  - REQ-AGENTS-GUARDED-TTY-EXECUTION-001
  - REQ-AGENTS-GUARDED-TTY-EXECUTION-002
---

# Guarded TTY Execution System Design

## Purpose and boundaries

The agent system owns the model-visible tool, provider capability negotiation,
execution binding, and durable receipt. Codex App Server owns terminal
allocation. The executor and deployment guard continue to own process-tree,
filesystem, credential, and Docker confinement.

This design adds no host command runner. The only valid dispatch enters the
already-running codex-acp bridge through its ACP connection and reaches the App
Server child inside the same guarded process tree.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-GUARDED-TTY-EXECUTION-001` | [Capability and tool contract](#capability-and-tool-contract), [Control flow](#control-flow) |
| `REQ-AGENTS-GUARDED-TTY-EXECUTION-002` | [Security boundary](#security-boundary), [Audit contract](#audit-contract) |

## Components and responsibilities

- `internal/mcp/profile` provides the backend-owned allow capability for a
  Codex ACP Kanban task session. It never accepts a tool list from the agent.
- `internal/mcp/server.Server` exposes `guarded_tty_exec_kandev` only when both
  the backend allow capability and a live agentctl bridge executor are present.
  Its schema accepts an argv array only.
- `internal/mcp/handlers` receives the trusted `MCPExecutionContext`, validates
  task ownership and current execution identity, claims an audit record, and
  calls the execution-bound runtime capability.
- `internal/agent/runtime` exposes a narrow TTY execution interface. The
  lifecycle implementation selects the exact current `agentctl.Client`; no
  caller supplies an endpoint or executor identity.
- `internal/agent/runtime/agentctl.Client` carries the request over the existing
  authenticated, execution-bound WebSocket stream.
- `internal/agentctl/server/api.Server` accepts the new agent stream action and
  delegates only to an adapter implementing the guarded-TTY interface.
- `internal/agentctl/server/adapter/transport/acp.Adapter` validates the
  versioned initialize advertisement, then probes the active session after
  session creation or load. A successful exact response enables the local MCP
  tool and triggers an atomic tool-catalog rebuild; failure removes or omits it.
- `@agentclientprotocol/codex-acp` validates the active session, derives cwd and
  sandbox state from that session, generates the process ID, and calls App
  Server `command/exec` with `tty: true`.
- The task message repository stores the audit claim and terminal result as
  typed metadata without a new database table.

## Capability and tool contract

The initialize metadata key is `guardedTtyExec`. Its exact value advertises
capability `kandev.guarded-tty-exec`, contract version `1`, capability method
`_kandev/guarded_tty/capability`, and execution method
`_kandev/guarded_tty/exec`. The capability probe accepts only the active
`sessionId` and must echo the same capability, version, methods, and session.
An empty object, method-not-found response, malformed version, different agent,
or stale session is unsupported.

The backend profile is an allow gate, not proof of runtime support. The local
MCP server registers the tool only after the active adapter probe succeeds.
`SetTools` publishes one `tools/list_changed` notification, and the existing
session-owned MCP attachment evidence records the actual catalog served to the
model.

The tool input contains one required `argv` array. Validation enforces fixed
element count, per-element bytes, combined bytes, and non-empty argv before any
backend action. The model cannot request a cwd, environment, sandbox,
permission profile, process ID, timeout above the fixed ceiling, or interactive
terminal operation.

The shared limits are 64 arguments, 4,096 bytes per argument, 8,192 bytes for
the combined argument vector, and 65,536 output bytes. Kandev never forwards a
caller-selected timeout. The upstream bridge owns its fixed 10-second App
Server operation ceiling plus a one-second cleanup grace period.

## Control flow

1. The lifecycle manager resolves a Codex ACP task profile and marks guarded TTY
   as allowed for that execution.
2. Agentctl validates the initialize advertisement and, after creating or
   loading an ACP session, calls the exact capability extension for that active
   session.
3. On an exact supported response, agentctl installs an adapter-backed executor
   on its local Kandev MCP server. The server atomically adds the tool.
4. Codex observes the updated catalog and selects `guarded_tty_exec_kandev`.
5. The MCP server forwards argv to the backend. Lifecycle middleware attaches
   `MCPExecutionContext` from the owning `AgentExecution`, not from payload.
6. The backend validates the server-derived Kanban principal and exact live
   execution, creates a pending audit claim, and sends a nested request over
   that execution's full-duplex agentctl stream.
7. Agentctl calls the bridge extension. The bridge resolves the active session,
   derives cwd and sandbox state, generates an unpredictable process ID, and
   dispatches App Server `command/exec` with streaming and `tty: true`.
8. The bridge accepts only matching `command/exec/outputDelta` events, applies
   output and time ceilings, and returns exactly one receipt.
9. Kandev validates the receipt, finalizes the audit, and returns the same
   attestation ID and bounded result through the MCP call.

Incoming MCP requests already dispatch on their own backend goroutine. Nested
agentctl requests still require concurrency, cancellation, disconnect, and
race tests so neither the original MCP request nor ordinary command events can
block the stream reader.

## Audit contract

The audit uses an opaque Kandev-generated attestation ID. The pending claim
records normalized argv plus server-derived execution, task, session,
workspace, optional authenticated actor, fixed `codex-acp` agent identity,
principal surface, and creation time. Finalization is compare-and-set by
attestation ID and records one of success, stale, timeout, cancelled, overflow,
or provider failure. A transport disconnect is a provider failure because no
provider receipt can prove whether dispatch occurred.

Successful finalization also records extension contract and bridge versions,
App Server method `command/exec`, requested and dispatched TTY booleans,
provider-generated process ID, trusted cwd, output byte count and digest,
timestamps, completion count, and exit code. The bounded MCP result can contain
output for the model. The audit metadata does not duplicate raw output or store
environment values.

The audit message ID is the same server-generated attestation ID returned in the
tool receipt, linking the model-visible result to the durable claim and provider
evidence.

## Security boundary

The trusted identity chain is:

`AgentExecution` -> in-session MCP context -> task owner authorization -> exact
`agentctl.Client` -> active ACP adapter -> active codex-acp session -> App Server
child.

Every hop rejects a missing, stopped, replaced, or mismatched identity. Payload
identity fields are not part of the public schema. The backend never accepts an
agentctl URL, workspace path, executor selection, or guard attestation from the
model.

Deployment repair `6fcc88f689dae9797dd131229167a98d0e955d43` remains
deployment-only. It wraps exact npx codex-acp launches in bubblewrap and changes
only `CODEX_CONFIG.features.unified_exec` to `false` while preserving every
other key. The bridge and App Server remain children inside that wrapper. The
TTY extension neither reads nor writes `CODEX_CONFIG` and never enables
`unified_exec`.

App Server `command/exec` receives the active session's sandbox policy from the
bridge, never a caller value. The outer executor/guard remains authoritative if
the App Server sandbox regresses. Docker tokens, credential broker values, and
mounts remain those of the existing execution and are never returned in a
receipt.

## Failure and recovery

- Unsupported or malformed bridge capability omits the tool.
- A bridge disconnect removes the executor and tool for the current MCP server.
- Validation, ownership, stale-execution, and confinement failures create or
  finalize a denial without provider dispatch.
- Timeout, cancellation, output overflow, and disconnect close the one-shot
  bridge operation and ignore late deltas or completions.
- A malformed receipt fails closed and records provider failure; it never
  converts an unknown result into success.
- Restart creates a new capability probe and execution binding. Audit rows from
  the prior execution remain terminal or pending evidence but cannot be reused.

## Observability

Structured logs and traces use attestation, execution, session, contract
version, outcome, duration, output size, and exit status. They exclude argv,
raw output, environment, cwd outside the task-relative projection, credentials,
and sandbox contents.

Tests and PR evidence record the exact deployment baseline, bridge version,
model tool call, `tty: true` dispatch receipt, terminal checks, denial result,
and simultaneous non-TTY completion.

## Related decisions

- [Keep Guarded TTY Dispatch Inside the Active Agent Execution](../../../decisions/2026-08-30-guarded-tty-inside-agent-execution.md)
- [Agent Client Protocol Codex ACP Bridge](../../../decisions/0034-agentclientprotocol-codex-acp.md)
- [Bound MCP Tool Definition Details to Current Sessions](../../../decisions/2026-08-18-session-mcp-tool-definition-details.md)
- [Keep Pending Agent Permission Authority in the Live Runtime](../../../decisions/2026-08-11-live-agent-permission-authority.md)
