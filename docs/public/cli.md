---
title: "CLI"
description: "Install, start, and operate Kandev from the command line."
---

# Kandev CLI

The `kandev` command starts a local Kandev backend, which serves the web UI, HTTP API, WebSocket API, and MCP endpoint. Use it when you want a browser-based installation or a headless/service process. For a packaged system WebView and desktop updates, use the [desktop app](./desktop-app.md) instead.

## Supported release targets

| OS | Architectures | Install channels |
|---|---|---|
| macOS | Apple silicon (`arm64`), Intel (`x64`) | Homebrew, npm/npx |
| Linux | `arm64`, `x64` | Homebrew, npm/npx |
| Windows | `x64` | npm/npx |

The npm package is a small Node.js shim. It selects an exact, same-version native runtime package for `process.platform` and `process.arch`, then starts its `kandev` binary. npm 7 or later is required because the native packages are platform-specific optional dependencies. There is no native Windows ARM64 npm package; running the x64 package under Windows emulation is OS-dependent and is not a tested release target.

Every runtime bundle contains the backend, `agentctl`, the embedded web application, and Linux/macOS `agentctl` helpers used by supported remote executors. Node.js is used only to select the npm runtime; the application itself is native.

## Install

### Homebrew

Homebrew is available on macOS and Linux:

```bash
brew install kdlbs/kandev/kandev
kandev --version
```

### npm or npx

Use a global install for a persistent command:

```bash
npm install -g kandev@latest
kandev --version
```

Or run the release selected by npm's `latest` tag without a global install:

```bash
npx -y kandev@latest
```

If an npm policy such as `--omit=optional` prevents optional dependencies from being installed, Kandev cannot find its native runtime.

## Start and stop

`run` is the default command. These are equivalent:

```bash
kandev
kandev run
```

On a normal start, the launcher:

1. validates the installed runtime bundle;
2. creates the Kandev data directory with owner-only permissions where the platform supports Unix modes;
3. selects backend and `agentctl` ports;
4. starts and supervises the backend;
5. waits up to 45 seconds for `/health`; and
6. opens the printed local URL in the default browser.

The launcher remains in the foreground. Press `Ctrl+C` or terminate it to stop the backend and its managed children cleanly. A force-kill can leave worktree processes or containers running; inspect them before deleting data.

Use headless mode for SSH sessions, containers, or an external reverse proxy:

```bash
kandev --headless
# Alias:
kandev --no-browser
```

The URL is still printed. `KANDEV_NO_BROWSER=1` also suppresses browser launch.

## Commands and options

```text
kandev [run] [options]
kandev start [options]
kandev service <action> [service options]
```

| Option | Meaning |
|---|---|
| `--port <1-65535>` | Request an exact backend port. `--backend-port` is an alias; `--port=<port>` forms also work. |
| `--headless`, `--no-browser` | Do not open a browser. |
| `--verbose`, `-v` | Show backend info output. |
| `--debug` | Show debug output and enable diagnostic endpoints and ACP frame logs. See the security warning below. |
| `--version`, `-V` | Print the native runtime version. |
| `--help`, `-h`, `help` | Print help. |

These commands and options describe the installed native launcher. Unknown arguments fail with exit status 2. In particular, the npm and Homebrew release entrypoints currently invoke that native launcher, which does **not** support `dev`, `--dev`, `--runtime-version`, `--web-internal-port`, or the removed `--web-port` spelling. The source-checkout development launcher has a separate contract described below.

### `dev` and the internal web port are source-checkout options

The repository's TypeScript launcher supports hot-reload development with this logical CLI syntax:

```text
kandev dev [--port <backend-port>] [--web-internal-port <web-port>]
kandev --dev [--port <backend-port>] [--web-internal-port <web-port>]
```

Run its normal setup path from the repository root with `make dev`. To pass the internal-port override directly, invoke the same package script used by that Make target:

```bash
cd apps
pnpm -C cli dev -- dev --web-internal-port 37430
```

