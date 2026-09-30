---
created: 2026-09-30
status: implemented
requirements:
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001
system_design:
  - ../../specs/integrations/system-design/github-authentication-02.md
legacy_specs: []
---

# Implementation Plan: Docker managed Git credential handoff

## Overview

[Issue #4081](https://github.com/kdlbs/kandev/issues/4081) reports broken Git
and managed `gh` authentication in Docker since versions 0.95/0.96.
This package restores the current managed contract and installed tools at agent
configuration. It preserves removal of obsolete authorization on reused instances.

The integration system owns task Git credential policy and broker scope.
Executor and agentctl code carry that contract into the worker.
The active criterion is `AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.15` in the
[authentication requirements](../../specs/integrations/requirements/github-authentication.md).
This repair reuses that criterion. It adds no product requirement or ADR.
The existing [credential policy decision](../../decisions/2026-07-27-task-git-credential-policy.md)
remains authoritative.

Task 01 repairs installed-tool activation at agentctl configuration.
Task 02 completes the Docker lifecycle handoff and verifies Git and the managed CLI.
Both work orders are sequential.

## Confirmed cause and reproduction

Investigation used source revision `d05b0a91c80` on 2026-09-30.
The canonical issue has no comments or image attachments.
The issue is assigned to `carlosflorencio`.

`composeConfiguredAgentEnvironment` removes previous managed authorization and
tool variables before it merges the incoming environment.
That removal protects reused instances from obsolete leases.
Docker bootstrap sets the worker helper path in `ContainerManager.buildEnvVars`.
The orchestrator sets a helper path only for Local and Worktree requests.
`ExecutorInstance.ToAgentExecution` captures the request environment, so Docker's
worker helper binding does not enter the execution snapshot.
The lifecycle configure path normalizes only Kubernetes.
Agentctl then removes its initial worker helper path without a replacement.
The managed helper entry survives through the request and expands an empty executable.

Agentctl also removes the installed shim directory and Bash startup metadata.
The backend does not know those process-local paths.
An inherited PATH or `BASH_ENV` can still reference the shim, but later path
replacement and login-shell restoration lose the activation metadata.
This gap affects the shared configure path even when authorization survives.

The [earlier launch package](../managed-credential-launch-environment/plan.md)
already prevents an overlay-free lifecycle launch from stripping its broker values.
Current reproduction therefore retains the broker and lease but loses the helper.
The reported 0.96 environment lost the entire contract.
The repair must retain the earlier fix and cover both configure boundaries.

Temporary Go tests used `go test -overlay` with sources outside the repository.
No production or permanent test files changed during investigation.

| Probe | Observed result |
| --- | --- |
| Lifecycle snapshot through the real process-manager configure composition, Docker and Remote Docker | Both fail: broker and current lease present, helper path empty. |
| Real `git credential fill` with the configured environment | Fails with the reported `Permission denied` helper error and terminal prompts disabled. |
| Same Git subprocess with explicit current helper, shim directory, and startup path | Succeeds with synthetic credentials. |
| Existing Kubernetes handoff, restart normalization, and overlay-free launch tests | Pass. |
| Existing process-manager credential replacement, Git subprocess, and unmanaged legacy helper tests | Pass. |

## Scope

### In scope

- Current managed credentials plus executable worker helper paths on both Docker runtimes.
- Process-local tool activation after configuration, including repeated configuration and login shells.
- Removal of old authorization on nil, empty, partial, and unmanaged replacement requests.
- Synthetic Git and managed CLI evidence at the final configured environment.
- Compatibility with existing Kubernetes normalization and local/remote credential boundaries.

### Out of scope

- Restoring PID 1 authorization or removing credential cleanup.
- Credential policy, broker issuance, lease reissue, token permissions, and repository scope changes.
- New executor support, SSH/Sprites path normalization, image layout changes, or a release toggle.
- Rendered UI, translations, terminal reconnection behavior, or a browser test.
- Deployment, release, commit, push, PR creation, or live GitHub mutations during tests.

## Technical approach

### Agentctl tool activation

Capture only `CredentialHelperPathEnv`, `CredentialCLIShimDirEnv`, and
`CredentialCLIBashEnvEnv` from the initial instance environment in `process.NewManager`.
Keep this private installed-tool snapshot separate from mutable authorization.
Do not capture the lease, broker URL, identities, scopes, or bearer tokens.

Keep `removeObsoleteManagedCredentialEnvironment` and generated Git filtering.
After valid indexed composition, activate tools only for an incoming managed broker contract.
Use the captured helper path as a fallback for an absent request helper.
Use the process-local shim directory and startup path for CLI activation.
Reuse config's PATH and parent-hook logic through a small shared helper.
Do not re-read ambient credentials at configuration or command execution.

PATH activation must not accumulate the installed directory.
A request PATH replacement still receives the shim prefix in managed mode.
The Bash hook retains its original or replacement parent without recursive self-sourcing.
When managed mode ends, remove only the owned PATH entry and restore an owned
shim hook to its parent. Do not change an explicit unrelated user hook.
Capture installed paths even for an initially unmanaged instance so managed reactivation works.

Commit the final environment only after all composition succeeds.
Keep tracker, one-shot adapter, shells, and task commands on the same canonical slice.

### Docker lifecycle binding

Generalize `normalizeKubernetesManagedGitEnvironment` into an explicit runtime normalizer.
A small new file can own that function if its name no longer fits the Kubernetes file.
Call it at `configureAndStartAgent` and the existing Kubernetes restart site.
Map Docker and Remote Docker to `remoteAgentctlExecutablePath`.
Retain Kubernetes's current path.
Bind only when the effective environment carries the managed broker contract.
Preserve all other runtimes without guessing their executable paths.

The execution snapshot and ConfigureAgent request must contain the same bound helper path.
Fresh launch, current credential refresh, and reconnect/resume through this boundary share the binding.
An explicit empty or partial per-run replacement must still clear previous managed authorization.

### Compatibility matrix

| Runtime / contract | Transport and identity | Intended behavior | Evidence / fallback |
| --- | --- | --- | --- |
| Docker, managed | Container HTTP configure, current task/repository lease | Bind `/usr/local/bin/agentctl`, activate installed tools | Fresh and refreshed handoff tests, real Git and CLI evidence |
| Remote Docker, managed | SSH daemon connection, container HTTP configure, current lease | Same worker helper path and tools | Same handoff matrix, no live daemon required |
| Kubernetes, managed | Pod forwarding, session-scoped lease | Retain `/opt/kandev/agentctl` binding and restart semantics | Existing handoff and recovered-snapshot tests |
| Standalone, managed | Local HTTP, current lease | Keep supplied local helper path and installed tools | Shared configuration tests |
| SSH, Sprites, plugin remote | Provider-specific control paths | Shared tool activation only, no new lifecycle path mapping | Negative normalizer tests, existing provider coverage |
| Any runtime, unmanaged or authorization absent | No incoming managed broker contract | Clear old authorization and generated managed entries, deactivate owned tools | Nil/empty/partial, managed-to-unmanaged, and off-to-on tests |
| Unknown runtime | No established executable identity | Preserve explicit request data, do not infer a worker path | Negative normalizer test |

## Tests

The permanent regression tests below implement the evidence matrix for
`AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.15`. The credential-policy ADR and
design define removal and fail-closed compatibility.

| Test file | Implemented regression and evidence |
| --- | --- |
| `server/process/manager_managed_git_tools_test.go` | `TestManagerConfigureManagedGitTools` and `TestManagerConfigureManagedGitToolsDeactivateAndReactivate`: installed paths survive current managed authorization in overlay and complete modes; stale authorization is removed on nil, empty, and partial replacements. Bash-specific expectations run on Unix, while Windows still checks path and credential composition and verifies Bash variables are unchanged. |
| `server/process/manager_managed_git_tools_test.go` | `TestManagerConfiguredManagedGitToolsSnapshotRoundTrip`: sends the effective managed `AgentEnv` back through overlay and complete configuration. On Unix, the user hook marker survives and runs, then unmanaged configuration restores and runs the original hook. On Windows, the test confirms `BASH_ENV` is unchanged and no parent marker is synthesized. |
| `server/process/manager_managed_git_tools_test.go` | `TestManagerConfiguredManagedGitToolsSubprocess`: real Git lookup and a Bash subprocess use the final configured environment. The allowed repository succeeds, a foreign repository fails, and an ambient helper does not run. |
| `server/process/manager_managed_git_tools_test.go` | `TestManagerConfigureManagedGitToolsMalformedEnvironmentIsAtomic`: malformed indexed Git configuration leaves the command and environment unchanged. |
| `server/config/managed_git_tools_test.go` | `TestManagedGitToolsActivation`: PATH deduplication, spaces, parent-hook expansion, self-sourcing prevention, and selective deactivation. Windows assertions confirm activation does not synthesize a parent marker or modify `BASH_ENV`. |
| `runtime/lifecycle/manager_docker_managed_git_handoff_test.go` | `TestDockerManagedGitConfigureHandoff`: Docker and Remote Docker fresh/refresh/reconnect input reaches a real process manager with the worker path. Nil, empty, and partial replacements remove managed authorization. |
| `runtime/lifecycle/manager_docker_managed_git_handoff_test.go` | `TestNormalizeManagedGitHelperEnvironment`: explicit runtime mapping and unmanaged/unknown negative cases. |
| `cmd/agentctl/github_cli_configured_env_test.go` | `TestConfiguredManagedGitHubCLIEnvironment`: effective AgentEnv reaches the real shim function, redeems the current synthetic lease through an HTTP broker fixture, and gives the fake CLI the expected token. Foreign scope or broker failure launches no CLI and uses no host login. |

## End-to-end evidence

The package uses executable Git, Bash, and managed CLI boundary evidence rather
than a browser test. The defect occurs after agentctl environment composition.
A browser capture cannot prove that a Git helper receives the lease and executable path.
Fixtures must consume the effective post-Configure AgentEnv, not the original request map.
Use no real repository, token, push, or external network request.

The CLI test uses the existing `runGitHubCLIShim` broker and execution seams.
The lifecycle test follows the HTTP configure handoff into `process.Manager`.
Task 02 verifies that a configure failure prevents `Start`.

## Work orders

- [x] [Task 01: Restore installed managed tools during configuration](task-01-restore-managed-tools.md)
- [x] [Task 02: Complete Docker credential handoff and subprocess evidence](task-02-docker-handoff.md)

## Verification results

Investigation evidence:

- Temporary Docker and Remote Docker handoff probe: expected failure for the missing helper path.
- Temporary real Git probe: reported error reproduced, explicit tool handoff succeeds.
- Four targeted lifecycle tests: pass.
- Three targeted process-manager tests: pass.

Task 01 and Task 02 implementation results are recorded in their work orders.
Both work orders are complete.

Implementation verification:

- The new snapshot round-trip regression failed before correction because the
  configured parent marker was dropped in both overlay and complete modes.
- `go test ./internal/agentctl/server/process ./internal/agentctl/server/config -count=1`
  passed after the correction.
- `go test ./internal/agent/runtime/lifecycle -count=1` and
  `go test ./cmd/agentctl -count=1` passed. A combined first run hit a temporary
  directory cleanup race in `TestAgentctlResolverCachePruneDoesNotDelayDeadlineBoundLaunch`;
  the standalone lifecycle rerun passed.
- Windows cross-compilation of the process and config test packages passed.
- `go build ./...` passed from `apps/backend`.
- `python3 scripts/list-docs.py validate` and
  `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed.
Design-package validation before implementation passed:

- `python3 scripts/list-docs.py validate`: 333 decisions and 1262 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: both work orders covered, no errors. The local preflight supplied the planned runtime path and current document contents.
- All package relative Markdown links resolve to existing files.
- `git diff --check -- docs/specs docs/plans/docker-managed-git-credential-handoff`: passed.

The investigation's temporary overlay probes remained outside the repository.
The permanent implementation and regression tests now accompany the design
and work orders as the completed package. Investigation findings and
implementation results are recorded in their separate sections above.

## Related packages and documentation

The launch-environment, executor-host-gh-bridge, CLI-reentry, environment-scrub,
protocol-resolution, upgrade-recovery, origin-reconciliation, and noninteractive-Git
packages reference the same authentication requirement.
Their completed work remains evidence for the existing boundaries.
This package does not reopen their work orders or alter their reported results.

Public documentation impact was reviewed through `/docs-maintainer`.
[Task Git access](../../public/integrations.md#choose-task-git-credentials)
already promises the managed helper and shim, explicit-token precedence, and remote isolation.
This repair restores that contract without changing operator commands or options.
Only internal design and delivery records need updates.

## Risks

- Restoring authorization from the inherited environment can revive a stale lease. Restore only installed tool paths after the current request qualifies.
- An incoming host helper path cannot run inside Docker. Normalize the effective lifecycle environment before snapshot storage and configure delivery.
- PATH and Bash restoration can overwrite a user hook or duplicate the shim. Cover selective ownership, spaces, replacement, and repeated activation.
- A retained instance can switch policies. Off-to-on tests must prove tools survive privately while old authorization stays absent.
- The temporary probes did not launch a real Docker daemon or the reporter's custom image. Shared production composition and real Git evidence establish the defect, while container smoke remains optional operator evidence.
