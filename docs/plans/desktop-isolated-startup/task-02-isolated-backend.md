---
id: "02-isolated-backend"
title: "Start independent temporary windows"
status: done
wave: 2
depends_on:
  - "01-conflict-diagnostic"
plan: "plan.md"
requirements:
  - REQ-DESKTOP-ISOLATED-STARTUP-001
  - REQ-DESKTOP-ISOLATED-STARTUP-002
acceptance_criteria:
  - AC-DESKTOP-ISOLATED-STARTUP-001.1
  - AC-DESKTOP-ISOLATED-STARTUP-001.3
  - AC-DESKTOP-ISOLATED-STARTUP-002.1
  - AC-DESKTOP-ISOLATED-STARTUP-002.2
  - AC-DESKTOP-ISOLATED-STARTUP-002.3
  - AC-DESKTOP-ISOLATED-STARTUP-002.4
  - AC-DESKTOP-ISOLATED-STARTUP-002.5
system_design:
  - ../../specs/desktop/system-design/isolated-startup.md
---

# Task 02: Start independent temporary windows

## Summary

Make normal desktop startup classify a typed ownership conflict and start a
separate test GUI process on explicit action. Each test process gets a new OS
temporary home and independently owns its launcher/backend lifetime.

## In scope

- Parse complete marker lines across pipe chunks and expose a typed conflict
  state without trusting human error prose or stale lock metadata.
- Keep the normal single-instance plugin, while an internal test-process mode
  bypasses it and disables shared window-state writes and automatic updates.
- Add a startup-only native command that spawns `current_exe` in test mode;
  the conflict launcher remains open for another action.
- Create a random private OS-temporary home, pin empty config and in-home
  SQLite, filter conflicting inherited configuration, and preserve the selected
  runtime profile.
- Stop only the current test backend and remove only its own home after a
  confirmed clean stop. Retain it after uncertain shutdown.

## Out of scope

- Startup-page visual changes, translations, public documentation, and
  multiple backend/window pairs inside one process.

## Acceptance

1. Only a valid early-exit conflict marker enables the native launch action;
   each invocation spawns one distinct test process and leaves the launcher
   available for another invocation.
2. Two test processes can use distinct homes, SQLite targets, ports, and
   owned origins while normal single-instance focus remains intact.
3. Closing a test process reaps only its backend and deletes its exact home
   only after clean stop; forced or uncertain stop retains it.

## Verification

```bash
(cd apps/desktop/src-tauri && cargo test --features desktop-runtime backend::tests)
```

## Files likely touched

- `apps/desktop/src-tauri/src/backend.rs`
- `apps/desktop/src-tauri/src/main.rs`
- `apps/desktop/src-tauri/Cargo.toml` only if an existing dependency cannot
  safely allocate and manage the temporary directory

## Dependencies

Task 01's marker format.

## Risks

The launcher reports a clean exit only when every supervised process exits
gracefully with status zero and no forced cleanup, failure, or uncertain
status. The desktop removes a temporary home only after that confirmed exit
and backend readiness. The existing window-state store and updater are shared
across desktop processes and remain inactive in test mode.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/desktop/requirements/isolated-startup.md),
  [design](../../specs/desktop/system-design/isolated-startup.md),
  [decision](../../decisions/2026-09-25-temporary-desktop-test-processes.md),
  and existing Rust backend lifecycle tests.

## Results

Passed `cargo test --features desktop-runtime --lib` (97 tests),
`cargo check --features desktop-runtime --bin kandev-desktop`, and
`cargo fmt --all -- --check` from `apps/desktop/src-tauri`. The desktop now
waits beyond the launcher's graceful and forced-cleanup bounds and removes a
temporary home only when the launcher exits successfully. Startup conflict
classification waits for captured output streams to finish before using a
typed marker. Focused tests cover nonzero launcher exits, retained homes,
shutdown timing, delayed stderr, and retention after initial-navigation
failure. The launcher returns nonzero after forced, failed, nonzero, or
uncertain descendant shutdown. Focused launcher regressions and
`go test ./internal/launcher -count=1` pass; `make lint` reports zero issues.
The Linux packaged desktop smoke also passes after rebuilding
`apps/backend/bin/kandev`.
