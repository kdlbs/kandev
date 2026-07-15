# Kandev Desktop App

Kandev desktop is a Tauri app that starts the native Kandev runtime locally and shows the existing Kandev UI in a system WebView. It does not require Node.js at runtime. Node.js, pnpm, Rust, and the Tauri CLI are build-time tools only.

## Install Artifacts

GitHub releases publish desktop artifacts alongside the existing runtime tarballs. Desktop artifact names start with `kandev-desktop-<platform>-` and each artifact has a `.sha256` checksum.

Supported desktop platforms:

- `macos-arm64`
- `macos-x64`
- `linux-x64`
- `linux-arm64`
- `windows-x64`

macOS releases provide a `.dmg`, Windows releases provide an NSIS installer, and Linux releases
provide AppImage, `.deb`, and `.rpm` packages. AppImage is the portable Linux format and the only
Linux format supported by Kandev's in-app updater. Install `.deb` or `.rpm` when you want your
system package manager to own installation and removal.

When signing inputs are configured, macOS artifacts are Developer ID signed and notarized and Windows artifacts are code signed. When those inputs are missing, the release still publishes unsigned desktop development builds with a release-notes warning. Linux artifacts are checksum-gated; package-manager signatures can be added later.

Unsigned macOS or Windows desktop artifacts may require manual OS security bypasses and should not be presented as trusted downloads.

## Runtime Requirements

The desktop app packages the native Kandev runtime binaries and starts:

```text
kandev --headless --port <local-port>
```

The backend binds to `127.0.0.1` and serves the same embedded Vite UI used by the CLI/Homebrew/npm runtime.

Platform WebView requirements:

- macOS: system WebKit.
- Windows: Microsoft WebView2 runtime.
- Linux: WebKitGTK and related desktop libraries. The `.deb`/`.rpm` packages install declared
  dependencies; AppImage users must provide the host WebKitGTK/runtime libraries required by
  their distribution.

Kandev data still lives in the existing Kandev home directory, `~/.kandev` by default, unless overridden by existing environment/config settings.

## Desktop Updates

The desktop app checks the signed GitHub Releases update feed after startup and no more than once
every 24 hours while it remains open. It never downloads, installs, or restarts silently. Open
**Settings -> System -> Updates** or choose **Check for Updates** from the application menu to run
a fresh check, review the available version and release notes, and confirm installation.

Kandev verifies the Tauri updater signature before installing on every platform. Apple
notarization and Windows code signing are separate publisher-identity layers: when those
credentials are unavailable, Tauri-signed updates still work but the application remains an
unsigned development build and can trigger OS trust warnings. Linux in-app updates install the
signed AppImage update bundle. Before the updater restarts Kandev, the desktop shell stops the
backend process it owns. Worktrees, settings, tasks, and the SQLite database remain in the Kandev
data directory.

Linux `.deb` and `.rpm` installs do not self-update. Update those through your package manager or
by installing a newer package manually. When an automatic check is offline or the release feed is
unavailable, Kandev keeps running and tries again later; a manual check shows the error on the
Updates page. If installation fails, reopen that page and retry or install the matching release
artifact manually. Kandev rejects payloads with missing or invalid Tauri updater signatures and
rejects downgrades.

The npm and Homebrew channels continue to update through:

```bash
brew upgrade kandev
npm install -g kandev@latest
npx kandev@latest
```

Those channels are separate from desktop installers, even though all release artifacts share the same SemVer.

## Native Desktop Behavior

The desktop application menu provides New Task, contextual Close, Settings, zoom, full screen,
Help, and update commands. On macOS, `Cmd+,` opens the existing General settings page. `Cmd/Ctrl+W`
closes the top dialog or an eligible file/diff/commit/preview tab; when there is no closeable
context it does nothing and never shuts down the window or backend. Closing the main window with
the OS close control or choosing Quit exits Kandev and stops the owned backend.

Kandev can send native notifications when a task is waiting for input or a session fails. These
respect the existing notification preference. Desktop notification clicks may focus Kandev, but
Kandev does not infer task navigation from an ordinary focus or app activation event.

External web and email links open in the system browser or mail client. Kandev keeps its own
loopback routes, local previews, downloads, and blob URLs in the WebView.

## Agent CLI Discovery

Desktop apps launched from an OS app launcher may not inherit the same shell initialization as a terminal. Kandev preserves available environment variables and adds common user binary directories such as `/usr/local/bin`, `/opt/homebrew/bin`, `/usr/bin`, `/bin`, `~/.local/bin`, and common agent install directories.

If an agent CLI is not discoverable from the desktop app:

- Set the agent profile command to the full executable path.
- Confirm the executable works from a normal terminal.
- Put custom install directories in a location visible to GUI apps, or configure them through OS-level environment settings rather than only shell startup files.

Existing explicit command paths in Kandev settings remain authoritative over `PATH` lookup.

## Release Trust

See [desktop-tauri-signing.md](../desktop-tauri-signing.md) for the CI secrets and automatic signing behavior used for desktop releases.
