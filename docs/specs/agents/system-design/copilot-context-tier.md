---
status: current
system: agents
requirements:
  - REQ-AGENTS-COPILOT-CONTEXT-TIER-001
  - REQ-AGENTS-COPILOT-CONTEXT-TIER-002
  - REQ-AGENTS-COPILOT-CONTEXT-TIER-003
---

# Copilot Context Tier System Design

## Purpose and boundaries

The `agents` system owns provider capability normalization, CLI flag projection,
profile command preview, and ACP runtime configuration lifecycle. GitHub Copilot
CLI (`@github/copilot`) exposes context-window tier selection exclusively via
its process startup flag `--context <default|long_context>` while omitting a
matching option from ACP `configOptions` and rejecting runtime
`session/set_config_option` calls for `context_tier`.

This design bridges that gap inside Kandev's existing ACP compatibility and
lifecycle boundaries so every existing selector surface works without
provider-specific UI branching:

- **Owned here:** Copilot `context_tier` capability synthesis and native-wire
  precedence (`acpcompat` and `agentctl` Copilot ACP dialect), CLI/passthrough
  `--context` flag construction (`agents.CopilotACP`), profile command-preview
  `config_options` propagation, startup layer reconciliation, and idle
  live-session subprocess reload with `session/load` conversation continuity.
