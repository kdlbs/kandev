---
title: "CLI"
description: "Install, start, and operate Kandev from the command line."
---

# Kandev CLI

The `kandev` command starts a local Kandev backend, which serves the web UI, HTTP API, WebSocket API, and MCP endpoint. Use it when you want a browser-based installation or a headless/service process. For a packaged system WebView and desktop updates, use the [desktop app](desktop-app.md) instead.

## Quick path

1. Install with Homebrew or npm.
2. Run `kandev` and open the printed URL.
3. Use `--headless` for a server or SSH session.
4. Keep the backend on loopback unless a trusted proxy and authentication protect it.

## Supported release targets

| OS | Architectures | Install channels |
|---|---|---|
| macOS | Apple silicon (`arm64`), Intel (`x64`) | Homebrew, npm/npx |
| Linux | `arm64`, `x64` | Homebrew, npm/npx |
| Windows | `x64` | Scoop, npm/npx |

The npm package is a small Node.js shim. It selects an exact, same-version native runtime package for `process.platform` and `process.arch`, then starts its `kandev` binary. npm 7 or later is required because the native packages are platform-specific optional dependencies. There is no native Windows ARM64 npm package; running the x64 package under Windows emulation is OS-dependent and is not a tested release target.

Every runtime bundle contains the backend, `agentctl`, the embedded web application, and Linux/macOS `agentctl` helpers used by supported remote executors. Node.js is used only to select the npm runtime; the application itself is native.

## Install

### Homebrew

Homebrew is available on macOS and Linux:

```bash
brew install kdlbs/kandev/kandev
kandev --version
```

### Scoop

Scoop is available on Windows:

```bash
scoop bucket add kandev https://github.com/kdlbs/scoop-kandev
scoop install kandev
kandev --version
```

The bucket installs the native runtime bundle, so Node.js is not required to
install or start Kandev. Node.js is still needed for the agent CLIs Kandev
installs through its own interface.

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

### Release archive

Every release publishes a runtime archive per platform, named `kandev-<platform>.tar.gz`, where
`<platform>` is one of `linux-x64`, `linux-arm64`, `macos-x64`, `macos-arm64`, or `windows-x64`.
Pick the archive matching the machine, verify it against the `.sha256` published beside it, extract
it, and run the launcher from the extracted tree:

```bash
curl -fsSLO https://github.com/kdlbs/kandev/releases/latest/download/kandev-linux-x64.tar.gz
curl -fsSLO https://github.com/kdlbs/kandev/releases/latest/download/kandev-linux-x64.tar.gz.sha256
shasum -a 256 -c kandev-linux-x64.tar.gz.sha256
tar -xzf kandev-linux-x64.tar.gz
./kandev/bin/kandev --version
```

Verifying the checksum matters more here than with the package managers, which do that themselves.

The archive extracts to a `kandev/` directory containing `bin/`, and the launcher finds the rest of
the bundle relative to itself. The extracted directory can be moved anywhere; add `kandev/bin` to
`PATH` for a persistent command. Set `KANDEV_BUNDLE_DIR` only to point the launcher at a bundle it
is not part of.

### npm nightly

Stable remains npm's default `latest` tag. To opt into the current prerelease from `main`, install
or run the `nightly` tag explicitly:

```bash
npm install -g kandev@nightly
kandev --version

# One-off launch
npx -y kandev@nightly
```

A nightly version has the form `X.Y.(Z+1)-nightly.sha<12-hex-character SHA prefix>`, based on the latest
stable `X.Y.Z`. The launcher and all five platform runtime packages use that same immutable
version. Nightlies are best-effort daily snapshots. A new one appears only when `main` has changed
since the latest Stable release, so some days may have no new Nightly.

