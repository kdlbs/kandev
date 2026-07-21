---
title: "Desktop App"
description: "Install and run the Tauri desktop app with the native Kandev runtime."
---

# Kandev Desktop App

Kandev Desktop packages the native Kandev runtime inside a Tauri application and displays the normal web UI in the operating system WebView. Choose it for a native launcher, application menus, window-state persistence, desktop notifications, and signed in-app updates. It does not require Node.js, pnpm, Go, or Rust at runtime.

The [CLI](./cli.md) is a better fit for headless machines, browser-only access, or an OS service. Desktop and CLI releases use the same SemVer and the same default Kandev data layout, but they are separate installation and update channels.

## Supported artifacts

Download artifacts from [GitHub Releases](https://github.com/kdlbs/kandev/releases). Each desktop filename begins with `kandev-desktop-<platform>-`; use the installer format for your platform, not an updater archive.

| Platform label | Installer formats | In-app update support |
|---|---|---|
| `macos-arm64` | `.dmg` | Signed `.app.tar.gz` updater bundle |
| `macos-x64` | `.dmg` | Signed `.app.tar.gz` updater bundle |
| `linux-arm64` | AppImage, `.deb`, `.rpm` | AppImage installations only |
| `linux-x64` | AppImage, `.deb`, `.rpm` | AppImage installations only |
| `windows-x64` | NSIS `.exe` | Signed `.nsis.zip` updater bundle |

Windows ARM64 is not a native release target. x64 emulation may run the installer on some ARM Windows systems, but that is dependency-bound and not covered by the release matrix.

The `.app.tar.gz`, `.AppImage.tar.gz`, and `.nsis.zip` files are updater payloads; do not install them manually. Every published desktop artifact also has a sibling `.sha256` file.

## Verify a download

Run checksum verification from the directory containing both files. On macOS or Linux:

```bash
shasum -a 256 -c '<artifact-name>.sha256'
```

On Linux, `sha256sum -c '<artifact-name>.sha256'` is equivalent. On Windows PowerShell:

```powershell
$artifact = '.\<artifact-name>.exe'
$expected = (Get-Content "$artifact.sha256").Split()[0].ToLowerInvariant()
$actual = (Get-FileHash $artifact -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actual -ne $expected) { throw 'SHA-256 checksum mismatch' }
```

A checksum detects a damaged or substituted file only when you trust the release page from which the checksum was obtained. macOS Developer ID signing/notarization and Windows Authenticode signing are publisher-identity layers separate from the Tauri updater signature. Release automation can publish unsigned development installers when platform signing credentials are unavailable; the release notes identify that condition. Do not bypass an OS trust warning unless you have independently verified and accepted that unsigned build.

## Install

### macOS

1. Select the `macos-arm64` DMG for Apple silicon or `macos-x64` for an Intel Mac.
2. Verify the checksum.
3. Open the DMG and drag Kandev to **Applications**.
4. Start Kandev from Applications.

The app uses the system WebKit runtime. A properly signed release should pass Gatekeeper and notarization checks. To uninstall, quit Kandev and move **Kandev.app** from Applications to the Trash.

### Windows

1. Download the `windows-x64` NSIS `.exe` and its checksum.
2. Verify the checksum in PowerShell.
3. Run the installer and start Kandev from the Start menu.

Kandev uses Microsoft WebView2. Current Windows installations normally include it; if the window cannot create a WebView, install or repair the Microsoft WebView2 Runtime and retry. Uninstall through **Settings → Apps → Installed apps → Kandev → Uninstall**.

### Linux AppImage

AppImage is portable and is the only Linux installation type the in-app updater handles:

```bash
chmod +x './<downloaded-kandev.AppImage>'
'./<downloaded-kandev.AppImage>'
```

The host must provide the WebKitGTK and desktop libraries required by Tauri for that distribution. Desktop integration of a raw AppImage is distribution/tooling dependent.

### Linux package manager

Use `.deb` on Debian/Ubuntu-family systems or `.rpm` on RPM-family systems. Installing from a local file lets the package manager resolve declared dependencies:

```bash
sudo apt install './<downloaded-kandev.deb>'
```

```bash
sudo dnf install './<downloaded-kandev.rpm>'
```

Install a newer package the same way to update. Package-manager installs do not use Kandev's in-app updater; removal and package ownership remain with `apt`/`dnf`.

## Startup and local security

At launch, the desktop shell validates all packaged runtime binaries, selects a port, and starts:

```text
kandev --headless --port <desktop-port>
```

Desktop forces the backend to `127.0.0.1`, supplies a random 256-bit health token, and accepts readiness only when the loopback health response returns that token. The WebView's privileged desktop commands are also limited to the exact owned backend origin. This isolates the desktop control bridge from unrelated local pages; it is not a reason to expose another Kandev process to the network.

The preferred desktop port is `38430`. If only that port is unavailable, Kandev asks the OS for a free loopback port. `KANDEV_DESKTOP_PORT` can request a different port, but a non-UTF-8 value, a non-integer, `0`, or a value above `65535` is a fatal startup configuration error. If the requested port is occupied, the desktop still falls back to an OS-assigned loopback port.

Startup waits up to 60 seconds for the owned backend. A missing packaged binary, invalid port, configuration/database error, or early backend exit appears on the startup screen with the most recent captured backend output. Reinstall the complete artifact if runtime validation reports a missing `kandev`, `agentctl`, or remote helper.

## Data, processes, and cleanup

Desktop inherits normal backend configuration. By default, persistent data lives in `~/.kandev` (below the user profile directory on Windows). Set `KANDEV_HOME_DIR` in the desktop process environment before launch to isolate or relocate it; see [Configuration](./configuration.md). The desktop app's own platform app-data directory separately stores window geometry.

Only one desktop instance runs per OS user/application scope. Launching the app a second time shows, unminimizes, and focuses the existing main window; it does not start a second backend.

Closing the main OS window or choosing **Quit Kandev** quits the application and stops the backend it owns. There is no tray/background mode or desktop autostart service. On Unix, shutdown sends a graceful termination and force-kills after five seconds if needed; on Windows it terminates the owned process tree. Active external executor resources may have their own lifecycle—check [Executors](./executors.md) before manual cleanup.

The application menu exposes New Task, Settings (`Cmd/Ctrl+,`), contextual Close (`Cmd/Ctrl+W`), zoom, full-screen, Help, update, and Quit actions. Contextual Close asks the web UI to close its top dialog or eligible file/diff/commit/preview tab; if nothing is closeable it does not shut down the window or backend. The desktop shell saves window geometry in its platform app-data directory and clamps restored geometry to an available display.

Uninstalling the desktop application does not delete the Kandev home. Keep it to preserve workspaces and settings, or back it up and remove it separately only after all Kandev processes and executors are stopped. See [Operations](./operations.md).

## Agent CLI discovery

Applications started from Finder, Launchpad, desktop menus, or the Windows Start menu often receive less `PATH` configuration than terminal shells. Kandev preserves the available path and appends common locations:

- macOS/Linux: `/usr/local/bin`, `/usr/bin`, `/bin`, `/opt/homebrew/bin`, `~/.local/bin`, `~/.bun/bin`, and `~/.opencode/bin` (plus the Linuxbrew prefix on Linux);
- Windows: the user's roaming npm directory and Scoop shims.

If an agent command is still not found:

1. run the executable successfully in a normal terminal;
2. set the agent profile command to its full executable path; and
3. keep credentials and custom environment variables in the profile or an OS-level environment source visible to GUI applications, not only an interactive shell startup file.

Explicit profile command paths remain authoritative over `PATH` lookup.

## Updates

After the backend is ready, Kandev performs one update check immediately and then no more than once every 24 hours while that same process remains open. Use **Settings → System → Updates** or **Check for Updates…** in the application menu for a fresh check.

Kandev never downloads, installs, or restarts for an update without confirmation. It shows the target version and release notes, downloads the platform-specific payload after approval, verifies the Tauri signature against the public key embedded in the app, stops its backend, installs, and restarts. Only one check/install operation can run at a time, and downgrade payloads are rejected.

In-app updates are available only when release automation published a complete signed updater set and `latest.json`. If updater signing credentials were absent, normal installers can still exist while the update feed is intentionally omitted. Offline/feed failures do not stop Kandev; a manual check displays the error.

Linux `.deb` and `.rpm` installations must be updated through their package manager or by installing a newer package. The updater validates only an AppImage execution environment on Linux. If an update fails, keep the existing app closed only as long as the OS installer requires, then install the matching release artifact manually; the Kandev home is not part of the installer payload.

## Links, activation, and notifications

Kandev has no registered public URL scheme, file association, or command-line deep-link protocol. A second application launch only activates the existing window. Links and routes inside the Kandev UI still work normally, but an external `kandev://…` link is not a supported product path.

External `http`, `https`, and `mailto` destinations open through the system browser or mail client. The desktop bridge rejects URLs with embedded credentials, unsupported schemes, `localhost`/subdomains of `localhost`, loopback IPs, and unspecified IPs; Kandev routes, previews, downloads, and blob URLs remain in the WebView. RFC 1918/private-LAN hosts are not categorically blocked by this validator.

Native notifications are limited to a session waiting for input or failing, and respect the application's notification preference and OS permission. Requests carry task/session identifiers for validation and event de-duplication, but the desktop registers no notification activation/deep-link handler. Clicking a notification is therefore not a supported navigation path.

## Troubleshooting

### Desktop startup failed

Read the captured backend detail on the startup screen. Then check, in order:

- the artifact matches the current OS and architecture;
- the full application bundle was installed rather than copying a binary out of it;
- `KANDEV_DESKTOP_PORT` is valid or unset;
- `config.yaml` and `KANDEV_*` values are valid; and
- the configured Kandev home/database is writable and not already locked by an incompatible process.

Reinstalling repairs packaged program files but intentionally leaves user data untouched.

### Agent works in a terminal but not Desktop

Use the full command path in the agent profile and verify GUI-visible credentials. Shell aliases and functions are not executables and cannot be discovered by `PATH`.

### Update check reports no usable feed

Confirm network access to GitHub Releases and read that release's notes. An unsigned installer-only release deliberately has no `latest.json`; install a newer verified installer manually. Proxy/TLS interception can also prevent signature-feed access even when the application itself works offline.

### Linux AppImage does not open

Confirm its executable bit and launch it from a terminal to see missing-library output. Install the WebKitGTK/Tauri dependencies for the distribution, or use the provided `.deb`/`.rpm` so the package manager resolves dependencies.

## Release trust details

See [desktop-tauri-signing.md](../desktop-tauri-signing.md) for the release workflow's platform-signing inputs and fallback behavior. Tauri updater signatures, macOS notarization, Windows Authenticode, and published SHA-256 files serve different trust checks; one does not substitute for all the others.
