---
status: implemented
created: 2026-06-23
owner: tbd
---

# Tauri Desktop App

## Why

Users who prefer a desktop workflow should be able to install and open Kandev from the OS app launcher without running a terminal command or managing a browser tab. The installed desktop app should keep Kandev's current local-first behavior while avoiding a production Node.js runtime.

## What

- Kandev provides desktop installers for the same primary release platforms as the native runtime bundle: macOS arm64, macOS x64, Linux x64, Linux arm64, and Windows x64.
- Opening the desktop app starts a local Kandev backend and displays the existing Vite/React Kandev UI inside a native Tauri WebView window.
- The installed desktop app does not require Node.js. Node.js and pnpm remain build-time tooling in CI and local development only.
- The desktop app reuses the existing Go-served SPA with embedded Vite assets. It does not ship or start a separate web server.
- The desktop app reuses the existing Kandev data directory, SQLite database, worktrees, executor settings, integrations, and agent configuration.
- The desktop app owns the backend lifecycle for the process it starts: startup waits for `/health`, normal quit terminates child processes, and startup failure surfaces an error instead of leaving orphaned backend processes.
- The desktop app starts the backend on a loopback-only address and an available local port unless the user has explicitly configured otherwise through existing environment/config mechanisms.
- The desktop app launches the backend with a predictable process environment for GUI launches, including common user binary locations, so configured agent CLIs can be discovered as reliably as they are from terminal-launched Kandev.
- Opening a second desktop instance focuses the existing app window or exits without starting a second backend for the same desktop launch context.
- GitHub releases include desktop artifacts alongside the existing runtime tarballs and checksums.
- Unsigned desktop artifacts may be produced for development or internal validation, but public recommended desktop releases require macOS Developer ID signing/notarization and Windows code signing before they are presented as trusted user-facing downloads.
- Desktop release notes and install docs tell users which artifact to download for each operating system and call out any unsigned/pre-release limitations.

## Data model

The feature does not add backend database tables.

Desktop runtime state is process-local except for optional window placement/state stored by Tauri under the OS application data location. Kandev user data remains owned by the existing backend persistence model under `KANDEV_HOME_DIR` or the default `~/.kandev` location.

## API surface

### Desktop launch contract

The Tauri shell starts the packaged native Kandev launcher in headless mode with an explicit backend port:

```text
kandev --headless --port <available-loopback-port>
```

The launcher starts the backend through the existing hidden backend mode and keeps the supervisor process alive until the desktop app exits. The desktop shell polls:

```text
GET http://127.0.0.1:<port>/health
```

Desktop launches pass a per-launch `KANDEV_DESKTOP_HEALTH_TOKEN` to the native launcher
environment. The backend echoes the token in the `X-Kandev-Desktop-Health-Token` response
header on successful `/health` responses. The desktop shell treats `/health` as ready only
when the status is successful and the response header matches its generated token, so a
separate local process on the same loopback port cannot satisfy desktop readiness.

When `/health` reports ready for that launch, the desktop window navigates to:

```text
http://127.0.0.1:<port>/
```

### Backend bind contract

Desktop launches set the backend host to loopback:

```text
KANDEV_SERVER_HOST=127.0.0.1
```

The backend server honors `server.host` / `KANDEV_SERVER_HOST` when binding its listener. Existing CLI and service launches may keep their current default unless explicitly changed by their own specs.

### Desktop environment contract

The desktop shell passes an explicit environment to the native launcher:

- Existing user environment variables are preserved when available.
- `KANDEV_SERVER_HOST=127.0.0.1` is set for desktop launches.
- `PATH` includes common GUI-missing user binary locations such as `/usr/local/bin`, `/opt/homebrew/bin`, `/usr/bin`, `/bin`, `%USERPROFILE%/.local/bin`, and existing Kandev agent CLI install locations where applicable.
- Explicit agent command paths configured in Kandev settings remain authoritative over `PATH` lookup.

### Desktop artifact contract

Each released version includes platform desktop artifacts with checksums. Exact file names may follow Tauri's generated naming, but the release must expose these platform groups:

- macOS arm64: signed/notarized `.dmg` for public release, unsigned `.dmg` allowed only for internal validation.
- macOS x64: signed/notarized `.dmg` for public release, unsigned `.dmg` allowed only for internal validation.
- Windows x64: signed installer (`.exe` or `.msi`) for public release, unsigned installer allowed only for internal validation.
- Linux x64: AppImage and/or distro package generated by Tauri, with checksum.
- Linux arm64: AppImage and/or distro package generated by Tauri, with checksum.

### Existing backend API

The desktop app uses the existing backend HTTP, WebSocket, and boot-payload contracts. No new product API is required for the main Kandev UI.

## State machine

Desktop app lifecycle:

- `idle`: the app process has started but no backend child is running.
- `backend-starting`: the app has spawned the native launcher and is polling backend health.
- `ready`: the backend is healthy and the WebView is displaying the Kandev UI.
- `backend-restarting`: the launcher supervisor is replacing the backend child after an existing restart request.
- `stopping`: the user quit the app or the app is terminating its child processes.
- `failed`: startup failed before the UI became ready.