`--web-internal-port` accepts an integer from `1` through `65535`, including the `--web-internal-port=<port>` form. It controls the Vite development server that the Go backend reverse-proxies to; `--port` continues to control the backend URL. The flag is valid only with `dev` or `--dev`. `KANDEV_WEB_PORT` is its environment equivalent in dev mode and is ignored by `run` and `start`. Without either override, the source launcher prefers web port `37429` and selects a fallback if that port is unavailable.

The old `--web-port` spelling has been removed, not retained as an alias. The TypeScript launcher rejects both `--web-port <port>` and `--web-port=<port>` and directs callers to `--web-internal-port`; the native release launcher rejects both web-port spellings because it serves embedded web assets and has no separate web process.

### `start` is for a source build

`kandev start` makes the executable invoke its own embedded backend instead of resolving an installed bundle. It is a contributor/local-production-build path, not a second installation channel. From a checkout, use the Make targets so the correct binary and embedded web assets are built:

```bash
make build
make start
```

Development hot reload is also checkout-only:

```bash
make dev
```

`make dev` builds the remote `agentctl` helpers and invokes the repository's TypeScript development launcher. The `kandev dev` syntax above belongs to that source launcher; installing the npm or Homebrew release does not install it as a second runtime mode.

### OS service commands

The native CLI can install and manage a systemd user/system unit on Linux or a launchd agent/daemon on macOS:

```bash
kandev service install
kandev service status
kandev service logs --follow
```

Supported actions are `install`, `uninstall`, `start`, `stop`, `restart`, `status`, `logs`, and `config`. Installation accepts `--system`, `--port`, `--home-dir`, and `--no-boot-start`. In the current native installer, `--port` is written as `KANDEV_SERVER_PORT`, but the supervising launcher overwrites that value with its own automatic port selection; the option therefore does not reliably pin a service listener today. The service normally prefers `38429` and falls back when it is busy. Windows service installation is not implemented. The published native service path also does not write the legacy update metadata consumed by the Settings-page package updater: upgrade with Homebrew/npm, rerun `kandev service install` with the same flags so versioned executable/bundle paths are refreshed, then restart the service. See [Run as a service](./run-as-a-service.md) for privileges, paths, upgrades, and recovery.

## Ports and network exposure

| Process | Preferred port | Automatic behavior |
|---|---:|---|
| Backend (UI, HTTP, WebSocket, MCP) | `38429` | If no port was requested and this port cannot bind on loopback, try up to 10 random ports in `10000`-`60000`. |
| Core `agentctl` | `39429` | Uses the same automatic fallback strategy and never shares the backend port. |

There is no separate web-server port in an installed release: the backend serves embedded assets. If `--port`, `KANDEV_BACKEND_PORT`, or `KANDEV_PORT` specifies a port, the launcher does not substitute another one; backend startup fails if the configured listen address cannot bind it.

The launcher prints `http://localhost:<port>`, but the backend's default `server.host` is `0.0.0.0`. That can expose Kandev to other machines on the network, and the current local product path is not an authenticated multi-user boundary. Bind it to loopback unless remote access is deliberately protected:

```bash
KANDEV_SERVER_HOST=127.0.0.1 kandev
```

See [Configuration](./configuration.md) before putting Kandev behind a reverse proxy or publishing the port.

## Launcher environment

Flags take precedence over the equivalent port variables. `KANDEV_BACKEND_PORT` takes precedence over `KANDEV_PORT`.

| Variable | Default | Behavior |
|---|---|---|
| `KANDEV_BACKEND_PORT` | unset | Backend port when `--port` is absent. |
| `KANDEV_PORT` | unset | Compatibility backend-port alias. |
| `KANDEV_HOME_DIR` | `~/.kandev` | Root for application data, tasks, repositories, logs, and launcher state. |
| `KANDEV_DATABASE_PATH` | `<home>/data/kandev.db` | Advanced SQLite path override. See the backup caveat in [Configuration](./configuration.md). |
| `KANDEV_LOG_LEVEL` | `warn` from the launcher | Explicit backend log level; overrides `--verbose` and `--debug` log-level selection. |
| `KANDEV_HEALTH_TIMEOUT_MS` | `45000` | Positive integer startup-health timeout. Invalid or non-positive values fall back to 45 seconds. |
| `KANDEV_NO_BROWSER` | unset | The exact value `1` suppresses browser opening. |
| `KANDEV_BUNDLE_DIR` | selected by installer | Advanced packaging override. The npm shim and Homebrew wrapper set this; a bad path is a fatal runtime validation error. |
| `KANDEV_VERSION` | unset | Optional installer-supplied display metadata (the Homebrew wrapper sets it). |
| `KANDEV_SHUTDOWN_DEBUG` | unset | The exact value `1` prints launcher process IDs, commands, paths, signals, and graceful/forced shutdown decisions. Use temporarily for shutdown diagnosis. |

