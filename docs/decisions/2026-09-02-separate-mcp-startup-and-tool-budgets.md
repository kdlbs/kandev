# ADR-2026-09-02-separate-mcp-startup-and-tool-budgets: Separate MCP Startup and Tool-Call Budgets

**Status:** proposed
**Date:** 2026-09-02
**Area:** backend, agents

## Context

`ClaudeACP.Runtime` declared a single managed agent default,
`MCP_TIMEOUT=7200000`, to give Kandev's blocking MCP tools a two-hour budget.
In the Claude Code CLI that key does not only bound tool calls. It bounds the
MCP connect deadline and a first-turn wait that the CLI performs before it
dispatches the first prompt.

After a successful handshake, the CLI opens a `subscriptions/listen` request and
waits for it to return before dispatching the first turn. `subscriptions/listen`
is a server-to-client notification channel: a specification-conformant server
holds it open and sends nothing. The CLI therefore waits out its budget, cancels
with `notifications/cancelled`, opens a fresh listen, and proceeds. The block is
`MCP_TIMEOUT - 5000` ms.

Frame-traced with a logging proxy on 2026-09-02 and filed upstream as
`anthropics/claude-code#91414`:

```text
 5.629 -> server/discover        hdr=2026-07-28
 5.800 <- server/discover        200 1784b        (succeeds in 171 ms)
 5.806 -> subscriptions/listen   id=listen:0
20.812 -> notifications/cancelled requestId=listen:0
```

Confirmed to the millisecond at two values: `MCP_TIMEOUT=12000` blocks 7.002 s,
`MCP_TIMEOUT=20000` blocks 15.006 s. End-to-end `claude -p` measured 6.0 s with
no MCP server and 22.6 s with one HTTP MCP server. This matches the CLI's own
arithmetic, `max(MCP_TIMEOUT - 5000, floor(MCP_TIMEOUT / 3))`, which reduces to
`MCP_TIMEOUT - 5000` for any value at or above 7500 ms.

This is a client defect and it is not specific to any MCP server. It affects
every server that implements `subscriptions/listen` as specified. Kandev's
7200000 turned a 25-second startup cost into 1h 59m 55s, during which the agent
process is alive, emits nothing, and the task card reads generating.

The CLI exposes `MCP_TOOL_TIMEOUT` for the tool budget, clamped to
`[60000, 2147483647]`, and computes the per-request fetch deadline as
`max(MCP_TOOL_TIMEOUT, MCP_TIMEOUT)`.

## Decision

Kandev expresses MCP startup budgets and MCP tool-call budgets as two
independent managed agent defaults, and never uses one value for both.

For the Claude ACP runtime: `MCP_TIMEOUT=30000` and
`MCP_TOOL_TIMEOUT=7200000`. A test asserts both values and the startup bound,
because a regression here is invisible at runtime until it costs a user hours.

Kandev does not set `MCP_PROTOCOL_NEGOTIATION`.

## Consequences

The first-turn wait falls from 1h 59m 55s to 25 seconds, paid once per launch
whenever any MCP server is configured. Blocking Kandev MCP tools keep their
two-hour budget, because the per-request fetch deadline still resolves to
7200000 through `MCP_TOOL_TIMEOUT`.

The residual 25 seconds is the upstream defect, not a Kandev choice. It cannot
be reduced further by lowering `MCP_TIMEOUT` without also shortening the connect
deadline that the same key governs, so 30000 is where the two obligations meet.
If `anthropics/claude-code#91414` is fixed, the residual disappears with no
change on Kandev's side.

Any future agent runtime that needs a long tool budget must set
`MCP_TOOL_TIMEOUT`, not `MCP_TIMEOUT`.

## Alternatives Considered

- **Also set `MCP_PROTOCOL_NEGOTIATION=legacy` as defence in depth.** Rejected.
  Forcing the legacy handshake avoids the code path that opens
  `subscriptions/listen`, so it removes the wait outright. Against it: the value
  pins every MCP server in every Claude session in Kandev to an older handshake
  in order to route around a client bug that has been filed and will be fixed;
  the CLI gates era negotiation for HTTP behind its own rollout flag, so Kandev
  would be overriding a vendor rollout decision it cannot see; the pin is
  invisible at runtime and would silently outlive the defect it was added for.
  Once the wait is bounded at 25 seconds the failure it prevents is no longer
  severe. A user who needs it can set it on an agent profile, which outranks
  managed agent defaults. Revisit if the upstream fix stalls and 25 seconds per
  launch proves unacceptable.
- **Remove `MCP_TIMEOUT` and rely on the CLI default of 30000.** Rejected.
  It produces the same runtime value but records no intent at the declaration
  site, leaves nothing for a test to assert, and silently follows a future
  change to the vendor default.
- **Keep one key and lower it to 30000.** Rejected. It caps the first-turn wait
  but drops the per-request fetch deadline that Kandev's blocking clarification
  call depends on.
- **Detect the wait and recover.** Rejected as the fix for this defect. It
  treats a bounded configuration error as a runtime failure to be caught, and
  it is separately owned by
  [Agent Stall Recovery](../specs/agents/requirements/agent-stall-recovery.md).
