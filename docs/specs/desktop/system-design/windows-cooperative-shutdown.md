---
status: current
system: desktop
requirements:
  - REQ-DESKTOP-DESKTOP-TAURI-APP-001
created: 2026-10-20
owners:
  - tbd
---

# Windows Cooperative Shutdown System Design

## Purpose and boundaries

This design covers `AC-DESKTOP-DESKTOP-TAURI-APP-001.18`: how the desktop shell
and the launcher stop a console-less Windows child through its normal teardown.
The [desktop app design](desktop-tauri-app.md) keeps ownership of the backend
process lifetime, startup failure, and update restart.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-DESKTOP-DESKTOP-TAURI-APP-001` | [Stop request](#stop-request) |

## Stop request

The desktop shell is a GUI process, so the launcher it starts and the backend
the launcher starts have no console. `taskkill /T` without `/F` asks a process
to close by posting a window-close message, which a console-less process cannot
receive. Before this design, that request failed, the shell and launcher fell through to forced
tree cleanup, and the backend exited without its ordered teardown.

Each Kandev process that owns a signal-driven teardown registers a manual-reset
named event, `Local\KandevShutdown-<pid>`, through
`internal/common/winproc.ListenForShutdown`. The launcher registers it beside
its `os.Interrupt`/`SIGTERM` handler and the backend registers it in its startup
signal controller. A waiter forwards one `os.Interrupt` into a dedicated
cooperative channel when the event is set. The handler accepts that channel
only while waiting for the first signal; the second-signal wait that forces
exit reads OS signals alone. When `kandev start` or `kandev run` shares a
console with the backend, Ctrl+C reaches both processes and the launcher then
sets the backend's event, so this separation keeps that late request from
cutting the backend's teardown short. The two requests from one Ctrl+C can
also arrive in the other order: `winproc.ShutdownPairing` takes an OS signal
that is already waiting in preference to the cooperative request, and when the
cooperative request still starts shutdown it absorbs exactly one later OS
signal as that request's console pair, so only a further Ctrl+C forces exit.
Releasing the handler stops the waiter
and closes both handles, after which a stop request falls back. The event name
helper is untagged so every platform tests the name shared with the shell.

A parent sets the event for the exact child PID it still owns:
`windows_shutdown::request` in the shell and `winproc.RequestShutdown` in the
launcher's `terminateManagedProcess`. Senders only open an existing event; they
never create one. If the child has not registered the event, for example the
`make` wrapper used by the development launcher, the parent keeps the existing
`taskkill /T` request. Grace deadlines, second-signal handling, and forced tree
cleanup are unchanged. Other platforms keep their existing signals.

## Implementation plans

- [Windows cooperative shutdown fix](../../../plans/windows-cooperative-shutdown/plan.md)
