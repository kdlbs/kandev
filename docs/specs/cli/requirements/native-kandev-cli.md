---
status: draft
system: cli
created: 2026-06-15
owners:
  - tbd
---
# Native Kandev CLI Requirements

## Overview

Users start Kandev from the terminal through `kandev`, but the current launcher is a Node.js/TypeScript program even when Kandev is installed from Homebrew or run from a local production build. This adds a runtime dependency and extra moving parts to the most basic startup path. Users should keep the same `kandev` command while the launcher becomes a native executable.

## Requirements

### REQ-CLI-NATIVE-KANDEV-CLI-001: Native Kandev CLI

**Intent:** Users start Kandev from the terminal through `kandev`, but the current launcher is a Node.js/TypeScript program even when Kandev is installed from Homebrew or run from a local production build. This adds a runtime dependency and extra moving parts to the most basic startup path. Users should keep the same `kandev` command while the launcher becomes a native executable.

#### Acceptance criteria

- **AC-CLI-NATIVE-KANDEV-CLI-001.1:** `kandev` remains the public command for Homebrew, npm/npx, global npm installs, service units, and local development.
- **AC-CLI-NATIVE-KANDEV-CLI-001.2:** Homebrew and release bundle installs provide a native `bin/kandev` executable that can launch Kandev without executing the TypeScript CLI bundle.
- **AC-CLI-NATIVE-KANDEV-CLI-001.3:** The native `kandev` executable supports the public launcher commands users need after the web runtime merge: default run, `run`, `start`, `service`, `--help`, `--version`, `--port`, `--backend-port`, `--verbose`, `--debug`, and `--headless`.
- **AC-CLI-NATIVE-KANDEV-CLI-001.4:** Native `dev` mode is supported for source checkouts through the Go launcher (`kandev dev` / `--dev`); installed release entrypoints do not expose it — it requires the repository checkout that `make dev` provides.
- **AC-CLI-NATIVE-KANDEV-CLI-001.5:** The native launcher starts the backend as a supervised child process by re-executing the same `bin/kandev` binary in a hidden backend mode.
- **AC-CLI-NATIVE-KANDEV-CLI-001.6:** The hidden backend mode is not a public command and is not shown in normal help output.
- **AC-CLI-NATIVE-KANDEV-CLI-001.7:** Backend restarts restart only the backend child process; the launcher/supervisor remains alive unless the shutdown policy requires the whole app to exit.
- **AC-CLI-NATIVE-KANDEV-CLI-001.8:** npm/npx continues to expose the `kandev` command through the `kandev` npm package. The npm package may use a minimal JavaScript shim, but that shim only resolves the platform runtime package and execs its native `bin/kandev`.

## Migrated source detail

## Why

Users start Kandev from the terminal through `kandev`, but the current launcher is a Node.js/TypeScript program even when Kandev is installed from Homebrew or run from a local production build. This adds a runtime dependency and extra moving parts to the most basic startup path. Users should keep the same `kandev` command while the launcher becomes a native executable.

## What

- `kandev` remains the public command for Homebrew, npm/npx, global npm installs, service units, and local development.
- Homebrew and release bundle installs provide a native `bin/kandev` executable that can launch Kandev without executing the TypeScript CLI bundle.
- The native `kandev` executable supports the public launcher commands users need after the web runtime merge: default run, `run`, `start`, `service`, `--help`, `--version`, `--port`, `--backend-port`, `--verbose`, `--debug`, and `--headless`.
- Native `dev` mode is supported for source checkouts through the Go launcher (`kandev dev` / `--dev`); installed release entrypoints do not expose it — it requires the repository checkout that `make dev` provides.
- The native launcher starts the backend as a supervised child process by re-executing the same `bin/kandev` binary in a hidden backend mode.
- The hidden backend mode is not a public command and is not shown in normal help output.
- Backend restarts restart only the backend child process; the launcher/supervisor remains alive unless the shutdown policy requires the whole app to exit.
- npm/npx continues to expose the `kandev` command through the `kandev` npm package. The npm package may use a minimal JavaScript shim, but that shim only resolves the platform runtime package and execs its native `bin/kandev`.
- `make start` launches through the native `apps/backend/bin/kandev start` path after building local artifacts.
- Existing service installs continue to be managed by `kandev service ...`; newly generated service units execute the public `kandev` launcher path.
- Reinstalling or updating a system service preserves the service account recorded by the existing
  root-controlled Kandev unit or plist. The account changes only when the operator supplies
  `--run-as <user>` explicitly.
- A first system install invoked from a root login does not silently select root. It requires an
  explicit `--run-as <user>`; selecting root is allowed only as the explicit `--run-as root`
  choice. A first install through `sudo` may continue to select the non-root invoking user.
- Before rewriting or restarting a system service, the installer verifies that the configured
  Kandev home is owned by the selected service account. A mismatch fails with recovery guidance;
  the installer does not recursively change ownership.
