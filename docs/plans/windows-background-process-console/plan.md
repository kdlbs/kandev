---
status: done
created: 2026-10-01
requirements:
  - REQ-PLATFORM-WINDOWS-BACKGROUND-CONSOLE-001
system_design:
  - ../../specs/platform/system-design/windows-background-process-console.md
---

# Windows background process console

## Overview

When the Kandev backend runs without a console, every managed console helper
that it starts on Windows can open a visible console window. Short-lived Git
commands flash a window, and long-lived processes such as agentctl keep one
open. Set `HideWindow` on the existing Windows process attributes. This hides
new console windows while keeping the console handle available to helpers and
their console descendants.

## Evidence and root cause

On `fba6f2b32`, no backend code sets `CREATE_NO_WINDOW` or `HideWindow`. The
managed helper sites set only `CREATE_NEW_PROCESS_GROUP`, plus
`CREATE_SUSPENDED` where a Job Object is attached before the process resumes.
Those flags do not affect console allocation, so a console child of a process
without a console receives a new visible console. Microsoft documents that
`CREATE_NO_WINDOW` starts a console app without a console handle. This means
default console descendants can allocate their own visible console, and some
console APIs are unavailable to the managed helper.

A temporary harness on one Windows 11 machine started a test binary with
`DETACHED_PROCESS` and ran a managed Git command through `RunGitOutputClass`,
with a probe program in place of Git. It showed that the direct managed child
had no visible console window after the original change. The regression test
now also checks that the child retains a console handle and that a default
console descendant inherits the hidden console.

## Technical approach

Set `syscall.SysProcAttr.HideWindow` in each package's existing Windows-only
process-attribute function. Keep the `CREATE_NEW_CONSOLE` rejection in managed
Git and Cursor native MCP preparation to preserve those callers' existing
console-ownership contract. Keep `CREATE_NEW_PROCESS_GROUP` and
`CREATE_SUSPENDED` where the current lifecycle needs them. Leave the
pseudo-terminal paths, the CLI launcher, and commands without process
attributes unchanged.

## Delivery order

1. [Task 01: Start managed helpers without console windows](task-01-start-managed-helpers-without-console-windows.md)

## Risks

- **Console control events.** A helper started by a detached backend gets its
  own hidden console. A helper started by a backend with a terminal continues
  to share that terminal's console. `CREATE_NEW_PROCESS_GROUP` still disables
  Ctrl+C for the helper group. No code calls `GenerateConsoleCtrlEvent`. On
  Windows, Go's `os.Process.Signal` supports only `os.Kill` and returns
  `EWINDOWS` otherwise, so the agentctl launcher's graceful stop falls back to
  kill. Owners still stop helpers through their Job Objects and existing
  termination paths.
- **Graceful close.** A hidden console remains attached to the helper, so the
  existing `taskkill` request without `/F` retains a console target. A helper
  can still ignore a close request. Owners must keep their existing wait and
  forced-termination paths.
- **GUI programs.** Windows applies `HideWindow` to the first window of GUI
  programs too. The listed process paths must remain console-helper paths.

## Verification strategy

- Run the new Windows flag tests with a `-run` filter in each changed package.
- Run the five flag tests that the native Windows CI package step does not
  cover in a targeted CI step, mirrored in `make test-windows`.
- Run the existing Windows Job Object lifecycle tests for Git, agentctl
  processes, and ACP utility commands with a `-run` filter.
- Run backend lint for the changed packages with the base SHA.
- Run the specification and document catalog checks after doc edits.
