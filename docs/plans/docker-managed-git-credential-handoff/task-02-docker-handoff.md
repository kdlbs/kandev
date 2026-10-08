---
id: "02-docker-handoff"
title: "Complete Docker credential handoff and subprocess evidence"
status: done
wave: 2
depends_on:
  - "01-restore-managed-tools"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.15
system_design:
  - ../../specs/integrations/system-design/github-authentication-02.md
---

# Task 02: Complete Docker credential handoff and subprocess evidence

## Summary

Bind the managed Git helper to the installed executable for both Docker runtimes.
Verify the lifecycle configure handoff through the real process manager.
Prove that the final environment supports real Git and the managed CLI without stale authorization or fallback.

## In scope

- Generalize the existing Kubernetes normalizer for explicit Docker and Remote Docker runtimes.
- Bind the effective helper before execution snapshot storage and ConfigureAgent delivery.
- Retain Kubernetes restart normalization and avoid mappings for other runtimes.
- Add fresh, refreshed, and reconnect/resume environment handoff regressions.
- Verify the configured environment at the real managed CLI broker boundary.
- Preserve failure-before-Start behavior and nil/empty/partial replacement semantics.
- Record final work-order and package results.

## Out of scope

- New SSH, Sprites, or plugin executable-path mappings.
- Changing container bootstrap, image layouts, preparation scripts, or saved credentials.
- Live pushes, GitHub token discovery, Docker daemon provisioning, browser UI, or deployment.

## Acceptance

1. Managed Docker and Remote Docker configure requests and runtime snapshots use `/usr/local/bin/agentctl`. Kubernetes retains `/opt/kandev/agentctl`. Other runtimes receive no guessed path.
2. Fresh, refreshed, and reconnect/resume environments reach a real process manager with current broker identity and installed tools. Authorization-free replacements remain authorization-free, and failed configure delivery prevents Start.
3. The real managed CLI shim consumes post-Configure AgentEnv and redeems the current synthetic lease. A foreign scope or broker failure executes no CLI and uses no ambient login.

## TDD and test matrix

Start with `TestDockerManagedGitConfigureHandoff` in a new lifecycle test file.
Use `newManagedGitConfigureClient` as the HTTP/process-manager pattern.
Use a Docker-shaped request with broker identity and managed helper config but no helper path.
Seed the initial instance with installed tool metadata and obsolete synthetic authorization.
At the investigated revision, the resulting helper path is empty for both runtimes.
After Task 01, the inherited fallback can make an absent-path case succeed.
The supplied backend-host path case must still fail before this task's normalizer:
it retains the host path instead of `/usr/local/bin/agentctl`.
Use that case for this work order's red regression.

Cover fresh launch without `runtime_env`, refreshed lease through `SetExecutionEnv`,
and a recovered/reconnected execution snapshot without a worker helper path.
Cover nil, empty, and unrelated partial overlays that clear old managed values.
Cover a supplied backend-host helper path: Docker must replace it with the worker executable.
Assert equality between the bound execution snapshot and delivered helper contract.
Retain unrelated environment and inherited noncredential Git entries.

`TestNormalizeManagedGitHelperEnvironment` covers Docker, Remote Docker, Kubernetes,
standalone, SSH, Sprites, plugin remote, and unknown runtime values.
No broker means no synthesized helper on any runtime.
The Kubernetes recovered-snapshot and restart tests remain required.
A ConfigureAgent error must produce no Start request.

`TestConfiguredManagedGitHubCLIEnvironment` belongs in `cmd/agentctl`.
Construct a process manager with synthetic installed tools and obsolete authorization.
Configure it with a complete current broker identity and no process-local shim fields.
Pass its effective AgentEnv into the existing `runGitHubCLIShim` seams.
Use an `httptest` broker and fake real-CLI execution callback.
Assert the redeemed current lease, repository identity, and effective child token.
Use the existing shim-install and PATH-selection fixtures.
Cover foreign repository rejection, broker failure, and no fallback to an ambient host token or CLI login.
Keep token and lease values out of output and startup files.

