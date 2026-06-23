---
id: "02-tauri-desktop-scaffold"
title: "Tauri desktop scaffold"
status: done
wave: 2
depends_on: ["01-backend-desktop-launch-contract"]
plan: "plan.md"
spec: "../../specs/desktop-tauri-app/spec.md"
---

# Task 02: Tauri Desktop Scaffold

## Acceptance

- `apps/desktop` builds as a Tauri v2 desktop package in the pnpm workspace.
- The Tauri Rust side starts the native Kandev launcher in headless mode, polls `/health`, and navigates the main window to the backend URL.
- The backend process environment preserves inherited variables, forces loopback backend binding, and adds common GUI-missing user binary paths.
- Frontend JavaScript is not granted broad shell/filesystem permissions.

## Verification

```bash
rtk bash -lc 'cd apps && pnpm --filter @kandev/desktop build'
rtk bash -lc 'cd apps/desktop/src-tauri && cargo test'
```

## Files Likely Touched

- `apps/pnpm-workspace.yaml`
- `apps/package.json`
- `apps/desktop/package.json`
- `apps/desktop/index.html`
- `apps/desktop/src/main.ts`
- `apps/desktop/src/styles.css`
- `apps/desktop/src-tauri/Cargo.toml`
- `apps/desktop/src-tauri/build.rs`
- `apps/desktop/src-tauri/tauri.conf.json`
- `apps/desktop/src-tauri/src/main.rs`
- `apps/desktop/src-tauri/src/backend.rs`
- `apps/desktop/src-tauri/capabilities/default.json`
- `apps/backend/internal/launcher/service.go` only as a reference for existing service PATH conventions

## Dependencies

- Task 01.

## Inputs

- Spec sections: What, State machine, Permissions, Failure modes.
- Plan sections: Desktop App > Workspace package; Desktop App > Desktop UX.
- Existing runtime contract from Task 01.

## Output Contract

When complete, update this file's `status` to `done`, update the Wave 2 checkbox in `plan.md`, and report changed files, tests run, blockers, and residual risks.