The launcher also sets the selected server and `agentctl` ports for the backend. Treat its supervisor socket and manifest under `<home>/supervisor/` as private implementation state, not a control API.

`--debug` sets `KANDEV_DEBUG_AGENT_MESSAGES=true` and `KANDEV_DEBUG_PPROF_ENABLED=true`. ACP logs can contain full prompts, file content, and tool calls, while diagnostic endpoints expose process details. Use debug mode only on a trusted machine and remove retained debug logs afterward. [Configuration](./configuration.md) lists their location and retention controls.

## Data and cleanup

The default persistent root is `~/.kandev` (on Windows, `.kandev` below the user's profile directory). The SQLite database normally resides at `<home>/data/kandev.db`; repositories, task worktrees, logs, and encrypted settings also live below the home root.

Runtime program files live in the Homebrew Cellar, the global npm dependency tree, or npm's `_npx` cache. They are not application data. `npm config get cache` and `npm root -g` show the latter two roots.

Uninstalling the package does not remove `<home>`. Before removing that directory, stop Kandev and any service, confirm no task or executor is running, and take a backup. See [Operations](./operations.md) for safe database backup and restore procedures.

## Update

The installer owns CLI updates:

```bash
brew upgrade kandev
npm install -g kandev@latest
npx -y kandev@latest
```

Release packages pin the shim and native runtime packages to the same SemVer. Do not copy only one binary from a different release into a bundle. A service unit contains installation-specific executable and bundle paths; after the package upgrade, reinstall it with the same service flags and restart it as described above.

## Troubleshooting

### No runtime package found

Confirm npm 7 or later, an exact supported OS/architecture pair, and optional-dependency installation:

```bash
npm --version
npm install -g kandev@latest
kandev --version
```

Remove `--omit=optional` from npm configuration for this install. On unsupported targets, use a supported machine or remote environment; do not point `KANDEV_BUNDLE_DIR` at a bundle from another platform.

### A required binary or remote helper is missing

Reinstall or upgrade the whole package. Runtime validation intentionally fails when `kandev`, `agentctl`, or a required Linux/macOS remote helper is absent. Mixing archives or pruning package files produces this error.

### The requested port is already in use

Omit the explicit port to allow fallback, or choose a verified free port:

```bash
kandev --port 18080
```

If the failure remains, check both the configured `KANDEV_SERVER_HOST` and local listeners. The launcher's loopback preflight cannot guarantee that a later `0.0.0.0` bind will succeed.

### Startup health check times out

First run with visible logs, then increase the timeout only if startup is genuinely slow:

```bash
kandev --verbose
KANDEV_HEALTH_TIMEOUT_MS=90000 kandev --verbose
```

The launcher prints buffered backend output when startup fails. Common causes are an invalid `config.yaml`, a database migration/permission error, an occupied explicit port, or a damaged runtime bundle.

### Browser does not open

Open the printed URL manually. Linux requires a working `xdg-open`; WSL often needs manual browser launch. `--headless` and `KANDEV_NO_BROWSER=1` intentionally skip the opener.

### Configuration appears ignored

The public CLI has no `--config` option. `config.yaml` must be in the launcher's backend working directory or `/etc/kandev/`, and environment values override it. For predictable installed/service deployments, prefer explicit environment variables and verify the active home and database paths printed at startup. See [Configuration](./configuration.md).
