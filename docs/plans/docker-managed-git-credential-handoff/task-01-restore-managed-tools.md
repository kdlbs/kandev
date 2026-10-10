---
id: "01-restore-managed-tools"
title: "Restore installed managed tools during configuration"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.15
system_design:
  - ../../specs/integrations/system-design/github-authentication-02.md
---

# Task 01: Restore installed managed tools during configuration

## Summary

Keep installed GitHub tool paths available privately across instance configuration.
Activate them with the current managed broker contract after environment composition.
Preserve obsolete-credential removal and the canonical environment for every consumer.

## In scope

- Capture the initial helper path, shim directory, and startup path in `process.NewManager`.
- Keep authorization outside that private snapshot.
- Apply runtime tool activation after `composeConfiguredAgentEnvironment` succeeds.
- Reuse config's PATH and Bash behavior through a shared helper.
- Support request PATH replacement, repeated configure, parent hooks, and managed reactivation.
- Deactivate only Kandev-owned PATH and Bash entries when managed authorization disappears.
- Preserve atomic configuration and propagate the final slice to existing consumers.

## Out of scope

- Docker/Kubernetes lifecycle path mapping, owned by Task 02.
- Broker policy or issuance, live credentials, public APIs, rendered UI, and global Git files.
- Restoring broker credentials from the captured environment.

## Acceptance

1. Both Configure modes retain current request authorization and activate installed tools. No previous lease, identity, or scope reappears.
2. Nil, empty, partial-without-broker, and unmanaged replacements remove previous managed authorization and generated Git entries. Off-to-on restores tools with only the new contract.
3. Real Git and Bash subprocesses consume the configured slice. PATH replacement, repeated activation, parent hooks, and malformed-block rollback remain correct.

## TDD and test matrix

Start with `TestManagerConfigureManagedGitTools` in the new process test file.
Seed all three installed tool fields in the initial AgentEnv.
Configure a current broker contract without those tool fields.
Before correction, helper, shim directory, and startup path disappear.
This is the expected red result.

Cover overlay and complete-indexed modes.
Cover initial managed and initial unmanaged instances.
Cover a new lease and different repository identity after a previous managed lease.
Cover nil, empty, and partial replacements without a broker.
Cover an explicit profile token and then a new managed contract.
Use synthetic authorization values and assert that old values remain absent.

`TestManagedGitToolsActivation` covers a replaced PATH, repeated activation,
spaces in executable paths, and unchanged unrelated PATH entries.
Cover parent `BASH_ENV`, parameter expansion, changed parent hooks, and shim self-sourcing.
When managed mode ends, an owned hook restores its parent and an unrelated user hook remains.
A malformed indexed block must leave command and environment unchanged.

`TestManagerConfiguredManagedGitToolsSubprocess` runs real Git and Bash against
the effective environment. A synthetic helper checks the current lease and repository path.
An allowed lookup succeeds. A foreign repository or helper failure yields no credential.
An ambient helper placed before the managed reset must not become a failure fallback.
Inspect the existing tracker/one-shot/shell environment seams without production logs or secret output.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test ./internal/agentctl/server/process -run '^(TestManagerConfigureManagedGitTools|TestManagerConfiguredManagedGitToolsSubprocess|TestManagerManagedGitConfigurationReplacement|TestManagerConfiguredManagedGitSubprocess|TestManagerConfigurePreservesUnmanagedLegacyHelper)$' -count=1)
(cd apps/backend && go test ./internal/agentctl/server/config -run '^(TestManagedGitToolsActivation|TestCollectAgentEnvKeepsGitHubCLIShimAheadOfProfilePath|TestCollectAgentEnvGitHubCLIShimSurvivesLoginShell|TestCollectAgentEnvResolvesParameterizedBashEnv|TestCollectAgentEnvAvoidsManagedBashEnvSelfSourcing|TestCollectAgentEnvLeavesGitHubStartupHookUntouchedWithoutBroker)$' -count=1)
(cd apps/backend && go test ./internal/agentctl/server/process ./internal/agentctl/server/config -count=1)
git diff --check
```

Run the first new regression before production changes and record its expected failure.
Run the commands again after correction and record the results.
Keep POSIX subprocess skips narrow. Keep environment-composition tests platform-neutral.

## Files likely touched

- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/manager_managed_git_tools_test.go` (new)
- `apps/backend/internal/agentctl/server/config/config.go`
- `apps/backend/internal/agentctl/server/config/managed_git_tools_test.go` (new)
- `docs/plans/docker-managed-git-credential-handoff/plan.md`
- This work order.

## Dependencies

None.

## Risks

- A broad saved environment revives old authorization. The saved fields must contain only installed tool paths.
- Deactivation can damage an unrelated PATH or startup hook. Remove only entries with exact installed-tool ownership.
- Existing callers use both configure modes. Preserve indexed merge and replacement semantics in both.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/integrations/requirements/github-authentication.md), criterion 001.15.
- [Managed configure design](../../specs/integrations/system-design/github-authentication-02.md#managed-tools-at-the-configure-boundary).
- [Credential policy ADR](../../decisions/2026-07-27-task-git-credential-policy.md).
- `manager_managed_git_handoff_test.go` for replacement and Git evidence.
- `config_test.go` for PATH and Bash parent-hook patterns.
- `apps/backend/internal/agentctl/AGENTS.md` for environment snapshots.

## Results

Implementation completed on 2026-09-30. The process manager now retains only
the installed helper, shim directory, and Bash startup path privately. It
reactivates them after a complete current broker contract is composed and
deactivates owned PATH and Bash entries when managed authorization disappears.
The current lease and identity still come only from the request.

Review follow-up preserves an incoming already-composed environment snapshot:
the owned wrapper is unwrapped after merge and before activation, so a valid
user parent hook is retained across repeated configuration. The process and
config environment tests also keep path and credential assertions active on
Windows while limiting Bash-specific assertions to Unix.

Verification:

- The new `TestManagerConfigureManagedGitTools` regression failed before the
  production change because the helper path was empty in both configure modes.
- The new `TestManagerConfiguredManagedGitToolsSnapshotRoundTrip` regression
  failed before review correction because the parent marker was dropped in
  both overlay and complete modes. It now verifies the hook runs and is restored
  after managed authorization is removed.
- `go test ./internal/agentctl/server/process ./internal/agentctl/server/config -count=1`
  passed after correction, including activation, replacement, subprocess,
  malformed-environment, and snapshot round-trip tests.
- Windows cross-compilation of both test packages passed. The Windows runtime
  assertions remain covered by the repository's Windows CI job.
