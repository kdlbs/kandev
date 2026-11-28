---
id: "05-windows-native-script-tests"
title: "Execute generated PowerShell on the Windows CI runner"
status: pending
wave: 5
depends_on:
  - "04-cim-controller-launch"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-WINDOWS-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-WINDOWS-001.5
  - AC-EXECUTORS-SSH-WINDOWS-001.6
  - AC-EXECUTORS-SSH-WINDOWS-001.7
  - AC-EXECUTORS-SSH-WINDOWS-001.8
  - AC-EXECUTORS-SSH-WINDOWS-001.9
  - AC-EXECUTORS-SSH-WINDOWS-001.13
  - AC-EXECUTORS-SSH-WINDOWS-001.14
system_design:
  - ../../specs/executors/system-design/ssh-windows-remote.md
---

# Task 05: Execute Generated PowerShell on the Windows CI Runner

## Summary

Prove the PowerShell semantics that fake-server tests cannot prove (F6). Wire
one stable test prefix into Windows CI (F10).

## In scope

- Add `executor_ssh_windows_exec_windows_test.go` with `//go:build windows`.
  - Its helper runs a generated command the way stock OpenSSH does:
    `exec.Command("cmd.exe", "/d", "/c", command)`, with the envelope on stdin.
  - Each case runs under `powershell` and, when `exec.LookPath("pwsh")`
    succeeds, under `pwsh`.
- Use a Go helper process as the stand-in controller. Copy the test binary to
  `<tmp>\bin\agentctl.exe` and start it with `--workdir <task dir>` and a
  `-test.run` sleep helper. Its image name and command line then match a real
  launch.
- Add these cases. Each one fails before its fix where a fix is needed:
  - `TestSSHWindowsExecEnvelopeImportsEnvironment`: quotes, `$`, backticks,
    and non-ASCII values reach the script unchanged.
  - `TestSSHWindowsExecPropagatesExitCodes`: `exit 7`, a throw, and a failing
    native command return 7, 1, and that command's status.
  - `TestSSHWindowsExecDefaultPrepare`: uses a local bare Git fixture in a path
    with spaces and an apostrophe.
    - A fresh clone succeeds and the branch is checked out.
    - Rerunning reuses the checkout.
    - A different origin aborts with non-zero status and the conflict message.
  - `TestSSHWindowsExecLargeCustomPrepare`: a 64 KiB script runs through
    `cmd.exe /c`.
  - `TestSSHWindowsExecStopTerminatesTreeAndRemovesSessionDir`: the stand-in
    starts one child.
    - Stop ends both processes and removes the session directory.
    - A PID with a different workdir is not stopped.
  - `TestSSHWindowsExecIdentityAndOrphanInventory`: real `Get-CimInstance`
    output parses into the expected process, task ID, and pidfile claim.
  - `TestSSHWindowsExecLogReadWhileWriterHoldsFile`: the log is readable while
    the stand-in holds it open for writing.
  - `TestSSHWindowsExecReplacesRunningHelper`: covers helper replacement while
    the stand-in runs from `agentctl.exe`. It uses the fake SSH server with
    local exec and real-filesystem SFTP, which proves rename-aside on NTFS.
- Add a step to the `native` suite in `.github/workflows/backend-tests.yml`:
  `go test -trimpath -tags fts5 -race -v -run '^TestSSHWindows' ./internal/agent/runtime/lifecycle ./internal/ssh`.
- Add the same prefix filter to `test-windows` in `apps/backend/Makefile`.
  Replace the fork's alternation list.

## Exclusions

- Real SSH to a Windows host (task 06).
- Changes to production code, except fixes found by these tests. Record each
  fix in Results.

## Acceptance conditions

1. Every case passes on the Windows CI runner under Windows PowerShell 5.1. It
   also passes under PowerShell 7 when `pwsh` is installed.
2. CI and `make test-windows` run the same `^TestSSHWindows` set.
3. A deliberately broken loader, such as one that drops `exit` codes, fails at
   least one case. Revert the break before commit.

## Verification

```bash
cd apps/backend
go vet -tags fts5 ./internal/agent/runtime/lifecycle ./internal/ssh
go test -trimpath -tags fts5 -count=1 -run '^TestSSHWindows' ./internal/agent/runtime/lifecycle ./internal/ssh
```

On Windows, run the same command under `make test-windows`. Record the PR's
`native` suite job URL and result.

## Lifecycle evidence

| Scenario | Criterion | Test |
| --- | --- | --- |
| Stale owner and PID reuse | `001.13` | `TestSSHWindowsExecStopTerminatesTreeAndRemovesSessionDir` |
| Orphan ownership | `001.14` | `TestSSHWindowsExecIdentityAndOrphanInventory` |
| Concurrent work | `001.6` | `TestSSHWindowsExecReplacesRunningHelper` |

## Likely files and risks

- New `executor_ssh_windows_exec_windows_test.go`;
  `.github/workflows/backend-tests.yml`; `apps/backend/Makefile`.
- Runner image changes can remove `pwsh`. The `pwsh` cases skip, and
  `powershell` cases still run.
- Windows Defender on hosted runners can slow the copied test binary. Use
  bounded waits and diagnostic logs, not sleeps.

## Results

Pending.
