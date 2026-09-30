---
id: "01-start-managed-helpers-without-console-windows"
title: "Start managed helpers without console windows"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WINDOWS-BACKGROUND-CONSOLE-001
acceptance_criteria:
  - AC-PLATFORM-WINDOWS-BACKGROUND-CONSOLE-001.1
  - AC-PLATFORM-WINDOWS-BACKGROUND-CONSOLE-001.2
  - AC-PLATFORM-WINDOWS-BACKGROUND-CONSOLE-001.3
system_design:
  - ../../specs/platform/system-design/windows-background-process-console.md
---

# Task 01: Start managed helpers without console windows

## Summary

Add `CREATE_NO_WINDOW` to the Windows creation flags of every Job-Object-managed
helper, so that no helper opens a console window when the backend has no
console.

## In scope

- Managed Git preparation in `internal/common/subproc`.
- Cursor native MCP preparation in `internal/agent/mcpconfig`.
- ACP utility commands in `internal/agentctl/server/utility`.
- Agent, script, piped, and editor-server processes in
  `internal/agentctl/server/process`.
- The agentctl launch in `internal/agent/runtime/agentctl/launcher`.
- The agent process trees of `internal/agent/acpdbg` and
  `internal/agent/codexdbg`.
- Windows build-tagged tests for each flag set and for the
  `CREATE_NEW_CONSOLE` rejection.
- A targeted step in the native Windows CI job, mirrored in
  `make test-windows`, for the five flag tests that its package step does not
  run.

## Out of scope

- The embedded shell and ConPTY terminals.
- Processes supervised by `internal/launcher`.
- Commands that Kandev starts without process attributes.
- The desktop shell.

## Acceptance

- `AC-PLATFORM-WINDOWS-BACKGROUND-CONSOLE-001.1` through
  `AC-PLATFORM-WINDOWS-BACKGROUND-CONSOLE-001.3` describe the behavior covered
  by this work order.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/common/subproc -run '^TestWindowsPrepareGitLifecycleCommandRequestsNoConsoleWindow$' -count=1
cd apps/backend && go test -tags fts5 ./internal/agent/mcpconfig -run '^TestPrepareNativeMCPProcessRequestsNoConsoleWindow$' -count=1
cd apps/backend && go test -tags fts5 ./internal/agentctl/server/utility -run '^TestWindowsACPCommandProcAttrRequestsNoConsoleWindow$' -count=1
cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process -run '^TestWindowsProcGroupsRequestNoConsoleWindow$' -count=1
cd apps/backend && go test -tags fts5 ./internal/agent/runtime/agentctl/launcher -run '^TestBuildSysProcAttrRequestsNoConsoleWindow$' -count=1
cd apps/backend && go test -tags fts5 ./internal/agent/acpdbg ./internal/agent/codexdbg -run '^TestConfigureProcessTreeRequestsNoConsoleWindow$' -count=1
cd apps/backend && go test -tags fts5 ./internal/common/subproc -run '^(TestWindowsManagedGitJobCleanup|TestWindowsManagedGitAskpassIsExecutedAndDenied)$' -count=1
cd apps/backend && go test -tags fts5 ./internal/agentctl/server/utility -run '^TestWindowsACPCommandLifecycleJobKillsDescendants$' -count=1
cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process -run '^(TestWindowsProcessLifecycleJobKillsDescendants|TestWindowsProcessRunnerReapsDescendantAfterLeaderExit)$' -count=1
# The native Windows CI step for the five flag tests outside its package step:
cd apps/backend && go test -race -v -run '^(TestWindowsPrepareGitLifecycleCommandRequestsNoConsoleWindow|TestPrepareNativeMCPProcessRequestsNoConsoleWindow|TestWindowsACPCommandProcAttrRequestsNoConsoleWindow|TestConfigureProcessTreeRequestsNoConsoleWindow)$' ./internal/common/subproc ./internal/agent/mcpconfig ./internal/agentctl/server/utility ./internal/agent/acpdbg ./internal/agent/codexdbg
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Results

- The seven new flag tests failed before the change because
  `CREATE_NO_WINDOW` (`0x8000000`) was not set, and pass after it.
- The existing Git, agentctl process, and ACP utility lifecycle tests pass
  with the new flag.
- The new CI step command passes on Windows 11, and the matching
  `make test-windows` line passes under `cmd.exe`.
- A temporary harness ran managed Git from a process without a console. The
  child had a visible console window before the change and a windowless
  console after it.
- `golangci-lint run` for the seven changed packages with the base SHA reported
  0 issues.
