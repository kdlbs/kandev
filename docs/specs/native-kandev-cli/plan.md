---
spec: native-kandev-cli
created: 2026-06-15
status: draft
---

# Implementation Plan: Native Kandev CLI

## Overview

Refactor the current backend command into a callable backend app, then make `apps/backend/cmd/kandev` a unified native executable that dispatches public launcher commands by default and hidden backend mode via `__backend`. Port the existing TypeScript launcher behavior into Go in phases, starting with `start` so `make start` stops depending on `pnpm`/`tsx`, then `run`, restart supervision, services, `dev`, release packaging, and finally the npm shim.

---

## Backend

### Backend App Extraction

Files:

- `apps/backend/cmd/kandev/main.go`
- `apps/backend/internal/backendapp/app.go` (new)

Changes:

- Move the current server startup logic from `cmd/kandev/main.go` into `internal/backendapp`.
- Expose `func Run(args []string) int` or equivalent.
- Use a private `flag.FlagSet` inside backend mode rather than package-global `flag.CommandLine`, so launcher parsing and backend parsing cannot conflict.
- Preserve current backend flags: `-port`, `-log-level`, `-help`, `-version`.
- Keep build metadata (`Version`, `Commit`, `BuildTime`) available to both launcher and backend mode.

Reason:

- A single `bin/kandev` binary needs to run either launcher code or backend server code without building a second backend artifact.

### Unified Entrypoint

Files:

- `apps/backend/cmd/kandev/main.go`

Changes:

- Replace direct backend startup with dispatch:

```go
func main() {
    os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
    if len(args) > 0 && args[0] == "__backend" {
        return backendapp.Run(args[1:])
    }
    return launcher.Run(args)
}
```

- Keep `__backend` hidden from public help.
- Add tests for dispatch if `run` can be tested without launching the server.

Reason:

- The public executable remains `kandev`, while the launcher can supervise a restartable backend child by spawning itself.

### Launcher Package

Files:

- `apps/backend/internal/launcher/cli`
- `apps/backend/internal/launcher/ports`
- `apps/backend/internal/launcher/paths`
- `apps/backend/internal/launcher/env`
- `apps/backend/internal/launcher/bundle`
- `apps/backend/internal/launcher/health`
- `apps/backend/internal/launcher/process`
- `apps/backend/internal/launcher/supervisor`
- `apps/backend/internal/launcher/service`

Changes:

- Port current behavior from `apps/cli/src/args.ts`, `ports.ts`, `constants.ts`, `shared.ts`, `health.ts`, `web.ts`, `process.ts`, and `supervisor/*`.
- Keep packages narrow to satisfy backend lint limits.
- Use standard library APIs first:
  - `net.Listen("tcp", "127.0.0.1:0")` for free ports.
  - `net/http` for health polling.
  - `os/exec` for child processes.
  - `os/signal` and platform-specific process group handling for shutdown.

Reason:

- This replaces the Node launcher logic while preserving behavior.

### `start` Mode

Files:

- `apps/backend/internal/launcher/start`
- `Makefile`

Changes:

- Detect repo root using current `findRepoRoot` semantics.
- Resolve local backend by using `os.Executable()` and spawning `<self> __backend`.
- Start backend and wait for health; the backend serves the embedded Vite SPA.
- Update `make start` to invoke `apps/backend/bin/kandev start`.

Reason:

- This is the quickest user-visible reduction in Node launcher dependency for local production startup.

### `run` Mode

Files:

- `apps/backend/internal/launcher/run`
- `apps/backend/internal/launcher/bundle`
- `.github/workflows/release.yml`

Changes:

- Validate release bundle layout with one `bin/kandev` executable.
- Resolve runtime via `KANDEV_BUNDLE_DIR` and explicit bundle context.
- Spawn backend using `<bundle>/bin/kandev __backend`.
- Preserve quiet backend log buffering and verbose/debug/headless behavior.
- Decide separately whether to port `--runtime-version`; if deferred, return a clear unsupported/deprecated error.

Reason:

- This replaces the installed runtime launch path used by Homebrew/manual bundles.

### Restart Supervisor

Files:

- `apps/backend/internal/launcher/supervisor`
- existing backend restart adapter call sites, if they assume manifest shape

Changes:

- Port manifest/control/socket behavior from `apps/cli/src/supervisor/*`.
- Manifest records:
  - `backend_executable`: absolute path to `bin/kandev`
  - `argv`: `["__backend"]`
  - backend env allowlist
  - mode, port, cwd, home dir
- Restart control restarts only the backend child process.

Reason:

- Preserves ADR 0019 while avoiding a separate backend binary.

### Service Management

Files:

- `apps/backend/internal/launcher/service`
- `apps/backend/internal/launcher/service/systemd`
- `apps/backend/internal/launcher/service/launchd`
- `Makefile` service targets

Changes:

- Port service argument parsing, help, config output, metadata, systemd unit rendering, launchd plist rendering, install/uninstall/status/logs/restart.
- Generated units execute public `kandev`.
- Service launch path starts backend through hidden child mode.
- Preserve stale-service detection using installed metadata.

Reason:

- Services are part of the public launcher surface and must stop depending on the Node CLI.

### Dev Mode

Files:

- `apps/backend/internal/launcher/dev`
- `Makefile`

Changes:

- Port repo-root detection, dev env, production DB backup guard, and task-workspace detection.
- Prefer spawning `<self> __backend` with dev env if it preserves current behavior.
- Keep `pnpm --filter @kandev/web dev` for the web dev server.
- Preserve Windows `winjob` wrapping if still needed.
- Switch `make dev` only after `dev` parity is proven.

