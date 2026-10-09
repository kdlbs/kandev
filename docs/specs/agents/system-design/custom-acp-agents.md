---
status: draft
system: agents
requirements:
  - REQ-AGENTS-CUSTOM-ACP-001
  - REQ-AGENTS-CUSTOM-ACP-002
created: 2026-09-22
updated: 2026-10-09
owners:
  - jnmanso
---
# Operator-Registered Agent System Design

## Purpose and boundaries

This design owns the technical contract for REQ-AGENTS-CUSTOM-ACP-001 and REQ-AGENTS-CUSTOM-ACP-002: the stored shape of an
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
| AC-AGENTS-CUSTOM-ACP-001.8 | [Terminal MCP save acknowledgement](#terminal-mcp-save-acknowledgement) |
| AC-AGENTS-CUSTOM-ACP-001.9 | [Terminal MCP save acknowledgement](#terminal-mcp-save-acknowledgement) |
| AC-AGENTS-CUSTOM-ACP-001.10 | [Terminal MCP save acknowledgement](#terminal-mcp-save-acknowledgement) |
| AC-AGENTS-CUSTOM-ACP-002.1 | `internal/agent/agents` (declares restore); `internal/agent/runtime/lifecycle` and the ACP adapter (unchanged) |
| AC-AGENTS-CUSTOM-ACP-002.2 | ACP adapter capability gate (unchanged) |
| AC-AGENTS-CUSTOM-ACP-002.3 | `internal/agent/runtime/lifecycle` failure classification (unchanged) |
| AC-AGENTS-CUSTOM-ACP-002.4 | `internal/agent/agents` (`CustomACPAgent.Runtime`) |

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

## Terminal MCP save acknowledgement

The agent system owns this boundary because it owns the configured custom definition and its
profile membership. The Agents settings page mounts `CustomTUIMcpCard` next to
`AgentProfilesSubList`, supplying both from current `settingsAgents.items`. The strategy control
saves immediately and independently of profile drafts. Built-in and ACP definitions remain ineligible.

### Endpoint field ownership

`PATCH /api/v1/agents/tui/:id/mcp` calls `SetCustomTUIAgentMCPStrategy`, which changes
`TUIConfig.MCPStrategy` and coupled `SupportsMCP`, persists the definition, replaces the registry
entry, and invalidates discovery. `customTUIAgentDTO` returns a full `AgentDTO`, including separately
read profiles and capability metadata. `updateCustomTUIAgentMCPStrategy` normalizes those profiles
before returning the response. A full response shape does not give this save ownership of those fields.

The card obtains its owning store through the existing `useAppStoreApi`. After transport acceptance,
it reads current `settingsAgents.items` and synchronously uses the existing `setSettingsAgents`
action to publish a narrowly changed list. No await separates that read from publication.

- Match the requested agent identity against the accepted response and the current existing row.
- Patch only accepted `supports_mcp` and `tui_config.mcp_strategy`, merging the latter into the
  current row's TUI config. Preserve its command, display name, protocol, and other config fields.
- An omitted or empty accepted strategy represents Off. Use the accepted value rather than
  re-deriving it from the user's requested key.
- Preserve all current profiles, all other rows, current list membership/order, and all unrelated
  target metadata, including timestamps and capability information. Do not spread the response.
- If the current target is absent or no longer an eligible custom terminal definition, publish no
  replacement for it. Do not insert an agent from the ACK or restore a prior profile list.
- Do not write the separate `agentProfiles`, `availableAgents`, or discovery slices.

Existing `agent.profile.created/updated/deleted` handlers remain authoritative for profile events;
their normalized state and flattened profile options must survive this ACK. The existing
`agent.settings.updated` broadcast remains unchanged. Cross-client competing writes to the same
strategy are outside this local publication repair; no timestamp, revision, or request framework is added.

### Visible behavior, failures, and mobile

Current strategy values are read from subscribed current agent props. Only the saving card's control
is disabled while pending; sibling cards can save independently. A rejection keeps current store data
and uses the existing handled-error suppression or ordinary error toast, with `saving` cleared by the
existing settle path. No new copy, layout, navigation, touch target, or scroll owner is introduced.

Desktop and phone reuse the current Agents card and `MCPStrategySelect`, whose coarse-pointer
trigger/options already have a 44px minimum. This is state normalization within an existing component;
the mobile-parity exception permits focused rendered component integration coverage without new
Playwright cases. Existing desktop/mobile strategy-selector specs cover selector geometry rather than
this ACK race and must not be cited as race evidence.

### Regression boundary

Render the actual card and selector under the actual `StateProvider` (which creates `createAppStore`).
A subscribed test consumer supplies current agent props and captures `useAppStoreApi`; use real
`AgentProfilesSubList` for profile-row consequences. Only defer the external `fetchJson` transport;
leave the settings API, store, provider, actions, normalization, and registered WS handlers real.

While the PATCH is pending, deliver actual profile events through `registerAgentsHandlers` and
publish membership/settings changes through existing store actions. Prove before ACK that those
changes reached the store/render, then release the response and assert preserved rows and strategy.
Exercise two actual cards with both PATCH requests admitted before either ACK, resolving in both
orders. Include target deletion, ordinary on/off success, and failures with current changes.
Use distinct profile IDs per test to avoid unrelated module-level profile tombstones. Settle all
deferred requests before cleanup. Do not mock publication or test an isolated merge helper.

Task-start profile choices consume separate `agentProfiles` options. The card currently replaces only
`settingsAgents`; source evidence therefore proves profile settings disappearance and disagreement
between projections, not loss of the task-start option. Assert that the live flattened projection is
unchanged by this ACK without inventing a task-launch or catalogue-fetching repair.

## Related implementation plans

- [MCP acknowledgement publication repair](../../../plans/custom-tui-mcp-ack-publication/plan.md).
- [Operator-registered ACP agents](../../../plans/custom-acp-agents/plan.md) retains its original scope.
- [Live registry discovery](../../../plans/agent-discovery-live-registry/plan.md) retains its completed scope.

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

## Session restore is negotiated per connection

Restore has two gates, and before this change an operator-registered ACP agent never reached the
second one:

1. **Declared:** `lifecycle.SessionManager.createOrLoadSession` attempts a restore only when
   `Runtime().SessionConfig.NativeSessionResume` is true and a stored provider session ID exists.
   Every built-in ACP agent except Droid sets it. `CustomACPAgent.Runtime()` left `SessionConfig` at
   its zero value, so every reconnect sent `session/new` and replaced the provider conversation.
2. **Negotiated:** `acp.Adapter.LoadSession` reads the capabilities the agent returned from
   `initialize` on that connection. It sends `session/resume` when
   `sessionCapabilities.resume` is advertised, falls back to `session/load` when resume returns
   method-not-found and `loadSession` is advertised, and returns the canonical
   `agent does not support session loading (LoadSession capability is false)` error when neither is
   advertised. The lifecycle classifies that error as a compatibility failure and sends
   `session/new`.

`CustomACPAgent.Runtime()` sets `SessionConfig{NativeSessionResume: true}`. The declared gate then
defers to the negotiated one, which already knows what this particular CLI supports. No other
`SessionConfig` field changes: `HistoryContextInjection` stays off because a restoring agent does
not need history injected, and `NewSessionOnWorkspaceRebind` stays off to match the built-in default.

### Evidence that motivated the change

A backend log from a real operator-registered agent showed `native_session_resume: false` together
with a stored `existing_session_id`, followed by `sending ACP session/new request`. The same CLI,
driven directly over stdio, advertised `loadSession: true` and `sessionCapabilities.resume`, and after
its process was killed accepted both `session/load` and `session/resume` for the exact ID that
`session/new` had returned; a follow-up prompt showed the conversation was intact. The ID was lost in
Kandev, not in the agent.

### Alternatives considered

| Option | What the operator does | Result for a CLI that cannot restore | Decision |
| --- | --- | --- | --- |
| Enable the declared gate for every ACP definition | Nothing | Adapter reports the capability mismatch; lifecycle sends `session/new` | Chosen |
| Per-definition "restore sessions" setting | Finds and enables a new control | Same as today | Rejected: duplicates the per-connection negotiation, and adds a stored field, API surface, and copy in six locales |
| Derive the gate from the capability probe's `load_session` | Nothing | Same as chosen | Rejected: the probe result is an in-memory cache that can be stale or missing at launch, while the adapter reads the live `initialize` response |

### Risk

The lifecycle recognizes only specific unknown-session error shapes after they cross the agentctl
WebSocket boundary as text. A CLI that answers an unknown ID with another shape (for example
JSON-RPC `-32002` with a custom message) is classified as inconclusive: the stored ID is kept and the
recovery failure is shown, per REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001. That is the existing
contract for built-in agents and is out of scope here.
