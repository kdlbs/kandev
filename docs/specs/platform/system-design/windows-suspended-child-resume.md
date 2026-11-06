---
status: current
system: platform
requirements:
  - REQ-PLATFORM-WINDOWS-SUSPENDED-RESUME-001
---

# Windows Suspended Child Resume System Design

## Purpose and boundaries

`internal/common/winproc` owns the resume step that every Windows suspended
start shares. The agentctl wrapper in `internal/agentctl/server/winproc`
delegates to it. Callers keep their process attributes and Job Object
lifetimes; see [Managed Git execution](git-subprocess-execution.md) and
[Windows background process console](windows-background-process-console.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-WINDOWS-SUSPENDED-RESUME-001` | [Resume path](#resume-path), [Alternatives](#alternatives) |

## Resume path

`InstallKillOnCloseJobForSuspendedProcess` creates the Job Object, assigns the
suspended child, and only then calls `ResumeSuspendedProcess`. A failure in any
step closes the job and terminates the suspended child.

`ResumeSuspendedProcess` opens the child with `PROCESS_SUSPEND_RESUME` and calls
`NtResumeProcess` from `ntdll.dll` on that handle. The kernel decrements the
suspend count of each thread in that process once, the same effect as calling
`ResumeThread` on each of its threads. The cost depends only on the child's own
threads. A child created with `CREATE_SUSPENDED` has one thread.

## Alternatives

- **Keep the primary thread handle from creation.** Go's `os/exec` closes the
  thread handle that `CreateProcess` returns. Keeping it would mean starting
  every caller through `windows.CreateProcess` and rebuilding the
  standard-handle, environment, and wait handling of `os/exec`. That is a large
  change for the same result.
- **Keep the Toolhelp snapshot.** `TH32CS_SNAPTHREAD` always copies every
  thread on the system; its process ID parameter does not filter threads.

`NtResumeProcess` is exported by `ntdll.dll` but is not part of the documented
Win32 API. The export is checked before the call. If it is missing, the resume fails with an error
and the owner terminates the suspended child, so containment is preserved.

## Verification

Focused Windows checks verify that one resume is issued on a process handle,
that a failure status is reported, and that an unopenable process is rejected
before any resume. An entry-point check creates a hidden suspended child,
assigns it through `InstallKillOnCloseJobForSuspendedCommand`, and observes its
normal exit. These checks can run through a temporary Go overlay without adding
a permanent test suite.
