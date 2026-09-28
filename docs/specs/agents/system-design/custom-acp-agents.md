---
status: draft
system: agents
requirements:
  - REQ-AGENTS-CUSTOM-ACP-001
created: 2026-09-22
updated: 2026-09-22
owners:
  - jnmanso
---
# Operator-Registered Agent System Design

## Purpose and boundaries

This design owns the technical contract for REQ-AGENTS-CUSTOM-ACP-001: the stored shape of an
operator-registered definition, which agent type each protocol builds, and the three places that
previously assumed every such definition was terminal passthrough.

It does not change how built-in agents are declared, detected, or launched.

## Requirement mapping

| Acceptance criterion | Owner |
| --- | --- |
| AC-AGENTS-CUSTOM-ACP-001.1 | `internal/agent/discovery` |
| AC-AGENTS-CUSTOM-ACP-001.2 | `internal/agent/discovery`, `internal/agent/settings/controller` |
| AC-AGENTS-CUSTOM-ACP-001.3 | `internal/agent/registry`, `internal/agent/settings/models` |
| AC-AGENTS-CUSTOM-ACP-001.4 | `internal/agent/agents` |
| AC-AGENTS-CUSTOM-ACP-001.5 | `internal/agent/settings/controller` |
| AC-AGENTS-CUSTOM-ACP-001.6 | `internal/agent/registry`, `internal/agent/settings/handlers` |
| AC-AGENTS-CUSTOM-ACP-001.7 | `internal/agentctl/server/utility` |

## Discovery reads the registry per sweep

`discovery.Registry` holds `*registry.Registry` and resolves enabled, non-virtual agents inside
`detectAll`. The boot-time capture is removed, along with the `KnownAgent`/`Definitions()` pair that
carried it and the `IsInstalled` pass it ran at startup.

Detection results stay cached for `defaultCacheTTL`. Because a sweep can outlive an invalidation, the
cache is generation-fenced: `InvalidateCache` bumps a counter, `Detect` captures it before sweeping,
and the write at the end is skipped when the captured generation is stale. Without the fence a sweep
that started before a delete would publish the deleted agent back into the cache.

`CreateCustomTUIAgent`, `DeleteAgent`, and `SetCustomTUIAgentMCPStrategy` invalidate after their
registry mutation commits.

## Protocol lives in the stored definition

`models.TUIConfigJSON.protocol` carries `registry.CustomAgentProtocol`. The zero value is
`CustomAgentProtocolTerminal`, which is what every row written before the field existed decodes to, so
no migration runs. `CustomAgentProtocolACP` is the only other accepted value.

`registry.buildCustomAgent` switches on it:

- terminal builds an `agents.TUIAgent` with the resolved MCP strategy, exactly as before;
- ACP builds an `agents.CustomACPAgent` and rejects a non-empty MCP strategy;
- anything else returns `ErrUnknownCustomAgentProtocol`.

`controller.CustomAgentSpecFromStored` is the single spec builder shared by the MCP-strategy change and
the boot replay, so a field added to the stored config cannot reach one path and miss the other.

## CustomACPAgent is deliberately not a TUIAgent

`agents.IsPassthroughOnly` is a type assertion on `*TUIAgent`, and the host-utility probe covers only
`InferenceAgent` implementers. `CustomACPAgent` therefore implements `Agent` plus `InferenceAgent`,
reports `agent.ProtocolACP` from `Runtime()`, and deliberately does not implement `PassthroughAgent`: a
definition carries one command, and the command that starts an ACP server is not the command that
renders an interactive terminal.

`IsInstalled` reports `SupportsMCP = true` because resolved servers travel in ACP `session/new` rather
than through a config-file strategy.

## The probe accepts an operator-registered command

`acp_executor.go` resolved every spawn against `allowedProbeCommands`, a map of compiled-in literals
that exists so static analysis can follow a literal to `exec.Command`. An operator-registered command
cannot be in that map.

`resolveSpawnCommand` keeps that path for built-ins and returns the command unchanged when the agent
marked it operator-defined (`InferenceConfig.OperatorDefined`, carried to agentctl on
`InferenceConfigDTO`). The exception is bounded to a command the install operator typed in Settings:
the session path already spawns that exact string, and agentctl's piped runner already accepts an
arbitrary command from its request, so the allow-list denied the probe without denying execution.