Nightly does not move `latest` and does not publish a Homebrew formula, Desktop updater feed,
container tag, Git tag, or GitHub Release. Use it for prerelease testing, not unattended production
rollout.

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
5. derives one or more `/health` targets from the effective server binds;
6. waits up to 45 seconds for any target to return the launcher's health token (the backend is alive and its socket is bound);
7. waits for `/ready` (the backend has finished startup recovery and can serve real requests); this wait is unbounded by design, since recovery can legitimately take much longer than 45 seconds; and
8. opens the reachable access URL in the default browser.

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
| `--debug` | Record debug output in the backend file and enable diagnostic endpoints and ACP frame logs; stdout remains concise. See the security warning below. |
| `--version`, `-V` | Print the native runtime version. |
| `--help`, `-h`, `help` | Print help. |

These commands and options describe the installed native launcher. Unknown arguments fail with exit status 2. In particular, the npm and Homebrew release entrypoints currently invoke that native launcher, which does **not** support `dev`, `--dev`, `--runtime-version`, `--web-internal-port`, or the removed `--web-port` spelling. The source-checkout development launcher has a separate contract described below.

<details>
<summary>Source checkout and service commands</summary>

### `dev` and the internal web port are source-checkout options

The native launcher supports hot-reload development with this CLI syntax:

```text
kandev dev [--port <backend-port>] [--web-internal-port <web-port>]
kandev --dev [--port <backend-port>] [--web-internal-port <web-port>]
```

Run its normal setup path from the repository root with `make dev`. To pass the internal-port override directly, invoke the launcher binary:

```bash
make dev WEB_PORT=37430
# or, after `make -C apps/backend build`:
apps/backend/bin/kandev dev --web-internal-port 37430
```

`--web-internal-port` accepts an integer from `1` through `65535`, including the `--web-internal-port=<port>` form. It controls the Vite development server that the Go backend reverse-proxies to; `--port` continues to control the backend URL. The flag is valid only with `dev` or `--dev`. `KANDEV_WEB_PORT` is its environment equivalent in dev mode and is ignored by `run` and `start`. Without either override, the launcher prefers web port `37429` and selects a fallback if that port is unavailable.

The old `--web-port` spelling has been removed, not retained as an alias. The launcher rejects both `--web-port <port>` and `--web-port=<port>` and directs callers to `--web-internal-port`; outside dev mode the release launcher rejects both web-port spellings because it serves embedded web assets and has no separate web process.

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

`make dev` builds the launcher plus the host `agentctl` and, on hosts other than Linux/amd64, one Linux/amd64 helper. It then invokes the repository's native Go launcher. The `kandev dev` syntax above belongs to that source launcher; installing the npm or Homebrew release does not install it as a second runtime mode.

### OS service commands

The native CLI can install and manage a systemd user/system unit on Linux or a launchd agent/daemon on macOS:

```bash
kandev service install
kandev service status
kandev service logs --follow
```

Supported actions are `install`, `uninstall`, `start`, `stop`, `restart`, `status`, `logs`, and `config`. Installation accepts `--system`, `--run-as <user>` (only with `--system`), `--port`, `--home-dir`, and `--no-boot-start`. Reinstalling an existing Kandev-managed system service preserves its account unless `--run-as` is supplied explicitly. A first system install from a root login requires `--run-as`, including `--run-as root` when root is intentional. In the current native installer, `--port` is written as `KANDEV_SERVER_PORT`, but the supervising launcher overwrites that value with its own automatic port selection; the option therefore does not reliably pin a service listener today. The service normally prefers `38429` and falls back when it is busy. Windows service installation is not implemented. Managed user services write owner-only install metadata so **Settings > System > Updates** can offer the guarded Apply action; system services and failed guarded updates use the package manager followed by `kandev service install` and `kandev service restart`. See [Run as a service](run-as-a-service.md) for privileges, paths, upgrades, and recovery.

</details>

## Ports and network exposure

| Process | Preferred port | Automatic behavior |
|---|---:|---|
| Backend (UI, HTTP, WebSocket, MCP) | `38429` | If no port was requested and this port cannot bind on loopback, try up to 10 random ports in `10000`-`60000`. |
| Core `agentctl` | `39429` | Uses the same automatic fallback strategy and never shares the backend port. |