- Service units preserve install-time Node/npm/npx bin directories when they are discoverable, so agents can still invoke npm-managed CLIs under service managers that do not source shell profiles.
- Startup output continues to show the URL users should open, the MCP URL, database path when applicable, and log level when applicable.
- Production `run` and `start` do not execute a Node.js web runtime; the Go backend serves the embedded Vite SPA assets.

## API surface

### Public CLI

```text
kandev [--port <port>] [--verbose] [--debug] [--headless]
kandev run [--port <port>] [--verbose] [--debug] [--headless]
kandev start [--port <port>] [--verbose] [--debug] [--headless]
kandev service install [--system] [--run-as <user>] [--port <port>] [--home-dir <path>] [--no-boot-start]
kandev service uninstall|start|stop|restart|status|logs|config [--system]
kandev --version
kandev --help
```

Port flags and environment variables:

- `--port` and `--backend-port` select the public backend port.
- `KANDEV_PORT` and `KANDEV_BACKEND_PORT` provide backend port defaults.
- There is no production web port flag or environment variable. The Go backend serves the embedded SPA on the backend port.
- `--runtime-version` is not supported by the native launcher and is rejected as a usage error.

Logging/debug flags:

- `--verbose` shows backend info logs.
- `--debug` shows debug logs and enables current debug environment behavior.
- `--headless` skips browser opening and prints the ready URL.

System-service identity flags:

- `--run-as <user>` is valid only for `kandev service install --system`.
- On reinstall, omitting `--run-as` preserves the account in the existing Kandev-managed
  root-controlled unit or plist, even when the update command is run from a different login.
- On a first install, omitting `--run-as` selects a non-root `SUDO_USER` when available. A root
  login without that context must choose explicitly, including when the desired account is root.
- The service-owned `service/install.json` metadata records the effective account but is not
  trusted by a privileged installer as authority for choosing a system account.

### Hidden backend mode

The native launcher has a private backend mode:

```text
kandev __backend [backend flags]
```

This mode starts the backend server and is invoked only by the launcher/supervisor. It is intentionally hidden from public help output. Existing backend diagnostic flags may remain available in this mode for tests and direct diagnostics.

### Runtime bundle

Release bundles expose this layout:

```text
kandev/
├── bin/kandev
├── bin/agentctl
├── bin/agentctl-linux-amd64
├── bin/agentctl-darwin-arm64
└── bin/agentctl-darwin-amd64
```

`bin/kandev` is both the public launcher and the hidden backend-mode executable.

### Supervisor manifest

When backend restart supervision is enabled, the launch manifest records the same executable plus hidden backend argv:

```json
{
  "version": 1,
  "backend_executable": "/absolute/path/to/bin/kandev",
  "argv": ["__backend"],
  "cwd": "/absolute/path/to/bin",
  "env": {
    "KANDEV_SERVER_PORT": "38429",
    "KANDEV_AGENT_STANDALONE_PORT": "39429",
    "KANDEV_RESTART_ADAPTER": "supervisor"
  },
  "home_dir": "/absolute/path/to/home",
  "port": 38429,
  "mode": "run",
  "created_at": "timestamp"
}
```

## State machine

Launcher process lifecycle:

- `idle`: command parsed but no children started.
- `backend-starting`: launcher has spawned `kandev __backend` and is waiting for backend health.
- `ready`: backend is healthy and serving API, WebSocket, and SPA routes; browser may be opened unless headless.
- `restarting-backend`: supervisor received a restart request and is replacing only the backend child.
- `shutting-down`: launcher is terminating child processes after signal, service stop, or child exit policy.
- `failed`: startup failed and the launcher exits non-zero after surfacing the actionable error.

Transitions:

- `idle` -> `backend-starting`: user runs `kandev`, `kandev run`, `kandev start`, or service unit starts.
- `backend-starting` -> `failed`: backend exits or health timeout expires.
- `backend-starting` -> `ready`: backend health endpoint reports ready.
- `ready` -> `restarting-backend`: backend restart adapter requests restart.
- `restarting-backend` -> `ready`: replacement backend becomes healthy.
- any active state -> `shutting-down`: signal, service stop, or non-restartable child exit.

## Failure modes

- If a required backend artifact is missing in `start` mode, `kandev start` exits non-zero with a message that tells the user to run `make build`.
- If the runtime bundle is missing required files in `run` mode, `kandev run` exits non-zero and names the missing artifact.
- If the backend does not become healthy before timeout, the launcher exits non-zero and includes captured backend output when running in quiet mode.
- If the backend child exits unexpectedly after startup, the launcher either restarts it through the supervisor path when the exit is restart-controlled or shuts down the app with the backend exit code.
- If npm/npx optional runtime resolution fails, the JavaScript shim exits non-zero with an actionable message explaining that the platform runtime package is missing.
- If a privileged reinstall finds an existing Kandev-managed system unit or plist, it fails before
  rewriting or restarting the service when that root-controlled definition is malformed or its
  account cannot be resolved.
- If a first system install is run from a root login without `--run-as`, it exits with usage
  guidance instead of silently creating a root service.
