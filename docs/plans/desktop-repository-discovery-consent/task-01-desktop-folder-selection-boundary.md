---
id: "01-desktop-folder-selection-boundary"
title: "Add the desktop folder-selection boundary"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - "REQ-WORKSPACES-LOCAL-REPOSITORIES-002"
acceptance_criteria:
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-002.4"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-002.5"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-002.10"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-002.12"
  - "AC-WORKSPACES-LOCAL-REPOSITORIES-002.17"
system_design:
  - "../../specs/workspaces/system-design/local-repositories.md"
---

# Task 01: Add the Desktop Folder-Selection Boundary

## Summary

Add the narrow native picker boundary and the backend launch marker. Expose a
typed client capability without general filesystem authority.

## In scope

- Add `tauri-plugin-dialog` to the desktop runtime and lockfile.
- Add an origin-checked custom command, capability, and command permission.
- Export the picker through `src/lib.rs` for library-target tests.
- Add macOS usage descriptions and the desktop process marker.
- Add the typed boot capability and frontend adapter.

## Out of scope

- Save discovery roots or scan repositories.
- Grant direct dialog-plugin or filesystem-plugin access to the WebView.

## Acceptance

- The custom command accepts no path and returns selected, cancelled, or failed.
- The Tauri capability permits the custom command, but no generic dialog command.
- Rust and frontend tests cover origin rejection and browser unavailability.

## Verification

```bash
cd apps/desktop/src-tauri && rtk cargo fmt --check
cd apps/desktop/src-tauri && rtk cargo test --features desktop-runtime
cd apps/desktop/src-tauri && rtk cargo check --features desktop-runtime
cd apps/backend && rtk go test ./internal/common/config ./internal/backendapp
cd apps/web && rtk pnpm exec vitest run lib/desktop
```

Manual macOS verification opens and cancels the directory picker from an
unsigned build. Record the bundle usage text that appears.

## Files likely touched

- `apps/desktop/src-tauri/Cargo.toml`
- `apps/desktop/src-tauri/Cargo.lock`
- `apps/desktop/src-tauri/src/main.rs`
- `apps/desktop/src-tauri/src/lib.rs`
- New focused Rust picker module
- `apps/desktop/src-tauri/src/backend.rs`
- `apps/desktop/src-tauri/capabilities/default.json`
- New custom permission under `apps/desktop/src-tauri/permissions/`
- `apps/desktop/src-tauri/tauri.conf.json`
- New macOS `Info.plist` or equivalent bundle configuration
- `apps/backend/internal/common/config/catalog.go`
- Backend boot-payload types and tests
- `apps/web/lib/desktop/` adapter files and tests

## Dependencies

None.

## Risks

- A plugin permission can widen WebView authority if it exposes the plugin API.
- The binary target has `test = false`, so tests must use the library target.

## Parallelism

`sequential`

## Inputs

- REQ-WORKSPACES-LOCAL-REPOSITORIES-002 and its mapped design sections.
- Current Tauri notification and external-link origin checks.
- Current boot-payload capability pattern.

## Results

Completed.

- Added the feature-gated Tauri dialog dependency and one origin-checked
  `pick_directory` command with cancellation and failure outcomes.
- Added the desktop launch marker, boot capability, custom command permission,
  and macOS folder-usage bundle text. The WebView adapter selects the native
  command only when both the client and backend capability are present.
- Verification passed: Rust formatting, `cargo test --features
  desktop-runtime --lib` (66 tests), `cargo check --features desktop-runtime`,
  backend config/runtime tests, and focused folder-picker Vitest tests.

Manual macOS NSOpenPanel and unsigned-update privacy QA remains platform-dependent.
