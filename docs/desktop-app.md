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

Public recommended macOS artifacts must be Developer ID signed and notarized. Public recommended Windows artifacts must be code signed. Linux artifacts are checksum-gated; package-manager signatures can be added later.

Unsigned macOS or Windows desktop artifacts are for internal validation only. They may require manual OS security bypasses and should not be presented as trusted public downloads.

## Runtime Requirements

The desktop app packages the native Kandev runtime binaries and starts:

```text
kandev --headless --port <local-port>
```

The backend binds to `127.0.0.1` and serves the same embedded Vite UI used by the CLI/Homebrew/npm runtime.

Platform WebView requirements:

- macOS: system WebKit.
- Windows: Microsoft WebView2 runtime.
- Linux: WebKitGTK and package dependencies installed by the `.deb`/`.rpm` package.

Kandev data still lives in the existing Kandev home directory, `~/.kandev` by default, unless overridden by existing environment/config settings.

## Updates

The first desktop release does not include an in-app Tauri updater. Update by downloading and installing a newer desktop artifact from GitHub Releases. Existing worktrees, settings, tasks, and the SQLite database remain in the Kandev data directory.

The npm and Homebrew channels continue to update through:

```bash
brew upgrade kandev
npm install -g kandev@latest
npx kandev@latest
```

Those channels are separate from desktop installers, even though all release artifacts share the same SemVer.

## Agent CLI Discovery

Desktop apps launched from an OS app launcher may not inherit the same shell initialization as a terminal. Kandev preserves available environment variables and adds common user binary directories such as `/usr/local/bin`, `/opt/homebrew/bin`, `/usr/bin`, `/bin`, `~/.local/bin`, and common agent install directories.

If an agent CLI is not discoverable from the desktop app:

- Set the agent profile command to the full executable path.
- Confirm the executable works from a normal terminal.
- Put custom install directories in a location visible to GUI apps, or configure them through OS-level environment settings rather than only shell startup files.

Existing explicit command paths in Kandev settings remain authoritative over `PATH` lookup.

## Release Trust

See [desktop-tauri-signing.md](desktop-tauri-signing.md) for the CI secrets and signing expectations used for trusted desktop releases.
