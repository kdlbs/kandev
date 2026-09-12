---
id: "02-runtime-propagation"
title: "Complete runtime propagation"
status: complete
wave: 2
depends_on: ["01-host-cli-route"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-002
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-002.2
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-002.3
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-002.4
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-002.5
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-002.6
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-002.7
system_design:
  - ../../specs/integrations/system-design/github-authentication-02.md
---

# Task 02: Complete runtime propagation

## Summary

Carry the bridge through each local launch path and the final child environment.
Prove that a reused executor or late profile token cannot defeat credential precedence.
Compose indexed Git definitions at the strict source-aware boundary and replace stale generated
entries at the agentctl configure/start boundary.

## In scope

- Cover full launch and `configureResumeGitHubCredentials` through the shared credential route.
- Extend prepared-workspace configuration with the resolved executor identity and repository set before agent start.
- Reuse preparation's resolved repositories instead of repeating materialization or remote reconciliation.
- Replace obsolete generated entries when the host, token, or policy changes on resume or prepared start.
- Preserve environment delivery errors instead of reporting successful bridge activation after failed delivery.
- Exercise lifecycle profile merges and `CollectAgentEnvWithError` using the supplied multi-entry host environment.
- Cover the same effective Git environment for agent processes and existing task shell/process consumers.
- Treat complete lifecycle snapshots and request-only configure calls as separate environment contracts.
- Remove obsolete marker-owned entries when a later preparation has no bridge, while preserving user entries.

## Out of scope

No new UI, snapshot schema, remote credential forwarding, or unrelated generic environment refactor.

## Acceptance

1. Launch, resume, and prepared start each pass with current eligibility and no duplicate indexed entries.
2. Late profile tokens remain the effective credential, and managed broker failure never invokes the host helper.
3. Real Git succeeds through the final environment without Git file changes, including a login shell that replaces `PATH`.

## TDD cases

Add the plan's launch/resume/prepared-workspace and child-environment tests before changing each path.
Use a spy host CLI to assert invocation counts and credential source without exposing real secrets.
Test logout between preparations, executor-to-managed transition, a newly explicit token, and failed environment delivery.
Test the bridge with a successful user helper, a failed managed helper, and a token supplied by a late profile merge.
For a failed explicit token, assert that no stored-login retry occurs.
Test `HOME` and `GH_CONFIG_DIR` overrides so probe/helper account selection stays consistent.

For process-level evidence, use real Git in an isolated home with a fake CLI.
Pass inherited and task blocks through the actual agentctl environment collector.
Assert unchanged hooks and notes, one generated helper, and no changed Git configuration files.
Use an executable path with spaces and a login shell that replaces `PATH`.
Keep platform-specific shell tests explicitly gated, while portable policy and composition tests run everywhere.

## Verification

From the repository root:

```bash
(cd apps/backend && go test -tags fts5 ./internal/orchestrator/executor -run 'TestExecutorHostGHBridge|TestConfigureGitHubCredentialBroker|TestConfigureGitCredentialBroker' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestBuildEnvForExecutionHostGHBridge|TestMergeEnvFillMissing' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/config -run 'TestCollectAgentEnv.*(HostGHBridge|GitConfig)' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process -run 'Test.*(HostGHBridge|GitEnv)' -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_host_gh_bridge_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/profile_env.go`
- `apps/backend/internal/agentctl/server/config/config.go`
- `apps/backend/internal/agentctl/server/config/config_test.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_env_test.go`

Only change runtime composition where the failing boundary tests require it.
Read `apps/backend/internal/agentctl/AGENTS.md` before agentctl implementation.

## Dependencies

Task 01.

## Risks

Early request maps do not contain every eventual profile variable.
Prepared-workspace starts can bypass the full credential route or retain an old generated environment.

## Parallelism

`sequential`

## Inputs

- Authentication criteria 002.2 through 002.7 and the design's runtime propagation section.
- `configureExistingWorkspace`, `configureResumeGitHubCredentials`, and `buildEnvForExecution`.
- `profile_env.go`, `CollectAgentEnvWithError`, and nearby indexed Git configuration regressions.

## Results

- Carried the effective bridge environment through initial launch, resume, and
  prepared-workspace agent starts.
- Composed agent-profile, executor-profile, managed-runtime, and repository Git
  blocks at strict resolution, then sent the resulting lifecycle snapshot through
  an explicit agentctl replacement mode.
- Refreshed agent and executor profile credential-store values before the host probe,
  preserved request precedence, replaced stale marker-owned entries, and returned
  environment delivery errors.
- The process manager tests verify both request-only overlay reconfiguration and
  complete replacement reconfiguration. They preserve user hooks/notes and remove
  generated helpers when the next request supplies none.
- The lifecycle test verifies that a prepared execution with an existing runtime
  snapshot sends one complete indexed block with `replace_env: true`, so the first
  agent start cannot append the snapshot to itself.
- `TestBuildEnvForExecutionHostGHBridge_ComposesStrictProfileBlocks` verifies the
  production strict resolver composes agent-profile, executor-profile, and managed
  indexed Git definitions while retaining the supplied entries.
- `TestLaunchPreparedSessionProbesEffectiveProfileCredentialStore` verifies both
  profile-directory mismatch directions through the production prepared-session route.
- `(cd apps/backend && go test -tags fts5 ./internal/orchestrator/executor -run 'TestExecutorHostGHBridge|TestConfigureGitHubCredentialBroker|TestConfigureGitCredentialBroker' -count=1)` passed.
- `(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestBuildEnvForExecutionHostGHBridge|TestMergeEnvFillMissing' -count=1)` passed.
- `(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/config -run 'TestCollectAgentEnv.*(HostGHBridge|GitConfig)' -count=1)` passed.
- `(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process -run 'Test.*(HostGHBridge|GitEnv)' -count=1)` passed.
- The final affected-package run passed with `-count=1` for the executor, lifecycle,
  agentctl client/API/config/process, backendapp, environment, and `gitconfigenv` packages.
- `make -C apps/backend lint` passed with zero issues on the final tree.
- `make -C apps/backend build` passed.
- `git diff --check` passed.