Task 01 supplies real Git and Bash evidence from the same configured environment boundary.
No browser test is required because the package changes no rendered UI.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test ./internal/agent/runtime/lifecycle -run '^(TestDockerManagedGitConfigureHandoff|TestNormalizeManagedGitHelperEnvironment|TestKubernetesManagedGitConfigureHandoff|TestRestartedKubernetesManagedGitNormalizesRecoveredSnapshot|TestConfigureAndStartAgentKeepsLaunchManagedGitCredentials|TestHasRuntimeEnvOverlayDistinguishesAbsentAndEmptyOverlay)$' -count=1)
(cd apps/backend && go test ./cmd/agentctl -run '^(TestConfiguredManagedGitHubCLIEnvironment|TestPrepareGitHubCLIShimPublishesCredentialHelperExecutable|TestInstallGitHubCLIShimLoginShellRestoresManagedTools|TestGitHubCLIShimRefreshesAndIsolatesEachInvocation|TestGitHubCLIShimSelectsRepositoryLease)$' -count=1)
(cd apps/backend && go test ./internal/agent/runtime/lifecycle ./cmd/agentctl -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the new lifecycle regression before its production correction and record the expected failure.
Task 01's process/config commands must already pass and remain the evidence for its owned files.
Record every command result from its documented directory.
Do not mark the plan implemented until both work orders pass their checks.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_session_env.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_kubernetes_refresh.go`
- `apps/backend/internal/agent/runtime/lifecycle/managed_git_environment.go` (new, if the generalized helper moves)
- `apps/backend/internal/agent/runtime/lifecycle/manager_docker_managed_git_handoff_test.go` (new)
- `apps/backend/cmd/agentctl/github_cli_configured_env_test.go` (new)
- `docs/specs/integrations/system-design/github-authentication-02.md` (only if implementation clarifies this package's design)
- `docs/plans/docker-managed-git-credential-handoff/plan.md`
- This work order.

## Dependencies

[Task 01](task-01-restore-managed-tools.md).
The effective environment must restore installed shim metadata before the CLI handoff evidence can pass.

## Risks

- An incorrect runtime mapping can overwrite a valid local or remote path. Use explicit runtime constants and negative cases.
- Request-only test capture misses agentctl stripping. Follow the HTTP configure call into a real process manager.
- A fake CLI supplied with the request map bypasses the defect. Supply only the final configured environment.
- A container's clone can succeed while its agent fails. Clone success is not acceptance evidence for this task.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/integrations/requirements/github-authentication.md), criterion 001.15.
- [Docker worker helper design](../../specs/integrations/system-design/github-authentication-02.md#docker-worker-helper-binding).
- [Existing launch package](../managed-credential-launch-environment/plan.md).
- `manager_managed_git_handoff_test.go` and `manager_launch_credentials_test.go`.
- `container.go`, `executor_backend.go`, and orchestrator `executor_credentials.go` for the request/bootstrap distinction.
- `github_cli_shim_test.go` for synthetic broker and real-CLI seams.

## Results

Implementation completed on 2026-09-30. The lifecycle helper now binds managed
Git credential helpers to `/usr/local/bin/agentctl` for Docker and Remote Docker,
retains Kubernetes' `/opt/kandev/agentctl` mapping, and leaves other runtimes
unchanged. Binding occurs after fresh-snapshot or per-run-overlay composition
and before the execution snapshot and configure request are stored or sent.

The lifecycle regression follows fresh launch, lease refresh, recovered snapshot,
and nil/empty/partial overlays through a real process manager for both Docker
runtimes. A configure failure still prevents Start. The managed CLI regression
consumes the final process-manager environment, redeems a synthetic current
lease, rejects a foreign repository, and launches no CLI on broker failure.

Verification:

- The new Docker handoff regression failed before the normalizer change because
  Docker and Remote Docker delivered `/host/bin/agentctl` (or no helper) to the
  worker configure boundary.
- Focused lifecycle and agentctl CLI regression commands passed.
- `go test ./cmd/agentctl -count=1` passed.
- `go test ./internal/agent/runtime/lifecycle -count=1` passed when run alone.
  The first combined invocation reported a temporary directory cleanup race in
  `TestAgentctlResolverCachePruneDoesNotDelayDeadlineBoundLaunch`; the isolated
  rerun passed.
- `python3 scripts/list-docs.py validate` validated 333 decisions and 1262
  specifications; `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed.

A live Docker daemon and the reporter's custom image were not used; the regression
exercises the real lifecycle configure handoff and real process-manager
composition without container provisioning.
