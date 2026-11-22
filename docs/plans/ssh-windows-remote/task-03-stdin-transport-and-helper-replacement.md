---
id: "03-stdin-transport-and-helper-replacement"
title: "Fixed-length PowerShell transport and running-helper replacement"
status: done
wave: 3
depends_on:
  - "02-port-windows-ssh-lifecycle"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-WINDOWS-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-WINDOWS-001.5
  - AC-EXECUTORS-SSH-WINDOWS-001.6
  - AC-EXECUTORS-SSH-WINDOWS-001.9
  - AC-EXECUTORS-SSH-WINDOWS-001.19
system_design:
  - ../../specs/executors/system-design/ssh-windows-remote.md
---

# Task 03: Fixed-Length PowerShell Transport and Running-Helper Replacement

## Summary

Keep every Windows command line below the `cmd.exe` limit by moving script
bodies to standard input (F3). Allow helper replacement while a session runs
the previous helper (F4).

## In scope

- Change `WrapPowerShell` to encode only the fixed loader from the design's
  "PowerShell command transport" section.
  - Replace `buildPowerShellEnvInitScript` and the `powerShellStdinEnvImport`
    prefixing with `buildPowerShellEnvelope(login, env, script, input)`.
  - Every Windows call site sends that envelope on stdin:
    - probe, home, cached-helper, mkdir, and launch commands
    - log, pidfile, identity, stop, and orphan commands
    - broker, prepare, cleanup, and contribution commands
    - reconcile, which passes its body as `input`
- Change `decodePowerShellEncodedCommand` users in tests to decode the
  envelope through a test helper. The fake server matches on the decoded
  envelope `script`.
- Add failing tests first:
  - `TestSSHWindowsCommandLineStaysBelowCmdLimit`: each Windows command builder
    produces a command line under 8,191 characters. Use the default prepare
    script and a 64 KiB custom prepare script.
  - `TestSSHWindowsEnvelopeKeepsSecretsOffCommandLine`: secret environment
    values never appear in the command string.
  - `TestSSHWindowsReplacesRunningHelperByRenamingAside`: the fake server's
    `sftpHandler` request server imitates Windows. Rename onto an existing file
    fails, and removing an in-use file fails. The upload must succeed, keep the
    old image as `agentctl.exe.old-*`, and update the sidecar.
  - `TestSSHWindowsHelperReplacementRestoresPreviousOnFailure`: a failed final
    rename restores `agentctl.exe`.
- Implement rename-aside in `ensureWindowsAgentctlOnHost`. Use a Windows-only
  helper, and leave `renameSFTPUpload` unchanged for POSIX. Clean up stale
  aside files on a best-effort basis.

## Exclusions

- POSIX command transport.
- The controller launcher (task 04).
- Running real PowerShell (task 05).

## Acceptance conditions

1. No Windows command line exceeds 8,191 characters for any script size.
2. Environment values and scripts travel only on stdin.
3. A changed helper replaces an in-use helper without stopping the running
   session's process or failing the new launch.

## Verification

```bash
cd apps/backend
go test -trimpath -tags fts5 -race -count=1 -run '^TestSSHWindows' ./internal/agent/runtime/lifecycle ./internal/ssh
go test -trimpath -tags fts5 -count=1 -run 'SSH' ./internal/agent/runtime/lifecycle
cd ../.. && bash scripts/lint-go-changed $(git diff --name-only origin/main -- 'apps/backend/*.go')
```

## Lifecycle evidence

| Scenario | Criterion | Test |
| --- | --- | --- |
| Concurrent work | `001.6` | `TestSSHWindowsReplacesRunningHelperByRenamingAside` |

## Likely files and risks

- `executor_ssh_winremote.go`, `executor_ssh_windows_scripts.go`,
  `executor_ssh_credentials.go`, `executor_ssh_scripts.go`, `executor_ssh.go`,
  `ssh_fake_server_test.go`, and the three `executor_ssh_winremote_*_test.go`
  files.
- A script block changes `exit` semantics. Task 05 proves propagation on real
  PowerShell 5.1 and 7. Do not treat this work order as proof.
- Probes that consumed stdin for another purpose must take that data from
  `input`.

## Results

Completed on 2026-11-22.

Implementation:

- `executor_ssh_powershell.go` holds the loader, `powerShellEnvelope`,
  `newPowerShellCommand`, and the three run helpers. `WrapPowerShell`,
  `buildPowerShellEnvInitScript`, and the production
  `decodePowerShellEncodedCommand` are removed.
- The loader semantics were proven on the real test host before coding (see
  below). Two behaviors required loader changes. A dot-sourced script hides a
  failing last statement, so the loader appends `$__kLastOk = $?`. PowerShell
  5.1 writes redirected errors as `#< CLIXML`, so the loader catches errors and
  writes plain messages.
- `WrapLoginShell` returns to its upstream POSIX-only form. The stop,
  remove-directory, and orphan-inventory builders return scripts.
  `removeRemoteDir` dispatches the one shared caller.
- `sftpReplaceWindowsFile` installs the helper and its sidecar by renaming the
  existing file aside. `sftpUploadBytes` returns to its upstream form, so the
  fork's `renameSFTPUpload` is removed.
- Found during the port: the catalog's remote-auth setup scripts are POSIX
  shell, and the fork ran them under PowerShell. They are now skipped on Windows
  with a warning (`AC-EXECUTORS-SSH-WINDOWS-001.19`,
  `TestSSHWindowsSkipsPOSIXAuthSetupScripts`).
- The fake SFTP filesystem now refuses `rmdir` on a file. pkg/sftp retries a
  failed file removal as a directory removal, and the old fake deleted an
  in-use file through that path.

| Command | Result |
| --- | --- |
| `go test -race -run '^TestSSHWindows'` on `lifecycle` and `ssh` | Passed. Red first: build failures for the new transport, and the running-helper upload failed with `SSH_FX_FAILURE` |
| SSH, SFTP, reclaim, orphan, and Remote Docker tests across `lifecycle`, `ssh`, and `runtime` | 5 failures, all in the baseline set |
| `lint-go-changed` on every changed Go file | 0 issues |
| Production loader on the test host through stock `cmd.exe` (Windows PowerShell 5.1) | Command line 4,183 characters. Login `PATH` finds Git. Env with quotes, `$`, backtick, and CJK round-trips. `input` is delivered. `exit 7` returns 7, a failing last native command returns 5, and a 64 KiB script runs |

Lifecycle evidence:

| Scenario | Criterion | Test | Result |
| --- | --- | --- | --- |
| Concurrent work | `001.6` | `TestSSHWindowsReplacesRunningHelperByRenamingAside`, `TestSSHWindowsHelperReplacementRestoresPreviousOnFailure` | Passed |
