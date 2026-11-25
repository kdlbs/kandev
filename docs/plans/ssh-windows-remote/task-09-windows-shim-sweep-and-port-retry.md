---
id: "09-windows-shim-sweep-and-port-retry"
title: "Sweep stale gh shims and retry excluded ports on Windows hosts"
status: done
wave: 8
depends_on:
  - "06-docs-copy-and-host-validation"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-WINDOWS-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-WINDOWS-001.21
  - AC-EXECUTORS-SSH-WINDOWS-001.23
system_design:
  - ../../specs/executors/system-design/ssh-windows-remote.md
---

# Task 09: Sweep Stale gh Shims and Retry Excluded Ports on Windows Hosts

## Summary

A Windows stop forcibly ends `agentctl`, so every session left a
`%TEMP%\kandev-github-cli-*` shim directory. Separately, a launch failed
outright when the chosen port fell inside a Windows excluded port range.

Hold an owner lock in each shim directory and sweep released directories at
start. Classify a refused bind on the launch stderr as a port collision.

## In scope

- `cmd/agentctl/github_cli_shim.go`: create and hold `.owner.lock`. On Windows,
  call `sweepStaleGitHubCLIShimDirs` before installing a shim.
- `executor_ssh_windows_launch.go`: map `HTTP server failed to bind` on the
  launch stderr to `errSSHAgentctlPortInUse`.
- Tests for both, plus `./cmd/agentctl` in the Windows SSH CI step.

## Exclusions

- Removing shim directories that an older `agentctl` created without a lock.
- Reading the excluded port ranges before choosing a port. The retry already
  covers a refused bind.

## Acceptance conditions

1. The sweep removes a directory with a released lock and keeps one with a held
   lock, one without a lock, and unrelated directories.
2. A launch whose stderr reports a bind failure returns
   `errSSHAgentctlPortInUse` with the port. Any other launch failure does not.
3. On the test host, the next launch removes the shim directories that forced
   stops left.

## Verification

```bash
cd apps/backend
go test -trimpath -race ./cmd/agentctl -run 'GitHubCLIShim'
go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle -run '^TestSSHWindowsLaunch'
GOOS=windows GOARCH=amd64 go test -c -o /tmp/agentctl.test.exe ./cmd/agentctl
```

Run the Windows binary on the test host with `-test.run 'GitHubCLIShim'`.

## Likely files and risks

- `github_cli_shim.go`, `github_cli_shim_sweep_test.go`,
  `executor_ssh_windows_launch.go`, `executor_ssh_windows_launch_retry_test.go`.
- The sweep relies on Windows refusing to delete an open file. On other
  platforms it does not run, because a delete succeeds there regardless.

## Results

Completed on 2026-11-23.

The shim tests passed on macOS and on the test host, including
`TestSSHWindowsGitHubCLIShimSweepKeepsLiveOwner`. The bind-failure test failed
before the change. In a product run on the test host, the next launch removed
13 shim directories that forced stops had left, and kept the live owner's
directory and a lockless fixture.
