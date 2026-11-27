---
created: 2026-11-22
updated: 2026-11-22
status: in_progress
requirements:
  - REQ-EXECUTORS-SSH-WINDOWS-001
  - REQ-EXECUTORS-SSH-WINDOWS-002
system_design:
  - ../../specs/executors/system-design/ssh-windows-remote.md
  - ../../specs/release/system-design/compact-runtime-distribution.md
legacy_specs: []
---

# Implementation Plan: SSH Executor Windows Remote Hosts

## Overview

This plan lands Windows support for the SSH executor upstream. It uses two clean
PRs built from a fork implementation that already exists. The
[requirements](../../specs/executors/requirements/ssh-windows-remote.md) and
[system design](../../specs/executors/system-design/ssh-windows-remote.md) own
the behavior. This plan owns delivery order, the PR split, and evidence.

There is no new UI. Task 05 changes only three description strings.

## Continuation context

The implementation source is commit `0191092a3f` on the fork's `main` branch,
"feat(ssh): support Windows remote executor via PowerShell and agentctl.exe".
The fork's `32e795de6 release: 0.102.1` commit is fork-local and excluded from
every PR. Cross-compiled for `windows/amd64`, `agentctl` is a 63.4 MiB PE32+
executable, 28.2 MiB after `gzip -9`. The design's
[real-host evidence](../../specs/executors/system-design/ssh-windows-remote.md#real-host-evidence)
records the probes that shaped the transport and launch.

## Findings before upstream review

| # | Finding | Evidence | Resolution |
| --- | --- | --- | --- |
| F1 | `defaultReleaseBaseURL` points at the fork's releases | `agentctl_resolver.go` diff | Task 01 restores `kdlbs/kandev` |
| F2 | The `KANDEV_REMOTE_HELPER_BASE_URL` environment variable bypasses the typed configuration catalog | `os.Getenv` in `NewAgentctlResolverWithOptions` | Task 01 removes it (`AC-EXECUTORS-SSH-WINDOWS-002.4`) |
| F3 | The default prepare command has 13,171 characters; `cmd.exe` allows 8,191 | Measured on the rebased tree; reproduced on the real host | Task 03 adds stdin envelope transport |
| F4 | Replacing `agentctl.exe` while any session runs it fails | Remove-then-rename in `renameSFTPUpload` | Task 03 renames the old file aside |
| F5 | The Windows shell probe offers `cmd` and `bash`, which reach POSIX branches with Windows paths | `windowsCandidateShells`, `isPowerShellShell` call sites | Task 02 limits and validates the shell |
| F6 | Tests never execute generated PowerShell | The fake server matches substrings and returns canned output | Task 05 adds Windows-native execution tests |
| F7 | `isRemoteAgentctlAlive` takes an optional variadic platform | Its signature | Task 02 makes the platform explicit |
| F8 | Windows passthrough launches are unguarded | No platform check on the passthrough path | Task 02 rejects them (`AC-EXECUTORS-SSH-WINDOWS-001.17`) |
| F9 | Specs, public docs, and UI copy still say Linux and macOS only | `ssh-executor.md` out-of-scope list, `executors.json` | Tasks 01 and 06 |
| F10 | The CI filter is an 11-alternative regex copied into two files | `backend-tests.yml`, `Makefile` | Task 05 uses a `TestSSHWindows` prefix |
| F11 | OpenSSH ends a `Start-Process` controller when its launch channel closes | Real-host probe | Task 04 launches through `Win32_Process.Create` |
| F12 | The fork ships and accepts `windows/arm64`, which is now out of scope | Fork helper tables and `requireSupportedRemotePlatform` | Task 01 removes the helper; task 02 rejects the platform |

## Decisions

The user decided these points on 2026-11-22:

- Scope `windows/amd64` only. Windows ARM64 hosts are rejected as unsupported,
  and no `windows/arm64` helper ships. This drops real-host scenario H9.
- Real-host scenario H7 may set `DefaultShell` to PowerShell on the test host.
  The previous state (value absent) must be restored afterward.

## PR strategy

Open two PRs from fresh branches off upstream `main`. Do not branch from the
fork's `main`, because it carries `release: 0.102.1` and is 29 commits behind.

1. **PR A: `feat(release): publish Windows agentctl remote helpers`** (task 01).
   This PR changes release, bundle, Desktop, and container packaging. SSH still
   rejects Windows, so user behavior does not change. Release owners review it
   separately, and it carries the size cost. Its first commit adds this design
   package, because the release design links to
   `REQ-EXECUTORS-SSH-WINDOWS-002`.
2. **PR B: `feat(ssh): support Windows remote hosts`** (tasks 02 through 06).
   This PR is stacked on PR A. Keep it in draft until PR A merges, then rebase
   it onto `main`. Use one commit per work order. Its executor-only diff also
   contains public docs, copy, spec promotion, and work-order results.

PR B cannot merge first. The helper resolver needs the Windows manifest entries
from PR A.

Each PR body records its evidence table and size impact. PR B also records the
real-host validation matrix from task 06 and links the requirement and design.

## Dependency order

| Wave | Work order | Depends on | PR |
| --- | --- | --- | --- |
| 1 | [01 Publish Windows helpers](task-01-publish-windows-helpers.md) | None | A |
| 2 | [02 Port the Windows SSH lifecycle](task-02-port-windows-ssh-lifecycle.md) | 01 | B |
| 3 | [03 Fix the command line and helper replacement](task-03-stdin-transport-and-helper-replacement.md) | 02 | B |
| 4 | [04 Launch the controller through WMI](task-04-cim-controller-launch.md) | 03 | B |
| 5 | [05 Execute PowerShell on Windows CI](task-05-windows-native-script-tests.md) | 04 | B |
| 6 | [06 Docs, copy, and real-host evidence](task-06-docs-copy-and-host-validation.md) | 02, 05 | B |

The order is sequential. Each work order changes code that the next one tests.

## Lifecycle evidence

| Scenario | Criterion | Invariant | Evidence |
| --- | --- | --- | --- |
| Restart and recovery | `001.12` | A live controller is reattached without replacement | `TestSSHWindowsBackendShutdownPreservesAgentctlAndResumeReconnects`, `TestSSHWindowsRecoverInstancesDetailedReattachesAndDiscoversControlPort`; real host (task 06) |
| Confirmed absence | `001.12` | An exited controller clears its metadata and follows replacement | `TestSSHWindowsResumeClearsMetadataWhenRemoteProcessExited` |
| Stale owner and PID reuse | `001.13` | A reused PID without the recorded workdir survives | `TestSSHWindowsPersistedStopVerifiesIdentityAndGuardsSessionDir`; Windows-native identity test (task 05) |
| Concurrent work | `001.6` | Helper replacement does not stop a running session | New SFTP rename-aside test (task 03); Windows-native running-image test (task 05) |
| Orphan ownership | `001.14` | Only proven orphans stop | `TestSSHWindowsOrphanSweepInventoriesAndStopsOrphanedAgentctl`; Windows-native inventory test (task 05) |
| Channel close | `001.11` | The controller survives its launch channel | Real-host probe (F11); Windows-native survival test (task 04); real-host H3 (task 06) |
| Entry paths | `001.1`, `.3` | Connection test, shell probe, and launch use one detection path | `TestSSHWindowsDetectRemoteInfoFallsBackToPowerShell`, `TestSSHWindowsShellProbeOffersOnlyPowerShell`, `TestSSHWindowsCreateInstancePowerShellEndToEnd` |

Pause and resume have no separate SSH path. Idle parking uses the stop and
resume paths above. Duplicate delivery is excluded. The lost-race path only
copies the winning session's persisted platform, which
`TestSSHWindowsCreateInstancePowerShellEndToEnd` covers.

## Risks

- **WMI availability.** The launcher depends on the WMI service and on the SSH
  user's right to call `Win32_Process.Create`. A local account in the default
  configuration has both. A hardened host can deny them, in which case launch
  fails with the WMI return code. Public docs name that requirement.
- **Distribution size.** Complete bundles and the container image grow by about
  63 MiB, and release assets by about 28 MiB. The PR A body must state this.
- **PowerShell 5.1 and 7 differences.** Task 05 runs both on CI.
- **Endpoint protection.** Defender can quarantine an unsigned `agentctl.exe`.
  Public docs name that failure mode and its symptoms.
- **Cross-compile vet.** `GOOS=windows go vet` on packages with CGO SQLite fails
  locally without a Windows CGO toolchain. Use the Windows CI suite instead.

## Verification strategy

Each work order lists exact commands. The PR evidence comes from targeted Go
tests, the Windows `native` CI suite, release contract tests, and a real-host
matrix. Treat unchanged baseline failures in this macOS environment as
environmental. Investigate and record any new failure.
