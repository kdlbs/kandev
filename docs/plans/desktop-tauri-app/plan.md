---
spec: docs/specs/desktop-tauri-app/spec.md
created: 2026-06-23
status: done
---

# Implementation Plan: Tauri Desktop App

## Overview

Add `apps/desktop` as a Tauri v2 shell that packages the existing native Kandev runtime, starts it in headless mode, waits for backend health, and loads the Go-served SPA in a desktop WebView. The implementation starts by tightening the backend/launcher contract for loopback desktop launches, then scaffolds the app, wires release packaging, adds signing gates, and finishes with desktop smoke coverage and docs.

---

## Architecture Records

- Spec: `docs/specs/desktop-tauri-app/spec.md`
- Decision: `docs/decisions/0026-tauri-desktop-shell.md`
- Existing dependencies: ADR-0021 Go-served SPA with boot state, ADR-0022 embedded Vite assets, and the native launcher spec at `docs/specs/native-kandev-cli/spec.md`.

---

## Backend

### Loopback bind contract

Files:

- `apps/backend/internal/common/config/config.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/helpers_test.go` or a focused backend server test
- `apps/backend/internal/launcher/env.go`
- `apps/backend/internal/launcher/start_test.go` or `apps/backend/internal/launcher/run_test.go`

Use the existing `ServerConfig.Host` field when constructing the HTTP server address and listener address. Preserve the current default unless a caller sets `server.host` / `KANDEV_SERVER_HOST`. The desktop app will set `KANDEV_SERVER_HOST=127.0.0.1`.

Add tests proving:

- `buildHTTPServer` uses the configured host in `http.Server.Addr`.
- `startHTTPServer` listens on the configured host/port.
- launcher env construction can pass a backend host override for desktop/headless launches without changing normal CLI defaults.

### Headless launcher lifecycle contract

Files:

- `apps/backend/internal/launcher/start.go`
- `apps/backend/internal/launcher/health.go`
- `apps/backend/internal/launcher/process.go`
- Existing launcher tests under `apps/backend/internal/launcher/`

Keep `kandev --headless --port <port>` as the desktop process contract. It must supervise `kandev __backend`, wait for `/health`, skip opening a browser, and remain alive until the backend exits or the parent process terminates. Desktop-specific process termination should use the same process-group/job-object cleanup behavior already covered by the native launcher.

---

## Desktop App

### Workspace package

Files:

- `apps/pnpm-workspace.yaml`
- `apps/package.json`
- New `apps/desktop/package.json`
- New `apps/desktop/index.html`
- New `apps/desktop/src/main.ts`
- New `apps/desktop/src/styles.css`
- New `apps/desktop/src-tauri/Cargo.toml`
- New `apps/desktop/src-tauri/build.rs`
- New `apps/desktop/src-tauri/tauri.conf.json`
- New `apps/desktop/src-tauri/src/main.rs`
- New `apps/desktop/src-tauri/src/backend.rs`
- New `apps/desktop/src-tauri/capabilities/default.json`

Add a minimal Tauri/Vite desktop package named `@kandev/desktop`. The app's first screen is a compact startup surface, not a marketing page. Rust-side Tauri code locates the packaged runtime resources, picks an available loopback port, spawns the native launcher in headless mode, polls `/health`, and navigates the main window to the backend URL.

Do not expose shell execution to frontend JavaScript. Backend process control should live in Rust-side Tauri code.

### Runtime resources

Files:

- New `scripts/release/prepare-desktop-runtime.sh`
- New `scripts/release/verify-desktop-runtime.sh`
- `apps/desktop/src-tauri/tauri.conf.json`
- `apps/desktop/src-tauri/src/backend.rs`

Create a deterministic resource layout for the platform runtime extracted from the existing release bundle:

```text
apps/desktop/src-tauri/resources/kandev/
└── bin/
    ├── kandev[.exe]
    ├── agentctl[.exe]
    └── agentctl-linux-amd64
```

The helper validates the exact binaries required by `internal/launcher/bundle.go` so the desktop app and existing runtime tarballs cannot drift.

### Desktop process environment

Files:

- `apps/desktop/src-tauri/src/backend.rs`
- `apps/backend/internal/launcher/env.go`
- `apps/backend/internal/launcher/service.go` for existing PATH precedent
- `docs/desktop-tauri-signing.md` or desktop install docs for troubleshooting notes

Build the backend process environment explicitly. Preserve inherited variables, force `KANDEV_SERVER_HOST=127.0.0.1`, and normalize `PATH` for GUI app launches so agent CLIs installed under common user locations are visible. Existing explicit agent command paths in settings remain authoritative; this task only makes default command lookup reliable.

### Desktop UX

Files:

- `apps/desktop/src/main.ts`
- `apps/desktop/src/styles.css`
- `apps/desktop/src-tauri/src/main.rs`
- `apps/desktop/src-tauri/src/backend.rs`

The startup surface shows only launch status and errors. Once the backend is ready, the WebView loads the existing Kandev UI. The app should support standard OS close/quit behavior and single-instance focus.

---

## Release Pipeline

### Build matrix

Files:

- `.github/workflows/release.yml`
- `scripts/release/verify-desktop-assets.sh`
- `apps/cli/src/release-config.test.ts`

Add a `build-desktop` job that depends on `build-bundles`, downloads the matching `bundle-<platform>` artifact, prepares desktop runtime resources, runs the Tauri build on the appropriate OS runner, checks artifact sizes/checksums, and uploads `desktop-<platform>` artifacts.

The initial matrix should mirror current runtime support:

- `macos-arm64`
- `macos-x64`
- `linux-x64`
- `linux-arm64`
- `windows-x64`

