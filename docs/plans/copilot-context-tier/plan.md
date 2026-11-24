---
created: 2026-11-23
status: completed
requirements:
  - REQ-AGENTS-COPILOT-CONTEXT-TIER-001
  - REQ-AGENTS-COPILOT-CONTEXT-TIER-002
  - REQ-AGENTS-COPILOT-CONTEXT-TIER-003
system_design:
  - ../../specs/agents/system-design/copilot-context-tier.md
legacy_specs: []
---

# Implementation Plan: Copilot Context Tier Selection

## Overview

Expose GitHub Copilot's context-window tier (`default` / `long_context`) across
agent profile settings, workflow session-override rules, pre-launch session
pickers, CLI command preview, and live Task Chat on both desktop and phone.
Implementation runs in two ordered vertical slices:

1. **Task 01** normalizes `context_tier` in `acpcompat` and `agentctl`'s Copilot
   dialect (including post-mutation and `ConfigOptionUpdate` snapshots), wires
   `--context <default|long_context>` into `CopilotACP` / `MockAgent` command
   builders and host utility probes, and passes `config_options` through the
   profile `CommandPreviewCard`.
2. **Task 02** builds on Task 01 to merge profile and persisted runtime
   `ConfigOptions` into lifecycle launch/restart commands, skip unsupported
   startup `SetConfigOption("context_tier", ...)` RPCs when the subprocess
   already launched at that tier, reload idle live Copilot ACP subprocesses with
   `session/load` conversation continuity (plus 0-turn `session/new` fallback
   and active-turn rejection), and prove desktop and phone E2E flows.

This order lets Task 01 establish and unit-test the canonical option and argv
contracts before Task 02 integrates live subprocess replacement and browser E2E.

## Scope

### In scope

- Synthesizing `context_tier` (`Context Window`: `Default`, `Long Context`) for
  `copilot-acp` when omitted on the ACP wire, while deferring to any future
  native Copilot wire option.
- Preserving `context_tier` and its active subprocess `currentValue` across
  `session/new`, `session/load`, `SetModel` / `SetConfigOption` responses, and
  `ConfigOptionUpdate` stream notifications.
- Emitting `--context <default|long_context>` in `CopilotACP.BuildCommand` and
  `BuildPassthroughCommand` (respecting explicit user `--context` CLI flags).
- Forwarding `config_options` from `CommandPreviewCard` to
  `POST /api/v1/agent-command-preview/:agentName`.
- Merging profile and session runtime `ConfigOptions` before building launch and
  restart argv in lifecycle.
- Reloading an idle live `copilot-acp` subprocess via `agentctl` when a
  synthesized `context_tier` changes, restoring conversation state via
  `session/load` (with 0-turn `session/new` fallback), preserving `model`,
  `mode`, and remaining `config_options`, and rejecting while a turn is active.
- Desktop and phone (`mobile-chrome`) Playwright E2E coverage using the
  `KANDEV_E2E_MOCK_COPILOT` + `KANDEV_MOCK_COPILOT_NATIVE` harness.

### Out of scope

- Maintaining a static per-model `long_context` allowlist in Kandev.
- Interrupting an active prompt turn to apply `context_tier`.
- Live reload of retained PTY CLI passthrough sessions.
- Provider-specific option branching in `ModelConfigSelector` or
  `ModelConfigSheet`.

## Technical approach

### Slice 1: ACP compatibility, dialect updates, CLI argv, and command preview

- **`apps/backend/internal/agentctl/acpcompat/copilot.go` + `acpcompat.go`:**
  Add `NormalizeCopilotConfigOptions(options, activeContextTier)` and
  `ParseCopilotContextTierFromArgs(args)`. Insert `context_tier` (`type: "select"`,
  `category: "context_window"`, `options: ["default", "long_context"]`) right
  after `reasoning_effort` (or after `model` when `reasoning_effort` is absent)
  unless a native context option (`context_tier`, `contextTier`, `context`, or
  `category == "context_window"`) already exists.
- **`apps/backend/internal/agentctl/server/adapter/transport/acp/`:**
  Update `copilotDialect` (`dialect_copilot.go`) to parse active `context_tier`
  from adapter launch `cfg.Args` and track whether `context_tier` was
  synthesized. Route `emitAuthoritativeConfigOptionsWithPolicy`
  (`adapter_session.go`) and `ConfigOptionUpdate` (`adapter_updates.go`) through
  `a.dialect.sessionConfigOptions(...)` so model switches never drop
  `context_tier` or reset `currentValue`.
