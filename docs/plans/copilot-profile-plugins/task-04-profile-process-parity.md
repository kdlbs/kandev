---
id: "04-profile-process-parity"
title: "Apply plugin selections to other profile process paths"
status: pending
wave: 5
depends_on: ["03-acp-process-loading"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-COPILOT-PLUGINS-001
  - REQ-AGENTS-COPILOT-PLUGINS-004
  - REQ-AGENTS-COPILOT-PLUGINS-005
  - REQ-AGENTS-COPILOT-PLUGINS-007
acceptance_criteria:
  - AC-AGENTS-COPILOT-PLUGINS-001.3
  - AC-AGENTS-COPILOT-PLUGINS-001.6
  - AC-AGENTS-COPILOT-PLUGINS-004.3
  - AC-AGENTS-COPILOT-PLUGINS-004.5
  - AC-AGENTS-COPILOT-PLUGINS-004.6
  - AC-AGENTS-COPILOT-PLUGINS-005.3
  - AC-AGENTS-COPILOT-PLUGINS-005.4
  - AC-AGENTS-COPILOT-PLUGINS-007.3
  - AC-AGENTS-COPILOT-PLUGINS-007.4
system_design:
  - ../../specs/agents/system-design/copilot-profile-plugins.md
---

# Task 04: Cover other profile process paths

## Summary

Apply the same plugin preference to local profile-owned terminal, utility, and capability-probe processes.
Expose a safe deferred-mount preview contract without resolving directories for the browser.

## In scope

- Apply the shared adapter after terminal environment composition for passthrough fresh/resume/restart paths.
- For nonempty typed selections, supply Copilot's selected managed `InteractiveCommand(version)` through `PassthroughOptions.BaseCommand`.
  Empty selections retain the current unversioned Copilot passthrough base. Other providers retain their own runtime rules.
- Prepare plugins in `ExecuteProfilePrompt` and profile capability/model-option probe paths with one profile/environment snapshot per operation.
- Extend saved/draft launch requests with optional selections and include effective IDs in HMAC/cache identity and stale-result fencing.
- Keep inventory-only discovery unmounted so removed selections can be repaired.
- Keep agent-wide boot/install/version probes independent from every concrete profile's plugins.
- Add a preview request/response representation for deferred plugin IDs/counts without native execution, filesystem reads, or executable placeholders.
- Preserve current ACP MCP injection, structured wrapper/terminal quoting, secret resolution, and bounded operation cleanup.

## Out of scope

Selector markup, translated preview rendering, agent-wide plugin defaults, new global runtime policy, or new remote support.
WO-05 owns the rendered preview changes.

## Acceptance

1. Terminal and utility/probe argv tests prove the same roots, selected runtime, manual-flag behavior, and final environment as ACP task processes.
2. Changed profile selections invalidate only matching profile probe contexts. Agent-wide probes and empty selections add no plugin work.
3. Preview exposes safe deferred metadata only and never turns unresolved IDs into a copyable executable command.

## Verification

Start with failing tests. Run from the repository root.

```bash
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle -run 'TestCopilotPluginPassthrough|TestCopilotPluginEffectiveEnvironment' -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/hostutility -run 'TestCopilotPluginProfileUtility|TestCopilotPluginProfileProbe|TestCopilotPluginAgentWideProbe' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agent/hostutility -run 'TestCopilotPluginProfileProbe|TestCopilotPluginAgentWideProbe' -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/settings/controller ./internal/agent/settings/handlers -run 'TestCopilotPluginPreview|TestCopilotPluginDiscoveryContext|TestProfileDiscovery' -count=1)
(cd apps/backend && go test -trimpath ./internal/agentctl/server/utility -run 'TestSanitizeEnv|TestCopilotPlugin' -count=1)
git diff --check
```

Add actual changed boundary tests to the commands before completion. Verify every named new test is discovered rather than accepting “no tests to run”.
Run changed-file Go lint with an explicit list after tests.
If an optional selected field is added to the shared probe request, retain compatibility tests for non-Copilot three-field callers.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough.go` and focused `copilot_plugins_passthrough_test.go` (new).
- `apps/backend/internal/agent/hostutility/{public.go,types.go,profile_probe.go,profile_capability_cache.go}` and focused plugin tests (new).
- `apps/backend/internal/agent/settings/dto/dto.go`, `controller/profile_discovery.go`, and related request/context tests.
- Existing command-preview DTO/controller/handler code located through `preview` handlers, with `copilot_plugins_preview_test.go` (new).
- Shared child environment composition tests if this work order changes that boundary.

## Dependencies

WO-03 supplies the shared preparation API and tested local executor/environment policy.

## Risks

Copilot passthrough currently uses a different base-command path from ACP.
Appending flags before a terminal string is formed is necessary to preserve directories with spaces.
Profile caches without the new selection input can leak stale capabilities across drafts.
Do not pass unresolved secrets or selection state into agent-wide probes.

## Parallelism

`sequential`

## Inputs

- [Design](../../specs/agents/system-design/copilot-profile-plugins.md#process-coverage) and command-preview section.
- `passthroughAgentCommandWithPreparation`, `freshPassthroughCommandForResolved`, and `resumePassthroughCommandWithPreparation`.
- `hostutility.ExecuteProfilePrompt`, `ProbeProfileCapabilities`, `resolveProfileModelConfig`, and existing cache-generation tests.

## Results

Pending 2026-10-28. The code paths are implemented but fail closed with `policy_unverified` until WO-00 supplies native policy evidence.

- `go test -trimpath ./internal/agent/runtime/lifecycle -run 'TestCopilotPluginPassthrough|TestCopilotPluginInitialPassthrough|TestCopilotPluginEffectiveEnvironment' -count=1`: passed.
- Initial, resume, reset, and fallback passthrough starts all apply plugin preparation after final environment composition and before process creation. The initial boundary rejects a nonempty policy-unqualified selection, an unsupported executor, and a wrapper without creating a PTY.
- `go test -trimpath ./internal/agent/hostutility -run 'TestCopilotPlugin' -count=1`: passed, including wrapper rejection and no later runtime-selection read.
- `go test -trimpath ./internal/agent/settings/controller -run 'TestCopilotPluginPreview' -count=1`: passed.
- Passthrough uses the selected managed interactive command only for nonempty typed selections. Utility/probe paths reject the same policy-unqualified context, and agent-wide empty contexts run no inventory.
- Command preview returns deferred stable IDs, count, and an incomplete-base flag without roots or executable placeholders.
