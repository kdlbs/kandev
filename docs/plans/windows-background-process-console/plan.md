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

When the Kandev backend runs without a console, every managed helper that it
starts on Windows opens a visible console window. Short-lived Git commands
flash a window, and long-lived processes such as agentctl keep one open. Add
`CREATE_NO_WINDOW` to the existing creation flags of the Job-Object-managed
helpers so that they never open a console window.

## Evidence and root cause

On `fba6f2b32`, no backend code sets `CREATE_NO_WINDOW` or `HideWindow`. The
managed helper sites set only `CREATE_NEW_PROCESS_GROUP`, plus
`CREATE_SUSPENDED` where a Job Object is attached before the process resumes.
Those flags do not affect console allocation, so a console child of a process
without a console receives a new visible console.

A temporary harness on one Windows 11 machine started a test binary with
`DETACHED_PROCESS` and ran a managed Git command through `RunGitOutputClass`,
with a probe program in place of Git. Without the new flag, the probe had its
own console with a visible window. With the flag, it had its own console and
no window.

## Technical approach

Add the flag in each package's existing Windows-only process-attribute
function. Keep the `CREATE_NEW_CONSOLE` rejection in managed Git and Cursor
native MCP preparation, because Windows ignores `CREATE_NO_WINDOW` together
with that flag. Leave the pseudo-terminal paths, the CLI launcher, and commands
without process attributes unchanged.

## Delivery order

1. [Task 01: Start managed helpers without console windows](task-01-start-managed-helpers-without-console-windows.md)

## Risks

- **Console control events.** The helpers no longer share the parent's
  console, so console control events from that console, such as Ctrl+Break or
  a console close, no longer reach them. No code calls
  `GenerateConsoleCtrlEvent`, and `CREATE_NEW_PROCESS_GROUP` already disabled
  Ctrl+C for them. On Windows, Go's `os.Process.Signal` supports only
  `os.Kill` and returns `EWINDOWS` otherwise, so the graceful stop paths already
  fall back to kill. Owners still stop the helpers through their Job Objects
  and existing termination paths. If binding agentctl to its Job Object fails,
  which the launcher logs as a warning, agentctl also no longer receives a
  terminal's console close and relies on the backend's shutdown path.
- **Console hosts.** Each helper now gets its own hidden `conhost.exe`, also on
  terminal and desktop launches where it previously shared the parent's
  console. Descendants of a helper share that helper's console.

## Verification strategy

- Run the new Windows flag tests with a `-run` filter in each changed package.
- Run the five flag tests that the native Windows CI package step does not
  cover in a targeted CI step, mirrored in `make test-windows`.
- Run the existing Windows Job Object lifecycle tests for Git, agentctl
  processes, and ACP utility commands with a `-run` filter.
- Run backend lint for the changed packages with the base SHA.
- Run the specification and document catalog checks after doc edits.
