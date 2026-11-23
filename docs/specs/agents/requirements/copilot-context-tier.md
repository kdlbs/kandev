---
status: active
system: agents
created: 2026-11-23
owners:
  - kandev
---

# Copilot Context Tier Requirements

## Overview

GitHub Copilot CLI (`@github/copilot`) supports selecting a model's standard or
extended context window through its startup `--context <default|long_context>`
argument (`contextTier`), and preserves conversation transcripts across process
restarts through ACP `session/load`. However, Copilot's ACP wire surface omits
context window selection from `configOptions` and rejects runtime
`session/set_config_option` mutations for `context_tier` with JSON-RPC `-32602`.

As a result, Kandev users configuring a Copilot agent profile, workflow session
override, pre-launch task session, or live Task Chat model selector cannot view
or choose Copilot's context length when selecting a model.

The `agents` system owns this contract because it owns provider capability
normalization, CLI command construction, profile command preview, and ACP
session configuration lifecycle. Shared desktop and phone selector presentation
continues to follow [Dynamic Provider Model Options](dynamic-provider-options.md)
and [Session configuration feedback](session-configuration-feedback.md).

## Terminology

- **Context tier:** Copilot's context-window selection (`default` for standard
  context or `long_context` for extended context on eligible models), exposed in
  Kandev as normalized option `context_tier`.
- **Synthesized context-tier option:** The normalized `select` option added by
  Kandev's Copilot compatibility layer when the Copilot ACP runtime omits a
  native context-tier entry from `configOptions`.
- **Idle conversation-preserving reload:** Stopping only the child Copilot ACP
  subprocess while no turn is active, starting a replacement Copilot subprocess
  with the updated `--context` flag, and restoring prior conversation state via
  ACP `session/load` (or `session/new` when zero turns were persisted).

## Confirmation and design boundaries

**Confirmed by the user:**

- Users must be able to select Copilot's context length when choosing a Copilot
  model.
- When changing Copilot's context length in an already-started session, Kandev
  shall automatically reload the Copilot subprocess when the session is idle and
  preserve the existing conversation via `session/load` (without interrupting an
  active turn).

## Requirements

### REQ-AGENTS-COPILOT-CONTEXT-TIER-001: Normalized Copilot context window option

**Intent:** Users can discover and select Copilot's context tier alongside model
and reasoning options across settings, workflow rules, pre-launch pickers, and
live sessions on desktop and phone.

**User story:** As a developer using GitHub Copilot in Kandev, I want to choose
between `Default` and `Long Context` whenever I select a Copilot model, so that
long-horizon coding and review sessions can use extended model context windows.

#### Acceptance criteria

- **AC-AGENTS-COPILOT-CONTEXT-TIER-001.1:** When a Copilot ACP capability probe
  (`GET /api/v1/agent-models/copilot-acp`,
  `POST /api/v1/agent-models/copilot-acp/resolve`, or
  `POST /api/v1/tasks/:id/sessions/:sessionId/resolve-model-config`) or live
  Copilot ACP session omits a native context-tier option on the wire, Kandev
  shall include a normalized `select` option with `id: "context_tier"`,
  `name: "Context Window"`, `category: "context_window"`, and ordered values
  `default` (`Default`) and `long_context` (`Long Context`).
- **AC-AGENTS-COPILOT-CONTEXT-TIER-001.2:** When a Copilot ACP runtime natively
  advertises a context-tier option (`context_tier`, `contextTier`, or `context`)
  in ACP `configOptions`, Kandev shall preserve the provider's native option
  definition, choices, and runtime mutability without duplicating or overriding
  it.
- **AC-AGENTS-COPILOT-CONTEXT-TIER-001.3:** Authoritative post-mutation
  `configOptions` responses (such as after changing `model` or
  `reasoning_effort`) and `ConfigOptionUpdate` stream notifications on a Copilot
  ACP session shall retain `context_tier` with the running subprocess's active
  `currentValue` (`default` or `long_context`).
- **AC-AGENTS-COPILOT-CONTEXT-TIER-001.4:** Desktop and phone model selectors in
  Agent Profile settings, Workflow session-override editors, pre-launch session
  pickers, and live Task Chat shall expose and apply `context_tier` through the
  shared `ModelConfigSelector` and `ModelConfigSheet` contracts without
  hardcoding Copilot option IDs in React components, preserving 44px phone touch
  targets, single vertical scroll ownership, and zero document horizontal
  overflow.

### REQ-AGENTS-COPILOT-CONTEXT-TIER-002: Launch, preview, and restart flag propagation

**Intent:** Copilot subprocesses and command previews consistently receive the
selected `--context` CLI argument across ACP, passthrough, pre-launch, and
restart paths.

**User story:** As a user configuring a Copilot profile or launching a task, I
want my chosen context tier to pass `--context <tier>` to Copilot and appear in
the profile command preview, so that launched processes actually run with the
configured context window.

