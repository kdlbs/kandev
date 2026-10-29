---
id: "04-qualified-process-loading"
title: "Load profile plugins under native Copilot authority"
status: done
wave: 3
depends_on:
  - "01-canonical-path-expectations"
  - "02-actionable-policy-recovery"
plan: "plan.md"
requirements:
  - REQ-AGENTS-COPILOT-PLUGINS-001
  - REQ-AGENTS-COPILOT-PLUGINS-003
  - REQ-AGENTS-COPILOT-PLUGINS-004
  - REQ-AGENTS-COPILOT-PLUGINS-005
  - REQ-AGENTS-COPILOT-PLUGINS-007
acceptance_criteria:
  - AC-AGENTS-COPILOT-PLUGINS-001.3
  - AC-AGENTS-COPILOT-PLUGINS-003.5
  - AC-AGENTS-COPILOT-PLUGINS-004.1
  - AC-AGENTS-COPILOT-PLUGINS-004.2
  - AC-AGENTS-COPILOT-PLUGINS-004.3
  - AC-AGENTS-COPILOT-PLUGINS-004.4
  - AC-AGENTS-COPILOT-PLUGINS-004.5
  - AC-AGENTS-COPILOT-PLUGINS-004.7
  - AC-AGENTS-COPILOT-PLUGINS-004.8
  - AC-AGENTS-COPILOT-PLUGINS-005.1
  - AC-AGENTS-COPILOT-PLUGINS-005.2
  - AC-AGENTS-COPILOT-PLUGINS-005.3
  - AC-AGENTS-COPILOT-PLUGINS-005.4
  - AC-AGENTS-COPILOT-PLUGINS-005.5
  - AC-AGENTS-COPILOT-PLUGINS-005.8
  - AC-AGENTS-COPILOT-PLUGINS-007.3
  - AC-AGENTS-COPILOT-PLUGINS-007.4
  - AC-AGENTS-COPILOT-PLUGINS-007.6
system_design:
  - ../../specs/agents/system-design/copilot-profile-plugins.md
  - ../../specs/agents/system-design/copilot-profile-plugins-native-qualification.md
---

# Task 04: Native profile process loading

## Summary

Remove the artificial policy-only gate and carry valid profile selections
through every supported new-process boundary. Copilot remains the policy
authority under the
[accepted decision](../../decisions/2026-10-28-copilot-native-policy-authority.md).
This is not permission to bypass native controls.

## In scope

- Remove `policyVerifiedVersions` and the `RequireMountPolicy` prerequisite.
  Do not replace them with a per-platform certification gate or default-allow
  for unknown adapter versions.
- Keep `RuntimeSupported` and make discovery's `mount_permitted` agree with
  supported native preparation. Missing enterprise fixtures must not reject
  `1.0.88`/`1.0.104` before root/context validation.
- Prove real configure/start or inference payloads for fresh ACP, promotion,
  replacement, retry, new-process resume/recovery, passthrough
  initial/resume/fallback/reset, utility prompts, and both profile probes.
- Assert selected runtime, final environment/cwd, separate argv, manual-mount
  deduplication/conflicts, and intact ACP MCP attachments.
- Use one authorized profile snapshot for selection, command, and environment.
  A channel/barrier test must expose mixed-revision preparation.
- Host utility inventory uses `InteractiveCommand(version)` and final workdir.
- Preflight Copilot before stopping the local process or entering isolated SSH
  stop. Preserve Cursor/SSH MCP retirement ordering and owned-resource cleanup.
- Preserve empty, unknown-version, invalid-root, isolated/wrapped,
  agent-wide, retained-process, and native-rejection negative controls.
- Update old tests that inferred unsupported status from the empty policy set.
  Legacy reason-code tests may inject that exact typed error; they must not
  reintroduce a live gate or mutate global version maps.

## Out of scope

Native policy evaluation/certification, admin access, credential operations,
runtime pin changes, native bypass flags, remote plugin loading, new settings,
and broad reset refactors.

## Acceptance

1. Supported versions and valid contexts/roots reach native preparation;
   unsupported versions and contexts still reject. Discovery uses the same
   predicate, without an enterprise-fixture/platform-certification dependency.