There is no separate web-server port in an installed release: the backend serves embedded assets. For the `run`, `start`, and development launcher flows, if `--port`, `KANDEV_BACKEND_PORT`, or `KANDEV_PORT` specifies a port, the launcher checks it before declaring the backend ready, does not substitute another one, and fails startup if the configured listen address cannot bind it. `kandev service install --port` remains the separate installer behavior described above.

The launcher derives readiness targets and the access URL from the effective `server.host` or `server.hosts` values. A specific IP or hostname is probed directly. An IPv4 wildcard is probed through `127.0.0.1`, while the default local browser/access URL remains `http://localhost:<port>` to preserve the established browser origin. An IPv6 wildcard is probed and accessed through `[::1]`. With multiple binds, the launcher probes all targets concurrently but preserves target priority: a lower-priority success is selected only after higher-priority targets complete without the launcher's health token. It prefers a loopback target for the printed or opened access URL.

When it can enumerate non-loopback interfaces, the launcher also prints `network:` URLs for each unique non-loopback, non-link-local address on the same port, including local-network and Tailscale addresses. IPv6 addresses are shown in brackets. If the effective bind restricts the listener, the launcher only prints matching addresses. These lines are informational and are omitted if interface discovery is unavailable.

The `dev`, `start`, and `run` startup banners also print a `version:` line. Its value matches `kandev --version`; an unstamped local build reports `dev`.

The backend's default `server.host` is `0.0.0.0`. That can expose Kandev to other machines on the network, and the current local product path is not an authenticated multi-user boundary. The launcher uses `localhost` for its default local access URL, but the backend still listens on every interface. Bind it to loopback unless remote access is deliberately protected:

```bash
KANDEV_SERVER_HOST=127.0.0.1 kandev
```

See [Configuration](configuration.md) before putting Kandev behind a reverse proxy or publishing the port.

## Launcher environment

Flags take precedence over the equivalent port variables. `KANDEV_BACKEND_PORT` takes precedence over `KANDEV_PORT`.

Every CLI launch automatically uses the startup configuration discovery in
[Configuration](configuration.md): `config.yaml` in the working directory,
then `<KANDEV_HOME_DIR>/config.yaml` (or `~/.kandev/config.yaml`), then
`/etc/kandev/config.yaml`. The first existing file is authoritative; Kandev
does not merge candidates, and an unreadable or invalid first file stops the
launch. There is no public `--config` flag. Service launches use the same
discovery and carry the selected file into the managed backend process.

| Variable | Default | Behavior |
|---|---|---|
| `KANDEV_BACKEND_PORT` | unset | Backend port when `--port` is absent. |
| `KANDEV_PORT` | unset | Compatibility backend-port alias. |
| `KANDEV_HOME_DIR` | `~/.kandev` | Root for application data, tasks, repositories, logs, and launcher state. |
| `KANDEV_DATABASE_PATH` | `<home>/data/kandev.db` | Advanced SQLite path override. System backups use the sibling `backups/` directory. See [Configuration](configuration.md). |
| `KANDEV_LOG_LEVEL` | `warn` from the launcher | Explicit backend log level; overrides `--verbose` and `--debug` log-level selection. |
| `KANDEV_HEALTH_TIMEOUT_MS` | `45000` | Positive integer startup-health timeout. Invalid or non-positive values fall back to 45 seconds. |
| `KANDEV_NO_BROWSER` | unset | The exact value `1` suppresses browser opening. |
| `KANDEV_BUNDLE_DIR` | selected by installer | Advanced packaging override. The npm shim and Homebrew wrapper set this; a bad path is a fatal runtime validation error. |
| `KANDEV_VERSION` | unset | Optional installer-supplied display metadata (the Homebrew wrapper sets it). |
| `KANDEV_SHUTDOWN_DEBUG` | unset | The exact value `1` prints launcher process IDs, commands, paths, signals, and graceful/forced shutdown decisions. Use temporarily for shutdown diagnosis. |

