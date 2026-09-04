---
created: 2026-09-03
status: done
requirements:
  - REQ-EXECUTORS-SSH-EXECUTOR-001
system_design:
  - ../../specs/executors/system-design/ssh-executor.md
legacy_specs: []
---

# Implementation Plan: SSH Dead Runtime Resume

## Overview

Issue [#3330](https://github.com/kdlbs/kandev/issues/3330) reports that an SSH
session cannot resume after Kandev intentionally stops its remote controller.
A focused lifecycle test reproduces the failure: the confirmed-dead PID error
escapes the SSH resume preflight before normal instance creation can replace the
runtime.

The correction classifies the remote process probe result and converts only a
confirmed-absent controller into fresh runtime creation. SSH transport and
session failures remain fail-closed.

## Scope

### In scope

- Preserve the task workspace and ACP provider conversation when replacing a
  confirmed-dead SSH runtime.
- Clear stale controller metadata before normal SSH instance creation.
- Distinguish a remote non-zero process probe from an indeterminate SSH probe.
- Add focused regression coverage for replacement and fail-closed behavior.

### Out of scope

- Idle-runtime eviction or a background reaper.
- Changes to stop semantics, provider retry policy, or non-SSH executors.
- Database, WebSocket, API, or frontend changes.

## Technical approach

- In `executor_ssh_operations.go`, add a process-liveness probe that returns an
  error when SSH cannot establish or complete the remote command. Keep the
  existing boolean helper for best-effort status and startup polling callers.
- In `executor_ssh.go`, use the classified probe during resume. On a confirmed
  absent process, close the stale SSH client, clear session-runtime metadata
  with `clearSSHResumeRuntimeMetadata`, and return successfully so
  `CreateInstance` launches a new controller. Preserve the remote task
  directory, executor connection metadata, and ACP resume token.
- Keep existing hard-error behavior for connection, authentication, host trust,
  and indeterminate process-liveness failures.

## Tests

- `AC-EXECUTORS-SSH-EXECUTOR-001.9`: update
  `TestSSHExecutorResumeRemoteInstance/dead_remote_pid` in
  `executor_ssh_stop_resume_test.go` to require successful fallback, stale
  runtime metadata removal, and preserved task-directory metadata.
- `AC-EXECUTORS-SSH-EXECUTOR-001.10`: add focused probe coverage in
  `executor_ssh_operations_remote_test.go` and resume coverage proving a closed
  SSH client remains an error.

## Work orders

- [x] [Task 01: Recover from a dead SSH runtime](task-01-recover-dead-ssh-runtime.md)

## Verification results

- RED: `TestSSHExecutorResumeRemoteInstance/dead_remote_pid_falls_back_to_fresh_create`
  failed with `ssh resume: agentctl pid 4242 not alive on remote`.
- RED: `TestSSHExecutorResumeRemoteInstance/malformed_remote_pid_remains_fail_closed`
  failed because malformed runtime identity incorrectly returned `nil` after
  the initial fallback change.
- GREEN: `cd apps/backend && go test -race
  ./internal/agent/runtime/lifecycle -run
  'Test(SSHExecutorResumeRemoteInstance|ProbeRemoteAgentctlLiveness)$'
  -count=1` passed with 12 tests.
- Compatibility: the existing liveness and lifecycle preflight tests passed
  with 4 tests.
- `python3 scripts/lint-spec-files.test.py` passed with 30 tests.
- `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` and `gofmt -l` passed.

## Risks

- Treating all probe errors as absence could start a second controller during a
  transient SSH failure. The classified probe must accept only a completed
  remote command with a non-zero status as confirmed absence.
- Clearing the remote task directory would lose workspace continuity. The
  existing metadata helper must preserve it.
