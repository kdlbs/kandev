---
created: 2026-10-12
status: done
requirements:
  - REQ-DESKTOP-DESKTOP-TAURI-APP-001
system_design:
  - ../../specs/desktop/system-design/desktop-tauri-app.md
legacy_specs: []
---

# Implementation Plan: Restore the desktop window size on its saved basis

## Overview

The desktop shell saves the main window's outer frame size but restores it as a client-area size.
On a decorated window, every restart grows the window by its frame.
This plan applies the saved size on the same basis it was recorded, as specified in
[window state](../../specs/desktop/system-design/desktop-tauri-app.md#window-state).
It satisfies the existing criterion `AC-DESKTOP-DESKTOP-TAURI-APP-001.6` in the
[desktop app requirements](../../specs/desktop/requirements/desktop-tauri-app.md).
No new requirement is needed.

## Evidence and root cause

The inspected source is `apps/desktop/src-tauri/src/window_state.rs` with Tauri `2.11.5`,
`tauri-runtime-wry 2.11.4`, and `tao 0.35.3`, as pinned by the desktop crate.

```text
WindowStateStore::capture
  -> WebviewWindow::outer_position / outer_size        (frame bounds, physical pixels)
  -> saved as WindowBounds { x, y, width, height, maximized }

WindowStateStore::restore
  -> visible_restore(saved, work areas)                 (outer-size clamp)
  -> WebviewWindow::set_size(PhysicalSize(width, height))
     -> tauri-runtime-wry WindowMessage::SetSize -> tao Window::set_inner_size   (client area)
  -> WebviewWindow::set_position(PhysicalPosition(x, y))
     -> tao Window::set_outer_position                  (frame origin, consistent with capture)

main.rs setup: restore(&window) then save(&window) immediately
```

Position and maximized state already use one basis.
Size does not: the outer size is applied as an inner size.
The immediate save then records `saved + frame`, so the error compounds on each launch.
A clamped oversized window also ends up larger than the work area it was clamped to.

A temporary Cargo probe called the real `visible_restore` and modeled the cycle above.
It used synthetic frames of 16x39 and 22x53 physical pixels for a decorated window.

- `1280x900` grew to `1296x939`, `1312x978`, and `1328x1017` over three restarts with the 16x39 frame.
- With the 22x53 frame, the same window grew to `1346x1059`.
- An oversized window clamped to a `1920x1032` work area was restored at `1936x1071` and `1942x1085`.

The probe exited non-zero for all four cases.
The existing `window_state::tests` suite passed on the same source, because no test covers the applied size.

## Scope

### In scope

- Convert the restored outer size into the client size that reproduces it, using the frame
  measured from the live window (outer size minus inner size).
- Keep the saved file format, saved values, position handling, maximized handling, minimized and
  full-screen skips, and display clamping unchanged.
- Add source-adjacent unit tests for the size conversion.

### Out of scope

- Per-monitor DPI translation when the saved position lies on a display with a different scale
  factor, monitor management, or new persisted fields.
- macOS overlay title-bar behavior, which reports no frame and is unaffected.
- Web UI, SPA bridge, permissions, menus, or public documentation changes.

## Technical approach

Add a pure `client_size(saved_width, saved_height, frame_width, frame_height)` helper in
`window_state.rs`. It subtracts the frame with saturation and keeps each dimension at least 1.
Add a `desktop-runtime` helper that measures the frame from `outer_size()` and `inner_size()`.
If either size is unavailable, it returns no frame, which keeps the previous behavior rather than
failing restore. `restore` passes the converted size to `set_size` and keeps `last_normal` on the
outer basis so later maximized saves are unchanged.

## Tests

| Acceptance | Test file and regression |
| --- | --- |
| `AC-DESKTOP-DESKTOP-TAURI-APP-001.6` | `src/window_state.rs`: client size plus the frame equals the saved outer size, including after a display clamp; a frameless window keeps its saved size; an oversized frame never yields a zero size |

Write the round-trip test first. Show that it fails when the applied size equals the saved outer
size, which is the current behavior. Then implement the conversion.

## Native acceptance

A real restart on a decorated Windows or Linux window is the user-visible proof.
Unit tests prove the arithmetic and the basis, not the platform window.
Record native acceptance as pending unless a disposable desktop build is run on such a host.

## Mobile and browser parity

This is native desktop shell geometry. Browser and mobile clients have no persisted window
size, so there is no parity surface. The SPA and its responsive layouts are unchanged.

## Work orders

- [x] [Task 01: Restore the saved frame size](task-01-restore-saved-frame-size.md)

## Verification results

Implementation checks on 2026-10-12, from `apps/desktop/src-tauri` on Windows:

- Before the conversion, with `client_size` returning the saved size: `cargo test --locked --lib window_state::`
  failed 3 of 8 tests. Three restarts reached `1328x1017` instead of `1280x900`. The clamped window
  reached `1936x1119` against a `1920x1080` monitor. An oversized frame returned `(10, 20)` instead
  of `(1, 1)`. The frameless test passed, as intended, because it protects unchanged behavior.
- After the conversion: `cargo test --locked --lib window_state::` passed, 8 tests.
- `cargo fmt --all -- --check`: passed.
- `cargo check --locked --features desktop-runtime --lib`: passed. Its one warning is an
  unchanged `backend.rs` unused `mut`.
- `python3 scripts/lint-spec-files.py --all`, `python3 scripts/list-docs.py validate`, and
  `git diff --check`: passed.
- Native acceptance on a decorated Windows or Linux window: pending. No desktop build was launched,
  and the cross-scale-factor case is unverified.

## Risks

- The frame is measured before the window moves to its saved position. A display with another
  scale factor can have a different frame; that case remains unverified and out of scope.
- If the platform reports inner and outer sizes that differ for reasons other than decorations,
  the conversion follows the platform's own outer size, which is what capture records.
