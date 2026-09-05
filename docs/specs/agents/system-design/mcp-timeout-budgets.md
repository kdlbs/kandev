---
status: draft
system: agents
requirements:
  - REQ-AGENTS-MCP-TIMEOUT-BUDGETS-001
created: 2026-09-02
owners:
  - kandev
---

# Agent MCP Timeout Budgets System Design

## Purpose and boundaries

This design owns the managed agent default environment values that express MCP
time budgets for an agent runtime. It uses, but does not own, the environment
resolution and precedence contract in
[Executor-Profile Environment Precedence](../../executors/system-design/executor-profile-env-precedence-01.md),
and the blocking behavior of Kandev's own MCP handlers.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-MCP-TIMEOUT-BUDGETS-001` | [Components and responsibilities](#components-and-responsibilities), [Data and contracts](#data-and-contracts), [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- `apps/backend/internal/agent/agents/*.go`: each agent implementation returns a
  `RuntimeConfig` whose `Env` map declares that runtime's managed agent
  defaults. `ClaudeACP.Runtime` is the only implementation that declares MCP
  time budgets.
- `apps/backend/internal/agent/runtime/lifecycle/environment_resolution.go`:
  `appendAgentRuntimeDefaults` converts each `RuntimeConfig.Env` entry into a
  `runtimeenv.Definition` at origin `managed agent defaults`, skipping any key a
  higher-precedence definition already declared.
- `apps/backend/internal/agent/runtime/lifecycle/manager_startup.go`: the
  non-strict assembly path applies the same `RuntimeConfig.Env` entries with
  the same "existing key wins" rule.
- `apps/backend/internal/mcp/handlers/handlers.go`: holds a clarification MCP
  request open while it waits for a user answer. It consumes the tool-call
  budget; it does not set it.

## Data and contracts

The Claude Code CLI reads two independent environment values.

| Key | Meaning in the CLI | Kandev managed default |
| --- | --- | --- |
| `MCP_TIMEOUT` | MCP connect deadline, and the deadline of the CLI's first-turn wait on the `subscriptions/listen` stream. CLI default is 30000 ms, clamped to at most 2147483647. | `30000` |
| `MCP_TOOL_TIMEOUT` | Per-call tool budget and the per-request fetch deadline floor, clamped to `[60000, 2147483647]`. | `7200000` |

Verified against the shipped CLI (version 2.1.258):

- `MCP_TIMEOUT` reader: `n && n > 0 ? Math.min(n, 2147483647) : 30000`.
- The connect deadline uses that reader directly. Under era negotiation the
  connect that carries the subscription stream uses
  `max(MCP_TIMEOUT - 5000, floor(MCP_TIMEOUT / 3))`, which is
  `MCP_TIMEOUT - 5000` for any value at or above 7500 ms.
- The CLI opens `subscriptions/listen` and waits for that request to return
  before it dispatches the first turn. `subscriptions/listen` is a
  server-to-client notification channel: a specification-conformant server
  holds it open and sends nothing. The CLI therefore waits out
  `MCP_TIMEOUT - 5000`, cancels with `notifications/cancelled`, reopens a fresh
  listen, and proceeds. The binary carries the matching telemetry and retry
  path (`tengu_mcp_listen_reopen`, `subscriptions/listen re-open attempt`).
- The per-request fetch deadline is
  `max(clamp(MCP_TOOL_TIMEOUT, 60000, 2147483647), MCP_TIMEOUT)`, so setting
  `MCP_TOOL_TIMEOUT` to 7200000 preserves the two-hour request budget that
  `MCP_TIMEOUT=7200000` previously produced as a side effect.

The first-turn wait is an upstream client defect, filed as
`anthropics/claude-code#91414`. It is not a property of any MCP server. Kandev's
contract is to bound its cost, not to work around it.

`30000` is deliberately explicit rather than omitted. It records Kandev's
intended bound at the declaration site, keeps AC-002 testable, and does not
depend on the CLI keeping 30000 as its own default.

## Control flow

1. `ClaudeACP.Runtime` returns `Env` containing both keys.
2. `appendAgentRuntimeDefaults` adds each key at origin
   `managed agent defaults`, tier `TierAgentRuntimeDefault`, unless the key is
   already defined by a higher-precedence origin.
3. `runtimeenv.Resolve` produces the launch environment.
4. The agent process starts. Its MCP connect deadline and its pre-prompt wait
   are bounded by `MCP_TIMEOUT`. Its tool calls and MCP fetch deadline are
   bounded by `MCP_TOOL_TIMEOUT`.

## Failure and recovery

With any MCP server configured, the CLI delays its first turn by
`MCP_TIMEOUT - 5000` ms. At the managed default of 30000 that is 25 seconds,
paid once per launch; at the previous value of 7200000 it was 1h 59m 55s. The
degraded outcome is a bounded delay before the first token, after which the
session proceeds normally with all tools available. A blocking Kandev MCP tool
call is unaffected because it is governed by `MCP_TOOL_TIMEOUT`.

`MCP_TIMEOUT` remains the connect deadline, so it cannot be reduced to an
arbitrarily small value purely to shrink the first-turn wait without also
shortening the time a legitimately slow server has to connect. 30000 keeps the
CLI's own connect semantics unchanged.

A user who needs a different bound sets `MCP_TIMEOUT` or `MCP_TOOL_TIMEOUT` on
an agent profile or executor profile. Both origins outrank
`managed agent defaults`, and `appendAgentRuntimeDefaults` skips a key that is
already defined, so the override applies without an environment conflict.

## Persistence

None. These values are computed per launch and are not stored.

## Security

Neither value is a secret. Both are already carried as plain literals in the
launch environment.

## Observability

`Manager.logEnvironmentOverrides` already emits `environment override applied`
with winning and losing origins when a profile replaces a managed agent
default, so an override of either budget is visible in backend logs.

## Related decisions

- [ADR-2026-09-02-separate-mcp-startup-and-tool-budgets](../../../decisions/2026-09-02-separate-mcp-startup-and-tool-budgets.md)
