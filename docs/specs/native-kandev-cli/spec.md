---
status: draft
created: 2026-06-15
owner: tbd
---

# Native Kandev CLI

## Why

Users start Kandev from the terminal through `kandev`, but the current launcher is a Node.js/TypeScript program even when Kandev is installed from Homebrew or run from a local production build. This adds a runtime dependency and extra moving parts to the most basic startup path. Users should keep the same `kandev` command while the launcher becomes a native executable.

## What

- `kandev` remains the public command for Homebrew, npm/npx, global npm installs, service units, and local development.
- Homebrew and release bundle installs provide a native `bin/kandev` executable that can launch Kandev without executing the TypeScript CLI bundle.
- The native `kandev` executable supports the public launcher commands users need after the web runtime merge: default run, `run`, `start`, `dev`, `service`, `--help`, `--version`, `--port`, `--backend-port`, `--verbose`, `--debug`, and `--headless`.
- The native launcher starts the backend as a supervised child process by re-executing the same `bin/kandev` binary in a hidden backend mode.
- The hidden backend mode is not a public command and is not shown in normal help output.
- Backend restarts restart only the backend child process; the launcher/supervisor remains alive unless the shutdown policy requires the whole app to exit.
- npm/npx continues to expose the `kandev` command through the `kandev` npm package. The npm package may use a minimal JavaScript shim, but that shim only resolves the platform runtime package and execs its native `bin/kandev`.
- `make start` launches through the native `apps/backend/bin/kandev start` path after building local artifacts.
- Existing service installs continue to be managed by `kandev service ...`; newly generated service units execute the public `kandev` launcher path.
- Startup output continues to show the URL users should open, the MCP URL, database path when applicable, and log level when applicable.
- Production `run` and `start` do not execute a Node.js web runtime; the Go backend serves the embedded Vite SPA assets.

## API surface

### Public CLI

```text
kandev [--port <port>] [--verbose] [--debug] [--headless]
kandev run [--port <port>] [--verbose] [--debug] [--headless]
kandev start [--port <port>] [--verbose] [--debug] [--headless]
kandev dev [--port <port>]
kandev --dev [--port <port>]
kandev service install|uninstall|start|stop|restart|status|logs|config [--system]
kandev --version
kandev --help
```

Port flags and environment variables:

- `--port` and `--backend-port` select the public backend port.
- `KANDEV_PORT` and `KANDEV_BACKEND_PORT` provide backend port defaults.
- There is no production web port flag or environment variable. The Go backend serves the embedded SPA on the backend port.

Logging/debug flags:

- `--verbose` shows backend info logs.
- `--debug` shows debug logs and enables current debug environment behavior.
- `--headless` skips browser opening and prints the ready URL.

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
└── bin/agentctl-linux-amd64
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

## Persistence guarantees

- Service installation metadata remains durable across restarts and upgrades.
- Supervisor manifest/control files remain under the configured Kandev home directory and are overwritten on each launch with the current executable path and backend argv.
- The launcher does not persist child process state beyond the existing supervisor manifest. After a launcher restart, it starts a fresh backend child.
- User data, database files, task worktrees, and backend-managed persistence remain owned by existing backend persistence behavior; the launcher migration does not change their retention.

## Scenarios

- **GIVEN** Kandev is installed from Homebrew, **WHEN** the user runs `kandev --help`, **THEN** the help output describes the public launcher commands and does not show `__backend`.
- **GIVEN** Kandev is installed from Homebrew, **WHEN** the user runs `kandev --version`, **THEN** the command prints the installed Kandev version without executing the Node CLI bundle.
- **GIVEN** a valid release bundle, **WHEN** the user runs `kandev --headless`, **THEN** the native launcher starts `kandev __backend`, waits for the backend to serve API and SPA routes, and prints the backend URL.
- **GIVEN** a local checkout with built backend and web artifacts, **WHEN** the user runs `make start`, **THEN** the Makefile launches through `apps/backend/bin/kandev start` and does not invoke `pnpm -C cli dev -- start`.
- **GIVEN** the backend has requested a restart through the restart adapter, **WHEN** the launcher receives the restart request, **THEN** only the `kandev __backend` child process is replaced.
- **GIVEN** the user presses Ctrl-C while Kandev is running, **WHEN** the launcher handles the signal, **THEN** it terminates the backend child process before exiting.
- **GIVEN** a new service install on Linux or macOS, **WHEN** the user runs `kandev service install`, **THEN** the generated service unit executes the public `kandev` launcher path.
- **GIVEN** a user runs `npx kandev@latest --help`, **WHEN** npm has installed the platform runtime package, **THEN** the npm shim execs the runtime package's native `bin/kandev` and the user sees the same public help output.
## Out of scope

- Making `npx` itself Node-free.
- Introducing `kandevctl` as a user-facing command.
- Requiring a separate shipped backend binary such as `kandev-backend`.
- Changing task, workflow, agent, or integration behavior after the backend has started.

## Open questions

- Should hidden backend mode be selected by the `__backend` argv token or by `KANDEV_INTERNAL_ROLE=backend`? Recommendation: `__backend`.
- Should `--runtime-version <tag>` be ported in the first native launcher implementation or deferred/deprecated?
- Should one release ship both the old Node CLI bundle and the native `bin/kandev` for compatibility while Homebrew/npm packaging switches over?
