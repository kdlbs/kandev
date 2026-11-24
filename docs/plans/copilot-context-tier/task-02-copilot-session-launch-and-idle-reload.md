---
id: "02-copilot-session-launch-and-idle-reload"
title: "Copilot session launch and conversation-preserving idle reload"
status: completed
wave: 2
depends_on:
  - "01-copilot-context-tier-discovery-and-flags"
plan: "plan.md"
requirements:
  - REQ-AGENTS-COPILOT-CONTEXT-TIER-001
  - REQ-AGENTS-COPILOT-CONTEXT-TIER-002
  - REQ-AGENTS-COPILOT-CONTEXT-TIER-003
acceptance_criteria:
  - AC-AGENTS-COPILOT-CONTEXT-TIER-001.4
  - AC-AGENTS-COPILOT-CONTEXT-TIER-002.4
  - AC-AGENTS-COPILOT-CONTEXT-TIER-003.1
  - AC-AGENTS-COPILOT-CONTEXT-TIER-003.2
  - AC-AGENTS-COPILOT-CONTEXT-TIER-003.3
  - AC-AGENTS-COPILOT-CONTEXT-TIER-003.4
  - AC-AGENTS-COPILOT-CONTEXT-TIER-003.5
system_design:
  - ../../specs/agents/system-design/copilot-context-tier.md
---

# Task 02: Copilot session launch and conversation-preserving idle reload

## Summary

Propagate profile and persisted session runtime `ConfigOptions` into lifecycle
Copilot command construction on launch, context reset, and process restart, skip
redundant startup `SetConfigOption("context_tier", ...)` RPCs when the launched
subprocess already matches, stage idle live Copilot `context_tier` changes
immediately without stopping the subprocess, and reconcile the Copilot ACP
subprocess once before the next prompt via `agentctl` + `session/load` (with
0-turn `session/new` fallback and active-turn rejection). Extend `mock-agent`'s
Copilot native wire fixture and verify desktop and phone (`mobile-chrome`) flows
end to end.

## In scope

- Merge `profileInfo.ConfigOptions`, persisted `sessionRuntimeOverrides`, and
  optional restart overrides into `agents.CommandOptions.ConfigOptions` in
  `buildAgentCommandWithContext` and `buildFreshAgentCommandWithProfile`
  (`manager_launch.go`), and pass `runtimeConfigOverride` through
  `prepareAgentRestart` (`manager_interaction.go`).
- Skip sending `SetConfigOption("context_tier", ...)` during
  `applyProfileSessionLayers`, `applyRuntimeSessionLayers`, and
  `restoreSessionRuntimeConfigWithClient` (`session.go`) when a synthesized
  Copilot `context_tier` already matches the running execution's `--context`
  launch value.
- Implement lazy staging and pre-prompt reconciliation for synthesized
  `context_tier` on `copilot-acp` (`copilot_context_tier.go`,
  `manager_interaction.go`, `manager_events.go`):
  - Validate `default` | `long_context` (rejecting invalid values with
    `SessionConfigurationRejectedError`).
  - Reject with `SessionConfigurationRejectedError` (`status: "rejected"`) when
    a prompt turn or context reset is active, leaving the active turn and SQLite
    `task_sessions.config_options` untouched.
  - When idle, run `prepareAgentRestart` + `ValidateLaunchWrapper` whenever the
    selected tier differs from the running subprocess's `--context` flag, stage
    `context_tier` in `ModelState` immediately without stopping the subprocess,
    and preserve staged `context_tier` across `session_models` stream events.
  - Before dispatching the next prompt turn, replace the child subprocess once
    via `agentctl` when the staged `context_tier` differs from the running
    subprocess's `--context` flag, restore the conversation via `LoadSession`
    (falling back to `NewSession` only when Copilot reports session-not-found on
    a 0-turn session), and re-apply `model`, `mode`, and remaining
    `config_options` without publishing `AgentBootReady` or `AgentContextReset`.
- Extend `apps/backend/cmd/mock-agent/` (`copilot_native_wire.go` / `handler.go`)
  when `KANDEV_MOCK_COPILOT_NATIVE=true` to record `--context <tier>`, reject
  `session/set_config_option("context_tier", ...)` like real Copilot, support
  `session/load` across subprocess reloads for sessions with >=1 turn (and
  session-not-found for 0-turn sessions), and expose `/e2e:copilot-context-info`
  for deterministic E2E assertions.
- Add desktop (`copilot-context-tier.spec.ts`) and phone
  (`mobile-copilot-context-tier.spec.ts`) Playwright E2E coverage, plus record
  the `.agents/skills/tdd/references/lifecycle-evidence.md` scenario results in
  `Results`.

## Out of scope

- `acpcompat` option synthesis, `CopilotACP.BuildCommand` flag formatting, and
  `CommandPreviewCard` wiring (completed in Task 01).