#### Acceptance criteria

- **AC-AGENTS-COPILOT-CONTEXT-TIER-002.1:** When building a `copilot-acp`
  command for ACP execution or CLI passthrough and effective
  `config_options["context_tier"]` is `"default"` or `"long_context"`, Kandev
  shall append `--context <value>` to the Copilot argument vector; when
  `context_tier` is unset or empty, Kandev shall omit `--context`.
- **AC-AGENTS-COPILOT-CONTEXT-TIER-002.2:** When user-configured profile CLI
  flags already contain `--context` or `--context=<value>`, Kandev shall keep
  the explicit CLI flag authoritative without emitting a second `--context`
  pair. Unsupported `context_tier` values outside `default` and `long_context`
  shall be ignored during argv construction and rejected on live mutation.
- **AC-AGENTS-COPILOT-CONTEXT-TIER-002.3:** Agent Profile command preview
  (`POST /api/v1/agent-command-preview/copilot-acp` and `CommandPreviewCard`)
  shall include the draft `config_options` in its preview request and render the
  resulting `--context <value>` flag in both ACP and CLI passthrough modes.
- **AC-AGENTS-COPILOT-CONTEXT-TIER-002.4:** Fresh ACP task launch, pre-launch
  session overrides (`saved_for_launch`), workflow pre-launch configuration,
  context reset, and process restart shall merge persisted session runtime
  `config_options` over profile `config_options` before constructing the Copilot
  launch command, and startup session-layer application shall treat a
  synthesized `context_tier` matching the launched subprocess as satisfied
  without issuing an unsupported `session/set_config_option` RPC.

### REQ-AGENTS-COPILOT-CONTEXT-TIER-003: Idle live-session reload with conversation continuity

**Intent:** Changing Copilot's context tier in an idle live session applies
immediately by reloading the Copilot subprocess with `--context <value>` while
preserving conversation history and active session settings.

**User story:** As a user in an active Task Chat conversation with Copilot, I
want switching `Context Window` while Copilot is idle to reload Copilot with the
new context tier and keep my existing conversation intact, so that I do not lose
context or have to start a new task session.

#### Acceptance criteria

- **AC-AGENTS-COPILOT-CONTEXT-TIER-003.1:** When a synthesized `context_tier` is
  requested on an initialized Copilot ACP session whose running subprocess
  already uses that tier, Kandev shall confirm `effective: <value>` and
  `confirmed: true` without restarting the subprocess.
- **AC-AGENTS-COPILOT-CONTEXT-TIER-003.2:** When a synthesized `context_tier`
  changes to a valid tier (`default` or `long_context`) on an idle initialized
  Copilot ACP session, Kandev shall validate replacement launch arguments, stop
  only the child Copilot subprocess via `agentctl`, launch a replacement
  `copilot --acp --context <value>` subprocess, restore prior conversation state
  via `session/load` (or initialize via `session/new` when zero turns were
  persisted by Copilot), restore effective `model`, `mode`, and remaining
  `config_options`, persist `context_tier`, and return a confirmed
  `SessionConfigurationResult` without emitting an `AgentContextReset` event.
- **AC-AGENTS-COPILOT-CONTEXT-TIER-003.3:** When replacement command validation
  or explicit Copilot profile-plugin preparation fails before subprocess
  handoff, Kandev shall abort the reload and leave the existing Copilot
  subprocess running with its current session identity, conversation, and
  `context_tier`.
- **AC-AGENTS-COPILOT-CONTEXT-TIER-003.4:** When a prompt turn or context-reset
  operation is actively in flight on the Copilot execution, Kandev shall reject
  a live synthesized `context_tier` change with a readable
  `SessionConfigurationRejectedError` (`status: "rejected"`) without
  interrupting the active turn or mutating persisted session runtime
  `config_options`.
- **AC-AGENTS-COPILOT-CONTEXT-TIER-003.5:** When a user applies a composed live
  selection (`model`, `context_tier`, and dependent options such as
  `reasoning_effort`) from Task Chat, Kandev shall preserve the newly confirmed
  `model` across the `context_tier` subprocess reload and apply subsequent
  options against the reloaded subprocess, surfacing one coherent confirmation
  in Task Chat per `REQ-AGENTS-SESSION-CONFIG-FEEDBACK-001`.

## Out of scope

- Inferring or maintaining a hardcoded static allowlist of Copilot models that
  honor `long_context`; Copilot's CLI and backend remain authoritative for how
  `--context long_context` behaves on each selected model.
- Interrupting or cancelling an in-flight Copilot prompt turn to apply a live
  `context_tier` change.
- Reloading retained live CLI passthrough (PTY) sessions automatically when
  profile settings change; passthrough sessions consume `--context` at process
  launch and explicit passthrough reset.
- Provider-specific React branches in `ModelConfigSelector` or
  `ModelConfigSheet`.