- **`apps/backend/internal/agent/utility/acp_executor.go`:**
  Pass `requestedConfigOptions` into `CommandOptions.ConfigOptions` when probing
  an agent/session model config and pass the requested/CLI `context_tier` into
  `acpcompat`.
- **`apps/backend/internal/agent/agents/`:**
  Add `ConfigOptions map[string]string` to `CommandOptions` and
  `PassthroughOptions`. Add shared helper `appendCopilotContextTierFlag` used by
  `CopilotACP.BuildCommand`, `CopilotACP.BuildPassthroughCommand`, and
  `MockAgent` when `a.ID() == CopilotACPAgentID`. Skip appending when
  `CLIFlagTokens` already contains `--context` or `--context=...`.
- **`apps/backend/internal/agent/settings/controller/agent_config.go` & `apps/web/`:**
  Add `ConfigOptions map[string]string` (`json:"config_options,omitempty"`) to
  `CommandPreviewRequest` (Go + TS `settings-api.ts` / `app/actions/agents.ts`)
  and pass `draft.configOptions` from `agent-profile-page.tsx` into
  `CommandPreviewCard`.

### Slice 2: Lifecycle launch merging, idle reload, and E2E proof

- **`apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`:**
  Merge `profileInfo.ConfigOptions`, persisted `sessionRuntimeOverrides`, and
  any explicit restart override into `CommandOptions.ConfigOptions` inside
  `buildAgentCommandWithContext` and `buildFreshAgentCommandWithProfile`.
- **`apps/backend/internal/agent/runtime/lifecycle/session.go`:**
  In `applyProfileSessionLayers`, `applyRuntimeSessionLayers`, and
  `restoreSessionRuntimeConfigWithClient`, treat a synthesized Copilot
  `context_tier` that already matches the running execution's `--context` launch
  value as satisfied without calling `client.SetConfigOption`.
- **`apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`, `manager_events.go`, and `copilot_context_tier.go`:**
  Intercept synthesized `context_tier` on `copilot-acp` inside
  `applySessionConfigOption`:
  - Reject invalid values and busy turns (`AgentStatusRunning`, pending
    dispatched prompt, active tool, or tracked activity) via
    `SessionConfigurationRejectedError`.
  - When idle, run `preflightCopilotContextReload` (`prepareAgentRestart` +
    `ValidateLaunchWrapper`) whenever `value` differs from the running
    subprocess's startup `--context` flag, stage `context_tier` in `ModelState`
    (`preserveStagedCopilotContextTier` in `manager_events.go` prevents stale
    `session_models` flicker), and return `Confirmed: true, Effective: value`
    immediately without stopping the idle subprocess.
  - Before dispatching the next prompt turn (`reconcileCopilotContextTierBeforePrompt`
    in both `PromptAgentWithDispatchCallbackAndSubmissionID` and
    `PromptAgentWithAdmissionCallbackAndSubmissionID`), replace the child
    subprocess once through `agentctl` when the effective `context_tier`
    differs from `execution.AgentArgs`, call `LoadSession(priorACPSessionID, ...)`
    (falling back to `NewSession` on 0-turn session-not-found), and restore
    `model`, `mode`, and remaining `config_options` without emitting
    `AgentBootReady` or `AgentContextReset`. `ResetAgentContext` falls back to
    `RestartAgentProcess` when a `context_tier` change is staged.
- **`apps/backend/cmd/mock-agent/`:**
  When `KANDEV_MOCK_COPILOT_NATIVE=true`, parse `--context <tier>` at startup,
  omit native `context_tier` from wire `configOptions` while rejecting
  `session/set_config_option("context_tier", ...)`, persist session turn state
  for `session/load` across subprocess restarts (while returning
  session-not-found on 0-turn sessions), and support `/e2e:copilot-context-info`
  reporting the subprocess's `--context` flag and prior turn count.

### Provider compatibility matrix

