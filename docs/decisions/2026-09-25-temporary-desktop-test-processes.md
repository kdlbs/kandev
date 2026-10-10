# ADR-2026-09-25-temporary-desktop-test-processes: Separate processes for desktop test windows

**Status:** accepted
**Date:** 2026-09-25
**Area:** frontend

## Context

The installed desktop app normally shares the selected Kandev home with a CLI
launch and admits only one GUI process. The backend correctly rejects another
process that tries to own the same home or database. Developers also need to
open several disposable desktop instances at the same time without stopping
their normal Kandev instance.

## Decision

Keep the single-instance guard for normal desktop launches. An explicit
temporary-test launch runs as a separate Tauri process without that guard. Each
test process allocates a private directory under the operating system's
temporary directory, pins its own SQLite database inside it, and owns one
launcher/backend tree. A normal desktop window that encounters an ownership
conflict remains open as a launcher and may start any number of these test
processes. Each test process removes only its own directory after a confirmed
clean backend stop; a crash or uncertain shutdown leaves it for later operating
system cleanup or manual inspection.

## Consequences

- Normal desktop launches still focus the existing window, and the backend's
  home/database locks remain the authority for persistent-state ownership.
- Each test window has an independent backend, port, health token, origin, and
  lifetime. A test window can close without stopping the normal instance or
  another test window.
- Test data is disposable. The startup UI must say that clean quit removes it,
  and must make the directory visible for diagnosing a failed start.
- Test processes must not write the normal desktop window-state file or run a
  competing automatic updater. Residual temporary directories after a crash
  must never be treated as active instances merely because they exist.

## Alternatives Considered

- Multiple backend/window pairs in one Tauri process would require per-window
  backend state, origin authorization, and shutdown routing throughout the
  existing single-backend shell. That is a larger and more fragile boundary.
- One reusable test home would prevent two concurrent test windows from using
  it and would require users to manage isolation manually.
- Retaining a unique app-data home for every test would allow concurrency but
  accumulate persistent test data contrary to the temporary-use goal.
