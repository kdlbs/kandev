---
id: "01-restore-saved-frame-size"
title: "Restore the saved frame size"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-DESKTOP-DESKTOP-TAURI-APP-001
acceptance_criteria:
  - AC-DESKTOP-DESKTOP-TAURI-APP-001.6
system_design:
  - ../../specs/desktop/system-design/desktop-tauri-app.md
---

# Task 01: Restore the saved frame size

## Summary

Apply the saved outer window size as the client size that reproduces it.
Restarting without resizing must keep the window at its saved outer size.

## In scope

- Add a pure `client_size` helper in `apps/desktop/src-tauri/src/window_state.rs` that removes
  the frame from the saved size with saturation and a minimum of 1 per dimension.
- Add a `desktop-runtime` helper that measures the frame as `outer_size() - inner_size()` and
  reports no frame if either size is unavailable.
- Use the converted size in `WindowStateStore::restore`. Keep `last_normal`, position,
  maximized, minimized, full-screen, and clamp behavior on the existing outer basis.
- Follow the [window state design](../../specs/desktop/system-design/desktop-tauri-app.md#window-state).

## Out of scope

- Cross-display scale-factor translation, monitor management, or persisted-format changes.
- macOS overlay chrome, menus, SPA code, permissions, and public documentation.

## Acceptance

1. For a measured frame, the applied client size plus the frame equals the saved outer size,
   including a size clamped to a display work area.
2. A window without a measurable frame keeps its saved size unchanged.
3. A frame at least as large as the saved size yields a positive size, not zero.

## Regression strategy

In the existing `window_state::tests` module, add round-trip, frameless, and oversized-frame tests
for `client_size`. Combine one test with `visible_restore` so the clamped outer size still fits
the work area after the frame is added back. First run the round-trip test against an applied size
equal to the saved outer size, the current behavior, and confirm it fails.

## Verification

Run from `apps/desktop/src-tauri`:

```bash
cargo fmt --all -- --check
cargo test --locked --lib window_state::
cargo check --locked --features desktop-runtime --lib
git diff --check
```

The feature-enabled check needs the repository's native Tauri prerequisites.
A real decorated-window restart remains pending unless a disposable desktop build is run.
Do not report native window behavior as verified from the unit tests.

## Files likely touched

- `apps/desktop/src-tauri/src/window_state.rs`

## Dependencies

None.

## Risks

- The frame is measured on the display where the window is created, before it moves.
- A measurement failure keeps the previous behavior instead of failing restore.

## Parallelism

`sequential`

## Inputs

- [Desktop app requirements](../../specs/desktop/requirements/desktop-tauri-app.md), criterion `.6`.
- [Window state design](../../specs/desktop/system-design/desktop-tauri-app.md#window-state).
- Evidence and probe results in [the plan](plan.md#evidence-and-root-cause).

## Results

Implemented `client_size` and the `desktop-runtime` `frame_size` measurement in
`window_state.rs`. `restore` applies the converted client size. `last_normal`, position,
maximized state, and display clamping keep their outer basis.

Added `repeated_restarts_keep_the_saved_outer_size`,
`clamped_bounds_still_fit_the_monitor_once_the_frame_is_added`,
`frameless_windows_keep_the_saved_size`, and
`a_frame_larger_than_the_saved_size_keeps_a_positive_size`. The first, second, and fourth failed
while the applied size still equaled the saved size. They pass after the conversion. The results
are recorded in [the plan](plan.md#verification-results).

Native acceptance on a decorated window remains pending. The unit tests prove the size
conversion, not the platform window.