The launcher also sets the selected server and `agentctl` ports for the backend. Treat its supervisor socket and manifest under `<home>/supervisor/` as private implementation state, not a control API.

`--debug` sets `KANDEV_DEBUG_AGENT_MESSAGES=true` and
`KANDEV_DEBUG_PPROF_ENABLED=true` without selecting the `dev` profile. The
`make start-debug` launcher also defaults the browser title to `Debug Kandev`;
`make dev` selects the development profile and uses `Dev Kandev`. Debug events
go to `<home>/logs/backend-logs.log`; warn and above still appear on stdout.
ACP logs can contain full prompts, file content, and tool calls, while
diagnostic endpoints expose process details. Use debug mode only on a trusted
machine and remove retained debug logs afterward. [Configuration](configuration.md)
lists locations and retention.

## Data and cleanup

The default persistent root is `~/.kandev` (on Windows, `.kandev` below the user's profile directory). The SQLite database normally resides at `<home>/data/kandev.db`; its snapshots reside at `<home>/data/backups/`. With `KANDEV_DATABASE_PATH`, Kandev uses `backups/` beside the configured database file. Repositories, task worktrees, logs, and encrypted settings also live below the home root.

Runtime program files live in the Homebrew Cellar, the global npm dependency tree, or npm's `_npx` cache. They are not application data. `npm config get cache` and `npm root -g` show the latter two roots.

Uninstalling the package does not remove `<home>`. Before removing that directory, stop Kandev and any service, confirm no task or executor is running, and take a backup. See [Operations](operations.md) for safe database backup and restore procedures.

## Update

The installer owns CLI updates. Update persistent Stable installs with:

```bash
brew upgrade kandev
scoop update kandev
npm install -g kandev@latest
```

Use `npm install -g kandev@nightly` for a persistent npm Nightly install. `npx -y kandev@latest`
and `npx -y kandev@nightly` launch one-off copies from the requested channel; they do not update a
global package. A verified managed npm/npx user service can also select **Nightly** under
**Settings > System > Updates**. Stable is selected by default; Desktop, Homebrew, system-service,
unmanaged, local-checkout, and unknown installs cannot change this setting.

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

The launcher prints buffered backend output once when startup fails, followed by a bounded summary. The summary includes the effective bind addresses, every attempted health target and its last safe outcome, the selected configuration file, the effective `server.host` source, the backend log path, one next step, and a link to this troubleshooting guide. It never prints the launcher's health token or sensitive configuration values.

Use the failure class to choose the next action:

| Failure class | Meaning and recovery |
|---|---|
| Early backend exit | The child stopped before readiness. Read the named backend log for the startup error. |
| Unreachable backend | No target accepted a connection. Check the effective binds, firewall rules, and environment overrides. |
| Unhealthy HTTP response | A target answered with a non-success status. Inspect that status and the backend log. |
| Different process | A selected port answered without the launcher's token. Free the port or choose another backend port. |

If the backend is still running when readiness expires, the summary says that the launcher stopped it after readiness failed. It does not describe that case as a backend crash. Common underlying causes include an invalid `config.yaml`, a database migration or permission error, an occupied explicit port, or a damaged runtime bundle.

ACP probe closure messages occur after an agent protocol probe and do not cause a launcher `/health` timeout. Likewise, an `X-Forwarded-Host` warning means that a reverse proxy reached a running backend without trusted-proxy configuration; it is not a launcher readiness failure. Configure the immediate proxy peer under `server.trustedProxies` as described in [Configuration](./configuration.md).

### Browser does not open

Open the printed URL manually. Linux requires a working `xdg-open`; WSL often needs manual browser launch. `--headless` and `KANDEV_NO_BROWSER=1` intentionally skip the opener.

### Configuration appears ignored

The public CLI has no `--config` option. Confirm the first existing candidate
in the working-directory, home, and `/etc/kandev/` order, and remember that
environment values override YAML. For predictable installed/service
deployments, verify the active home and database paths printed at startup.
See [Configuration](configuration.md).