| Provider | Transport | Capability shape | Intended behavior | Verification evidence | Unsupported-shape fallback |
| --- | --- | --- | --- | --- | --- |
| `copilot-acp` (current `@github/copilot` 1.0.x) | ACP over `agentctl` | CLI `--context <default\|long_context>` + `session/load`; no wire `context_tier` | Synthesize `context_tier`, pass `--context`, reload idle subprocess with `session/load` | Go unit/lifecycle tests + desktop/mobile E2E | Invalid tier rejected; busy turn rejected with Retry |
| `copilot-acp` (future native ACP option) | ACP over `agentctl` | Native `context_tier` / `contextTier` / `context` in `configOptions` | Preserve native wire option and mutate directly via `session/set_config_option` | `acpcompat`, dialect, and lifecycle unit tests | Standard ACP rejection handling |
| `copilot-acp` (CLI passthrough) | PTY via `agentctl` | CLI `--context <default\|long_context>` | Pass `--context` on initial launch, preview, and passthrough reset | `copilot_acp_test.go` + `agent_config_test.go` | Retained PTY is not auto-reloaded |
| Other ACP providers (`opencode-acp`, `grok`, `claude-acp`, `codex-acp`, `devin-acp`) | ACP over `agentctl` | Provider-native `configOptions` (plus existing Grok shim) | Untouched; `NormalizeCopilotConfigOptions` runs only for `copilot-acp` | Existing `acpcompat` + adapter regression tests | Existing generic ACP behavior |

### Why this is the simplest sufficient solution

Reusing `acpcompat`, `ModelConfigSelector`, `applyModelConfiguration`, and
`prepareAgentRestart` requires zero database migrations, zero new HTTP routes,
and zero provider-specific branches in React selector components while
preserving Copilot plugins, model/reasoning selections, and conversation state.

## ASCII UI preview

### `UI-01: Copilot model configuration selector (Desktop Popover and Phone Drawer)`

Entry points:
- Agent Profile Settings (`Profile start model settings` trigger)
- Workflow Step Session Override rule (`Model` trigger)
- Task Chat composer toolbar (`ModelSelector` trigger, pre-launch and idle live)

Desktop composition (`!isMobile`, anchored `Popover`):

```text
+--------------------------------------------------+
| [ GPT-6.2 Sol * Long Context              v ]    | <-- Trigger (28px height)
+--------------------------------------------------+
| Search models...                             [Q] | <-- Fixed search input
+--------------------------------------------------+
| * GPT-6.2 Sol                         1x         | <-- Internal vertical
|   Powerful * Medium cost                         |     model list scroller
|   GPT-6.2 Astra                       1x         |
|   Gemini 4 Carbon                     1x         |
+--------------------------------------------------+
| Reasoning Effort        [ medium            v ]  | <-- Fixed secondary
| Context Window          [ Long Context      v ]  |     options + footer
| Allow All               [ Off               v ]  |     (28px desktop controls)
+--------------------------------------------------+
|                                      [  Apply  ] | <-- Primary action
+--------------------------------------------------+
```

Phone composition (`isMobile`, `ModelConfigSheet` inset bottom `Drawer`, `max-h-[90dvh]`):

```text
+--------------------------------------+
|       ( drawer drag handle )         | <-- Fixed drawer header
| Select model                     [X] |     ([X] close button: 44x44px)
+--------------------------------------+
| [ Search models...                 ] | <-- Single internal vertical scroller
|                                      |     (overflow-y-auto overscroll-contain,
| * GPT-6.2 Sol                   1x   |      16px input font, zero horizontal
|   Powerful * Medium cost             |      document overflow)
|   Gemini 4 Carbon               1x   |
| ------------------------------------ |
| Reasoning Effort                     |
| [ medium                         v ] | <-- Full-width touch select (>=44px)
| Context Window                       |
| [ Long Context                   v ] | <-- Synthesized context_tier select
| Allow All                            |     ("Default" | "Long Context", >=44px)
| [ Off                            v ] |
|                                      |
| [              Apply               ] | <-- Primary action (>=44px + safe-area)
+--------------------------------------+
```

### `UI-02: Copilot profile command preview card`

Entry point: `Settings -> Agents -> Copilot -> <Profile>` (`CommandPreviewCard`, desktop and phone stacked card):

```text
+------------------------------------------------------------------+
| >_ Command preview                                               |
|                                                                  |
| Flags configured here are passed to the ACP bridge.              |
| +--------------------------------------------------------------+ |
| | copilot --acp --model gpt-6.2-sol --context long_context [C] | |
| +--------------------------------------------------------------+ |
+------------------------------------------------------------------+
```