Transitions:

- `idle` -> `backend-starting`: user opens the desktop app.
- `backend-starting` -> `ready`: `/health` returns success and the WebView navigates to the backend URL.
- `backend-starting` -> `failed`: the launcher exits, a required bundled binary is missing, the backend cannot bind, or health times out.
- `ready` -> `backend-restarting`: the existing backend restart supervisor handles a restart request.
- `backend-restarting` -> `ready`: the replacement backend becomes healthy on the same launch URL.
- any active state -> `stopping`: the user quits the app or the OS terminates the app.
- `stopping` -> `idle`: child processes are terminated and the app exits.

## Permissions

- The desktop app runs with the current OS user's permissions and does not require administrator/root privileges for normal install or launch.
- Backend filesystem, Docker, SSH, Git, and credential access remains governed by existing Kandev backend and executor behavior.
- The WebView does not receive broad shell/filesystem permissions. Process spawning for the backend is owned by the Tauri Rust side, not by arbitrary frontend code.
- Signing credentials are CI secrets. They are never committed to the repository or exposed to the desktop app runtime.

## Failure modes

- Missing packaged `kandev`, `agentctl`, or required helper binaries cause startup to fail with a visible desktop error that names the missing artifact.
- If the selected backend port is unavailable, the desktop app chooses a different available port before launch. If no port can be selected, startup fails visibly.
- If the backend exits before `/health` succeeds, the desktop app shows a startup failure and includes the captured launcher/backend output when available.
- If `/health` does not succeed before the configured timeout, the desktop app terminates the launched child process tree and shows a timeout error.
- If the WebView runtime is unavailable or broken on the host OS, the app fails using the platform's normal Tauri/WebView error behavior; the backend process must not remain running.
- If an agent CLI cannot be found from a GUI-launched environment, the desktop app/backend surfaces the same setup-health guidance as browser/CLI Kandev rather than failing silently.
- If signing/notarization secrets are absent in CI, release automation may still build explicitly unsigned internal artifacts, but public trusted desktop artifacts are not marked complete.

## Persistence guarantees

- Kandev data created through the desktop app survives app quit and process restart using the same persistence guarantees as CLI-launched Kandev.
- In-flight backend process state does not survive app quit. On next launch, the desktop app starts a fresh backend process, and backend recovery follows existing Kandev recovery behavior.
- Optional window size/position persistence survives app restart if the Tauri window-state plugin is enabled.
- Desktop installer artifacts do not alter existing npm/Homebrew/manual runtime update semantics.

## Scenarios

- **GIVEN** Kandev is installed as a desktop app on a machine without Node.js, **WHEN** the user opens the app, **THEN** the backend starts and the Kandev UI appears in the desktop window.
- **GIVEN** the desktop app is already running, **WHEN** the user opens the app again, **THEN** the existing window is focused and no second backend is started for that desktop app instance.
- **GIVEN** the backend has not finished startup, **WHEN** the desktop app is open, **THEN** the user sees a startup/loading state until `/health` succeeds or startup fails.
- **GIVEN** the backend becomes healthy, **WHEN** the desktop app navigates to the backend URL, **THEN** the existing Kandev boot payload hydrates the UI without a separate Node.js web runtime.
- **GIVEN** an agent CLI is installed in a common user binary location but the desktop app was opened from the OS launcher, **WHEN** the user starts a session using that agent, **THEN** Kandev can discover the agent command or surfaces existing setup-health guidance.
- **GIVEN** the user quits the desktop app, **WHEN** the app exits normally, **THEN** the launcher/backend child process tree is terminated.
- **GIVEN** a bundled backend binary is missing, **WHEN** the user opens the desktop app, **THEN** startup fails with a visible error naming the missing binary.
- **GIVEN** the release workflow builds desktop artifacts for a version, **WHEN** the GitHub release is published, **THEN** desktop installers and checksums are attached alongside the existing runtime tarballs.
- **GIVEN** a public macOS desktop release, **WHEN** the user downloads and opens the `.dmg`, **THEN** the app is signed and notarized so macOS does not report the download as damaged or from an unidentified developer.
- **GIVEN** a public Windows desktop release, **WHEN** the user runs the installer, **THEN** the installer is code signed to reduce SmartScreen trust warnings.

## Out of scope

- Replacing the existing npm, Homebrew, Docker, or runtime tarball channels.
- Requiring `npx` or npm installs to be Node-free.
- Mobile Tauri targets.
- App Store, Microsoft Store, Snapcraft, AUR, or Homebrew Cask distribution.
- In-app Tauri updater support in the first desktop release.
- Running Kandev as a background tray app after all windows close.
- Rewriting the Kandev frontend or backend in Rust.

## Implementation plan

- [Desktop Tauri App plan](../../plans/desktop-tauri-app/plan.md)
- [ADR-0026: Tauri desktop shell over native runtime](../../decisions/0026-tauri-desktop-shell.md)
