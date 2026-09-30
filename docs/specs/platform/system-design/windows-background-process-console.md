---
status: current
system: platform
requirements:
  - REQ-PLATFORM-WINDOWS-BACKGROUND-CONSOLE-001
---

# Windows Background Process Console System Design

## Purpose and boundaries

Platform owns the Windows creation flag that managed helper processes share.
Each owning package keeps its process lifetime. [Managed Git
execution](git-subprocess-execution.md) owns Git admission and cleanup, agentctl
owns agent and script processes, and the agent runtime owns the agentctl
launch. This design adds one creation flag to their existing settings.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-WINDOWS-BACKGROUND-CONSOLE-001` | [Console inheritance](#console-inheritance), [Creation flags](#creation-flags), [Excluded process paths](#excluded-process-paths) |

## Console inheritance

A console program started without console flags attaches to its parent's
console. When the parent has no console, Windows creates a new console with a
visible window for the child. A process has no console when its parent started
it with `DETACHED_PROCESS`, or when it is a GUI-subsystem program that has not
allocated one. On one Windows 11 machine, a probe started by Node.js 24
`spawn` with `detached: true` had no console, with or without
`windowsHide: true`, and a GUI-subsystem probe had no console either. The
end-to-end backend fixture starts `kandev __backend` with `detached: true`.

`CREATE_NO_WINDOW` makes Windows create a new console without a window for the
child, hosted by its own `conhost.exe`. The child does not attach to its
parent's console. Its standard handles do not change, so pipes from the parent
still carry its input and output.
Descendants started without console flags inherit the windowless console.
Windows ignores the flag for GUI programs and when a caller combines it with
`CREATE_NEW_CONSOLE` or `DETACHED_PROCESS`.

## Creation flags

| Package | Function | Windows creation flags |
| --- | --- | --- |
| `internal/common/subproc` | `prepareGitLifecycleCommand` | Caller flags, `CREATE_NEW_PROCESS_GROUP`, `CREATE_SUSPENDED`, `CREATE_NO_WINDOW` |
| `internal/agent/mcpconfig` | `prepareNativeMCPProcess` | Caller flags, `CREATE_NEW_PROCESS_GROUP`, `CREATE_SUSPENDED`, `CREATE_NO_WINDOW` |
| `internal/agentctl/server/utility` | `setACPCommandProcAttr` | `CREATE_NEW_PROCESS_GROUP`, `CREATE_SUSPENDED`, `CREATE_NO_WINDOW` |
| `internal/agentctl/server/process` | `setProcGroup`, `setManagedProcGroup`, `setAgentProcGroup` | `CREATE_NEW_PROCESS_GROUP`, `CREATE_NO_WINDOW`, plus `CREATE_SUSPENDED` for managed processes (scripts, piped commands, code-server) and agents |
| `internal/agent/runtime/agentctl/launcher` | `buildSysProcAttr` | `CREATE_NEW_PROCESS_GROUP`, `CREATE_NO_WINDOW` |
| `internal/agent/acpdbg`, `internal/agent/codexdbg` | `configureProcessTree` | `CREATE_NEW_PROCESS_GROUP`, `CREATE_SUSPENDED`, `CREATE_NO_WINDOW` |

`prepareGitLifecycleCommand` and `prepareNativeMCPProcess` reject a caller
that sets `CREATE_NEW_CONSOLE` before the process starts. That keeps the
windowless console in effect for every command they start.

Each process in the table is bound to a kill-on-close Job Object that its
owner holds. A suspended process is bound before it resumes. If binding the
agentctl process fails, the launcher logs a warning and continues, as before.
The Job Object and the owner's existing termination paths stop the process,
so detaching it from the parent's console does not change how it ends.

Console control events from the parent's console no longer reach these
processes. `CREATE_NEW_PROCESS_GROUP` already disables Ctrl+C for them. No
Kandev code calls `GenerateConsoleCtrlEvent`. On Windows, Go's
`os.Process.Signal` supports only `os.Kill` and returns `EWINDOWS` for
`os.Interrupt`, so the agentctl launcher's graceful stop already falls back to
kill.

Graceful `taskkill` without `/F` asks the process's windows to close. A process
that shares its parent's console owns no window, so the existing callers already
fall back to forced termination. A windowless console keeps that behavior.

## Excluded process paths

- `internal/agentctl/server/shell` prepares the embedded shell for
  `pty.StartWithSize`, which is an interactive pseudo-terminal session.
  `configureShellProcess` keeps its flags.
- `internal/common/ptyexec` starts interactive terminals through ConPTY. ConPTY
  creates the process itself and does not read `SysProcAttr`.
- `internal/launcher` supervises processes that have no Job Object. They keep
  the launcher's console, so closing the terminal still delivers the console
  close event to them.
- Commands started without process attributes inherit the console of the
  process that starts them. These include `gh`, `glab`, external editor
  launches, plugin processes, and the `taskkill` and `tasklist` calls that
  agentctl and the CLI launcher make.

## Verification

Windows build-tagged unit tests assert `CREATE_NO_WINDOW` for each function in
the table and the `CREATE_NEW_CONSOLE` rejection. The native Windows CI job
runs them: the `process` and agentctl `launcher` tests in its package step, and
the other five in a targeted step that `make test-windows` mirrors. Existing
Windows lifecycle tests cover start, output capture, and Job Object cleanup for
Git, agentctl processes, and ACP utility commands.
