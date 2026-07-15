---
id: "01-native-shell-commands"
title: "Native shell commands and window lifecycle"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/desktop-tauri-app/spec.md"
---

# Task 01: Native Shell Commands and Window Lifecycle

## Acceptance

- Native Application/File/View/Help menus expose Settings, New Task, Close Context, zoom
  in/out/reset, Check for Updates, Help links, full screen, and Quit with platform conventions.
- `Cmd/Ctrl+W` emits a typed Close Context event and has no native window-close fallback.
- Zoom is bounded, changes the WebView without resizing the window, and supports `+`, `=`, `-`,
  and `0` accelerators.
- macOS red close and Quit stop the backend and exit; second launch, Dock reopen while alive, and
  explicit focus requests show/unminimize/focus the existing window.
- Window size, position, and maximized state persist; minimized/off-screen state restores visibly.
- The bridge capability is restricted to the owned loopback origin and named commands/events.

## Files Likely Touched

- `apps/desktop/src-tauri/src/main.rs`
- `apps/desktop/src-tauri/src/lib.rs`
- New focused modules under `apps/desktop/src-tauri/src/` for menus/window state
- `apps/desktop/src-tauri/Cargo.toml`
- `apps/desktop/src-tauri/tauri.conf.json`
- `apps/desktop/src-tauri/capabilities/default.json`
- Rust unit tests alongside the new modules

## Verification

```bash
cd apps/desktop/src-tauri && rtk cargo fmt --check && rtk cargo test
cd apps && rtk pnpm --filter @kandev/desktop build
```

Test pure menu-to-event mapping, zoom clamping/reset, window restore clamping, and quit versus
updater-restart lifecycle decisions. A desktop smoke assertion must prove Close Context does not
produce a close request.

## Output Contract

Implement the frozen `v1` bridge adapter names from the plan, update this task to `done`, and check
its plan item only after verification passes.
