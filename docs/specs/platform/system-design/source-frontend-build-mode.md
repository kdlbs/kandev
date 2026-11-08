---
status: current
system: platform
requirements:
  - REQ-PLATFORM-SOURCE-FRONTEND-BUILD-MODE-001
---

# Source frontend build mode system design

## Purpose and boundaries

Platform owns source startup and the separation between compilation mode and runtime diagnostics.
This design covers the root Makefile entry points and their embedded frontend output.
It does not change the Vite configuration, React imports, backend configuration, or package scripts.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-PLATFORM-SOURCE-FRONTEND-BUILD-MODE-001` | [Compilation environment](#compilation-environment), [Diagnostic handoff](#diagnostic-handoff), [Verification](#verification) |

## Compilation environment

The root `Makefile` owns the frontend compilation environment for source builds.
Set `NODE_ENV=production` on each production web-build command, alongside the existing empty Vite API-port and debug values.

The POSIX recipes are `build-web`, `build-web-quiet`, and `build-web-e2e`.
The native Windows recipe is the frontend build inside `start-windows`.
Use a POSIX command environment assignment for the first group.
Use quoted cmd.exe syntax, `set "NODE_ENV=production"`, in the Windows recipe.
Each later Windows recipe runs in a separate shell.

`start`, `start-verbose`, and `start-debug` share `build-web-quiet`.
`build`, `runtime-bundle`, and service-bundle consumers reuse `build-web`.
The Windows verbose and debug aliases share `start-windows`.
`build-web-e2e` retains the package's `KANDEV_I18N_BUNDLE_PSEUDO=1` opt-in.

Do not export a global Make variable or change the installation commands.
The production value starts at the frontend build command and reaches its package lifecycle subprocesses.
It does not reach the preceding dependency installation or the subsequent backend launch.
The `dev` and `dev-web` paths retain their existing environments.

Vite's build mode and `NODE_ENV` are separate inputs.
Passing `--mode production` alone does not correct an inherited development `NODE_ENV`.
The [Vite environment contract](https://vite.dev/guide/env-and-mode.html#node-env-and-modes) describes this distinction.

## Diagnostic handoff

The existing native launcher continues to own runtime diagnostics:

1. `launcher.resolveLogLevel` selects the backend file threshold, subject to existing explicit overrides.
2. `backendEnvForConfig` sets `KANDEV_DEBUG_AGENT_MESSAGES=true` and `KANDEV_DEBUG_PPROF_ENABLED=true` for a debug launch.
3. `backendapp` passes `cfg.Debug.DevMode || cfg.Debug.PprofEnabled` to the web runtime configuration.
4. `webapp.BootPayloadScript` sets `window.__KANDEV_DEBUG` before application code runs.
5. `readBootPayload` also preserves the runtime flag during frontend hydration.
6. `isDebug` and `createDebugLogger` accept that flag in production bundles.

No runtime debug variable needs a build-time value for this flow.
React profiling remains a separate deliberate operation under the [React performance-track contract](https://react.dev/reference/dev-tools/react-performance-tracks).

## Verification

### Build command contract

Extend `scripts/check-make-shells`, which existing backend CI already invokes.
Execute the POSIX root web-build recipes with a temporary pnpm recorder and an inherited development environment.
The recorder captures only the target environment fields, arguments, and working directory.
Assert the production value at the executed command boundary.

Also cover unset, test, and production input values.
Verify that installation and development commands retain their input environment.
Verify that the existing Vite API-port and debug clearing remains intact.
Use dry-run recipe assertions for native cmd.exe syntax and alias forwarding.
These assertions prove recipe generation on Linux, not native Windows execution.

### Served debug source build

A focused Node/Playwright integration check launches the actual `make start-debug` path with inherited `NODE_ENV=development`.
It owns a temporary Kandev home, selected free ports, child processes, and a fresh headless browser.
It clears conflicting inherited Kandev selectors and configuration paths for its own child only.
It uses `KANDEV_BACKEND_PORT`, rather than the backend-only `KANDEV_SERVER_PORT`, for launcher port selection.
It binds the backend to loopback and suppresses automatic browser opening.

Install a minimal React DevTools observation hook before page scripts execute.
The installed React 19.2.3 renderer reports `bundleType=0` for production and `bundleType=1` for development.
Require a captured renderer with the expected package and version.
Also inspect the JavaScript assets actually served by Go for development-runtime sentinels.
An absent hook result or a failed asset fetch is a verification failure.

Assert the runtime debug global and capture an actual Kandev namespaced `console.debug` event during application activity.
Do not substitute a test-authored console message for logger evidence.
Verify the pprof index, backend debug file output, and the launcher's agent-message debug setting.
The existing launcher and webapp tests cover the diagnostic environment and boot-script ordering.

The check verifies the served asset identity against the fresh build before accepting the result.
It cleans up its browser and owned launcher tree in a `finally` block, including failure and cancellation paths.

### Memory evidence

The repair removes an unintended instrumentation source.
It does not establish a bound on total browser memory.
The implementation package defines a controlled repeat of the reported heap comparison.
Record timing-entry counts, retained memory where available, and renderer process memory as separate measurements.
Record unavailable measurements explicitly.
Do not clear performance entries during the comparison.

## Failure and compatibility

The existing quiet build recipe can suppress a build failure.
This repair does not redesign that recipe.
Verification must reject stale assets and failed builds, even when the outer recipe returns success.

No storage schema, public API, feature-flag registry, or dependency change is necessary.
The same frontend artifact serves desktop and phone clients.
There is no layout, navigation, touch, scrolling, or breakpoint change.

## Related decisions

- [Debug launches use the production profile](../../../decisions/2026-08-10-debug-launcher-profile-selection.md).
- [Embedded Vite assets](../../../decisions/0022-embedded-vite-assets.md).

The command-local environment assignment is a routine implementation of these existing boundaries.
A new ADR is not necessary.
