---
id: "04-updater-runtime-settings"
title: "Desktop updater runtime and settings adapter"
status: done
wave: 3
depends_on:
  [
    "01-native-shell-commands",
    "02-spa-contextual-commands",
    "03-updater-release-artifacts",
  ]
plan: "plan.md"
spec: "../../specs/desktop-tauri-app/spec.md"
---

# Task 04: Desktop Updater Runtime and Settings Adapter

## Acceptance

- Rust exposes bounded check, download/install, and relaunch operations with one in-flight update.
- Checks run after ready and no more than every 24 hours; manual checks always run immediately.
- Valid newer signed updates enter `available`; invalid signatures, wrong targets, current/older
  versions, and network failures do not stop the running app.
- Download/install/relaunch starts only after explicit confirmation in the existing
  `/settings/system/updates` page.
- Update restart cleanly stops the owned backend without replacing Tauri's updater exit code.
- The existing browser/service updater remains unchanged and the desktop page clearly represents
  desktop-app status, progress, current/latest versions, no-update state, and errors.

## Files Likely Touched

- `apps/desktop/src-tauri/src/main.rs`, `lib.rs`, and a new updater module
- `apps/desktop/src-tauri/Cargo.toml`
- `apps/desktop/src-tauri/tauri.conf.json`
- `apps/desktop/src-tauri/capabilities/default.json`
- `apps/desktop/src-tauri/src/backend.rs`
- `apps/web/components/settings/system/updates-card.tsx`
- `apps/web/components/settings/system/updates-card.test.tsx`
- New updater bridge/state modules under `apps/web/lib/desktop/`

## Verification

```bash
cd apps/desktop/src-tauri && rtk cargo fmt --check && rtk cargo test --features desktop-runtime
cd apps/web && rtk pnpm run typecheck
cd apps/web && rtk pnpm test updates-card
```

Use deterministic Rust tests for scheduling/state transitions and backend shutdown on restart.
Frontend tests must cover desktop and service adapters, confirmation gating, progress/errors, and
the unchanged narrow/mobile rendering path.

## Output Contract

Update this task to `done` and check its plan item only after both native and web update state
machines are verified.
