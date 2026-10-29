---
id: "03-acp-process-loading"
title: "Load profile plugins in local Copilot ACP processes"
status: done
wave: 4
depends_on: ["01-inventory-discovery", "02-profile-state"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-COPILOT-PLUGINS-003
  - REQ-AGENTS-COPILOT-PLUGINS-004
  - REQ-AGENTS-COPILOT-PLUGINS-005
  - REQ-AGENTS-COPILOT-PLUGINS-007
acceptance_criteria:
  - AC-AGENTS-COPILOT-PLUGINS-003.5
  - AC-AGENTS-COPILOT-PLUGINS-004.1
  - AC-AGENTS-COPILOT-PLUGINS-004.2
  - AC-AGENTS-COPILOT-PLUGINS-004.4
  - AC-AGENTS-COPILOT-PLUGINS-004.5
  - AC-AGENTS-COPILOT-PLUGINS-004.7
  - AC-AGENTS-COPILOT-PLUGINS-005.1
  - AC-AGENTS-COPILOT-PLUGINS-005.2
  - AC-AGENTS-COPILOT-PLUGINS-005.3
  - AC-AGENTS-COPILOT-PLUGINS-005.4
  - AC-AGENTS-COPILOT-PLUGINS-005.5
  - AC-AGENTS-COPILOT-PLUGINS-005.8
  - AC-AGENTS-COPILOT-PLUGINS-007.1
  - AC-AGENTS-COPILOT-PLUGINS-007.3
  - AC-AGENTS-COPILOT-PLUGINS-007.6
system_design:
  - ../../specs/agents/system-design/copilot-profile-plugins.md
---

# Task 03: Prepare ACP process mounts

## Summary

Load selected plugins before each supported local Copilot ACP subprocess starts.
Resolve roots from the final launch environment and keep retained processes unchanged.

## In scope

- Carry selection IDs with the same authorized profile snapshot used for process preparation.
- Add final preparation after effective environment composition and before `ConfigureAgent`/`Start`.
- Use the selected managed runtime and WO-01 adapter. Revalidate current roots instead of reusing absolute paths from a prior process.
- Emit separate `--plugin-dir`/root pairs in canonical ID order, preserving structured argv and wrapper application.
- Preserve manual plugin flags, suppress equal canonical roots, and reject conflicting roots for the same selected plugin.
- Cover fresh starts, workspace-only promotion, fresh replacement/context-reset fallback, managed retry, and new-process recovery/resume.
- Prove retained adoption and in-process ACP reset do not trigger plugin resolution or process termination.
- Admit only trusted backend-owned local/worktree executors. Reject nonempty isolated/unknown selections before host reads.
- Keep an empty selection as a no-op and preserve ACP MCP injection, native MCP configuration, and native auth ownership.

## Out of scope

Terminal/utility/probe/preview paths owned by WO-04, UI, remote mounts, plugin installation, credential handling, and runtime updates.

## Acceptance

1. Exact argv and final-child environment tests cover multiple plugins, spaces, config overrides, manual flags, and each new ACP process path.
2. Missing/unsafe/unsupported selections fail atomically with safe repair errors. Empty selections retain every executor's current behavior without discovery.
3. Existing MCP injection and retained sessions remain operational. Supported-version/root/context checks remain effective, while native Copilot owns policy/consent without an additional enterprise-certification gate.

## Verification

Use `/tdd` and the scoped backend testing guide. Run from the repository root.
Tests use fake commands and temporary plugin state, not installed Copilot or native credentials.

```bash
(cd apps/backend && go test -trimpath ./internal/agent/copilotplugins -run 'TestPluginArguments|TestPluginLaunchContext|TestPluginReadOnlyOperations' -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle -run 'TestCopilotPluginLaunchPaths|TestCopilotPluginEffectiveEnvironment|TestCopilotPluginManualFlags|TestCopilotPluginExecutorAdmission|TestCopilotPluginRetainedProcess|TestCopilotPluginMCPInjection' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle -run 'TestCopilotPluginLaunchPaths|TestCopilotPluginRetainedProcess' -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/agents -run 'TestCopilot|TestManagedNPMRuntime' -count=1)
git diff --check
```

Run `bash scripts/lint-go-changed` with the explicit changed Go-file list after tests.
If a common environment helper or agentctl child boundary changes, add its exact package/test command to Results before completion.
Include final-child HOME/COPILOT_HOME/PATH assertions, not only a pre-configuration map assertion.
Add platform-specific coverage through existing Windows CI guidance where native APIs are involved.

## Files likely touched

- `apps/backend/internal/agent/copilotplugins/{arguments.go,context.go}` and focused tests (new).
- `apps/backend/internal/agent/runtime/lifecycle/{types.go,profile_resolver.go,manager_launch.go,manager_startup.go,manager_interaction.go}`.
- A focused lifecycle `copilot_plugins.go`, `copilot_plugins_launch_test.go`, and effective-environment/executor tests (new).
- Existing recovery/promotion tests where they own process starts. Do not refactor recovery unrelated to preparation.
- Shared `internal/common/` environment helper only if final-child parity requires extraction.
- Existing agentctl configuration boundary tests if environment composition is changed.

## Dependencies

WO-01 provides the qualified resolver and WO-02 provides the durable/runtime profile field.
WO-00 bounds supported inventory/metadata adapters. Its diagnostic policy
limitations are not a production gate under the
[native-authority decision](../../decisions/2026-10-28-copilot-native-policy-authority.md).

## Risks

Building arguments too early can read another home. Reusing mutated argv can duplicate mounts on retry.
Some recovery paths adopt live processes instead of starting new ones. Their behavior must remain distinct.
An isolated executor can share a runtime label with a local path. Use trusted executor classification.

## Parallelism

`sequential`

## Inputs

- [Design](../../specs/agents/system-design/copilot-profile-plugins.md#process-preparation), effective launch context, executor, and native authority sections.
- `buildAgentCommandWithContext`, `configureAndStartAgent`, and `buildFreshAgentCommandWithProfile`.
- Existing workspace promotion, process replacement, recovery, environment, and MCP injection fixtures.

## Results

Follow-up execution is mapped to
[completion task 04](../copilot-profile-plugin-completion/task-04-qualified-process-loading.md),
including removal of the policy-only blocker and positive process-boundary
coverage for supported adapters.
Completed by that work order: supported versions now reach real Configure/Start
with coherent profile/runtime/environment, preserved MCP attachments, atomic
preflight, and native-inventory rejection/no-alternate-identity coverage.
Its Results retain exact commands/counts, the corrected macOS root assertions,
the unavailable historical implementation Red details, and the demonstrated
pristine-HEAD SSH teardown race exception. No privileged policy fixture or
native-policy certification is claimed.

### Historical pre-ADR results

The baseline results below describe the earlier rejection-only implementation.

- `go test -trimpath ./internal/agent/copilotplugins -run 'TestPluginArguments|TestPluginLaunchContext|TestPluginReadOnlyOperations|TestPolicyUnverified|TestWrapped|TestRuntimeVersion' -count=1`: passed.
- `go test -trimpath ./internal/agent/runtime/lifecycle -run 'TestCopilotPlugin' -count=1`: passed, including `TestCopilotPluginPolicyUnverifiedLaunch`, `TestCopilotPluginWrapperRejected`, and `TestCopilotPluginRuntimeSnapshotNotReread`.
- The ACP path derives the runtime version from the prepared argv and no longer rereads the active selection after command construction.
- Full `go test -trimpath ./internal/agent/runtime/lifecycle -count=1`: passed (post-fix run: 422.5s).
- Covers executor admission, empty no-op, wrapper rejection, policy rejection before native inventory, and runtime-command binding. Mount mechanics retain package-level argument/root tests but are not admitted for production starts.
- Native authenticated plugin loading remains unverified as recorded in WO-00.
