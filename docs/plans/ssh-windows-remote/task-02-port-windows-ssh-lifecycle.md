---
id: "02-port-windows-ssh-lifecycle"
title: "Port the Windows SSH lifecycle with shell and passthrough guards"
status: pending
wave: 2
depends_on:
  - "01-publish-windows-helpers"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-WINDOWS-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-WINDOWS-001.1
  - AC-EXECUTORS-SSH-WINDOWS-001.2
  - AC-EXECUTORS-SSH-WINDOWS-001.3
  - AC-EXECUTORS-SSH-WINDOWS-001.4
  - AC-EXECUTORS-SSH-WINDOWS-001.7
  - AC-EXECUTORS-SSH-WINDOWS-001.8
  - AC-EXECUTORS-SSH-WINDOWS-001.9
  - AC-EXECUTORS-SSH-WINDOWS-001.10
  - AC-EXECUTORS-SSH-WINDOWS-001.12
  - AC-EXECUTORS-SSH-WINDOWS-001.13
  - AC-EXECUTORS-SSH-WINDOWS-001.14
  - AC-EXECUTORS-SSH-WINDOWS-001.15
  - AC-EXECUTORS-SSH-WINDOWS-001.17
system_design:
  - ../../specs/executors/system-design/ssh-windows-remote.md
---

# Task 02: Port the Windows SSH Lifecycle with Shell and Passthrough Guards

## Summary

Bring the fork's Windows executor code onto the PR A branch. Close the shell
and passthrough gaps, and make platform dispatch explicit. This is PR B's first
commit.

## In scope

- Run `git checkout 0191092a3 -- <file>` for the remaining executor files:
  - The `lifecycle/` files from the fork diff, except:
    - `agentctl_resolver.go` and the manifest files (task 01)
    - `passthrough_remote_mcp.go`
    - the four Remote Docker rename files: `container_host_files.go`,
      `executor_remote_docker.go`, `remote_docker_inputs.go`, and
      `../remote_docker_probe.go`
  - `internal/ssh/handlers.go` and `internal/ssh/handlers_test.go`
- Remove the duplicate `sshRemoteGOOSWindows` definition from
  `executor_ssh_windows_paths.go` because task 01 defines it.
- Gate split, following the design's "Platform detection and persistence"
  section:
  - Keep `requireSupportedRemotePlatform` at four platforms.
  - Add `requireSupportedSSHPlatform` (four plus `windows/amd64`), and use it on
    the SSH executor paths that the fork changed.
  - Drop the fork's `SSHRequireSupportedRemoteDockerPlatform` rename.
- Leave `passthrough_remote_mcp.go` unchanged, because passthrough rejects
  Windows (F8).
- F5:
  - Set `windowsCandidateShells` to `powershell` and `pwsh`.
  - After platform detection and before preparation, `CreateInstance` rejects a
    Windows host whose resolved shell fails `isPowerShellShell`. Use a typed
    error that names the saved shell and the supported shells.
- F8: In the same check, reject `req.IsPassthrough` for Windows hosts.
- F7:
  - Replace `isRemoteAgentctlAlive(ctx, client, pid, platforms ...)` with an
    explicit `platform` parameter.
  - Update every caller. Linux and macOS callers pass their known platform or
    `SSHRemotePlatform{}`.
- Rename the Windows tests to the `TestSSHWindows` prefix. Examples:
  - `TestSSHWindowsCreateInstancePowerShellEndToEnd`
  - `TestSSHWindowsPathClassificationAndCleaning`
