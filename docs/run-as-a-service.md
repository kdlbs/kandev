# Run Kandev as a Service

Install Kandev as an OS-managed service (systemd on Linux, launchd on macOS) so it auto-starts and stays running. Updates remain manual: `npm i -g kandev@latest` or `brew upgrade kandev`, then re-run `kandev service install`.

This guide assumes you've already installed kandev via [Homebrew or npm](../apps/cli/README.md#quick-start) and that `kandev` works when run interactively.

> **Windows:** not yet supported. Track [#TODO](https://github.com/kdlbs/kandev/issues) for SCM support.

## Quick Start

```bash
# Laptop / single-user — runs as you, starts when you log in:
kandev service install

# Linux VPS / shared box — runs at boot, no login required:
sudo kandev service install --system
```

After install, kandev is reachable at `http://localhost:38429` (or `--port <N>` if you passed it).

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

```
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

Both `npm` and Homebrew install kandev under a versioned directory (e.g. `node_modules/kandev/0.49.0/`, `Cellar/kandev/0.49.0/`). When you upgrade, the old version directory is removed and replaced — but the unit file you installed earlier still hard-codes the old absolute path.

**Fix:** re-run install. It's idempotent.

```bash
npm i -g kandev@latest          # or: brew upgrade kandev
kandev service install          # rewrites the unit with the new paths
```

If you launch `kandev` interactively after an upgrade, it will detect a stale unit and print a one-line reminder. You can also check with `kandev service config` — the `cli entry:` and `node path:` lines tell you the paths that *would* be baked in by the next install.

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

The unit hard-codes absolute paths so it works in the empty `PATH` that systemd/launchd give a fresh service. A typical Linux user unit looks like:

```ini
# managed by kandev — regenerated by `kandev service install`
[Unit]
Description=Kandev autonomous agent platform
Documentation=https://github.com/kdlbs/kandev
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/node /usr/local/lib/node_modules/kandev/bin/cli.js --headless
Environment=KANDEV_HOME_DIR=/home/alice/.kandev
Environment=KANDEV_LOG_LEVEL=info
Environment=PATH=/usr/local/bin:/usr/bin:/bin:/opt/homebrew/bin:/home/linuxbrew/.linuxbrew/bin
Restart=on-failure
RestartSec=5s
KillMode=mixed
TimeoutStopSec=30s

[Install]
WantedBy=default.target
```

The `--headless` flag tells the CLI not to open a browser (you'll connect to it remotely or via `localhost`).

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

The OS service manager keeps running whatever `ExecStart` it has — it doesn't know about npm/brew upgrades. **Always re-run `kandev service install` after an upgrade** so the unit picks up the new paths, then `kandev service restart` to pick up new code.

## Updating: TL;DR

```bash
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