Structural requirements vs illustrative details:
- **Required (`AC-AGENTS-COPILOT-CONTEXT-TIER-001.1`, `001.4`, `002.3`, `003.5`):**
  `Context Window` appears in the existing secondary-options stack via generic
  `usableConfigOptions` (no hardcoded Copilot UI branch); phone uses the
  existing `ModelConfigSheet` bottom `Drawer` with one internal scroll region,
  `>=44px` touch targets, safe-area clearance, and zero document horizontal
  overflow; `CommandPreviewCard` reflects `--context <tier>` when `draft.configOptions`
  changes; Task Chat displays one muted system confirmation (or readable busy
  rejection with `Retry`) under `REQ-AGENTS-SESSION-CONFIG-FEEDBACK-001`.
- **Illustrative:** Exact model names, cost badges, and ASCII widths.

## Tests

| Acceptance Criterion | Test File | Test Method / Case |
| --- | --- | --- |
| `AC-AGENTS-COPILOT-CONTEXT-TIER-001.1` | `apps/backend/internal/agentctl/acpcompat/copilot_test.go` | `TestNormalizeCopilotConfigOptions_SynthesizesContextTierWhenAbsent` |
| `AC-AGENTS-COPILOT-CONTEXT-TIER-001.1` | `apps/backend/internal/agentctl/server/utility/copilot_probe_context_tier_test.go` | `TestProbeACPSession_CopilotSynthesizesContextTierWithoutWireSetRPC` |
| `AC-AGENTS-COPILOT-CONTEXT-TIER-001.2` | `apps/backend/internal/agentctl/acpcompat/copilot_test.go` | `TestNormalizeCopilotConfigOptions_PreservesNativeContextOption` |
| `AC-AGENTS-COPILOT-CONTEXT-TIER-001.3` | `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_copilot_test.go` | `TestCopilotDialect_PreservesActiveContextTierAcrossAuthoritativeUpdates` |
| `AC-AGENTS-COPILOT-CONTEXT-TIER-001.4`, `002.3` | `apps/web/app/settings/agents/[agentId]/profiles/[profileId]/command-preview-card.test.tsx` | `forwards configOptions to command preview and refreshes when context_tier changes` |
| `AC-AGENTS-COPILOT-CONTEXT-TIER-002.1`, `002.2` | `apps/backend/internal/agent/agents/copilot_acp_test.go` | `TestCopilotACP_BuildCommand_ContextTierAndExplicitCLIFlagPrecedence`, `TestCopilotACP_BuildPassthroughCommand_ContextTier` |
| `AC-AGENTS-COPILOT-CONTEXT-TIER-002.3` | `apps/backend/internal/agent/settings/controller/command_preview_test.go` | `TestPreviewAgentCommand_CopilotContextTierACPAndPassthrough` |
| `AC-AGENTS-COPILOT-CONTEXT-TIER-002.4` | `apps/backend/internal/agent/runtime/lifecycle/copilot_context_tier_test.go` | `TestBuildAgentCommandMergesCopilotContextTierWithRuntimePrecedence`, `TestCopilotSessionLayersSkipMatchingSynthesizedContextTier` |
| `AC-AGENTS-COPILOT-CONTEXT-TIER-003.1` | `apps/backend/internal/agent/runtime/lifecycle/copilot_context_tier_test.go` | `TestCopilotContextTierNoOpAndValidation` |
| `AC-AGENTS-COPILOT-CONTEXT-TIER-003.2` | `apps/backend/internal/agent/runtime/lifecycle/copilot_context_tier_test.go` | `TestCopilotIdleLiveSessionContextTierReload`, `TestCopilotZeroTurnReloadFallsBackToNewSession` |
| `AC-AGENTS-COPILOT-CONTEXT-TIER-003.3` | `apps/backend/internal/agent/runtime/lifecycle/copilot_context_tier_test.go` | `TestCopilotReloadPreflightFailureLeavesLiveProcessUntouched` |
| `AC-AGENTS-COPILOT-CONTEXT-TIER-003.4` | `apps/backend/internal/agent/runtime/lifecycle/copilot_context_tier_test.go` | `TestCopilotContextTierRejectedWhileTurnActive` |
| `AC-AGENTS-COPILOT-CONTEXT-TIER-003.5` | `apps/backend/internal/task/handlers/process_handlers_configuration_feedback_test.go` | `TestHTTPSetSessionConfigOptionCopilotContextTierBusyRejectedWithoutPersisting` |

## E2E tests