- Interrupting active turns or live-reloading retained PTY passthrough sessions.

## Acceptance

- Pre-launch, profile-configured, context-reset, and process-restarted Copilot
  ACP sessions launch with the effective `--context <tier>` and complete session
  initialization without `-32602` config-option errors.
- Changing `Context Window` in an idle Copilot Task Chat session stages
  immediately without blocking on subprocess restart, coalesces intermediate
  switches before the next prompt (performing zero restarts if toggled back to
  the active startup tier), reloads only the child Copilot subprocess once
  before the next prompt with the new `--context <tier>`, preserves prior
  conversation turns via `session/load` (or succeeds via `session/new` at 0
  turns), preserves composed `model` and `reasoning_effort` selections, leaves
  the live process untouched on plugin preflight failure, and rejects cleanly
  without interrupting an in-flight turn.
- Desktop (`chromium`) and phone (`mobile-chrome`) Playwright tests pass against
  freshly built backend and web artifacts, including phone `ModelConfigSheet`
  `>=44px` touch-target, internal-scroll, and zero-horizontal-overflow checks.

## ASCII UI preview

Excerpt from [`plan.md#ascii-ui-preview`](plan.md#ascii-ui-preview) covering
`UI-01: Copilot model configuration selector (Desktop Popover and Phone Drawer)`
for `AC-AGENTS-COPILOT-CONTEXT-TIER-001.4` and
`AC-AGENTS-COPILOT-CONTEXT-TIER-003.5`:

```text
Desktop (!isMobile, Popover)                 Phone (isMobile, ModelConfigSheet Drawer)
+----------------------------------------+   +--------------------------------------+
| * GPT-6.2 Sol                      1x  |   | Select model                     [X] |
|   Gemini 4 Carbon                  1x  |   +--------------------------------------+
+----------------------------------------+   | * GPT-6.2 Sol                   1x   |
| Reasoning Effort  [ high           v ] |   | ------------------------------------ |
| Context Window    [ Long Context   v ] |   | Reasoning Effort                     |
| Allow All         [ Off            v ] |   | [ high                           v ] |
+----------------------------------------+   | Context Window                       |
|                             [ Apply ]  |   | [ Long Context                   v ] |
+----------------------------------------+   | [              Apply               ] |
                                             +--------------------------------------+
Task Chat muted confirmation row (REQ-AGENTS-SESSION-CONFIG-FEEDBACK-001):
  Changed model to GPT-6.2 Sol, Context Window to Long Context, Reasoning Effort to high
```

## Verification

```bash
bash scripts/check-agentctl-deps.sh
(cd apps/backend && go test ./internal/agent/runtime/lifecycle/... ./internal/task/handlers/... ./cmd/mock-agent/...)
bash scripts/lint-go-changed \
  apps/backend/internal/agent/runtime/lifecycle/manager_launch.go \
  apps/backend/internal/agent/runtime/lifecycle/session.go \
  apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go \
  apps/backend/cmd/mock-agent/copilot_native_wire.go
make -C apps/backend build
(cd apps && pnpm --filter @kandev/web build:vite)
(cd apps/web && pnpm e2e:raw tests/chat/copilot-context-tier.spec.ts --project=chromium)
(cd apps/web && pnpm e2e:raw tests/chat/mobile-copilot-context-tier.spec.ts --project=mobile-chrome)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_test.go`
- `apps/backend/internal/task/handlers/process_handlers_test.go`
- `apps/backend/cmd/mock-agent/copilot_native_wire.go`
- `apps/backend/cmd/mock-agent/copilot_native_wire_test.go`
- `apps/backend/cmd/mock-agent/handler.go`
- `apps/web/e2e/tests/chat/copilot-context-tier.spec.ts`
- `apps/web/e2e/tests/chat/mobile-copilot-context-tier.spec.ts`

## Dependencies

- [Task 01: Copilot context-tier discovery, CLI flags, and command preview](task-01-copilot-context-tier-discovery-and-flags.md)

## Risks

- Lock ordering between `remoteInstanceLifecycleMu`, `sessionConfigurationMu`,
  and `contextResetAdmissionMu` must match `RestartAgentProcess` so concurrent
  prompts or context resets cannot deadlock.
- Reloading must close only per-stream handles (`CloseUpdatesStream` /
  `CloseWorkspaceStream`) rather than `client.Close()`, preserving the live
  `agentctl` server instance.

## Parallelism

`sequential`

## Inputs

- [`docs/specs/agents/requirements/copilot-context-tier.md`](../../specs/agents/requirements/copilot-context-tier.md)
- [`docs/specs/agents/system-design/copilot-context-tier.md`](../../specs/agents/system-design/copilot-context-tier.md)
- Lifecycle checklist: `.agents/skills/tdd/references/lifecycle-evidence.md`
- Existing restart path: `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go` (`restartAgentProcess`, `prepareAgentRestart`)
- Existing Copilot E2E pattern: `apps/web/e2e/tests/chat/copilot-native-compaction.spec.ts`

## Results

Completed with strict Red-Green-Refactor TDD across backend lifecycle, HTTP handlers, `mock-agent`, and desktop/mobile Playwright E2E:

- Added `apps/backend/internal/agent/runtime/lifecycle/copilot_context_tier.go` and `copilot_context_tier_test.go`, plus integration wiring across `manager_profile.go`, `manager_launch.go`, `manager_passthrough.go`, `manager_interaction.go`, `manager_events.go`, `manager_workspace_rebind.go`, and `session.go`:
  - Merged `profileInfo.ConfigOptions` and persisted/override `SessionRuntimeConfig.ConfigOptions` into `agents.CommandOptions.ConfigOptions` and `agents.PassthroughOptions.ConfigOptions` on launch, context reset, workspace rebind, and process restart.
  - Skipped redundant `SetConfigOption("context_tier", ...)` RPCs during `applyProfileSessionLayers`, `applyRuntimeSessionLayers`, and `restoreSessionRuntimeConfigWithClient` when a synthesized Copilot `context_tier` already matches the running execution's `--context` launch flag.
  - Implemented lazy staging + pre-prompt reconciliation (`applySynthesizedCopilotContextTier`, `preserveStagedCopilotContextTier`, `reconcileCopilotContextTierBeforePrompt`): validates `default` | `long_context`, rejects while a prompt turn or context reset is active (`status: "rejected"`), runs `prepareAgentRestart` + `ValidateLaunchWrapper` before accepting a changed tier, stages `context_tier` in `ModelState` immediately without stopping the idle subprocess, preserves staged `context_tier` across `session_models` stream events, and reloads the child subprocess once via `agentctl` + `LoadSession` (falling back to `NewSession` only on 0-turn session-not-found) right before the next prompt turn without emitting `AgentBootReady` or `AgentContextReset` (and routes `ResetAgentContext` with a staged `context_tier` through `RestartAgentProcess`).
- Added HTTP handler regression `TestCopilotContextTierActiveTurnRejectionReturnsRejectedWithoutPersisting` in `apps/backend/internal/task/handlers/process_handlers_configuration_feedback_test.go`.
- Extended `apps/backend/cmd/mock-agent/copilot_native_wire.go` (`copilot_native_wire_test.go`, `main.go`) to parse `--context <tier>`, reject `session/set_config_option("context_tier", ...)` with `-32602`, persist prompt turn state for `session/load` across subprocess reloads (and return `-32602` session-not-found on 0-turn sessions), and support `/e2e:copilot-context-info`.
- Added desktop (`apps/web/e2e/tests/chat/copilot-context-tier.spec.ts`) and phone (`apps/web/e2e/tests/chat/mobile-copilot-context-tier.spec.ts`) Playwright E2E coverage with shared helpers in `copilot-context-tier-helpers.ts`, including rapid multi-toggle coalescing (`loads === 1`) and unchanged-tier zero-restart verification.

### Lifecycle evidence (`.agents/skills/tdd/references/lifecycle-evidence.md`)

| Scenario | Acceptance Criterion | Observable Invariant | Regression Test | Exact Command | Result |
| --- | --- | --- | --- | --- | --- |
| Pause and resume | `AC-AGENTS-COPILOT-CONTEXT-TIER-003.1`, `003.2`, `003.5` | Changing `context_tier` while idle stages immediately without stopping the subprocess, preserves staged `context_tier` across `session_models` updates, and reconciles the Copilot subprocess once before the next prompt (`PromptAgent` and `PromptAgentWithAdmissionCallbackAndSubmissionID`) via `session/load` while preserving prior turns and composed `model` / `reasoning_effort` without `AgentBootReady` or `AgentContextReset`. | `TestCopilotIdleLiveSessionContextTierReload` (`copilot_context_tier_test.go`), `reloads an idle Copilot session into long_context while preserving conversation history` (`copilot-context-tier.spec.ts`) | `TMPDIR=/private/tmp go test -C apps/backend -race -count=1 ./internal/agent/runtime/lifecycle -run TestCopilotIdleLiveSessionContextTierReload` && `scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --no-build e2e/tests/chat/copilot-context-tier.spec.ts e2e/tests/chat/copilot-native-compaction.spec.ts -- --workers=1 --retries=0` | Passed |
| Restart and recovery | `AC-AGENTS-COPILOT-CONTEXT-TIER-002.4`, `003.2` | Profile + persisted session `config_options.context_tier` merge into fresh launch, context-reset (`ResetAgentContext` with staged `context_tier`), and process-restart `--context <tier>` argv; startup layer replay skips matching synthesized `context_tier` RPCs; 0-turn pre-prompt reloads recover via `session/new` while multi-turn `session/load` failures fail closed before `beforeAdmission` and recover on retry. | `TestCopilotLaunchAndRestartCommandsIncludeContextTier`, `TestCopilotStartupAndRebindSkipSynthesizedContextTierRPC`, `TestCopilotIdleLiveSessionContextTierReload` (`copilot_context_tier_test.go`) | `TMPDIR=/private/tmp go test -C apps/backend -race -count=1 ./internal/agent/runtime/lifecycle -run 'TestCopilotLaunchAndRestartCommandsIncludeContextTier|TestCopilotStartupAndRebindSkipSynthesizedContextTierRPC|TestCopilotIdleLiveSessionContextTierReload'` | Passed |
| Duplicate and stale delivery | `AC-AGENTS-COPILOT-CONTEXT-TIER-003.1`, `003.2` | Toggling `context_tier` away and back to the running subprocess's startup tier before the next prompt performs zero subprocess restarts (`loads` unchanged); pre-prompt reload binds fresh startup and delivery generations before reconnecting streams. | `TestCopilotIdleLiveSessionContextTierReload` (`copilot_context_tier_test.go`), `copilot-context-tier.spec.ts` | `TMPDIR=/private/tmp go test -C apps/backend -race -count=1 ./internal/agent/runtime/lifecycle -run TestCopilotIdleLiveSessionContextTierReload` | Passed |
| Ownership changes / replacement preflight safety | `AC-AGENTS-COPILOT-CONTEXT-TIER-003.3` | `prepareAgentRestart` + `ValidateLaunchWrapper` run during staging and before stopping the live Copilot subprocess; a wrapper/plugin preflight failure aborts cleanly and leaves the existing subprocess and `ACPSessionID` untouched. | `TestCopilotIdleLiveSessionContextTierReload/preflight_failure_before_stop_leaves_live_subprocess_untouched` (`copilot_context_tier_test.go`) | `TMPDIR=/private/tmp go test -C apps/backend -race -count=1 ./internal/agent/runtime/lifecycle -run TestCopilotIdleLiveSessionContextTierReload/preflight_failure_before_stop_leaves_live_subprocess_untouched` | Passed |
| Concurrent or merged work | `AC-AGENTS-COPILOT-CONTEXT-TIER-003.4`, `003.5` | Attempting a `context_tier` change while a turn is `RUNNING` rejects immediately (`configuration_result.status: "rejected"`, `confirmed: false`) without calling ACP `SetConfigOption`, leaves persisted `runtime_config.config_options.context_tier` unchanged, lets the active turn finish undisturbed, and succeeds once idle while preserving all completed turns. | `TestCopilotIdleLiveSessionContextTierReload/active_turn_and_invalid_values_are_rejected_without_stopping_process` (`copilot_context_tier_test.go`), `TestCopilotContextTierActiveTurnRejectionReturnsRejectedWithoutPersisting` (`process_handlers_configuration_feedback_test.go`), `rejects live context window switch during an active turn without interrupting it, then applies once idle` (`copilot-context-tier.spec.ts`) | `TMPDIR=/private/tmp go test -C apps/backend -race -count=1 ./internal/agent/runtime/lifecycle ./internal/task/handlers -run 'TestCopilotIdleLiveSessionContextTierReload|TestCopilotContextTierActiveTurnRejectionReturnsRejectedWithoutPersisting'` && `scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --no-build e2e/tests/chat/copilot-context-tier.spec.ts` | Passed |
| Entry paths (Profile Settings, Command Preview, Task Chat desktop & phone) | `AC-AGENTS-COPILOT-CONTEXT-TIER-001.4`, `002.3`, `003.2`, `003.5` | Profile settings, command preview (`--context long_context`), pre-launch/task launch, desktop Popover selector, and phone `ModelConfigSheet` bottom `Drawer` (`>=44px` touch targets, single internal scroll owner, zero horizontal overflow) exercise the same normalized `context_tier` contract end to end. | `command-preview-card.test.tsx`, `copilot-context-tier.spec.ts` (3 tests), `mobile-copilot-context-tier.spec.ts` (1 test) | `(cd apps/web && pnpm exec vitest run app/settings/agents/\\[agentId\\]/profiles/\\[profileId\\]/command-preview-card.test.tsx)` && `scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --no-build e2e/tests/chat/copilot-context-tier.spec.ts e2e/tests/chat/copilot-native-compaction.spec.ts -- --workers=1 --retries=0` && `scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --no-build --project mobile-chrome e2e/tests/chat/mobile-copilot-context-tier.spec.ts` | Passed |