2. Every named process boundary has production-shaped success/rejection tests
   for coherent identity, environment, cwd, argv, and MCP attachments. Native
   rejection is not retried through another runtime/identity.
3. Rejected preflight preserves the current process and identity; successful
   replacement stops/starts exactly once. Retained/empty paths do no new work.

## Verification

Use TDD from the worktree root:

```bash
(cd apps/backend && go test -json -trimpath -count=1 ./internal/agent/copilotplugins)
(cd apps/backend && go test -json -trimpath -count=1 ./internal/agent/settings/controller ./internal/agent/settings/handlers -run 'TestCopilotPlugin')
(cd apps/backend && go test -json -trimpath -count=1 ./internal/agent/runtime/lifecycle ./internal/agent/hostutility -run 'TestCopilotPlugin|TestSSHPassthrough.*Reset|TestPromotedPassthroughResumeAdoptsTerminalReadiness|TestPassthroughAdoption')
(cd apps/backend && go test -json -trimpath -race -count=1 ./internal/agent/runtime/lifecycle ./internal/agent/hostutility -run 'TestCopilotPlugin(Supported|ResetPreflight|ProfileSnapshot|Empty|Executor|AgentWide|Wrapper|NativeAuthority)|TestSSHPassthrough.*Reset|TestPassthroughAdoption')
git ls-files --modified --others --exclude-standard -z -- 'apps/backend/**/*.go' | xargs -0 bash scripts/lint-go-changed
git diff --check
```

Create/discover these regressions, not helper-only substitutes:
`TestCopilotPluginNativeAuthorityAdmission`,
`TestCopilotPluginDiscoveryMountPermission`,
`TestCopilotPluginSupportedLaunchPaths`,
`TestCopilotPluginSupportedProfileProcesses`,
`TestCopilotPluginResetPreflightPreservesProcess`, and
`TestCopilotPluginProfileSnapshotCoherence`.

Reset cases cover missing/conflicting roots, unsupported runtime/context, and
isolated rejection before stop, plus successful handoff. No timing sleeps for
the snapshot race. Record Red/Green commands, counts, and expanded lint inputs.
No privileged or native-auth-dependent run is needed for hermetic wiring.

## Files likely touched

- `copilotplugins/context.go`, `arguments.go`, admission/root tests.
- `runtime/lifecycle/copilot_plugins.go`, snapshot/launch owners
  `manager_launch.go`, `manager_interaction.go`, `profile_resolver.go`, and
  actual `configureAndStartAgent` owner as needed.
- `runtime/lifecycle/manager_passthrough_reset.go` and focused tests.
- `hostutility/copilot_plugins.go`, `profile_probe.go`, `public.go`, tests.
- Discovery controller and permission-projection tests.
- Narrow internal dependency fields only where real-boundary testing needs one.

Backend paths above are relative to `apps/backend/internal/agent/`.
Primary owns companion work-order and evidence/status updates.

## Dependencies

Completed WO-01/02. WO-03 provides diagnostic context, not an admission gate.

## Risks

Removing the artificial gate must not remove version/root/executor validation.
Snapshot races and remote MCP retirement still require explicit controls.
Preparation eligibility is not native policy approval or guaranteed tool use.

## Parallelism

`sequential`: shared provider, lifecycle, host utility, and discovery contracts.

## Inputs

