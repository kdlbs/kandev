---
id: "01-cooperative-shutdown-event"
title: "Deliver cooperative shutdown to console-less children"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-DESKTOP-DESKTOP-TAURI-APP-001
acceptance_criteria:
  - AC-DESKTOP-DESKTOP-TAURI-APP-001.18
system_design:
  - ../../specs/desktop/system-design/windows-cooperative-shutdown.md
---

# Task 01: Deliver cooperative shutdown to console-less children

## Summary

Let the desktop shell and the launcher request a normal stop from a
console-less Windows child, so the backend runs its ordered teardown instead of
being force-killed after `taskkill /T` fails.

## In scope

- `internal/common/winproc` adds `ListenForShutdown` (all platforms) and the
  Windows-only `RequestShutdown` over a per-PID named event.
- The launcher's signal handler and the backend's startup signal controller
  register the listener beside their existing OS signals and release it on
  teardown.
- The launcher's Windows `terminateManagedProcess` and the shell's Windows
  `request_graceful_child_shutdown` set the event first and keep their existing
  `taskkill /T` request when it is unavailable.

## Out of scope

Grace deadlines, second-signal handling, forced cleanup, Unix and macOS
signaling, the development `make` wrapper, and taskkill output localization.

## Acceptance

1. A console-less child running the launcher signal handler exits cleanly after
   the launcher's graceful stop request, without forced cleanup.
2. The backend startup signal controller treats the request as its first signal
   and cancels startup; a request after a console signal does not force exit;
   when the request comes first, the OS signal from the same Ctrl+C does not
   force exit but a further one does; after release, a request fails so callers
   fall back.
3. The shell's `terminate_child_with_timeout` stops a console-less child that
   registered the event with a clean exit.

## Verification

- The three tests listed in the [plan](plan.md#verification) fail on the
  previous source and pass with this change.
- Launcher, backendapp, and desktop crate test results match the base revision
  apart from the new tests.

## Files likely touched

- `apps/backend/internal/common/winproc/shutdown_windows.go` (new)
- `apps/backend/internal/common/winproc/shutdown_name.go` (new)
- `apps/backend/internal/common/winproc/shutdown_pairing.go` (new)
- `.github/workflows/backend-tests.yml`
- `apps/backend/internal/common/winproc/shutdown_other.go` (new)
- `apps/backend/internal/launcher/process.go`
- `apps/backend/internal/launcher/process_group_windows.go`
- `apps/backend/internal/launcher/process_group_windows_test.go`
- `apps/backend/internal/backendapp/startup.go`
- `apps/backend/internal/backendapp/startup_shutdown_windows_test.go` (new)
- `apps/desktop/src-tauri/src/backend.rs`
- `apps/desktop/src-tauri/src/windows_shutdown.rs` (new)
- `docs/specs/desktop/requirements/desktop-tauri-app.md`
- `docs/specs/desktop/system-design/desktop-tauri-app.md`
- `docs/specs/desktop/system-design/windows-cooperative-shutdown.md` (new)

## Dependencies

None.

## Inputs

- [Owning requirement](../../specs/desktop/requirements/desktop-tauri-app.md)
- [Owning design](../../specs/desktop/system-design/windows-cooperative-shutdown.md)
- [Plan](plan.md)

## Risks

A sender that creates the event, a repeated request that forces exit, or a
listener that is never released would each break the existing fallback.

## Parallelism

`sequential`