Reason:

- Dev mode is user-facing for contributors but still intentionally depends on pnpm/Node for the web dev server.

---

## Frontend

No web UI changes are required by this spec.

---

## Packaging

### Release Bundles

Files:

- `.github/workflows/release.yml`
- `scripts/release/package-bundle.sh`
- release packaging scripts for runtime npm packages

Changes:

- Package one primary native executable:

```text
kandev/
├── bin/kandev
├── bin/agentctl
└── bin/agentctl-linux-amd64
```

- Stop requiring `dist/kandev/cli` for Homebrew; platform bundles contain native `bin/` artifacts with web assets embedded in `bin/kandev`.
- Add smoke tests for `bin/kandev --help`, `bin/kandev --version`, `bin/kandev run --headless`, and `bin/kandev service --help`.

### npm/npx Shim

Files:

- `apps/cli/bin/cli.js`
- `apps/cli/package.json`
- possibly a small `apps/cli/src` shim test if TypeScript remains

Changes:

- Replace full launcher implementation with a small JS shim.
- Shim maps platform/arch to `@kdlbs/runtime-*`.
- Shim resolves the runtime package and execs `bin/kandev(.exe)` with inherited stdio.
- Remove runtime dependencies from the published npm package; keep old TS launcher dependencies as dev-only test/build dependencies until the legacy source is deleted.

---

## Docs

Files:

- `apps/cli/README_internal.md`
- `apps/cli/README.md`
- `docs/specs/INDEX.md`
- root/scoped `AGENTS.md` if package layout guidance changes

Changes:

- Document that `kandev` is native for Homebrew/release bundles.
- Document that npm/npx still uses Node as the package dispatch mechanism.
- Remove stale references to Homebrew requiring `dist/cli.bundle.js`.

---

## Tests

- **What:** public launcher help hides internal backend mode.
  **File:** `apps/backend/internal/launcher/cli/*_test.go`
  **How:** table-driven parser/help test.

- **What:** `kandev __backend` dispatches to backend mode.
  **File:** `apps/backend/cmd/kandev/*_test.go` or `apps/backend/internal/backendapp/*_test.go`
  **How:** unit test around dispatchable function; avoid starting the full server where possible.

- **What:** port/env precedence matches current CLI behavior.
  **File:** `apps/backend/internal/launcher/env/*_test.go`, `apps/backend/internal/launcher/ports/*_test.go`
  **How:** table-driven unit tests for flags and env vars.

- **What:** `start` runs the native backend with embedded or filesystem Vite assets.
  **File:** `apps/backend/internal/launcher/start/*_test.go`
  **How:** temp directory fixtures for repo roots, `apps/web/dist`, and embedded-asset fallback.

- **What:** bundle validation accepts the single-binary layout.
  **File:** `apps/backend/internal/launcher/bundle/*_test.go`
  **How:** temp bundle fixtures with missing-artifact cases.

- **What:** health failures surface captured backend output.
  **File:** `apps/backend/internal/launcher/health/*_test.go` or `run/*_test.go`
  **How:** `httptest` and short-lived helper process.

- **What:** supervisor manifest restarts same binary with `__backend`.
  **File:** `apps/backend/internal/launcher/supervisor/*_test.go`
  **How:** golden manifest test and restart child test with helper executable.

- **What:** service units execute public `kandev`.
  **File:** `apps/backend/internal/launcher/service/systemd/*_test.go`, `apps/backend/internal/launcher/service/launchd/*_test.go`
  **How:** golden file tests for rendered unit/plist.

- **What:** npm shim resolves runtime package and execs native `bin/kandev`.
  **File:** `apps/cli/*`
  **How:** fixture `node_modules/@kdlbs/runtime-*` package and mocked child process spawn.

---

## E2E Tests

No browser E2E test is required because the feature has no web UI surface.

Add release/local smoke coverage instead:

- **Scenario:** GIVEN a local production build, WHEN `make start` runs, THEN it uses native `apps/backend/bin/kandev start`.
  **File:** release smoke script or backend integration test.
  **What to verify:** process starts and health endpoints become ready.

- **Scenario:** GIVEN a bundle fixture, WHEN `bin/kandev run --headless` runs, THEN backend and web become ready.
  **File:** release smoke script.
  **What to verify:** health endpoint and ready output.

---

## Implementation Waves

Wave 1: Backend extraction and dispatch

- Refactor backend app into callable package.
- Add hidden `__backend` dispatch.
- Add launcher skeleton and parser/help tests.

Wave 2: Shared launcher primitives and `start`

- Port env, ports, paths, health, browser/process helpers.
- Implement `start`.
- Switch `make start`.

Wave 3: Installed runtime `run`

- Port bundle validation and run launch path.
- Add same-binary backend spawning.
- Add bundle smoke tests.

Wave 4: Supervisor and services

- Port restart supervisor.
- Port systemd/launchd service management.
- Add golden tests.

Wave 5: Dev mode

- Port `dev`.
- Switch `make dev` only after parity is confirmed.

Wave 6: Packaging cutover

- Update release bundle shape.
- Convert npm package to thin shim.
- Remove old Node launcher bundle from Homebrew path.
- Update docs.

Wave 7: Cleanup and verification

- Delete obsolete TS launcher modules.
- Run format, tests, lint, and release smoke tests.

---

## Open Questions

- Use `__backend` argv token or `KANDEV_INTERNAL_ROLE=backend` env for hidden backend mode? Recommendation: `__backend`.
- Preserve `--runtime-version <tag>` in the first Go port, or defer/deprecate it?
- Ship old Node CLI fallback for one release while Homebrew/npm packaging switches over?
