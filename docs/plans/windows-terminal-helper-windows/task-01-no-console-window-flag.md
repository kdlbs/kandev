---
id: "01-no-console-window-flag"
title: "fix(windows): keep background helper consoles hidden"
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
  - AC-PLATFORM-WINDOWS-BACKGROUND-CONSOLE-001.5
system_design:
  - ../../specs/platform/system-design/windows-background-process-console.md
---

# Keep background helper consoles hidden

## Outcome and scope

Add CREATE_NO_WINDOW only when GetConsoleWindow reports no existing window.
When a console window exists, keep attaching as before. Apply the flag at the
four existing helper entrypoints while preserving their existing flags, stdio,
process groups, suspended launch and Job Object cleanup.

This is the independent port of the existing helper-console change. It excludes
managed Git, PTY, named-job recovery, suspended-child resume and cooperative
backend shutdown changes. Original source history and existing PRs are retained.

## Implementation acceptance

1. Console-present and console-absent inputs choose the expected conditional bit.
2. Caller creation flags and rejection of an explicit new console are preserved.
3. A real detached parent starts an invisible managed child and captures stdout.
4. Selected existing Windows lifecycle checks and Linux compilation pass.

## Verification and limits

Windows five-package selection with temporary console-present/absent probes
passed: common/winproc 0.020s, mcpconfig 0.024s, launcher 0.117s, process 3.142s
and utility 0.699s. The real DETACHED_PROCESS parent entered setProcGroup; the
managed child returned `n1b_managed_hidden=true console_hwnd=0x0` through captured
stdout and exited successfully (0.07s). Existing Job Object descendant cleanup,
leader-exit reap and invalid-PID refusal checks passed.

Linux amd64 CGO-disabled compilation of the four caller packages passed. The
normal commit hooks remain required before publication. Temporary overlays do
not become permanent test cases or public probe APIs.

Commands from apps/backend:

```text
go test -tags=fts5 -overlay=<temporary-overlay> -count=1 -timeout=120s -v ./internal/common/winproc ./internal/agent/mcpconfig ./internal/agent/runtime/agentctl/launcher ./internal/agentctl/server/process ./internal/agentctl/server/utility -run=<selected console flags, hidden child and existing Windows lifecycle checks>
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags=fts5 ./internal/agent/mcpconfig ./internal/agent/runtime/agentctl/launcher ./internal/agentctl/server/process ./internal/agentctl/server/utility
```

The existing stdin-close, grace, forced tree termination and Job Object paths
remain. The windowless-helper WM_CLOSE/taskkill graceful effect is unmeasured;
this work does not claim equal console ownership or equal graceful close effects.

Documentation correction: the [Win32 process creation flags contract](https://learn.microsoft.com/en-us/windows/win32/procthread/process-creation-flags)
leaves the console handle unset for CREATE_NO_WINDOW; it does not promise
descendant console inheritance. The measured null window handle, captured stdout
and Job Object results remain valid. This comment/spec correction changes no
creation flags or execution code and does not require repeating those checks.

- [Plan](plan.md)
- [System design](../../specs/platform/system-design/windows-background-process-console.md)
