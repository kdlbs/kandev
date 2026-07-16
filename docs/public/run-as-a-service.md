---
title: "Run as a Service"
description: "Run Kandev as a persistent service on your own machine or infrastructure."
---

# Run Kandev as a Service

Install Kandev as an OS-managed service (systemd on Linux, launchd on macOS) so it auto-starts and stays running. User-mode services installed by `kandev service install` can self-update from the System → Updates page. Non-service installs and `--system` services still update manually: `npm i -g kandev@latest` or `brew upgrade kandev`, then re-run `kandev service install`.

This guide assumes you've already installed kandev via [Homebrew or npm](cli.md#installation) and that `kandev` works when run interactively.

> **Windows:** not yet supported. See [open issues mentioning Windows](https://github.com/kdlbs/kandev/issues?q=is%3Aissue+windows) for SCM support progress, or open a new one if there isn't one yet.

## Quick Start

```bash
# Laptop / single-user — runs as you, starts when you log in:
kandev service install

# Linux VPS / shared box — runs at boot, no login required:
sudo kandev service install --system
```

After install, kandev is reachable at `http://localhost:38429` (or `--port <N>` if you passed it).

## Run the Current Checkout as a Service

When developing from a cloned repo, use the root Make targets instead of the globally installed `kandev` binary. They install dependencies, build the currently checked-out branch, assemble a local release-style bundle under `dist/kandev`, and install the service with `KANDEV_BUNDLE_DIR` pointing at that bundle.

```bash
git checkout my-branch
make service-install
```

Useful targets:

```bash
make service-install          # user service from the current checkout
make service-install PORT=3000
make service-install HOME_DIR=/path/to/kandev-home
make service-install NO_BOOT_START=1
make service-install-system   # system service install; other targets below use the user service
make service-status
make service-logs
make service-logs-follow
make service-start
make service-stop
make service-restart
make service-uninstall
make service-config
```

The service runs the built snapshot in `dist/kandev`, not live source files. After switching branches or changing code, rerun `make service-install` to rebuild and refresh the service unit. Checkout-based services are marked as a local install kind, so the System → Updates page will not offer one-click npm/Homebrew self-update; update by rebuilding from the desired branch.

## User Mode vs `--system` Mode

|                                  | **User mode** (default)                                                                            | **`--system` mode**                                                                       |
| -------------------------------- | -------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| Install requires sudo            | No                                                                                                 | Yes                                                                                       |
| Unit location (Linux)            | `~/.config/systemd/user/kandev.service`                                                            | `/etc/systemd/system/kandev.service`                                                      |
| Unit location (macOS)            | `~/Library/LaunchAgents/com.kdlbs.kandev.plist`                                                    | `/Library/LaunchDaemons/com.kdlbs.kandev.plist`                                           |
| Daemon runs as                   | You                                                                                                | `$SUDO_USER` if invoked via `sudo`, else the current user                                 |
| Auto-starts on reboot            | **Linux:** only after `sudo loginctl enable-linger $USER` (run once). **macOS:** only at next login. | Always, regardless of login state                                                         |
| Survives logout / SSH disconnect | **Linux:** yes, after linger. **macOS:** no (use `--system` instead).                              | Yes                                                                                       |
| Default `KANDEV_HOME_DIR`        | `~/.kandev`                                                                                        | `/var/lib/kandev`                                                                         |
| Logs                             | `journalctl --user-unit kandev` (Linux) · `~/.kandev/logs/service.err` (macOS)                     | `sudo journalctl -u kandev` (Linux) · `/var/lib/kandev/logs/service.err` (macOS)          |
| Best for                         | Laptop, workstation                                                                                | Headless Linux VPS, Mac mini server, shared box                                           |

**30-second rule of thumb: VPS → `--system`. Laptop → default.**

Linux user-mode with `loginctl enable-linger` is functionally equivalent to system mode for a single-user VPS, but the linger one-liner itself requires sudo — so you're not avoiding sudo, just deferring it. For a VPS, `--system` is one less thing to remember after a reboot.

## Commands

```bash
kandev service install [--system] [--port <port>] [--home-dir <path>] [--no-boot-start]
kandev service uninstall [--system]
kandev service start|stop|restart|status [--system]
kandev service logs [-f] [--system]
kandev service config [--system]
```

| Command                       | What it does                                                                                                                                                              |
| ----------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `install`                     | Writes the unit file, reloads the service manager, enables auto-start, and starts the service. Then polls `/health` for up to 30s and dumps logs if it doesn't come up.   |
| `uninstall`                   | Stops the service, disables auto-start, removes the unit file.                                                                                                            |
| `start` / `stop` / `restart`  | Control the running service without touching the unit file.                                                                                                               |
| `status`                      | Print the OS service-manager view (systemd / launchctl).                                                                                                                  |
| `logs [-f]`                   | Dump the last 200 lines, or stream if `-f`. journalctl on Linux; `tail` on macOS log files.                                                                               |
| `config`                      | Print the resolved paths, env vars, and whether the service is currently installed / active. Useful for diagnosis — read-only, no privileges needed.                      |

### Flags

- `--system` — system-level install. Requires sudo. See the comparison above.
- `--port <N>` — bake `KANDEV_SERVER_PORT=<N>` into the unit. Defaults to 38429.
- `--home-dir <PATH>` — bake `KANDEV_HOME_DIR=<PATH>` into the unit. Defaults to `~/.kandev` (user mode) or `/var/lib/kandev` (`--system`).
- `--no-boot-start` — Linux user-mode only. Skip the `loginctl enable-linger` hint at the end of install.
- `-f`, `--follow` — only for `logs`. Stream rather than dump.

## After an Upgrade

If Kandev is running as a user-mode service installed by `kandev service install`, open **Settings → System → Updates** and use **Apply update** when it appears. The button is shown only when the backend can prove it is running from a kandev-managed service unit/plist with valid service metadata.

Both npm runtime packages and Homebrew install kandev under versioned package-manager paths. Manual upgrades replace those paths, so the unit file must be refreshed afterward.

**Manual fix:** re-run install. It's idempotent.

```bash
npm i -g kandev@latest          # or: brew upgrade kandev
kandev service install          # rewrites the unit with the new paths
```

If you launch `kandev` interactively after an upgrade, it will detect a stale unit and print a one-line reminder. You can also check with `kandev service config` to see the paths that would be baked in by the next install.

System services (`kandev service install --system`) do not expose UI self-update. Update them from a privileged shell, then re-run `sudo kandev service install --system`.

## Linux Boot-Start (`loginctl enable-linger`)

Linux user services normally run only while you're logged in. To keep kandev running across reboots without an active SSH session, run **once**:

```bash
sudo loginctl enable-linger $USER
```

After this, your user's systemd instance is started at boot by `systemd-logind`, and your enabled user units (including kandev) start with it. To disable later:

```bash
sudo loginctl disable-linger $USER
```

If you'd rather not deal with linger and you're already comfortable with sudo, install with `--system` instead — it sidesteps the issue entirely.

## What's Inside the Unit File

The unit hard-codes absolute paths so it works in the empty `PATH` that systemd/launchd give a fresh service. When Node tooling is installed through a per-user manager such as nvm, fnm, asdf, Volta, or mise, `kandev service install` also bakes the detected `node`/`npm`/`npx` bin directory into `PATH`; re-run the install command after changing the active Node version. For fnm, the installer resolves multishell symlinks when possible so the unit records the stable Node installation path instead of the temporary multishell directory. If the service file still points at a stale fnm multishell path, re-run `kandev service install` from a shell where `node`, `npm`, and `npx` resolve through valid symlinks. For `--system` installs, some `sudo` configurations reset `PATH` with `secure_path`; run the install command from an environment where `node`, `npm`, and `npx` still resolve, or put stable symlinks in the run-as user's `~/.local/bin`.

A typical Linux user unit looks like:

```ini
# managed by kandev — regenerated by `kandev service install`
[Unit]
Description=Kandev autonomous agent platform
Documentation=https://github.com/kdlbs/kandev
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/lib/node_modules/@kdlbs/runtime-linux-x64/bin/kandev --headless
Environment=KANDEV_HOME_DIR=/home/alice/.kandev
Environment=KANDEV_LOG_LEVEL=info
Environment=PATH=%h/.local/bin:%h/.bun/bin:%h/.opencode/bin:/usr/local/bin:/usr/bin:/bin:/opt/homebrew/bin:/home/linuxbrew/.linuxbrew/bin
Environment=KANDEV_RUNNING_AS_SERVICE=true
Environment=KANDEV_SERVICE_MODE=user
Environment=KANDEV_SERVICE_MANAGER=systemd
Environment=KANDEV_INSTALL_KIND=npm
Environment=KANDEV_SERVICE_METADATA=/home/alice/.kandev/service/install.json
Restart=on-failure
RestartSec=5s
KillMode=mixed
TimeoutStopSec=30s

[Install]
WantedBy=default.target
```

The `--headless` flag tells the CLI not to open a browser (you'll connect to it remotely or via `localhost`). The `KANDEV_SERVICE_*` variables and `<home>/service/install.json` metadata let the backend verify that UI self-update is safe before it shows the Apply button.

## Troubleshooting

### `systemctl: command not found` / `launchctl: command not found`

Kandev's service support requires either systemd (most Linux distros) or launchd (macOS). It does not currently support OpenRC, SysV init, or Windows SCM. You can still run kandev as a daemonized process with `nohup` / `screen` / `tmux`, or wrap it in your init system of choice using the launcher info from `kandev service config`.

### "service did not become healthy within 30s"

The install succeeded (unit file written, service told to start) but kandev's HTTP `/health` endpoint never responded. `install` dumps the last 50 lines of logs when this happens — common causes:

- Port already in use → pass `--port <other>`.
- Cold-disk + slow first launch → re-run `kandev service install`; the second start is usually fast enough.
- Missing dependency on the unit's `PATH` (e.g. `git`, `docker`) → install the missing tool and `kandev service restart`.

### The unit warns about a "file that doesn't look like a kandev-managed file"

`kandev service install` refuses to silently clobber files it didn't write. If you (or another tool) had previously put something at `~/.config/systemd/user/kandev.service` or the equivalent on macOS, install will:

1. Copy the existing file to `<path>.bak`
2. Write the kandev unit in its place
3. Print a `WARNING` line so you notice

Inspect the `.bak` if you're not sure what was there.

### Service runs as `root` when you wanted it to run as your user

This happens with `--system` if `SUDO_USER` isn't set (e.g. you logged in as root directly rather than `sudo`'ing). Either run install via `sudo` from your normal user, or hand-edit the `User=` (Linux) / `UserName=` (macOS) directive in the unit file.

### After upgrading, the service silently keeps running the old version

The OS service manager keeps running whatever `ExecStart` it has — it doesn't know about npm/brew upgrades. For managed user services, use **Apply update** on the Updates page. Otherwise, **always re-run `kandev service install` after an upgrade** so the unit picks up the new paths, then `kandev service restart` to pick up new code.

## Updating: TL;DR

```bash
# Option A: managed user service
# Use Settings -> System -> Updates -> Apply update

# Option B: manual / system service
# 1. Update the binary
npm i -g kandev@latest          # or: brew upgrade kandev

# 2. Refresh the unit file (rewrites paths to point at the new version)
kandev service install          # add --system if that's what you used originally

# 3. (install does this automatically) Verify it came back up
kandev service status
```

## Uninstalling

```bash
kandev service uninstall          # or: sudo kandev service uninstall --system
```

This stops the service, removes its unit/plist, and reloads the service manager. Data in `~/.kandev` (or `/var/lib/kandev`) is left intact — delete it manually if you want a clean slate.
