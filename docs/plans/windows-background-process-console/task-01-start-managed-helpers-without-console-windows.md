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
  - AC-PLATFORM-WINDOWS-BACKGROUND-CONSOLE-001.4
system_design:
  - ../../specs/platform/system-design/windows-background-process-console.md
---

# Task 01: Start managed helpers without console windows

## Summary

Set `HideWindow` on every in-scope Windows process attribute. This hides a new
console window while keeping the console handle available to the helper and
its default console descendants.

## In scope

- Managed Git preparation in `internal/common/subproc`.
- Cursor native MCP preparation in `internal/agent/mcpconfig`.
- ACP utility commands in `internal/agentctl/server/utility`.
- Agent, script, piped, and editor-server processes in
  `internal/agentctl/server/process`.
- The agentctl launch in `internal/agent/runtime/agentctl/launcher`.
- The agent process trees of `internal/agent/acpdbg` and
  `internal/agent/codexdbg`.
- Windows build-tagged tests for each process attribute, the
  `CREATE_NEW_CONSOLE` rejection, and hidden console inheritance from a
  detached parent.
- A targeted step in the native Windows CI job, mirrored in
  `make test-windows`, for the helper tests that its package step does not run.

## Out of scope

- The embedded shell and ConPTY terminals.
- Processes supervised by `internal/launcher`.
- Commands that Kandev starts without process attributes.
- The desktop shell.

## Acceptance

- `AC-PLATFORM-WINDOWS-BACKGROUND-CONSOLE-001.1` through
  `AC-PLATFORM-WINDOWS-BACKGROUND-CONSOLE-001.4` describe the behavior covered
  by this work order.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/common/subproc -run '^(TestWindowsPrepareGitLifecycleCommandHidesConsoleWindow|TestWindowsDetachedParentKeepsConsoleHiddenForDescendants)$' -count=1
cd apps/backend && go test -tags fts5 ./internal/agent/mcpconfig -run '^TestPrepareNativeMCPProcessHidesConsoleWindow$' -count=1
cd apps/backend && go test -tags fts5 ./internal/agentctl/server/utility -run '^TestWindowsACPCommandProcAttrHidesConsoleWindow$' -count=1
cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process -run '^TestWindowsProcGroupsHideConsoleWindow$' -count=1
cd apps/backend && go test -tags fts5 ./internal/agent/runtime/agentctl/launcher -run '^TestBuildSysProcAttrHidesConsoleWindow$' -count=1
cd apps/backend && go test -tags fts5 ./internal/agent/acpdbg ./internal/agent/codexdbg -run '^TestConfigureProcessTreeHidesConsoleWindow$' -count=1
cd apps/backend && go test -tags fts5 ./internal/common/subproc -run '^(TestWindowsManagedGitJobCleanup|TestWindowsManagedGitAskpassIsExecutedAndDenied)$' -count=1
cd apps/backend && go test -tags fts5 ./internal/agentctl/server/utility -run '^TestWindowsACPCommandLifecycleJobKillsDescendants$' -count=1
cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process -run '^(TestWindowsProcessLifecycleJobKillsDescendants|TestWindowsProcessRunnerReapsDescendantAfterLeaderExit)$' -count=1
# The native Windows CI step for the console tests outside its package step:
cd apps/backend && go test -tags fts5 -race -v -run '^(TestWindowsPrepareGitLifecycleCommandHidesConsoleWindow|TestWindowsDetachedParentKeepsConsoleHiddenForDescendants|TestPrepareNativeMCPProcessHidesConsoleWindow|TestWindowsACPCommandProcAttrHidesConsoleWindow|TestConfigureProcessTreeHidesConsoleWindow)$' ./internal/common/subproc ./internal/agent/mcpconfig ./internal/agentctl/server/utility ./internal/agent/acpdbg ./internal/agent/codexdbg
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Results

- The follow-up replaces `CREATE_NO_WINDOW` with `HideWindow`; this keeps a
  console handle for the helper and its default console descendants.
- The detached-parent regression test checks that the managed child and its
  default console descendant share one console window handle and a hidden window.
- The targeted Windows CI command now uses `-tags fts5`, matching
  `make test-windows`, and includes the detached-parent regression test.
- Verification results for this follow-up are recorded after the focused
  checks complete.