### GitHub release publishing

Files:

- `.github/workflows/release.yml`
- `scripts/release/verify-desktop-assets.sh`
- `scripts/release/update-homebrew-tap.sh` only if release asset assumptions need to ignore desktop artifacts

Update `publish-release` to download and publish desktop artifacts alongside the existing `kandev-*.tar.gz` runtime bundles. Keep existing npm and Homebrew publishing tied to runtime tarballs only.

### Signing

Files:

- `.github/workflows/release.yml`
- `apps/desktop/src-tauri/tauri.conf.json`
- New `docs/desktop-tauri-signing.md`
- GitHub Actions release docs/comments in `.github/workflows/release.yml`

Add signing inputs/secrets behind conditional release steps:

- macOS: Developer ID Application certificate, Apple team/provider metadata, notarization credentials.
- Windows: certificate/key-vault or custom signing command wiring.
- Linux: checksum required; GPG/AppImage signing optional.

Desktop signing should be automatic when secrets are configured. If signing inputs are missing or incomplete, the workflow should still publish unsigned desktop artifacts with a release-notes warning.

---

## Frontend

The existing `apps/web` product UI should not need route or state changes. Desktop-specific frontend work is limited to the Tauri startup surface under `apps/desktop`. If implementation discovers that the existing SPA needs to distinguish desktop WebView mode, add the smallest runtime flag necessary and document it in the spec before broadening scope.

---

## Tests

- **What:** backend honors configured host when constructing server addresses.
  **File:** `apps/backend/internal/backendapp/helpers_test.go` or a new focused test file.
  **How:** table-driven Go test over host/port config.

- **What:** launcher/headless env includes desktop host override only when requested.
  **File:** `apps/backend/internal/launcher/start_test.go` or `run_test.go`.
  **How:** Go unit test around env/config helpers.

- **What:** missing runtime resource binaries are detected before desktop launch.
  **File:** `apps/desktop/src-tauri/src/backend.rs` tests.
  **How:** Rust unit tests with temporary resource directories.

- **What:** backend command builder passes `--headless`, `--port`, and loopback env.
  **File:** `apps/desktop/src-tauri/src/backend.rs` tests.
  **How:** Rust unit tests over pure command-building helpers.

- **What:** desktop process environment preserves user env and adds common GUI-missing binary paths.
  **File:** `apps/desktop/src-tauri/src/backend.rs` tests.
  **How:** Rust unit tests around environment construction.

- **What:** desktop startup handles health success, timeout, and child-process exit.
  **File:** `apps/desktop/src-tauri/src/backend.rs` tests.
  **How:** Rust tests using a local fake HTTP server or injectable health client.

- **What:** release workflow requires desktop artifacts and leaves npm/Homebrew runtime publishing unchanged.
  **File:** `apps/cli/src/release-config.test.ts` or a shell test under `scripts/`.
  **How:** parse workflow/scripts and assert expected job names, artifacts, and publish filters.

- **What:** desktop runtime resource preparation validates the existing bundle shape.
  **File:** script test under `scripts/` or `apps/cli/src/release-config.test.ts`.
  **How:** create fake bundle directories and assert success/failure paths.

---

## E2E Tests

- **Scenario:** GIVEN a prepared Linux desktop runtime bundle, WHEN the desktop app is launched in CI, THEN the Tauri window loads the Kandev UI without Node.js at runtime.
  **File:** `apps/desktop/e2e/desktop-launch.spec.ts` or an equivalent Tauri WebDriver test.
  **What to verify:** startup screen transitions to the Kandev app shell and `/health` succeeds.

- **Scenario:** GIVEN the backend child fails during startup, WHEN the desktop app is launched, THEN the startup surface shows a failure and no child process remains running.
  **File:** `apps/desktop/e2e/desktop-launch.spec.ts` or Rust integration test if WebDriver cannot observe process state reliably.
  **What to verify:** visible error state plus process cleanup.

- **Scenario:** GIVEN the desktop app is already running, WHEN a second instance is opened, THEN the first window is focused and only one backend launcher process exists.
  **File:** `apps/desktop/e2e/desktop-single-instance.spec.ts` or Rust integration test.
  **What to verify:** single-instance behavior and no duplicate backend process.

Tauri desktop E2E should run on Linux with a fake display in CI first. macOS and Windows desktop smoke tests can be added after the Linux harness is stable.

---

## Implementation Waves

Wave 1:

- [x] [task-01-backend-desktop-launch-contract](task-01-backend-desktop-launch-contract.md)

Wave 2:

- [x] [task-02-tauri-desktop-scaffold](task-02-tauri-desktop-scaffold.md)
- [x] [task-03-desktop-runtime-resources](task-03-desktop-runtime-resources.md)

Wave 3:

- [x] [task-04-release-desktop-artifacts](task-04-release-desktop-artifacts.md)
- [x] [task-05-signing-and-release-trust](task-05-signing-and-release-trust.md)

Wave 4:

- [x] [task-06-desktop-e2e-smoke](task-06-desktop-e2e-smoke.md)
- [x] [task-07-docs-and-release-notes](task-07-docs-and-release-notes.md)

---

## Open Questions

- Which Windows installer target should be the public default: NSIS `.exe` or MSI? Recommendation: start with the Tauri default/recommended installer for direct GitHub release downloads, then add MSI only if enterprise deployment requires it.
- Which Linux artifact set should be public by default: AppImage only, `.deb` plus AppImage, or `.deb`/`.rpm` plus AppImage? Recommendation: start with AppImage plus the package type Tauri produces reliably in CI, then broaden after QA.
