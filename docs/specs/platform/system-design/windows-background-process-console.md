---
status: current
system: platform
requirements:
  - REQ-PLATFORM-WINDOWS-BACKGROUND-CONSOLE-001
---

# Windows Background Process Console System Design

## Purpose and boundaries

Platform owns the Windows process setting that managed console helpers share.
Each owning package keeps its process lifetime. [Managed Git
execution](git-subprocess-execution.md) owns Git admission and cleanup, agentctl
owns agent and script processes, and the agent runtime owns the agentctl
launch. This design hides newly created console windows while keeping helpers
attached to a console.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-WINDOWS-BACKGROUND-CONSOLE-001` | [Console inheritance](#console-inheritance), [Process attributes](#process-attributes), [Excluded process paths](#excluded-process-paths) |

## Console inheritance

A console program started without console flags attaches to its parent's
console. If the parent has no console, Windows creates a console with a visible
window for the child. A process has no console when its parent started it with
`DETACHED_PROCESS`, or when it is a GUI-subsystem program that has not allocated
one. On one Windows 11 machine, a probe started by Node.js 24 `spawn` with
`detached: true` had no console, with or without `windowsHide: true`, and a
GUI-subsystem probe had no console either. The end-to-end backend fixture starts
`kandev __backend` with `detached: true`.

Go's `syscall.SysProcAttr.HideWindow` sets `STARTF_USESHOWWINDOW` and
`SW_HIDE`. For a console program that creates a console, Windows applies this
setting to the new console window. The process remains attached to the console
and can use console APIs. Go's standard-handle setup remains separate, so
`os/exec` pipes continue to carry input and output.

When a managed helper starts from a backend with no console, the helper gets a
hidden console. A console descendant started with default flags attaches to
that same hidden console. This keeps descendants hidden and leaves console
APIs available. `CREATE_NO_WINDOW` is not suitable here: it starts a console
program without a console handle, so an unflagged console descendant can create
its own visible console. `HideWindow` also supplies an initial show state to a
GUI program, so the process paths in this design must remain console-helper
paths.

When the backend already has a console, the helper attaches to that console.
`HideWindow` does not hide the shared console window. This preserves terminal
visibility for a backend started from a terminal.

## Process attributes

| Package | Function | Windows process attributes |
| --- | --- | --- |
| `internal/common/subproc` | `prepareGitLifecycleCommand` | Caller flags, `HideWindow`, `CREATE_NEW_PROCESS_GROUP`, `CREATE_SUSPENDED` |
| `internal/agent/mcpconfig` | `prepareNativeMCPProcess` | Caller flags, `HideWindow`, `CREATE_NEW_PROCESS_GROUP`, `CREATE_SUSPENDED` |
| `internal/agentctl/server/utility` | `setACPCommandProcAttr` | `HideWindow`, `CREATE_NEW_PROCESS_GROUP`, `CREATE_SUSPENDED` |
| `internal/agentctl/server/process` | `setProcGroup`, `setManagedProcGroup`, `setAgentProcGroup` | `HideWindow`, `CREATE_NEW_PROCESS_GROUP`, plus `CREATE_SUSPENDED` for managed processes (scripts, piped commands, code-server) and agents |
| `internal/agent/runtime/agentctl/launcher` | `buildSysProcAttr` | `HideWindow`, `CREATE_NEW_PROCESS_GROUP` |
| `internal/agent/acpdbg`, `internal/agent/codexdbg` | `configureProcessTree` | `HideWindow`, `CREATE_NEW_PROCESS_GROUP`, `CREATE_SUSPENDED` |

`prepareGitLifecycleCommand` and `prepareNativeMCPProcess` reject a caller
that sets `CREATE_NEW_CONSOLE` before the process starts. This preserves the
existing console-ownership contract for these managed commands.

Each process in the table is bound to a kill-on-close Job Object that its owner
holds. A suspended process is bound before it resumes. If binding the agentctl
process fails, the launcher logs a warning and continues, as before.

`CREATE_NEW_PROCESS_GROUP` continues to disable Ctrl+C for the helper group.
No Kandev code calls `GenerateConsoleCtrlEvent`. With a detached backend, the
helper's hidden console is separate from the backend. With a terminal-run
backend, the helper shares the terminal console as it did before. On Windows,
Go's `os.Process.Signal` supports only `os.Kill` and returns `EWINDOWS` for
`os.Interrupt`, so the agentctl launcher's graceful stop already falls back to
kill.

Graceful `taskkill` without `/F` remains the first stop attempt for process
groups that use it. A hidden console remains attached to the helper, unlike a
`CREATE_NO_WINDOW` process, which has no console window or console handle. A
helper can still ignore or fail to handle a close request, so owners must keep
their existing wait, Job Object, and forced-termination paths. The detached
parent regression test checks that the managed helper and its default console
descendant use the same hidden console window; it does not claim that every
helper exits gracefully.

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

Windows build-tagged unit tests assert `HideWindow` and ensure these helpers do
not set `CREATE_NO_WINDOW`. A detached-parent regression test starts a managed
helper and a default console descendant, then checks that both have the same
console window handle and that the window stays hidden. The native Windows CI
job runs these tests;
the `process` and agentctl `launcher` tests run in its package step, and the
other helper tests run in a targeted step that `make test-windows` mirrors.
Existing Windows lifecycle tests cover start, output capture, and Job Object
cleanup for Git, agentctl processes, and ACP utility commands.
