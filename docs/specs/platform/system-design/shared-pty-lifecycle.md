---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-SHARED-PTY-001
created: 2026-10-13
owners:
  - kandev
---

# Shared PTY lifecycle system design

## Purpose and boundaries

`internal/common/ptyexec` is already the neutral shared PTY owner used by
`internal/agent/loginpty` and `internal/agentctl/server/process`. Platform owns
its descriptor lifetime; callers retain child-process and session policy.
This design restores the close behavior already expected by
`InteractiveRunner.Stop`, whose existing comment says closing the PTY delivers
SIGHUP. It introduces no new process ownership rule or public API.

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-SHARED-PTY-001` | Initialization, I/O and closure; failure cleanup; caller compatibility |

## Initialization, I/O and closure

In `pty_unix.go`, `Start` obtains the master from `pty.StartWithSize` and makes
its descriptor nonblocking before exposing `unixPTY` to a caller. The pinned
creack/pty v1.1.24 [documentation](https://raw.githubusercontent.com/creack/pty/v1.1.24/README.md)
explicitly requires this mode for Close to interrupt a pending Read.
The `os.File` poller continues to present normal blocking Read/Write semantics
to callers while allowing Close to wake a waiting operation.

`unixPTY.Resize` retains `SyscallConn().Control` and the existing ioctl. Do not
replace it with an API that calls `File.Fd()` and restores blocking mode.
Regression coverage must exercise resize before pending-read closure as well
as concurrent resize/read/close. The slave stays open during the close proof;
closing both endpoints first would hide the defect.

## Failure cleanup

`pty.StartWithSize` has already started the child when it returns a master.
If nonblocking initialization fails, close the master, kill the owned child if
still live, and reap it before returning. No caller has received the handle or
started its own waiter at this boundary. Preserve the initialization error and
report unexpected cleanup errors through the returned error chain. A private,
argument-injected setup helper may make this failure path testable; do not add
a mutable package-global syscall hook or runtime configuration.

## Caller compatibility

| Consumer | Intended behavior | Verification |
| --- | --- | --- |
| Unix agentctl interactive terminal | Close releases pending I/O; existing stop signals and two-second fallback remain | Real PTY close regression and interactive runner tests |
| Unix backend login/host shell | Same shared close semantics; reconnect and retained-session ownership unchanged | Login PTY stop, timeout and reconnect tests |
| Windows ConPTY | Existing implementation and synchronization unchanged | Windows compile and existing native Windows CI |
| Embedded `server/shell.Session` | Existing independent raw-PTY implementation unchanged | Retain shell shutdown suite and report its residual cost |

The embedded shell has separate `pty.StartWithSize` calls and a resize helper
that calls `Fd()`. Applying this change there requires its own evidence and
compatibility review; changing only the common wrapper must not be reported as
fixing every PTY path.

## Validation and measurement

Linux and macOS must execute the real-PTY regression. Cross-compilation is
compatibility evidence only. Preserve existing output, resize, stop, retained
host-shell and race coverage. A shell that deliberately ignores termination
must still reach the existing fallback.

Use the existing E2E reset fence and guarded one-worker runner to compare the
same queue and transcript files before and after. Keep test outcomes, source,
runner, cache state, retries and all reset operations in the report. Three
paired measurements precede a fixture-saving claim. Hosted critical path,
runner-minutes and workflow elapsed time remain separate measurements under
[CI performance](ci-performance.md).

## Implementation plan

- [Shared PTY close](../../../plans/shared-pty-close/plan.md).
