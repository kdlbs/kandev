---
title: "Run as a Service"
description: "Install Kandev under systemd or launchd and operate it safely."
---

# Run Kandev as a Service

The native Kandev launcher can install itself as a systemd service on Linux or a launchd service on macOS. Use this for a persistent workstation or server. Windows Service Control Manager, OpenRC, and SysV init are not supported.

Install Kandev first using a persistent [CLI installation](cli.md#install). Do not install a long-lived service from an ephemeral `npx` invocation: the generated unit records the absolute native executable and release-bundle paths.

> **Network security:** the backend listens on `0.0.0.0` by default, and the Kandev web, HTTP, and WebSocket endpoints do not provide an authentication boundary. Bind it to loopback and use an authenticated TLS reverse proxy or private VPN before allowing remote access. The `auth.jwtSecret` configuration field is not a web-login password. See [server configuration](configuration.md#root-and-server).

## Choose a service mode

| | User service (default) | System service (`--system`) |
| --- | --- | --- |
| Manager | `systemctl --user` or the user's launchd domain | system systemd or launchd domain |
| Privilege to install | Normal user | Root; normally invoke through `sudo` |
| Linux unit | `~/.config/systemd/user/kandev.service` | `/etc/systemd/system/kandev.service` |
| macOS plist | `~/Library/LaunchAgents/com.kdlbs.kandev.plist` | `/Library/LaunchDaemons/com.kdlbs.kandev.plist` |
| Default Kandev home | The installer process's `KANDEV_HOME_DIR` when set; otherwise `~/.kandev` | `/var/lib/kandev` |
| Process user | Current user | `$SUDO_USER` when installed through `sudo` from a non-root account; otherwise the current user |
| Best fit | Personal workstation; single-user Linux host with lingering enabled | Boot-time service independent of a login session |

On Linux, an enabled user service starts at boot only if that user's systemd manager runs at boot. Enable lingering once if that is the desired lifecycle:

```bash
sudo loginctl enable-linger "$USER"
```

Disable it later with `sudo loginctl disable-linger "$USER"`. A macOS LaunchAgent belongs to a logged-in GUI user; use a LaunchDaemon for a host-wide boot service.

## Install

### User service

```bash
kandev service install
kandev service status
kandev service logs
```

The installer writes the managed unit or plist and starts the process. It does **not** poll `/health`; use `status`, `logs`, and an explicit health request to verify startup:

```bash
curl --fail http://127.0.0.1:38429/health
```

`/health` reports backend readiness after routes and the agent registry are initialized and the HTTP listener is accepting connections. It is not a deep health check of the database, message bus, executors, or remote providers.

### System service

The native installer does not create or change ownership of the system service home. Create it for the account that will run the service before installing. The following invocation through `sudo` makes that account the invoking user:

```bash
KANDEV_BIN="$(command -v kandev)"
sudo install -d -o "$USER" -g "$(id -gn)" /var/lib/kandev /var/lib/kandev/logs
sudo "$KANDEV_BIN" service install --system
sudo "$KANDEV_BIN" service status --system
```

Apply the same ownership rule to a custom `--home-dir`. Root access is also required to write the system unit/plist and control the system service manager. If installation is run from a root login, rather than via `sudo` from the intended account, the service runs as root; prefer an explicit, non-root service account.

## Bind safely

The backend config loader searches `config.yaml` in its working directory and then `/etc/kandev`. launchd sets the working directory to the Kandev home, but the generated systemd unit does not. For a shared conventional path, create `/etc/kandev/config.yaml` before first start, or stop and restart after changing it:

```yaml
server:
  host: 127.0.0.1
```

The working-directory file wins when both exist, so on macOS merge or remove a conflicting `<KANDEV_HOME_DIR>/config.yaml` if `/etc/kandev/config.yaml` should be authoritative. The service unit has an intentionally small fixed environment and does not inherit arbitrary exports from the installing shell. Put supported settings in the configuration file; see [Configuration](configuration.md). A system service needs permission to read the file, while secret-bearing configuration should not be world-readable.

To access a loopback-only instance remotely, use SSH port forwarding:

```bash
ssh -L 38429:127.0.0.1:38429 user@server
```

Then open `http://127.0.0.1:38429` locally. For shared access, terminate TLS and enforce authentication in a reverse proxy or private access layer.

## Commands and flags

```text
kandev service install [--system] [--port <port>] [--home-dir <path>] [--no-boot-start]
kandev service uninstall [--system]
kandev service start|stop|restart|status [--system]
kandev service logs [-f] [--system]
kandev service config [--system]
```

- `--system` selects the system manager and paths shown above.
- `--home-dir <path>` records `KANDEV_HOME_DIR` in the unit. For a user service with no flag, the installer first honors its own `KANDEV_HOME_DIR` environment, then falls back to `~/.kandev`; system mode defaults to `/var/lib/kandev`.
- On Linux, `--no-boot-start` starts the service now without enabling the unit, but it does not disable a unit that was already enabled. Disable it explicitly when necessary. On macOS the generated plist has `RunAtLoad=false` but also `KeepAlive=true`; the installer stores it in launchd's normal discovery path, bootstraps and enables it, then kick-starts it. That combination does not provide a dependable “never start at login/boot” guarantee; manage future loading explicitly with launchd if that distinction matters.
- `-f` or `--follow` follows logs.
- `--port <port>` is accepted by the installer, but the current native launcher writes it as `KANDEV_SERVER_PORT` and then replaces that value during startup. Do not rely on this flag to choose the listener. Without an explicit launcher port, Kandev tries `38429` and chooses a free random port in `10000–60000` if needed; find the actual URL in the logs. This is a current implementation limitation.

`config` is diagnostic but deliberately small: it prints the OS manager, selected user/system mode, Kandev home, and unit/plist path. It does not prove that the service is installed or active and does not print all unit environment entries.

### Fixed-port operator workaround

Until the native installer port mismatch is fixed, a systemd drop-in can set the launcher variable it actually reads. Run `systemctl --user edit kandev.service` and add:

```ini
[Service]
Environment=KANDEV_BACKEND_PORT=3000
```

Then reload and restart:

```bash
systemctl --user daemon-reload
kandev service restart
```

For a system service, use `sudo systemctl edit kandev.service`, `sudo systemctl daemon-reload`, and `sudo "$(command -v kandev)" service restart --system`. This is an OS-level override, not a Kandev installer flag; audit the drop-in during upgrades. The current launchd installer has no equivalent managed fixed-port option. A hand-managed plist must set `KANDEV_BACKEND_PORT`, and `kandev service install` will rewrite that plist.

## What installation changes

On Linux, installation writes the unit, runs `daemon-reload`, and normally runs `enable --now`. With `--no-boot-start`, it runs `start` without changing enablement; any prior enabled state remains. The generated service:

- executes the absolute native `kandev --headless` path;
- records `KANDEV_HOME_DIR`, `KANDEV_LOG_LEVEL=info`, a service `PATH`, the release bundle, and package version;
- uses `Restart=on-failure`, a five-second restart delay, `KillMode=mixed`, and a 30-second stop timeout;
- orders startup after `network-online.target`.

On macOS, installation writes the plist, removes an already loaded job, bootstraps the new plist, and enables it. Standard output and error go to `<KANDEV_HOME_DIR>/logs/service.out` and `service.err`. The plist always has `KeepAlive=true`; `--no-boot-start` changes only `RunAtLoad` and adds an immediate kick-start.

If the target unit/plist already exists and lacks Kandev's managed marker, installation saves it as `<path>.bak` before replacing it. Review that backup; uninstall does not restore or remove it.

The service manager starts the control plane only. Agents still need their configured executor dependencies and credentials—for example Docker access for a Docker profile or network reachability and keys for SSH. See [Executors](executors.md).

## Operate and inspect

```bash
kandev service start
kandev service stop
kandev service restart
kandev service status
kandev service logs          # last 200 lines
kandev service logs --follow
```

Add `--system` to every command for a system service and invoke it with sufficient privilege. Linux log commands use journald. macOS log commands tail the two files under the Kandev home computed by that invocation, rather than reading the installed plist. If installation used a custom home, repeat `--home-dir <path>` (or the same `KANDEV_HOME_DIR`) with `service logs`; also repeat it with `service config` if that diagnostic should print the installed home.

Useful direct checks are:

```bash
# User systemd service
systemctl --user status kandev.service
journalctl --user-unit kandev.service -n 200 --no-pager

# System systemd service
sudo systemctl status kandev.service
sudo journalctl -u kandev.service -n 200 --no-pager
```

## Upgrade safely

The current native service implementation does not write the legacy install metadata used by the Settings update workflow. Consequently, **Settings → System → Updates cannot apply an update to this service**, including a user service.

Upgrade the package, reinstall with the same mode and home flags so absolute paths and bundle metadata are refreshed, then restart explicitly:

```bash
# npm example; use `brew upgrade kandev` for Homebrew
npm install --global kandev@latest
kandev service install --home-dir "$HOME/.kandev"
kandev service restart
kandev service status
```

For system mode:

```bash
KANDEV_BIN="$(command -v kandev)"
sudo "$KANDEV_BIN" service install --system --home-dir /var/lib/kandev
sudo "$KANDEV_BIN" service restart --system
sudo "$KANDEV_BIN" service status --system
```

Reinstalling an already active Linux unit performs `enable --now`, which does not guarantee a restart of that running process. The explicit `restart` is required to load the new executable. Back up persistent state before upgrades; see [SQLite backups](operations.md#sqlite-backups) or the PostgreSQL procedure under [Restore and recovery](operations.md#restore-and-recovery).

## Run a source checkout as a service

The repository Make targets build a release-style bundle under `dist/kandev` and install that snapshot. They do not run live source files.

```bash
make service-install
make service-status
make service-logs
```

Available user-service targets include `service-start`, `service-stop`, `service-restart`, `service-logs-follow`, `service-config`, and `service-uninstall`. `make service-install-system` installs the built checkout as a system service. `HOME_DIR=/path` and `NO_BOOT_START=1` pass their corresponding installer flags. `PORT=...` is exposed by the Makefile but has the native-launcher limitation described above.

After changing source or checking out another revision, rerun `make service-install`, then `make service-restart`. These targets are intended for development and still do not create service metadata for Settings-based update application.

## Uninstall and data cleanup

```bash
kandev service uninstall
# or
sudo "$(command -v kandev)" service uninstall --system
```

Uninstall stops/disables the job and removes the unit/plist. It leaves the Kandev home, database, repositories, logs, backups, and any `.bak` service definition intact. Remove those separately only after confirming the path and retaining any needed backup.

## Troubleshooting

### Service command is unsupported

`kandev service` supports only systemd on Linux and launchd on macOS. On Windows use a foreground process or a separately administered service wrapper; see [Windows support](windows-support.md). On a Linux distribution without systemd, write and own the init integration yourself.

### System service cannot create its database or logs

Check the process user in the unit and ownership of the full Kandev home path. The installer does not provision `/var/lib/kandev`:

```bash
sudo systemctl cat kandev.service
sudo namei -l /var/lib/kandev
```

Stop the service before correcting ownership, and grant access only to the intended service account.

### Port 38429 is not the logged URL

The native launcher selects another free port when `38429` is unavailable. Check `kandev service logs` (or its `--system` form), stop the conflicting process if appropriate, and restart. The current installer `--port` value does not reliably control this selection.

### User service disappears after logout or reboot

On Linux, enable lingering and ensure the unit is enabled. A Linux install with `--no-boot-start` does not enable the unit, although it preserves a prior enabled state:

```bash
sudo loginctl enable-linger "$USER"
systemctl --user enable kandev.service
```

On macOS, a LaunchAgent belongs to the user's login domain; choose system mode when the job must run without that user logged in.

### Service still runs the previous version

Reinstall using the upgraded `kandev` binary, preserve the original `--system` and `--home-dir` choices, and then run an explicit restart. Inspect `systemctl cat kandev.service` or the launchd plist if the recorded executable still points at an old package-manager path.

### Service starts but agents fail

Read service logs first. A service has a smaller `PATH` and no interactive shell environment, so tools or credentials visible in a terminal may be absent. Configure executor credentials through Kandev's profile/settings paths, use stable executable paths, and verify Docker/SSH/Sprites connectivity as described in [Executors](executors.md#troubleshooting).
