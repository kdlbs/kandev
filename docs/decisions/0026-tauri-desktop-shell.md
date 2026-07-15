# 0026: Tauri Desktop Shell Over Native Runtime

**Status:** accepted
**Date:** 2026-06-23
**Area:** frontend, backend, cli, infra

## Context

Kandev now has a native Go launcher/backend path and embeds the Vite SPA into the backend binary. Users can run Kandev from CLI-oriented channels, but a desktop install should open from the OS launcher, manage a local backend process, and avoid a production Node.js runtime. The desktop packaging choice needs to preserve the existing local-first backend, agent executors, and release versioning model.

## Decision

Add a Tauri v2 desktop app as a thin native shell over the existing Kandev runtime. The shell lives under `apps/desktop`, packages the platform runtime binaries as resources, launches the native Kandev launcher in headless mode, waits for the backend `/health` endpoint, and loads the existing Go-served SPA in a native WebView.

The installed desktop app will not include a Node.js runtime. Node.js and pnpm remain build-time tooling for Vite, TypeScript, Tauri CLI invocation, tests, and release automation.

Desktop launches set the backend to a loopback-only bind address and an app-selected local port. Desktop release automation signs macOS and Windows artifacts when CI signing secrets are configured; otherwise it publishes unsigned desktop development artifacts with a release-notes warning. The first desktop implementation did not include Tauri's in-app updater; updates continued through GitHub release downloads/manual reinstall pending a separate approved design.

**Amendment (2026-07-15):** ADR-0039 adds the approved native menu, updater,
notification, window-state, and external-link integration boundary. This ADR remains the decision
for the shell/runtime architecture; ADR-0039 supersedes only its initial no-updater scope.

## Consequences

The desktop app can reuse the existing backend, embedded web UI, data directory, agent runtime, and supervisor behavior instead of introducing a parallel application stack. Release artifacts grow by the Tauri shell and installer overhead, but avoid Electron's bundled Chromium and avoid a runtime Node.js dependency.

CI becomes more complex because desktop installers must be built on OS-specific runners and signing requires protected secrets. The desktop app also adds a new security boundary: the WebView must not get broad shell/filesystem permissions, and backend process spawning should stay in Rust-side Tauri code.

## Alternatives Considered

1. **Electron desktop app.** Rejected because it would add a bundled Chromium and Node.js runtime, increasing artifact size and conflicting with the no-production-Node direction established by ADR-0021 and ADR-0022.
2. **Rewrite the UI in a native toolkit.** Rejected because it would duplicate the existing React product surface and slow delivery without improving the backend/runtime model.
3. **Browser-only PWA.** Rejected because it does not solve OS-level desktop installation, app launcher integration, or process lifecycle ownership for the local backend.
4. **Tauri shell that embeds all UI assets separately from the Go backend.** Rejected for the first implementation because the backend already embeds and serves the release-quality Vite SPA. Duplicating the assets would increase packaging complexity and risk frontend/backend version drift.
