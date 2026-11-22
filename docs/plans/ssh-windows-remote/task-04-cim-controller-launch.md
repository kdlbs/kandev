---
id: "04-cim-controller-launch"
title: "Launch the Windows controller through WMI"
status: pending
wave: 4
depends_on:
  - "03-stdin-transport-and-helper-replacement"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-WINDOWS-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-WINDOWS-001.9
  - AC-EXECUTORS-SSH-WINDOWS-001.11
  - AC-EXECUTORS-SSH-WINDOWS-001.12
  - AC-EXECUTORS-SSH-WINDOWS-001.13
system_design:
  - ../../specs/executors/system-design/ssh-windows-remote.md
---

# Task 04: Launch the Windows Controller Through WMI

## Summary

Replace the fork's `Start-Process` launch. Win32 OpenSSH ends that process when
its launch channel closes (F11). This work order implements the design's
"Controller launch, readiness, and resume" section.

## In scope

- Rewrite the script in `startRemoteWindowsAgentctlOnPort`:
  - Build a merged environment block: the launching process environment,
    overlaid with the envelope `env` and `AGENTCTL_PORT`.
  - Call `Invoke-CimMethod Win32_Process Create` with the `cmd.exe /d /c`
    redirection wrapper, the task directory as `CurrentDirectory`, a hidden
    window, and that environment block.
  - Fail on a non-zero return value and include the code.
  - Poll for the `agentctl.exe` child of the wrapper, with a 10-second bound.
  - Write that child's PID to `agentctl.pid` and print it. When no child
    appears, terminate the wrapper tree and report the log tail.
- Collapse `agentctl.err.log` into `agentctl.log`, because the wrapper redirects
  both streams. Update the log readers and their fake-server cases.
- Add failing tests first:
  - `TestSSHWindowsLaunchUsesWMIWithMergedEnvironment`: the decoded launch
    script calls `Win32_Process` `Create`. Secret values appear only in the
    stdin envelope. The script builds the environment from
    `[Environment]::GetEnvironmentVariables()` plus the envelope.
  - `TestSSHWindowsLaunchRecordsAgentctlChildPID`: the fake output reports a
    wrapper PID and a child PID. The instance records the child PID, which the
    launch receipt validation accepts.
  - `TestSSHWindowsLaunchFailsOnWMIReturnCode`: a non-zero return fails the
    launch with that code.
  - `TestSSHWindowsLaunchStopsWrapperWhenChildMissing`: a missing child ends the
    wrapper tree and reports the log tail.
- Add a `//go:build windows` survival case to task 05's test file, or create
  that file here. `TestSSHWindowsExecLaunchSurvivesParentExit` runs the launch
  script through `cmd.exe /c` and waits for the PowerShell parent to exit. It
  then asserts that the stand-in `agentctl.exe` is alive, reads its environment
  marker from the log, and stops it.

## Exclusions

- Task scheduler, service, or `psexec` launch paths.
- Launch changes for Linux or macOS.

## Acceptance conditions

1. The launched controller is a WMI child, outside the channel's job, and the
   recorded PID is the `agentctl.exe` PID.
2. Environment values never appear in a command line, and the child keeps
   `PATH` and `USERPROFILE`.
3. The launch failure modes return their specified errors and leave no process
   behind.

## Verification

```bash
cd apps/backend
go test -trimpath -tags fts5 -race -count=1 -run '^TestSSHWindows' ./internal/agent/runtime/lifecycle ./internal/ssh
cd ../.. && bash scripts/lint-go-changed $(git diff --name-only origin/main -- 'apps/backend/*.go')
```

Then run the real-host survival smoke against the Windows test host,
following task 06 H3, on an isolated instance. Record the result here.

## Lifecycle evidence

| Scenario | Criterion | Test |
| --- | --- | --- |
| Channel close | `001.11` | `TestSSHWindowsExecLaunchSurvivesParentExit`; real host H3 |
| Restart and recovery | `001.12` | `TestSSHWindowsBackendShutdownPreservesAgentctlAndResumeReconnects`, rerun after the launcher change |
| PID ownership | `001.13` | `TestSSHWindowsLaunchRecordsAgentctlChildPID` |

## Likely files and risks

- `executor_ssh_winremote.go`, `executor_ssh_winremote_deploy_launch_test.go`,
  `executor_ssh_winremote_stop_reconnect_test.go`, `ssh_fake_server_test.go`.
- `Win32_ProcessStartup.EnvironmentVariables` replaces the environment. Never
  pass a partial block. The real-host probe showed that a partial block loses
  `PATH`.
- Parent-PID polling races with a fast-failing `agentctl.exe`. Treat a wrapper
  that has exited with no child as a launch failure, and include the log tail.

## Results

Pending.
