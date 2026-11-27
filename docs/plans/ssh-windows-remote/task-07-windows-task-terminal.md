---
id: "07-windows-task-terminal"
title: "Run the task terminal under ConPTY on Windows hosts"
status: done
wave: 7
depends_on:
  - "06-docs-copy-and-host-validation"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-WINDOWS-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-WINDOWS-001.20
system_design:
  - ../../specs/executors/system-design/ssh-windows-remote.md
---

# Task 07: Run the Task Terminal Under ConPTY on Windows Hosts

## Summary

Task 06 found that agentctl's interactive shell could not start on Windows
(`failed to start PTY: unsupported`), so the task terminal panel was excluded.
Start `shell.Session` through the shared `ptyexec` package, which already uses
ConPTY on Windows, and remove the exclusion.

## In scope

- `internal/agentctl/server/shell/session.go`: replace `creack/pty` with
  `ptyexec.Start` and `PtyHandle.Resize`. Reap through `waitShellCommand`.
- `process_group_windows.go`: make `configureShellProcess` a no-op, because
  ConPTY creates the process. Attach the kill-on-close job to the running
  process. Reap with `cmd.Process.Wait`.
- `wait_other.go`: keep `cmd.Wait` on other platforms.
- Remove the terminal-panel exclusion from the requirement. Document the
  terminal in the design and the public docs.

## Exclusions

- SSH passthrough terminals on Windows hosts.
- A second PTY implementation. `ptyexec` stays the single shared owner.

## Acceptance conditions

1. On Windows, a `shell.Session` running `powershell.exe` echoes input and
   accepts a resize.
2. On a Windows host, the product terminal WebSocket shows a PowerShell prompt
   in the task directory and echoes a command.

## Verification

```bash
cd apps/backend
go test -trimpath -race ./internal/agentctl/server/shell ./internal/common/ptyexec
GOOS=windows GOARCH=amd64 go test -c -o /tmp/shell.test.exe ./internal/agentctl/server/shell
```

Run the Windows test binary on the test host with
`-test.run '^TestSSHWindowsShellSessionRunsUnderConPTY$'`.

## Likely files and risks

- `session.go`, `process_group_windows.go`, `process_group_windows_test.go`,
  `wait_other.go`, `session_conpty_test.go`, and the excluded-process list in
  `docs/specs/platform/system-design/windows-background-process-console.md`.
- The job is attached after ConPTY starts the shell. A descendant started before
  that is outside the job. An interactive shell starts none before input.

## Results

Completed on 2026-11-23.

The shell and `ptyexec` tests passed on macOS. On the test host,
`TestSSHWindowsShellSessionRunsUnderConPTY` passed, and the product terminal
WebSocket opened a PowerShell 7 prompt in the task directory.