- Add failing tests first:
  - `TestSSHWindowsRejectsNonPowerShellSavedShell`, with `bash` and `cmd`.
  - `TestSSHWindowsRejectsPassthroughLaunch`.
  - `TestSSHWindowsShellProbeOffersOnlyPowerShell`.
  - `TestSSHWindowsRemoteDockerRejectsWindowsHost`, using
    `SSHRequireSupportedRemotePlatform`, the Remote Docker gate.
  - A credential-helper test for a POSIX helper path that contains a space. It
    pins the deliberate quoting change in `gitCredentialHelperCommand`.
  - `TestSSHWindowsRejectsARM64Host`: a `KANDEV_WIN_INFO` probe that reports
    `ARM64` yields `windows/arm64`, which fails with the unsupported-platform
    error. That error lists `windows/amd64` as the only Windows platform.
- F12: `requireSupportedSSHPlatform` and its error text omit `windows/arm64`. Update the fork's ported tests that expect it to be supported:
  `TestRequireSupportedRemotePlatform` and
  `TestRequireSupportedRemotePlatformPreservesRawDetail`. Arch normalization
  still maps `ARM64`, so a rejection names the real platform.

## Exclusions

- The command transport and helper replacement (task 03). Keep the fork's
  `WrapPowerShell` until then.
- The WMI controller launcher (task 04).
- Windows-native script execution tests and CI wiring (task 05).
- Docs and copy (task 06).

## Acceptance conditions

1. Linux and macOS SSH tests show the same results as baseline. All ported
   `TestSSHWindows*` tests pass.
2. A Windows host with a `bash` or `cmd` saved shell, or with a passthrough
   launch, fails before preparation with the specified errors. A Windows shell
   probe lists only PowerShell shells.
3. No production Windows dispatch uses a variadic platform parameter.

## Verification

```bash
cd apps/backend
go test -trimpath -tags fts5 -race -count=1 -run '^TestSSHWindows|^TestSSHWindowsShellProbeOffersOnlyPowerShell$|^TestRequireSupportedRemotePlatform|^TestNormalizeSSHRemotePlatform|^TestSSHDefaultShellForPlatform|^TestQuoteWindowsArg|^TestMaterializeSSHSkillManifest' ./internal/agent/runtime/lifecycle ./internal/ssh
go test -trimpath -tags fts5 -count=1 -run 'SSH|RemoteDocker' ./internal/agent/runtime/... ./internal/ssh/...
go test -trimpath -tags fts5 -count=1 ./internal/agent/runtime/lifecycle/ ./internal/ssh/ ./internal/backendapp/
cd ../.. && bash scripts/lint-go-changed $(git diff --name-only origin/main -- 'apps/backend/*.go')
```

Compare the third command's failures with `origin/main` in the same
environment. Only a new failure counts as a regression.

## Lifecycle evidence

Record each of these tests with its command and result:

| Scenario | Test |
| --- | --- |
| Restart and recovery | `TestSSHWindowsBackendShutdownPreservesAgentctlAndResumeReconnects`, `TestSSHWindowsRecoverInstancesDetailedReattachesAndDiscoversControlPort` |
| Confirmed absence | `TestSSHWindowsResumeClearsMetadataWhenRemoteProcessExited` |
| PID reuse | `TestSSHWindowsPersistedStopVerifiesIdentityAndGuardsSessionDir` |
| Orphan ownership | `TestSSHWindowsOrphanSweepInventoriesAndStopsOrphanedAgentctl` |
| Entry paths | `TestSSHWindowsDetectRemoteInfoFallsBackToPowerShell`, `TestSSHWindowsShellProbeOffersOnlyPowerShell` |

## Likely files and risks

- `executor_ssh.go`, `executor_ssh_operations.go`, `executor_ssh_winremote.go`,
  `executor_ssh_windows_paths.go`, `executor_ssh_windows_scripts.go`,
  `internal/ssh/handlers.go`.
- Go lint limits: the shell guard can push `CreateInstance` over the function
  length or complexity limit. Extract `validateSSHWindowsLaunch(req, platform,
  shell)`.
- Upstream may have changed the same functions after `5e4c1d3a5`. The merge-tree
  dry run was clean on `e3b5949fa`. Re-run it before checkout if `main` moved.

## Results

Pending.