- **Adjacent contracts used without redefinition:**
  - [Dynamic Provider Model Options](../requirements/dynamic-provider-options.md)
    (`REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-001`, `004`) owns the generic
    `ModelConfigSelector`, `ModelConfigSheet`, and model-resolver HTTP shape.
  - [Session configuration feedback](../requirements/session-configuration-feedback.md)
    (`REQ-AGENTS-SESSION-CONFIG-FEEDBACK-001`) owns muted Task Chat confirmation,
    rejection, and retry notices.
  - [Copilot profile plugins](copilot-profile-plugins.md)
    (`REQ-AGENTS-COPILOT-PLUGINS-004`) owns `--plugin-dir` preflight and the
    replacement-before-handoff guarantee (`AC-AGENTS-COPILOT-PLUGINS-004.8`).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-COPILOT-CONTEXT-TIER-001` | [Components and responsibilities](#components-and-responsibilities), [Data and contracts](#data-and-contracts), [UI and mobile composition](#ui-and-mobile-composition) |
| `REQ-AGENTS-COPILOT-CONTEXT-TIER-002` | [Data and contracts](#data-and-contracts), [Control flow](#control-flow), [Persistence](#persistence) |
| `REQ-AGENTS-COPILOT-CONTEXT-TIER-003` | [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery), [Observability](#observability) |

## Components and responsibilities

1. **ACP Compatibility Normalizer (`apps/backend/internal/agentctl/acpcompat/copilot.go`)**
   - Defines canonical constants (`CopilotContextTierOptionID = "context_tier"`,
     `CopilotContextTierCategory = "context_window"`,
     `CopilotContextTierDefault = "default"`,
     `CopilotContextTierLong = "long_context"`).
   - Provides `NormalizeCopilotConfigOptions(options []types.SessionConfigOption, activeContextTier string) ([]types.SessionConfigOption, bool)`
     which returns the normalized slice and whether `context_tier` was
     synthesized (vs natively supplied by Copilot).
   - Provides `ParseCopilotContextTierFromArgs(args []string) (tier string, explicit bool)`
     and `IsValidCopilotContextTier(value string) bool`.

2. **agentctl ACP Transport Dialect & Session Updates (`apps/backend/internal/agentctl/server/adapter/transport/acp/`)**
   - `copilotDialect` inspects adapter launch `cfg.Args` via
     `acpcompat.ParseCopilotContextTierFromArgs` (defaulting to `"default"`) so
     every normalized snapshot reflects the live subprocess's `--context` flag.
   - `adapter_session.go` (`initialSessionConfigState` and
     `emitAuthoritativeConfigOptionsWithPolicy`) and `adapter_updates.go`
     (`ConfigOptionUpdate` handling) pass converted ACP `configOptions` through
     `a.dialect.sessionConfigOptions(...)`. This prevents post-`SetModel` /
     post-`SetConfigOption` snapshots from dropping synthesized options or
     resetting `context_tier.currentValue`.

3. **Host Utility ACP Probe (`apps/backend/internal/agent/utility/acp_executor.go`)**
   - Passes requested `config_options` into `CommandOptions` when spawning a
     sessionless probe and seeds `acpcompat.NormalizeConfigOptions` with the
     requested or CLI-derived `context_tier`.
   - Continues filtering `applyProbeConfigOptions` against raw wire
     `sessionResp.ConfigOptions` so synthesized `context_tier` never triggers a
     failing `session/set_config_option` RPC during settings discovery.

4. **Agent Command Builder & Preview (`apps/backend/internal/agent/agents/`, `apps/backend/internal/agent/settings/controller/agent_config.go`, `apps/web/app/settings/agents/[agentId]/profiles/[profileId]/command-preview-card.tsx`)**
   - Adds `ConfigOptions map[string]string` to `agents.CommandOptions`,
     `agents.PassthroughOptions`, backend `CommandPreviewRequest`, and frontend
     `CommandPreviewRequest` / `CommandPreviewCardProps`.
   - `CopilotACP.BuildCommand` and `CopilotACP.BuildPassthroughCommand` (plus
     `MockAgent` when impersonating `copilot-acp` in E2E) append
     `--context <tier>` when `ConfigOptions["context_tier"]` is `"default"` or
     `"long_context"` and `CLIFlagTokens` do not already specify `--context` or
     `--context=...`.

5. **Lifecycle Manager (`apps/backend/internal/agent/runtime/lifecycle/`)**
   - `manager_launch.go` (`buildAgentCommandWithContext` and
     `buildFreshAgentCommandWithProfile`) merges profile `ConfigOptions` with
     persisted `sessionRuntimeOverrides` (and any explicit restart override)
     before invoking `CommandBuilder`, ensuring fresh launches, pre-launch
     overrides, context resets, and process restarts include `--context <tier>`.
   - `session.go` (`applyProfileSessionLayers`, `applyRuntimeSessionLayers`,
     `restoreSessionRuntimeConfigWithClient`) skips sending `SetConfigOption`
     for synthesized `context_tier` when the running Copilot subprocess already
     launched at that tier.
   - `manager_interaction.go`, `manager_events.go`, and `copilot_context_tier.go`
     handle live `context_tier` mutations on `copilot-acp`:
     - If Copilot natively advertised `context_tier` on the wire (detected when
       the option is native rather than synthesized), delegates directly to
       `client.SetConfigOptionWithResult`.
     - Otherwise validates `value` (`default` | `long_context`), rejects with a
       readable `SessionConfigurationRejectedError` when a turn or reset holds
       admission, runs `preflightCopilotContextReload` (`prepareAgentRestart` +
       `ValidateLaunchWrapper`) when `value` differs from the running
       subprocess's startup `--context` flag, stages
       `context_tier.currentValue = value` in `execution.ModelState`, publishes
       `EventTypeSessionModels`, and returns `Confirmed: true, Effective: value`
       immediately without stopping the idle subprocess.
     - `preserveStagedCopilotContextTier` in `manager_events.go` overlays any
       staged synthesized `context_tier` onto incoming `session_models` events
       emitted by the still-running old subprocess (such as after live `SetModel`
       or `SetConfigOption("reasoning_effort", ...)`), preventing UI flicker.
     - `reconcileCopilotContextTierBeforePrompt` runs before prompt dispatch in
       both `PromptAgentWithDispatchCallbackAndSubmissionID` and
       `PromptAgentWithAdmissionCallbackAndSubmissionID`, performing at most one
       conversation-preserving subprocess reload (`session/load`, or
       `session/new` on 0-turn session-not-found) when the effective
       `context_tier` differs from the running subprocess's startup `--context`
       flag. `ResetAgentContext` falls back to `RestartAgentProcess` when a
       `context_tier` change is staged so the reset process launches with the
       staged `--context` flag.

## Data and contracts

### Normalized `SessionConfigOption` shape

When synthesized by `acpcompat` for `copilot-acp`:

```json
{
  "type": "select",
  "id": "context_tier",
  "name": "Context Window",
  "category": "context_window",
  "description": "Controls whether Copilot launches with the model's standard or extended context window.",
  "currentValue": "default",
  "options": [
    {
      "value": "default",
      "name": "Default",
      "description": "Standard context window."
    },
    {
      "value": "long_context",
      "name": "Long Context",
      "description": "Extended context window for supported models."
    }
  ]
}
```

- **Insertion order:** Placed immediately after `reasoning_effort` when present
  (otherwise immediately after `model`), keeping `allow_all` last.
- **Native wire detection:** If incoming `options` already contain an option
  with `id` matching `context_tier`, `contextTier`, or `context` (or
  `category == "context_window"`), `acpcompat` leaves the incoming slice
  untouched. To distinguish Kandev-synthesized options from future native
  Copilot options without changing public DTO schemas, lifecycle checks whether
  the running Copilot execution's CLI args match the requested tier and only
  intercepts `context_tier` on `copilot-acp` when `SetConfigOption` is not
  natively mutable (or via a lightweight internal helper checking whether
  Copilot supplied a native variant). Specifically, `acpcompat` stamps internal
  `Description` / helper predicate or `agentctl` handles native vs synthesized
  transparently; keeping synthesis awareness in lifecycle + `acpcompat` avoids
  adding wire fields while remaining forward-compatible if Copilot adds native
  `session/set_config_option("context_tier", ...)` support later (we can even
  attempt native `SetConfigOption` only when native, or detect synthesized via
  a bounded constant/predicate). Even simpler and completely deterministic:
  `agentctl`'s `copilotDialect` records `synthesizedContextTier bool` on the
  dialect/adapter when `NormalizeCopilotConfigOptions` synthesized the option!
  If Copilot ever ships native `context_tier` on the wire, `synthesizedContextTier`
  is `false` and `SetConfigOption` succeeds natively in-process; while
  `synthesizedContextTier` is `true`, `agentctl` returns a typed/structured
  unsupported-in-place error or lifecycle intercepts synthesized `context_tier`
  before RPC dispatch. Having lifecycle check `isSynthesizedCopilotContextTier`
  (and `agentctl` reject direct un-reloaded RPCs with a clear invalid-params
  message if ever called directly) keeps both layers consistent.

### Command preview HTTP contract

`POST /api/v1/agent-command-preview/:agentName` accepts optional
`config_options`:

```json
{
  "model": "gpt-6.2-sol",
  "config_options": {
    "context_tier": "long_context",
    "reasoning_effort": "high"
  },
  "permission_settings": { "auto_approve": false },
  "cli_passthrough": false,
  "cli_flags": [],
  "command_prefix": "",
  "copilot_selected_plugins": []
}
```

No database migration is required; `agent_profiles.config_options`,
`workflow_steps` session-config rules, and `task_sessions.config_options`
already persist `map[string]string` JSON maps.

## Control flow

### 1. Settings discovery and profile command preview

1. Browser opens a Copilot profile or workflow session-override editor.
2. Baseline (`GET /api/v1/agent-models/copilot-acp`) and model-aware probes
   (`POST /api/v1/agent-models/copilot-acp/resolve`) normalize raw ACP
   `configOptions` through `acpcompat`, returning `context_tier` alongside
   `reasoning_effort` and `allow_all`.
3. Selecting `Long Context` updates `draft.configOptions.context_tier`.
4. `CommandPreviewCard` includes `config_options: draft.configOptions` in
   `POST /api/v1/agent-command-preview/copilot-acp`, which passes
   `ConfigOptions` to `CopilotACP.BuildCommand` / `BuildPassthroughCommand` and
   returns `copilot --acp --model <m> --context long_context` (or `npx -y ...`).

### 2. Fresh task launch and pre-launch overrides

1. Before a session starts, `httpSetSessionConfigOption` records
   `task_sessions.config_options["context_tier"] = "long_context"` and returns
   `status: "saved_for_launch"`.
2. During `StartAgentProcess`, `buildAgentCommandWithContext` merges
   `profileInfo.ConfigOptions` + `sessionRuntimeOverrides.ConfigOptions` and
   passes the merged map in `agents.CommandOptions.ConfigOptions`.
3. `CopilotACP.BuildCommand` emits `--context long_context`.
4. `agentctl` starts `copilot --acp --context long_context`; `copilotDialect`
   parses `--context long_context` from `cfg.Args` and normalizes `session/new`
   `configOptions` with `context_tier.currentValue = "long_context"`.
5. `applyProfileSessionLayers` and `applyRuntimeSessionLayers` observe that
   `execution`'s active `context_tier` already equals `"long_context"` and skip
   redundant `SetConfigOption("context_tier", ...)` RPC calls.

### 3. Live Task Chat lazy staging and pre-prompt reload

1. User selects `Long Context` (optionally together with a new `model` and
   `reasoning_effort`) in Task Chat while Copilot is idle.
2. `applyModelConfiguration` (`apps/web/lib/apply-model-configuration.ts`)
   executes sequentially without blocking on any subprocess restart:
   - `applyBaseModel(model)` -> confirms new `model` on the current process.
   - `applyOption("context_tier", "long_context")`:
     a. `applySynthesizedCopilotContextTier` acquires `sessionConfigurationMu`
        then `acquireContextResetOperation` (matching `SetSessionModel` lock
        order), verifies `expectedModelID` when supplied, and validates `value`
        (`default` | `long_context`).
     b. Verifies the execution is initialized and idle (`AgentStatusReady` or
        `AgentStatusCompleted`, no pending dispatched prompt, active tool, or
        tracked turn activity). If busy, returns
        `SessionConfigurationRejectedError`
        (`"set config option failed: cannot change Copilot context length while a turn is running"`).
     c. When `value` differs from `launchedExecutionCopilotContextTier(execution)`,
        runs `preflightCopilotContextReload` (`prepareAgentRestart` +
        `client.ValidateLaunchWrapper` + `prepareCursorMCPAuth`) **before**
        accepting the change so broken wrapper/plugin flags fail immediately.
     d. Stages `context_tier.currentValue = value` in `execution.ModelState` via
        `stageCopilotContextTierState`, publishes `EventTypeSessionModels`, and
        returns `Confirmed: true, Effective: value` in `< 1ms` while leaving the
        idle subprocess untouched.
   - `applyOption("reasoning_effort", ...)` runs directly against the still-live
     subprocess; `preserveStagedCopilotContextTier` in `manager_events.go`
     preserves the staged `context_tier` on the resulting `session_models`
     stream snapshot.
3. `httpSetSessionConfigOption` persists `task_sessions.config_options["context_tier"] = value`
   and returns `status: "applied"`, producing the muted Task Chat confirmation
   message immediately. If the user toggles `Long Context -> Default` back to
   the running subprocess's startup tier before sending a message, zero
   subprocess restarts occur.
4. When the next prompt turn begins (`PromptAgentWithDispatchCallbackAndSubmissionID`
   or `PromptAgentWithAdmissionCallbackAndSubmissionID`),
   `reconcileCopilotContextTierBeforePrompt` checks `needsCopilotContextTierReload`:
   - Fast path: returns `nil` immediately when the execution is not `copilot-acp`
     or its effective `context_tier` already matches `execution.AgentArgs` and
     the ACP session is initialized.
   - Slow path: acquires `remoteInstanceLifecycleMu` ->
     `acquireContextResetExclusive` (same lock order as `RestartAgentProcess`),
     re-checks idle state and `needsCopilotContextTierReload`, closes
     update/workspace streams (`CloseUpdatesStream`, `CloseWorkspaceStream`),
     stops only the child Copilot subprocess via `client.Stop(ctx)`, starts the
     replacement `copilot --acp --context <value>` subprocess via `agentctl`
     (checking `client.Health` before `WaitForReady` to avoid unnecessary poll
     delays), reconnects streams under a new `deliveryStartupAttempt`
     generation, restores the conversation via `LoadSession` (falling back to
     `NewSession` only on 0-turn session-not-found), restores `Model`, `Mode`,
     and remaining `ConfigOptions`, and marks `AgentStatusReady` without
     publishing `AgentBootReady` (preserving orchestrator `RUNNING` state) or
     `AgentContextReset` before dispatching the prompt.

## UI and mobile composition

No new route, drawer, or bespoke Copilot control is introduced:

- **Desktop & phone selector:** `ModelConfigSelector` (`apps/web/components/model-config-selector.tsx`)
  already renders every non-`mode`, non-`model` `SelectConfigOption` via
  `usableConfigOptions`. On desktop it opens a `Popover`; on phone (`isMobile`
  via `useResponsiveBreakpoint`) it opens `ModelConfigSheet`
  (`apps/web/components/model-config-sheet.tsx`), an inset bottom `Drawer`
  (`max-h-[90dvh]`, single `overflow-y-auto overscroll-contain` scroll body,
  safe-area bottom padding, and 44px touch targets).
- **Agent Profile Command Preview:** `CommandPreviewCard`
  (`apps/web/app/settings/agents/[agentId]/profiles/[profileId]/command-preview-card.tsx`)
  adds `configOptions?: Record<string, string>` to its props and request key so
  selecting `Long Context` updates the previewed CLI string immediately on both
  desktop and phone settings layouts without horizontal page overflow.

## Failure and recovery

- **Invalid `context_tier` value:** Rejected with
  `SessionConfigurationRejectedError` (`status: "rejected"`) before touching the
  subprocess or SQLite runtime config.
- **Active turn in flight:** Non-blocking admission check rejects immediately
  with `SessionConfigurationRejectedError`; the active turn continues
  uninterrupted and the user can click `Retry` in Task Chat once idle.
- **Plugin or launch-wrapper preflight failure:** Fails before `client.Stop`,
  preserving the live Copilot subprocess, `ACPSessionID`, and conversation
  (`AC-AGENTS-COPILOT-PLUGINS-004.8`).
- **0-turn `session/load` miss:** Copilot writes `events.jsonl` lazily on the
  first turn; when `LoadSession` reports session-not-found on a 0-turn session,
  Kandev falls back to `NewSession` transparently since no turn history exists
  yet.
- **Post-stop subprocess or `LoadSession` failure (non-not-found):** Restores
  the previous `ACPSessionID` and `ModelState` on the execution, surfaces an
  execution error, and aborts prompt dispatch before `beforeAdmission` runs so a
  subsequent prompt retry detects `!execution.isSessionInitialized()` and
  recovers cleanly via `session/load`.

## Persistence

- Uses existing `agent_profiles.config_options`, workflow step session-override
  `config_options`, and `task_sessions.config_options` JSON columns.
- Precedence at launch/restart:
  `profile.config_options` < `workflow pre-launch config_options` <
  `task_sessions.config_options` < explicit user `cli_flags` (`--context`).
- Retained `agentctl` adoption across backend restart preserves `cfg.Args` in
  the running `agentctl` process, keeping `context_tier.currentValue` accurate.

## Security

- Only the closed enum `default` | `long_context` is ever emitted as a
  `--context` argument token; arbitrary user strings in `config_options` cannot
  inject extra CLI flags.
- Uses existing session execution authorization (`ScopeSessionExec` /
  `AuthorizeSessionAccess`) and agent-settings authorization; no new endpoints
  or elevated privileges are added.

## Observability

- Structured lifecycle logs (`component: "lifecycle"`) record Copilot
  context-tier reload start, `session/load` vs 0-turn `session/new` outcome,
  duration, and any preflight or reload failure with `execution_id` and
  `session_id` (never logging secrets or prompt contents).

## Related decisions

- [2026-10-28 Copilot Native Policy Authority](../../../decisions/2026-10-28-copilot-native-policy-authority.md)
- [2026-11-05 Copilot Plugin Contract Admission](../../../decisions/2026-11-05-copilot-plugin-contract-admission.md)