| Flow | Covered Criteria | Playwright Spec | Project |
| --- | --- | --- | --- |
| Copilot profile editor selects `Long Context`, updates command preview with `--context long_context`, launches task, and switches `Context Window` in idle Task Chat while preserving prior turn history and composed model/reasoning options | `AC-AGENTS-COPILOT-CONTEXT-TIER-001.1`, `001.3`, `001.4`, `002.1`, `002.3`, `002.4`, `003.1`, `003.2`, `003.5` | `apps/web/e2e/tests/chat/copilot-context-tier.spec.ts` | `chromium` |
| Active Copilot turn rejects live `Context Window` switch with readable notice without interrupting the turn, then applies once idle while preserving completed turns | `AC-AGENTS-COPILOT-CONTEXT-TIER-003.4` | `apps/web/e2e/tests/chat/copilot-context-tier.spec.ts` | `chromium` |
| Phone (`ModelConfigSheet` bottom `Drawer`) selects `Long Context` in Copilot profile settings and live Task Chat, verifying `>=44px` touch targets, single internal scroll owner, zero horizontal overflow, and preserved conversation after reload | `AC-AGENTS-COPILOT-CONTEXT-TIER-001.4`, `002.3`, `003.2`, `003.5` | `apps/web/e2e/tests/chat/mobile-copilot-context-tier.spec.ts` | `mobile-chrome` |

## Work orders

- [x] [Task 01: Copilot context-tier discovery, CLI flags, and command preview](task-01-copilot-context-tier-discovery-and-flags.md)
- [x] [Task 02: Copilot session launch and conversation-preserving idle reload](task-02-copilot-session-launch-and-idle-reload.md)

## Verification results

- `bash scripts/check-agentctl-deps.sh` — passed (`all release agentctl dependency graphs exclude task/service and Kubernetes packages`)
- `env -u GIT_CONFIG_COUNT TMPDIR=/private/tmp go test -C apps/backend -race -count=1 ./internal/agentctl/acpcompat ./internal/agentctl/server/adapter/... ./internal/agentctl/server/utility ./internal/agent/agents ./internal/agent/settings/controller ./internal/agent/settings/handlers ./internal/agent/runtime/lifecycle ./internal/task/handlers ./cmd/mock-agent -run 'Copilot|ConfigOption|CommandPreview'` — passed
- `bash scripts/lint-go-changed <all touched Go files>` — passed (`0 issues.`)
- `(cd apps/web && pnpm exec vitest run app/settings/agents/\\[agentId\\]/profiles/\\[profileId\\]/command-preview-card.test.tsx)` — passed (`7 passed`)
- `(cd apps && pnpm --filter @kandev/web lint)` — passed (`0 warnings`)
- `(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)` — passed
- `(cd apps/web && pnpm exec playwright test --config e2e/playwright.config.ts --project=chromium --project=mobile-chrome --project=containers --list --reporter=json)` — passed (`0 discovery errors`)
- `scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --no-build e2e/tests/chat/copilot-context-tier.spec.ts e2e/tests/chat/copilot-native-compaction.spec.ts -- --workers=1 --retries=0` — passed (`5 passed`)
- `scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --no-build --project mobile-chrome e2e/tests/chat/mobile-copilot-context-tier.spec.ts` — passed (`1 passed`)
- `python3 scripts/list-docs.py validate` & `python3 scripts/lint-spec-files.py --all` — passed

## Risks

- **Authoritative snapshot overwrite on model switch:** Copilot returns a full
  `configOptions` list (omitting `context_tier`) from `session/set_config_option("model", ...)`.
  Mitigated in Task 01 by routing `emitAuthoritativeConfigOptionsWithPolicy` and
  `ConfigOptionUpdate` through `a.dialect.sessionConfigOptions` seeded with the
  subprocess's launch `--context` tier.
- **0-turn reload before `events.jsonl` exists:** Copilot writes session files
  lazily on the first prompt turn. Mitigated in Task 02 by falling back from
  `LoadSession` to `NewSession` on session-not-found errors while re-applying
  `model`, `mode`, and remaining `config_options`.
- **Lock ordering during live reload:** `PromptAgent` holds
  `contextResetAdmissionMu.RLock()`. Mitigated in Task 02 by acquiring
  `remoteInstanceLifecycleMu` before `sessionConfigurationMu` and using
  non-blocking `contextResetAdmissionMu.TryLock()` to reject immediately when a
  turn is active instead of deadlocking or blocking behind a long turn.
