---
status: current
system: tasks
requirements:
  - REQ-TASKS-MCP-TOOL-NAMES-001
---

# MCP Tool Name Stability System Design

## Purpose and boundaries

This design preserves canonical `*_kandev` registrations while adapting the
per-instance MCP transport for agents that add the server name themselves. It
covers built-in tools on task, configuration, external, Office, and automation
profiles because those profiles share the same MCP registry and transport.

The agent system declares client behavior. The task system consumes that
capability during construction of the session-bound MCP server for the agent. The
ACP protocol and the external agent client remain unchanged.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-MCP-TOOL-NAMES-001` | [Capability contract](#capability-contract), [Transport name translation](#transport-name-translation), [Control flow](#control-flow), [Verification](#verification) |

## Confirmed current behavior

- `apps/backend/internal/mcp/server` registers built-in tools with canonical
  names such as `ask_user_question_kandev` and `get_task_plan_kandev`.
- `apps/backend/internal/agentctl/server/config` and
  `apps/backend/internal/agentctl/server/api` inject the local server with the
  name `kandev`.
- `apps/backend/internal/agent/agents.Auggie` opts into assumed HTTP and SSE MCP
  transports but does not describe its client-side tool namespacing behavior.
- The Auggie client appends `_kandev` to the already canonical tool name, so the
  model sees `*_kandev_kandev`. Other supported clients preserve the registered
  name.

The focused baseline packages passed 1,501 tests before this design was
written.

## Capability contract

`agents.RuntimeConfig` gains a boolean capability named
`NamespacesMCPToolsByServer`. Auggie sets it to `true`. The zero value is
`false`, so Cursor ACP, Codex ACP, Claude ACP, and every existing agent retain
their current behavior unless they explicitly opt in.

This is a declared runtime characteristic, not an `AgentType == "auggie"`
branch. ACP does not advertise this presentation behavior during initialize,
so Kandev cannot negotiate it from the protocol handshake.

The lifecycle copies the value through each executor
`agentctl.CreateInstanceRequest`, the control-client JSON contract,
`instance.CreateRequest`, `config.InstanceOverrides`, and
`config.InstanceConfig`. The capability terminates at construction of the
per-instance Kandev MCP server. It does not alter user-configured MCP servers.

## Transport name translation

The MCP server keeps its internal registry, validators, tool profiles, and
handlers keyed by canonical names. A per-instance presentation policy changes
only the local Kandev MCP transport:

1. If server namespacing is enabled, remove exactly one trailing `_kandev` on
   `tools/list`. Return the modified built-in definition.
2. Auggie appends the injected server name and presents the resulting canonical
   name to its model. For example, `ask_user_question` plus `kandev` becomes
   `ask_user_question_kandev`.
3. When the model calls that name, Auggie removes its server namespace and
   sends `ask_user_question` in `tools/call`.
4. If the canonical registered tool exists, map the bare transport name before
   MCP registry lookup. The mapped name is `ask_user_question_kandev`.
5. Argument validation, logging, and the registered handler receive the
   canonical name.

The translation reads the current registry at each list or call boundary.
Thus, `SetMode`, profile replacement, and dynamic tool updates cannot leave a
stale alias table. An exact registered name wins over a derived alias. Tests
assert that each current built-in `*_kandev` name has an unambiguous round trip.

Plugin-contributed tools use their independent `kandev_<plugin>_<action>`
namespace. They do not match the trailing-suffix translation and keep their
current transport behavior.

## Control flow

```text
Auggie RuntimeConfig (namespaces = true)
  -> lifecycle executor request
  -> agentctl instance configuration
  -> per-instance MCP presentation policy
  -> tools/list: canonical -> bare
  -> Auggie presentation: bare + _kandev -> canonical model name
  -> tools/call: Auggie removes _kandev -> bare
  -> MCP call boundary: bare -> canonical
  -> existing validator and handler
```

For an agent whose capability is false, both translation steps are no-ops. ACP
`session/new`, `session/load`, and session reset continue to inject a server
named `kandev`. Their existing transport filtering is unchanged.

## Prompt consistency

`apps/backend/internal/sysprompt` continues to emit canonical `*_kandev`
references. No agent-specific prompt variant is needed because the transport
adaptation makes the model-facing name canonical for both capability values.

The existing prompt-to-registry synchronization tests remain authoritative.
New coverage composes each translated tool name with the injected server name
and proves that the result equals the canonical prompt and registry name.

## Failure and recovery

- A missing capability propagation step would restore doubled names for that
  executor. Producer, JSON, instance-config, and consumer-boundary tests cover
  the complete chain.
- A name that cannot map to a canonical registration follows the existing MCP
  tool-not-found path. Kandev does not guess or dispatch a different tool.
- A future alias collision fails the all-profile round-trip test. It does not
  silently change the handler of an existing tool.
- Session reconnect and reset rebuild no persistent state. The immutable
  instance capability applies again during tool lists and calls.

## Persistence and security

The capability is derived from the selected built-in agent at launch and is not
persisted as user-controlled data. No database migration is required.

The translation changes names only. Existing session identity, profile
authority, argument validation, permissions, and backend dispatch checks remain
in force.

## Observability

The existing MCP attachment catalog records the tool definitions returned by
the per-instance endpoint. ACP debug logs remain the manual evidence for the
final model-facing name and call result. A live Auggie test must show
`ask_user_question_kandev` and `get_task_plan_kandev`, with no
`_kandev_kandev` occurrence.

## Verification

- Agent tests pin the Auggie capability to `true` and representative
  non-namespacing agents to `false`.
- Lifecycle and agentctl contract tests pin propagation to MCP server
  construction.
- MCP transport tests enumerate every built-in profile, prove the list-name
  round trip, and prove that a bare incoming call reaches its canonical
  handler.
- System-prompt synchronization tests prove canonical prompt names remain
  unchanged.
- The backend test and lint targets provide the final regression gate.

## Related decisions

- [Preserve Canonical MCP Tool Names Through Agent Namespacing](../../../decisions/2026-08-31-agent-aware-mcp-tool-names.md)
- [Task model unification](../../../decisions/0004-task-model-unification.md)
- [Per-CLI MCP server injection for passthrough mode](../../../decisions/0014-passthrough-mcp-injection-strategies.md)
- [Compose MCP Tool Profiles From Typed Context](../../../decisions/2026-08-08-mcp-tool-profiles.md)