- If the configured system Kandev home owner does not match the selected service account, install
  exits before changing the service definition and explains how to preserve the existing account
  or deliberately reconcile ownership. It does not add a global Git trust exception or chown the
  data tree automatically.

## Persistence guarantees

- Service installation metadata remains durable across restarts and upgrades.
- The effective system-service account remains stable across package upgrades and repeated
  `service install --system` calls unless an operator explicitly changes it with `--run-as`.
- For an existing system service, the root-controlled unit or plist is authoritative for service
  identity. Service-owned metadata may corroborate that identity but cannot authorize a
  privileged identity change.
- Supervisor manifest/control files remain under the configured Kandev home directory and are overwritten on each launch with the current executable path and backend argv.
- The launcher does not persist child process state beyond the existing supervisor manifest. After a launcher restart, it starts a fresh backend child.
- User data, database files, task worktrees, and backend-managed persistence remain owned by existing backend persistence behavior; the launcher migration does not change their retention.

## Scenarios

- **GIVEN** Kandev is installed from Homebrew, **WHEN** the user runs `kandev --help`, **THEN** the help output describes the public launcher commands and does not show `__backend`.
- **GIVEN** Kandev is installed from Homebrew, **WHEN** the user runs `kandev --version`, **THEN** the command prints the installed Kandev version without executing the Node CLI bundle.
- **GIVEN** a valid release bundle, **WHEN** the user runs `kandev --headless`, **THEN** the native launcher starts `kandev __backend`, waits for the backend to serve API and SPA routes, and prints the backend URL.
- **GIVEN** a local checkout with built backend and web artifacts, **WHEN** the user runs `make start`, **THEN** the Makefile launches through `apps/backend/bin/kandev start` and does not invoke the TypeScript CLI launcher (`kandev dev` likewise goes through the same Go binary).
- **GIVEN** the backend has requested a restart through the restart adapter, **WHEN** the launcher receives the restart request, **THEN** only the `kandev __backend` child process is replaced.
- **GIVEN** the user presses Ctrl-C while Kandev is running, **WHEN** the launcher handles the signal, **THEN** it terminates the backend child process before exiting.
- **GIVEN** a new service install on Linux or macOS, **WHEN** the user runs `kandev service install`, **THEN** the generated service unit executes the public `kandev` launcher path.
- **GIVEN** a system service already runs as `brewuser`, **WHEN** root later runs
  `kandev service install --system` during an upgrade without `--run-as`, **THEN** the generated
  definition still runs as `brewuser`.
- **GIVEN** an existing system service, **WHEN** an operator intentionally supplies
  `--run-as another-user`, **THEN** Kandev validates that account and the configured home owner
  before changing the service definition.
- **GIVEN** no existing system service and no non-root `SUDO_USER`, **WHEN** root runs
  `kandev service install --system` without `--run-as`, **THEN** installation fails and asks for an
  explicit account rather than defaulting to root.
- **GIVEN** a selected service account whose UID differs from the configured system home owner,
  **WHEN** installation runs, **THEN** it fails before rewriting or restarting the service and does
  not recursively chown the home.
- **GIVEN** service-owned install metadata names a more privileged account than the existing
  root-controlled service definition, **WHEN** a privileged reinstall runs, **THEN** the metadata
  cannot change the selected account.
- **GIVEN** a native user-service install on Linux or macOS, **WHEN** the service starts, **THEN** its environment and durable install metadata identify it as a kandev-managed service and the guarded System-page updater can launch the native self-update helper.
- **GIVEN** Node is managed by nvm/fnm/asdf/volta/mise and `node`, `npm`, or `npx` resolve from that manager at install time, **WHEN** the user runs `kandev service install`, **THEN** the generated service environment includes the detected Node tool bin directory in `PATH`.
- **GIVEN** a user runs `npx kandev@latest --help`, **WHEN** npm has installed the platform runtime package, **THEN** the npm shim execs the runtime package's native `bin/kandev` and the user sees the same public help output.

## Out of scope

- Making `npx` itself Node-free.
- Introducing `kandevctl` as a user-facing command.
- Requiring a separate shipped backend binary such as `kandev-backend`.
- Changing task, workflow, agent, or integration behavior after the backend has started.
- Automatically changing ownership, managing ACLs, or adding broad Git `safe.directory` entries
  when a system-service identity mismatch is detected.
- Managing Windows services or inferring service identity from an untrusted service-owned metadata
  file.

## Repair plan

See the
[system-service identity guardrails plan](../../../plans/system-service-identity-guardrails/plan.md).

## Decision

System-service account continuity and ownership validation follow
[ADR-2026-07-31-system-service-user-continuity](../../../decisions/2026-07-31-system-service-user-continuity.md).

## Open questions

- Should hidden backend mode be selected by the `__backend` argv token or by `KANDEV_INTERNAL_ROLE=backend`? Recommendation: `__backend`.
- Should `--runtime-version <tag>` be ported in a later native launcher implementation or remain deprecated?
- Should one release ship both the old Node CLI bundle and the native `bin/kandev` for compatibility while Homebrew/npm packaging switches over?
