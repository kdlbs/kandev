# 0048 — Plugins invoke a plugin-selected utility agent

- Status: accepted
- Date: 2026-07-21
- Area: backend, frontend, protocol
- Related: [0043 — Plugin host data API](0043-plugin-host-data-api.md),
  [0002 — Host utility agentctl for sessionless ACP flows](0002-host-utility-agentctl-for-sessionless-flows.md)
  (the inference tier this reuses),
  [0018 — Runtime settings overrides](0018-runtime-settings-overrides.md)

## Context

A plugin that wants to do an LLM step (summarize a conversation, classify an
issue) has no sanctioned way to run a completion: it would need to ship and
manage its own provider API key. Kandev already runs LLM completions on the
operator's configured agents; a plugin should be able to borrow one.

Kandev also already has the right primitive. ADR 0002's **host-utility tier**
(`internal/agent/hostutility.Manager.ExecutePrompt`) runs a one-shot,
non-interactive, sessionless completion against a warm agentctl instance —
exactly what title generation, commit messages, and the Slack assistant use.
There is no need for a new agent loop, and the interactive agent runtime
(`internal/agent/runtime`, `internal/office`) is the wrong tool: it is
streaming, stateful, and requires an executor + workspace + worktree.

## Decision

Add a capability-gated `Host.InvokeUtilityAgent`, backed by each plugin's
configuration and the existing host-utility tier.

1. **Plugin config: `utility_agent`.** Plugins declaring `agent_invoke` declare
   a `config_schema` field named `utility_agent`, with `type: string` and
   `format: utility-agent`. Settings > Plugins renders it as a picker of the
   existing built-in and custom utility agents. The picker displays the name
   but stores the selected agent's stable ID, scoped to the plugin; no global
   user setting or agent profile is involved.

2. **New capability `agent_invoke`.** A boolean `Capabilities.AgentInvoke`,
   enforced exactly like `state`/`secrets`: `Host.InvokeUtilityAgent` returns
   gRPC `PermissionDenied` (`capability 'agent_invoke' not declared`) when the
   manifest doesn't declare it.

3. **New RPC + SDK method.**
   `InvokeUtilityAgent(InvokeUtilityAgentRequest{prompt}) returns
   (InvokeUtilityAgentResponse{text})` on `service Host`; SDK
   `Host.InvokeUtilityAgent(ctx, prompt string) (string, error)`. The request
   message is the forward-compatible extension point (a future `system_prompt`
   or `max_tokens` is an added proto field, no SDK signature change).

4. **Reuse the host-utility tier (ADR 0002).** The kandev-side handler:
   gate `agent_invoke` → read the calling plugin's `utility_agent` config →
   resolve its ID through the established utility-agent configuration → call a
   narrow `utilityRunner.ExecutePrompt(ctx, agentType, model,
   "", prompt)` and return the text. `utilityRunner` is a thin
   `pluginsHostUtilityAdapter` over `hostutility.Manager` wired in `backendapp`,
   so `internal/plugins` never imports the agent runtime (the same
   cycle-avoidance as the Slack assistant's adapter and ADR 0043's data
   sources). No task, session, workspace, or worktree is involved.

5. **Typed "not configured" failure.** If no utility agent is selected — or the
   selected utility agent has since been deleted or disabled — the handler returns gRPC
   `FailedPrecondition` (`no utility agent configured` /
   `configured utility agent "<id>" not found`), never a silent empty
   completion. A stale selection is treated as "unconfigured", not an internal
   error.

## Consequences

- A plugin declares `capabilities.agent_invoke: true` and calls
  `host.InvokeUtilityAgent(ctx, prompt)` — no API key, no provider wiring. This
  is what unblocks the "My Daily Standup" plugin's summarization step.
- The operator stays in control: nothing runs until each plugin selects a
  configured utility agent, including its cost and availability characteristics.
- We reused the sessionless inference path instead of building an agent loop;
  the only net-new machinery is a plugin config picker and one gated RPC handler.
- The `InvokeUtilityAgentRequest`/`Response` proto is a public contract, extended
  additively.

## Alternatives considered

- **Let plugins bring their own API key.** Rejected: every plugin re-implements
  key management and secret storage, and the operator loses cost control. The
  whole point is to delegate to kandev's already-configured agent.
- **Run the completion through `internal/agent/runtime` / `office`.** Rejected:
  both are streaming, stateful, and require an executor + workspace; neither
  offers a synchronous prompt→text call. ADR 0002's host-utility tier already
  does exactly this, sessionlessly.
- **Reuse `default_utility_agent_id`.** Rejected: that default serves Kandev's
  internal utility calls and cannot express a plugin-specific selection.
