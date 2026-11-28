---
id: "03-stdin-transport-and-helper-replacement"
title: "Fixed-length PowerShell transport and running-helper replacement"
status: pending
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

Pending.
