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

The Claude Code CLI reads three independent environment values. Two are
Kandev-managed defaults; the third, `CLAUDE_CODE_MCP_TOOL_IDLE_TIMEOUT`, is
read by the CLI but not currently set by Kandev (see
[Idle watchdog](#idle-watchdog) below).

| Key | Meaning in the CLI | Kandev managed default |
| --- | --- | --- |
| `MCP_TIMEOUT` | MCP connect deadline, and the deadline of the CLI's first-turn wait on the `subscriptions/listen` stream. CLI default is 30000 ms, clamped to at most 2147483647. | `30000` |
| `MCP_TOOL_TIMEOUT` | Per-call tool budget, clamped to `[1000, 2147483647]` by the idle watchdog's `toolTimeoutMs` helper (see [Idle watchdog](#idle-watchdog)); separately floors the per-request fetch deadline at `[60000, 2147483647]` (see below). | `7200000` |
| `CLAUDE_CODE_MCP_TOOL_IDLE_TIMEOUT` | Per-tool-call idle watchdog: aborts a call if no bytes (response or `notifications/progress`) arrive for this long, independent of `MCP_TOOL_TIMEOUT`. | Not set by Kandev. |

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

### Idle watchdog

`MCP_TOOL_TIMEOUT` bounds total call duration; it does not bound silence.
Separately, the CLI runs a per-tool-call idle watchdog. This section and its
experiments were verified against a different binary than the CLI version
above: `node_modules/@anthropic-ai/claude-agent-sdk-darwin-arm64/claude`,
bundled by `@agentclientprotocol/claude-agent-acp@0.75.1` as
`claude-agent-sdk@0.3.257` (the managed default Claude ACP runtime pinned at
this commit; an operator selection can launch a different version — see
`apps/backend/internal/agent/agents/ACP_BRIDGE_VERSIONS.md`), not
`~/.local/share/claude/versions/*`. The watchdog function is rewritten
below with descriptive names from that binary's minified source (logic and
constants unchanged; the minified identifiers are not):

```js
function idleTimeoutMs(server) {
  const transport = server?.type ?? "stdio";
  if (new Set(["sse-ide", "ws-ide", "sdk"]).has(transport)) return 0; // disabled
  const idle = process.env.CLAUDE_CODE_MCP_TOOL_IDLE_TIMEOUT
    ?? (transport === "stdio" ? 1800000 : 300000);
  if (idle <= 0) return 0;
  const perServerTimeout = server?.timeout >= 1000 ? server.timeout : 0;
  // toolTimeoutMs(server) = clamp(perServerTimeout ?? MCP_TOOL_TIMEOUT ?? 1e8, 1000, 2147483647)
  return Math.min(Math.max(idle, perServerTimeout, 1000), toolTimeoutMs(server));
}
```

Kandev injects its MCP server as `type: "http"` (with an `"sse"` fallback;
see `apps/backend/internal/agentctl/server/api/agent.go`), which is neither
`stdio` nor in the disabled set, so the idle default is **300000 ms (300s)**.
The outer `Math.min` in the formula above means `MCP_TOOL_TIMEOUT` can only
lower that default, never raise it: at Kandev's managed `MCP_TOOL_TIMEOUT=7200000`
the clamp is a no-op and the effective idle deadline stays 300000 ms. But
`toolTimeoutMs`'s own floor is 1000 ms, not 60000, so an agent-profile or
executor-profile override of `MCP_TOOL_TIMEOUT` (see
[Failure and recovery](#failure-and-recovery)) can shrink the idle deadline far
below 300000 ms — e.g. `MCP_TOOL_TIMEOUT=10000` yields a 10s idle deadline. A
tool call that goes past its idle deadline without emitting a response or a
`notifications/progress` frame is aborted by the CLI with "sent no response or
progress for <n>s; aborting". Kandev's managed default keeps the 20s keepalive
below comfortably inside the deadline, but an override under roughly 20000 ms
drives the idle deadline below the keepalive interval and would abort a
blocking `ask_user_question_kandev` call outright: this is a real edge in the
supported override path, not just a record-accuracy nit.

`ask_user_question_kandev` (`apps/backend/internal/mcp/handlers/handlers.go`)
is the only Kandev MCP tool that blocks on a person, so it is the only one
this watchdog can plausibly hit. It survives because
`apps/backend/internal/mcp/server/handlers.go` streams a
`notifications/progress` frame every `askQuestionKeepAliveInterval` (20s),
comfortably inside the 300s floor.

Four throwaway experiments against that binary (`claude -p --mcp-config
--allowedTools`, a minimal streamable-HTTP MCP server whose tool never
returns) confirmed the formula and ruled out the obvious alternative fix:

| Config | Server-observed lifetime | Outcome |
| --- | --- | --- |
| `CLAUDE_CODE_MCP_TOOL_IDLE_TIMEOUT=10000` (10s), silent | ~35s | aborted at the overridden floor, not at 300s — confirms the CLI reads this env var |
| Default env, `MCP_TOOL_TIMEOUT=7200000`, silent (no progress) | 304.0s | aborted: "sent no response or progress for 300s; aborting" |
| `CLAUDE_CODE_MCP_TOOL_IDLE_TIMEOUT=7200000`, `MCP_TOOL_TIMEOUT=7200000`, silent | 358.1s (reproduced twice) | `The operation timed out.` — a second, lower client-side ceiling the env var does not lift |
| Default env, SSE + `notifications/progress` every 20s | 535.1s, still alive | ended only by the harness's own external kill |

Setting `CLAUDE_CODE_MCP_TOOL_IDLE_TIMEOUT` alongside `MCP_TOOL_TIMEOUT` is
**not** a viable fix on its own: it raises the floor from 300s to only
~358s, not to two hours, because of the second ceiling above. The progress
keepalive is what actually delivers the two-hour budget, and it already
ships. There is no per-server `timeout` field on the ACP wire
(`types.McpServer`, `jsonrpc.McpServer`) to raise the idle floor from
Kandev's side either.

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
call is not, by itself, governed by `MCP_TOOL_TIMEOUT`: the CLI's idle
watchdog (see [Idle watchdog](#idle-watchdog)) would abort it at 300s under
Kandev's managed `MCP_TOOL_TIMEOUT=7200000`. It survives because the server
streams a
`notifications/progress` keepalive well inside that window.

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