- [Parent preparation design](../../specs/agents/system-design/copilot-profile-plugins.md#process-preparation).
- [Native admission and process verification](../../specs/agents/system-design/copilot-profile-plugins-native-qualification.md).
- Current native diagnostics, existing lifecycle/utility tests, backend guidance.

## Results

Completed following the user's explicit implementation request. All Copilot
acceptance paths are covered. The extended race command retains the demonstrated
pre-existing macOS SSH teardown exception below; it is not reported as passing.

### Delivered behavior

- Removed the empty policy-only allowlist and `RequireMountPolicy`. Supported
  adapters remain exactly `1.0.88` and `1.0.104`; discovery uses the same predicate.
  Version/root/context/identity/wrapper/conflict checks remain effective.
- Actual agentctl Configure/Start boundaries cover fresh ACP, promotion,
  replacement, explicit retry, and new-process resume/recovery, preserving
  separate argv, selected runtime, final environment, manual deduplication,
  and ACP MCP attachments.
- Channel/barrier snapshot coverage prevents mixed profile revisions and
  confirms the next preparation sees the updated complete snapshot.
- Passthrough initial/resume/fallback/reset and retained paths are covered.
  Rejected Copilot preflight preserves the current process/identity before
  local or isolated SSH stop. Successful replacement stops/starts once.
  Existing Cursor/SSH MCP retirement ordering is preserved.
- Utility prompts, `ProbeProfileCapabilities`, and `ResolveModelConfig` use
  selected-runtime interactive inventory in the final workdir/environment.
  Their tests exercise real hostutility/agentctl probe requests, not only
  controller fakes. Legacy reason projections remain separate coverage.
- `TestCopilotPluginNativeInventoryFailurePreventsAlternateRuntimeOrProfileRetry`
  fails inventory at the real `configureAndStartAgent` caller boundary:
  one selected-runtime inventory attempt, no runtime reread or alternate
  profile, unchanged identities, and zero Configure/Start requests. Its
  otherwise-identical success control configures/starts exactly once.
  This was test-only contract coverage: the first run passed and no production
  Red is claimed.

### Verification

The four Go commands in Verification ran with JSON event capture. Final
portability runs retained the ordinary `/var/folders` TMPDIR alias returned by
`getconf DARWIN_USER_TEMP_DIR`.

| Required command | Top-level events | Subtest events | Result |
| --- | --- | --- | --- |
| Provider package | 22 pass, 1 skip | 16 pass | exit 0 |
| Settings controller/handlers | 25 pass | 11 pass | exit 0 |
| Lifecycle/hostutility and reset/adoption controls | 35 pass | 14 pass | exit 0 |
| Extended race selector | 18 pass, 1 fail | 9 pass, 2 fail | exit 1, existing SSH teardown |
| Changed-Go lint, 33 inputs below | Not applicable | Not applicable | exit 0, zero issues |
| `git diff --check` | Not applicable | Not applicable | exit 0 |

The provider skip is `TestProcessTerminatedTreatsZombieAsGone`.
After the final test-only caller regression, both commands below passed two
top-level tests and five subtests, with no skips:

```bash
(cd apps/backend && go test -json -trimpath -count=1 ./internal/agent/runtime/lifecycle -run '^TestCopilotPluginNativeInventoryFailurePreventsAlternateRuntimeOrProfileRetry$|^TestCopilotPluginSupportedLaunchPaths$')
(cd apps/backend && go test -json -trimpath -race -count=1 ./internal/agent/runtime/lifecycle -run '^TestCopilotPluginNativeInventoryFailurePreventsAlternateRuntimeOrProfileRetry$|^TestCopilotPluginSupportedLaunchPaths$')
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/copilot_plugins_launch_test.go apps/backend/internal/agent/runtime/lifecycle/copilot_plugins_process_test.go
git diff --check
```

Final two-file lint reported zero issues; whitespace passed. The Copilot race
selector without the unrelated SSH reset cases also passed:

```bash
(cd apps/backend && go test -trimpath -race -count=1 ./internal/agent/runtime/lifecycle ./internal/agent/hostutility -run 'TestCopilotPlugin(Supported|ResetPreflight|ProfileSnapshot|Empty|Executor|AgentWide|Wrapper|NativeAuthority)|TestPassthroughAdoption')
```

The extended race failures are
`TestSSHPassthroughResetReplacesChangedWorkspaceMCP/cursor_false` and
`pi_false`, at `manager_ssh_passthrough_test.go:58` inside
`t.Cleanup(manager.StopForTeardown)`, not initial process startup.
The primary reproduced the same EPERM on pristine HEAD
`fb4c2bc3ef98da2fc303fe05cb144bcd6c167d58`, without follow-up files, using:

```bash
(cd apps/backend && go test -json -trimpath -race -count=3 ./internal/agent/runtime/lifecycle -run '^TestSSHPassthroughResetReplacesChangedWorkspaceMCP$')
```

All three top-level runs failed, with eight passing/four failing subtests.
This baseline exception is disclosed rather than blocking unrelated delivery
or changing process-group production code. The disposable baseline archive was
removed; raw outputs remain private. The
[compatibility ledger](../copilot-profile-plugins/compatibility-evidence.md#completion-verification-exception-macos-ssh-teardown)
records the same limitation.

### Red evidence and portability correction

The implementation handoff confirmed assertion-level Red for the six named
regressions, but lost its original exact commands/counts. They are unavailable;
compiler/import/discovery failures are not represented as Red.
An intermediate reset assertion also exposed `stopCalls=0`; handoff correction
restored one stop before replacement.

The primary then found new raw-versus-canonical macOS path assertions.
The retained-alias lifecycle/hostutility Red command was
`go test -trimpath -count=1 ./internal/agent/runtime/lifecycle ./internal/agent/hostutility -run 'TestCopilotPlugin'`:
15 top-level failures and four subtest failures. Provider Red used the first
Verification command: 21 pass/1 fail/1 skip top-level and 14 pass/2 fail subtests.
Checked `filepath.EvalSymlinks` now canonicalizes expected resolver-generated
mounts, not fixture inputs or production validation. Equal manual mounts retain
their raw user-supplied argument. The final required non-race runs above passed.
These retained portability Red/Green results do not reconstruct the lost
historical implementation Red.

### Expanded changed-Go lint inputs

The exact 33 inputs were:

```text
apps/backend/internal/agent/copilotplugins/arguments_error_test.go
apps/backend/internal/agent/copilotplugins/native_acceptance_acp_test.go
apps/backend/internal/agent/copilotplugins/native_acceptance_cases_test.go
apps/backend/internal/agent/copilotplugins/native_acceptance_contract_test.go
apps/backend/internal/agent/copilotplugins/native_acceptance_process_other_test.go
apps/backend/internal/agent/copilotplugins/native_acceptance_process_unix_test.go
apps/backend/internal/agent/copilotplugins/native_acceptance_sse_test.go
apps/backend/internal/agent/copilotplugins/native_acceptance_test.go
apps/backend/internal/agent/runtime/lifecycle/copilot_plugins_passthrough_process_test.go
apps/backend/internal/agent/runtime/lifecycle/copilot_plugins_process_test.go
apps/backend/internal/agent/settings/handlers/copilot_profile_probe_reason_test.go
apps/backend/internal/agent/copilotplugins/arguments.go
apps/backend/internal/agent/copilotplugins/context.go
apps/backend/internal/agent/copilotplugins/copilotplugins_test.go
apps/backend/internal/agent/hostutility/copilot_plugins.go
apps/backend/internal/agent/hostutility/copilot_plugins_test.go
apps/backend/internal/agent/hostutility/profile_probe.go
apps/backend/internal/agent/hostutility/types.go
apps/backend/internal/agent/runtime/lifecycle/copilot_plugins.go
apps/backend/internal/agent/runtime/lifecycle/copilot_plugins_launch_test.go
apps/backend/internal/agent/runtime/lifecycle/copilot_plugins_passthrough_initial_test.go
apps/backend/internal/agent/runtime/lifecycle/copilot_plugins_passthrough_test.go
apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go
apps/backend/internal/agent/runtime/lifecycle/manager_interaction_test.go
apps/backend/internal/agent/runtime/lifecycle/manager_launch.go
apps/backend/internal/agent/runtime/lifecycle/manager_passthrough.go
apps/backend/internal/agent/runtime/lifecycle/manager_passthrough_reset.go
apps/backend/internal/agent/runtime/lifecycle/session_test.go
apps/backend/internal/agent/settings/controller/agent_config.go
apps/backend/internal/agent/settings/controller/copilot_plugin_discovery_test.go
apps/backend/internal/agent/settings/controller/profile_discovery.go
apps/backend/internal/agent/settings/controller/profile_discovery_test.go
apps/backend/internal/agent/settings/dto/dto.go
```

No native-auth-dependent rerun, managed-policy certification, runtime pin,
credentials, live instance, staging, commit, push, or PR changed. WO-05 owns
the remaining settings/support verification.
