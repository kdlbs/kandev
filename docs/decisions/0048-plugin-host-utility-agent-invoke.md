# 0048 — Plugins invoke a selected direct profile or utility agent

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

1. **Plugin config: direct profile first.** Plugins declaring `agent_invoke`
   can declare an `agent_profile` field with `type: string` and
   `format: agent-profile`. Settings > Plugins renders enabled, global,
   non-CLI profiles and stores the selected stable ID. A missing, deleted,
   disabled, CLI-passthrough, or workspace-scoped profile is a
   `FailedPrecondition`. When an `agent_profile` field is declared, it is the
   direct execution selection and takes precedence over `utility_agent`.
   Existing plugins may continue declaring `utility_agent` with
   `format: utility-agent`; that legacy selector resolves the utility agent's
   effective profile as before.

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
   gate `agent_invoke` → read the calling plugin's direct profile when declared
   (otherwise resolve the legacy utility agent) → validate direct-profile
   eligibility → call the narrow profile-aware utility runner and return the
   text. `utilityRunner` is a thin
   `pluginsHostUtilityAdapter` over `hostutility.Manager` wired in `backendapp`,
   so `internal/plugins` never imports the agent runtime (the same
   cycle-avoidance as the Slack assistant's adapter and ADR 0043's data
   sources). No task, session, workspace, or worktree is involved.

5. **Typed "not configured" failure.** An absent direct selection, or a
   deleted/ineligible direct profile, returns gRPC `FailedPrecondition`
   (`no agent profile configured` / `configured agent profile "<id>" not
   found`). Legacy utility-agent failures retain their existing messages.
   Capability denial is evaluated before either lookup; runner failures remain
   operational errors rather than configuration failures.

## Consequences

- A plugin declares `capabilities.agent_invoke: true` and calls
  `host.InvokeUtilityAgent(ctx, prompt)` — no API key, no provider wiring. This
  is what unblocks the "My Daily Standup" plugin's summarization step.
- The operator stays in control: nothing runs until each plugin selects a
  configured direct profile or utility agent, including the effective
  profile's cost, availability, launcher, and permission characteristics.
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
