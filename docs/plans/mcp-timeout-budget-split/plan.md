---
spec: docs/specs/agents/requirements/mcp-timeout-budgets.md
design: docs/specs/agents/system-design/mcp-timeout-budgets.md
decision: docs/decisions/2026-09-02-separate-mcp-startup-and-tool-budgets.md
created: 2026-09-02
status: done
---

# Implementation Plan: Split the Claude MCP Timeout Budgets

## Overview

Replace the single `MCP_TIMEOUT=7200000` managed agent default on the Claude
ACP runtime with two independent budgets, add the regression test the current
value never had, and audit the other agent runtimes for the same coupling.

Requirements: `REQ-AGENTS-MCP-TIMEOUT-BUDGETS-001`
(`AC-...-001.1` through `AC-...-001.5`).
Design: `docs/specs/agents/system-design/mcp-timeout-budgets.md`.

## Confirmed root cause

`apps/backend/internal/agent/agents/claude_acp.go:125-127` declares one managed
agent default, `MCP_TIMEOUT: "7200000"`, chosen so Kandev's blocking MCP tools
(`apps/backend/internal/mcp/handlers/handlers.go:3782` waits for a user answer)
have a two-hour budget.

In the Claude Code CLI that key governs two unrelated things: the MCP connect
deadline, and a first-turn wait the CLI performs before dispatching the first
prompt.

The handshake is not the problem. It succeeds. After it succeeds, the CLI opens
a `subscriptions/listen` request and waits for that request to RETURN before
dispatching the first turn. `subscriptions/listen` is a server-to-client
notification channel, so a specification-conformant server holds it open and
sends nothing. The CLI waits out its full budget, cancels with
`notifications/cancelled`, opens a fresh listen, and proceeds.

Frame trace from a logging proxy between the CLI and one MCP server:

```text
 5.629 -> server/discover        hdr=2026-07-28
 5.800 <- server/discover        200 1784b        (succeeds in 171 ms)
 5.806 -> subscriptions/listen   id=listen:0
20.812 -> notifications/cancelled requestId=listen:0
```

The block is exactly `MCP_TIMEOUT - 5000` ms, confirmed at two values:
`MCP_TIMEOUT=12000` blocks 7.002 s, `MCP_TIMEOUT=20000` blocks 15.006 s.
End-to-end `claude -p` measured 6.0 s with no MCP server and 22.6 s with one
HTTP MCP server.

This is an upstream client defect, filed as `anthropics/claude-code#91414`. It
is not a slow server, not a broken server, and not specific to any one server:
it affects every MCP server that implements `subscriptions/listen` as
specified.

Reading the shipped binary at `~/.local/share/claude/versions/2.1.258` shows the
same arithmetic:

- The `MCP_TIMEOUT` reader is `function Cc(){ let n = a.MCP_TIMEOUT; return
  n && n > 0 ? Math.min(n, 2147483647) : 30000 }`. The CLI default is 30000 ms.
- The connect that carries the subscription stream uses
  `Wr() = Math.max(Cc() - 5000, Math.floor(Cc() / 3))`, which is
  `MCP_TIMEOUT - 5000` for any value at or above 7500 ms, matching both
  measurements exactly.
- The cancel-and-reopen behavior is present as `tengu_mcp_listen_reopen` and a
  `subscriptions/listen re-open attempt` retry path.
- The first-turn wait is instrumented as `tengu_headless_mcp_prewait` with log
  markers `before_mcp_prewait` / `mcp_prewait_ms` / `after_mcp_prewait` /
  `before_ask`, and its deadline becomes the `MCP_TIMEOUT` reader when the CLI
  receives an explicit MCP configuration.

With `MCP_TIMEOUT=7200000` the wait is 7,195,000 ms, which is 1h 59m 55s. That
is the observed freeze: the process is alive, emits no tokens, and Kandev's card
stays at `foreground_activity=generating`.

`MCP_TOOL_TIMEOUT` is the correct key for the two-hour budget. The CLI clamps
it to `[60000, 2147483647]` and computes the per-request fetch deadline as
`max(clamp(MCP_TOOL_TIMEOUT), Cc())`, so moving 7200000 onto it preserves the
long request budget that `MCP_TIMEOUT=7200000` was producing as a side effect.

## Why this is invisible today

Nothing asserts either value. `apps/backend/internal/agent/agents/claude_acp_test.go`
covers the command, install script, and permission flags, and no test in
`apps/backend/` references `MCP_TIMEOUT`. A wrong value here changes only
latency, so it survived until it cost a user two hours.

## Decision on `MCP_PROTOCOL_NEGOTIATION`

Considered and rejected as a Kandev-set default. The argument is recorded in
full in the ADR. In short: forcing the legacy handshake avoids the code path
that opens `subscriptions/listen`, so it removes the wait outright, but it pins
every MCP server in every Claude session to an older handshake to route around
a filed client bug, overrides a vendor rollout the CLI gates behind its own
flag, is invisible at runtime, and would silently outlive the defect. Once the
wait is bounded at 25 s the failure it prevents is no longer severe. Users who
want it can set it on an agent profile, which outranks managed agent defaults.

## Work orders

| Order | Work order | Outcome |
| --- | --- | --- |
| 1 | [task-01-split-claude-mcp-budgets.md](task-01-split-claude-mcp-budgets.md) | Claude ACP declares two independent budgets, asserted by test |
| 2 | [task-02-audit-agent-runtime-mcp-budgets.md](task-02-audit-agent-runtime-mcp-budgets.md) | No other agent runtime couples the two budgets, asserted by test |

Sequential. Task 02 generalises the invariant task 01 establishes for one
runtime; running it first would have nothing to generalise.

## Risks

- **The tool-call idle watchdog may bind before `MCP_TOOL_TIMEOUT`.** The CLI
  also computes an idle timeout from `CLAUDE_CODE_MCP_TOOL_IDLE_TIMEOUT`,
  defaulting to 1800000 ms for stdio and 300000 ms for other transports, capped
  by the tool budget. Kandev serves its own MCP over HTTP/SSE. Task 01 must
  verify empirically whether a blocking `ask_user_question_kandev` call still
  survives past those bounds after the split. This risk exists today and is not
  introduced by the split, but the split is the right moment to establish it.
  If the idle watchdog binds first, record the finding and raise it as separate
  work rather than widening this fix.
- **Residual 25 s first-turn wait whenever any MCP server is configured.**
  This is `MCP_TIMEOUT - 5000` and is the upstream defect, not a Kandev choice.
  It cannot be reduced further by lowering `MCP_TIMEOUT` without also
  shortening the connect deadline the same key governs. Accepted and documented
  in the ADR; it disappears when `anthropics/claude-code#91414` is fixed, with
  no change on Kandev's side.

## Verification strategy

Unit tests at the agent-runtime declaration boundary, which is where the
contract lives. No browser test: the requirement has no UI surface. A manual
launch with at least one MCP server configured is the acceptance evidence for
the latency outcome, recorded in task 01's results.
