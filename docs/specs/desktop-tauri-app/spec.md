---
status: shipped
created: 2026-06-23
updated: 2026-07-15
owner: tbd
---

# Tauri Desktop App

## Why

Kandev's installed desktop app should behave like a native application without duplicating the
existing React product surface. Users need standard focused-window commands, reliable native
updates and notifications, and preservation of their window state while Kandev continues to use
the existing local Go backend and shared settings UI.

## What

The existing Tauri v2 shell remains a thin native host for the Go-served SPA. In addition to its
implemented launch and backend-lifecycle behavior, it provides:

- application menus with platform-appropriate accelerators;
- zoom in, zoom out, and actual-size commands (`Cmd` on macOS, `Ctrl` elsewhere);
- contextual `Cmd/Ctrl+W` behavior that never closes the window, backend, or application;
- `Cmd+,` on macOS to open the existing `/settings/general` page;
- New Task, Check for Updates, Help, external-link, and standard application commands;
- persisted window size, position, and maximized state, restored onto a visible display;
- signed, prompt-before-install desktop updates through the existing System > Updates page;
- native notifications for waiting-for-input and session-failure events;
- focus/reopen behavior for a second launch and Dock activation where supported.

The macOS red window close control quits Kandev and stops its owned backend. It does not hide the
window or leave a tray/background process. `Cmd+W` is deliberately separate from that lifecycle.

### Existing shell guarantees

This increment preserves the shipped desktop contract:

- installers target macOS arm64/x64, Linux arm64/x64, and Windows x64;
- the app starts the packaged native launcher and Go backend, then displays the existing embedded
  Vite/React SPA without requiring Node.js or a separate web server at runtime;
- desktop and CLI/browser launches share the existing Kandev data directory, database, worktrees,
  executor settings, integrations, and agent configuration;
- the desktop shell owns the backend process tree, waits for authenticated health readiness, and
  cleans up on startup failure, quit, update restart, or WebView failure;
- the backend binds to an app-selected loopback port, and the WebView navigates only after a
  per-launch health token proves that the responding process is the backend the shell started;
- GUI launches retain the predictable process environment and common user binary locations needed
  to discover configured agent CLIs;
- a second launch focuses the existing instance instead of starting another backend; and
- GitHub releases continue to include the existing installers, runtime archives, and checksums.

## Existing launch and data contract

The native shell continues to package the Kandev launcher/runtime without a production Node.js
dependency. It selects an available loopback port, starts:

```text
kandev --headless --port <available-loopback-port>
```

with `KANDEV_SERVER_HOST=127.0.0.1`, and waits for `GET /health`. Each launch supplies a generated
`KANDEV_DESKTOP_HEALTH_TOKEN`; readiness requires the same value in the successful response's
`X-Kandev-Desktop-Health-Token` header before the WebView navigates to the backend origin. A second
instance focuses the first and does not start another backend.

The shell preserves the existing GUI-launch `PATH` normalization and the existing Kandev data
directory, SQLite database, worktrees, executor settings, integrations, and agent configuration.
It owns cleanup of the launcher/backend process tree on quit, startup failure, and update restart.
The existing HTTP, WebSocket, and boot-payload product APIs remain the SPA's data plane; the
desktop bridge is only for native integration.

## Contextual close contract

One `Cmd/Ctrl+W` invocation closes at most one item, in this order:

1. The topmost dismissible overlay, such as the New Task dialog, a sheet, or a popover.
2. The focused closable document tab, including file, diff, commit, or preview tabs.
3. Nothing.

Alert/confirmation dialogs that intentionally require an explicit choice are not dismissible.
Closing a document must use the same unsaved-change guard as its visible close affordance.
Session, chat, terminal, task, and structural layout panels are not document tabs and are never
closed by this command. When there is no eligible context, the command is a no-op.

## Native menu contract

The focused desktop application exposes platform-standard menus. At minimum:

- **Application:** About, Settings/Preferences (`Cmd+,` on macOS), Hide/Services where supplied
  by the OS, Check for Updates, and Quit.
- **File:** New Task (`Cmd/Ctrl+N`) and Close Context (`Cmd/Ctrl+W`).
- **View:** Zoom In (`Cmd/Ctrl++` and `Cmd/Ctrl+=`), Zoom Out (`Cmd/Ctrl+-`), Actual Size
  (`Cmd/Ctrl+0`), and platform full-screen behavior.
- **Help:** documentation, repository/support, and release notes opened in the system browser.

These are focused-application accelerators, not global OS shortcuts. New Task invokes the existing
task-create flow when an active workspace permits it. Settings routes to the existing settings UI;
no native settings window is added.

## Update contract

The desktop updater is distinct from the service-only updater described by ADR-0012:

- It checks after the desktop app reaches `ready` and at most every 24 hours while running.
- A manual Check for Updates action always performs a fresh check.
- Finding an update never downloads, installs, or restarts silently. The existing
  `/settings/system/updates` page shows the version and release information and asks for explicit
  confirmation before download/install/relaunch.
- Automatic-check failures are non-blocking. Manual checks show actionable no-update or error
  status in the existing page.
- Only a newer version signed by the embedded Tauri updater key can be installed. Downgrades and
  payloads with missing or invalid updater signatures are rejected.
- Before relaunch, the desktop shell stops the owned backend process tree, then allows Tauri's
  updater restart path to replace and reopen the application.

The release publishes a signed static `latest.json` manifest and Tauri updater artifacts for:

- macOS arm64 and x64: updater `.app.tar.gz` bundles plus existing `.dmg` installers;
- Linux x64 and arm64: updater `.AppImage.tar.gz` bundles plus AppImage, `.deb`, and `.rpm`
  packages;
- Windows x64: NSIS updater bundle plus the existing installer.

AppImage is the auto-update path on Linux. `.deb` and `.rpm` users continue to update through
their package manager or manual installation. Update payload signatures use a dedicated Tauri
updater key; OS code signing/notarization remains a separate publisher-identity capability.

Decision: [ADR-0040](../../decisions/0040-separate-updater-integrity-from-os-publisher-identity.md).
Missing Apple or Windows signing credentials do not block updater artifacts signed by the Tauri
key. Release notes continue to identify those applications as unsigned development builds until
OS publisher-identity signing is configured.

## Notification contract

For desktop-owned launches, Kandev uses native OS notifications for:

- a session waiting for user input; and
- a session that failed.

Notification delivery respects the existing notification settings and suppresses duplicate
browser and backend shell-command notifications for the same desktop event. Browser- and
service-launched Kandev retain their existing delivery paths. Dock reopen and second-instance
activation show, unminimize, and focus the window but never infer notification routing. The
official Tauri notification plugin does not expose payload-aware body-click callbacks on desktop,
so a raw notification click cannot be correlated to an arbitrary payload on every supported OS.
Generic focus and activation events therefore never navigate. Repeated delivery of the same event
identity is deduplicated.

## Desktop bridge and permissions

The Go-served SPA may receive a small versioned set of desktop events and commands for menu
actions, updater state/actions, native notification delivery, focus, and safe external links. The
bridge is available only to the app's loopback origin and exposes no arbitrary shell command,
filesystem access, or unrestricted URL opening.

External `http`, `https`, and `mailto` links open in the system browser/client. Internal loopback
navigation, downloads, and blob URLs remain in the WebView unless an existing workflow specifies
otherwise.

The launch boundary remains:

```text
kandev --headless --port <available-loopback-port>
GET http://127.0.0.1:<port>/health
X-Kandev-Desktop-Health-Token: <per-launch-token>
```

Desktop launches set `KANDEV_SERVER_HOST=127.0.0.1`. The health response must echo the per-launch
token before the WebView navigates to the loopback SPA. Existing backend HTTP, WebSocket, and boot
payload contracts remain the product API; native integration does not replace them.

## State machines

The implemented launch lifecycle remains `idle` -> `backend-starting` -> `ready`, with
`backend-restarting`, `stopping`, and `failed` transitions as defined by the original desktop
release. Native integration adds:

### Update state

- `idle` -> `checking` for startup, periodic, or manual checks.
- `checking` -> `available`, `current`, or `error`.
- `available` -> `downloading` only after user confirmation.
- `downloading` -> `installing` -> `restarting`.
- Any failed update phase -> `error`, preserving the running version.

Only one check or install operation may be in flight. Quit and update-restart share the same
backend shutdown primitive, but updater restart must preserve Tauri's updater exit semantics.

### Window state

Size, position, and maximized state are persisted after stable changes. Minimized state is not
restored. If a saved position is outside all current displays, the window is clamped to a visible
display before being shown.

## Failure modes

- A menu command received before SPA readiness is ignored or replayed once when explicitly safe;
  it never becomes a window-close fallback.
- A failed zoom request leaves the previous zoom value intact and does not reload the SPA.
- Invalid, missing, mismatched-platform, or incorrectly signed updater metadata/payloads are
  rejected without stopping the running application.
- A missing packaged launcher/helper, backend bind failure, failed authenticated health check, or
  startup timeout produces a visible startup error and terminates the owned process tree.
- A GUI environment that cannot find an agent CLI surfaces the existing setup-health guidance.
- If backend shutdown fails during update restart, installation does not force an unclean restart;
  the user sees an actionable error and can quit normally.
- If notification permission is denied or the platform notifier is unavailable, Kandev retains
  its in-app indication and does not repeatedly prompt.
- A native notification never infers a task route from an ordinary focus, Dock reopen, or
  second-instance activation event.
- External links with unsupported schemes remain blocked.

## Persistence guarantees

- Kandev data continues to use the existing backend database and `KANDEV_HOME_DIR` behavior.
- Window state and non-sensitive updater bookkeeping may use the Tauri application data location.
- No updater signing secret is shipped; only the public verification key is embedded.
- Existing OS-unsigned development installers may still be produced and advertised through the
  in-app updater when their updater payloads carry a valid Tauri signature. A missing Tauri updater
  signing key cannot produce an installable updater manifest entry.
- Closing a document through `Cmd/Ctrl+W` preserves the same dirty-state guarantees as clicking
  its close control.
- Existing OS-signed/notarized installers, checksums, and unsigned-development-build warnings
  remain part of the desktop release contract in addition to updater artifacts.

## Scenarios

- **GIVEN** the desktop window is focused, **WHEN** the user presses `Cmd+-`, `Cmd++`, or `Cmd+0`,
  **THEN** the current WebView zoom decreases, increases, or resets without resizing the window.
- **GIVEN** the machine has no Node.js runtime, **WHEN** the packaged desktop app starts, **THEN**
  its bundled native launcher/backend and embedded SPA become ready without a Node web server.
- **GIVEN** New Task is the topmost dismissible dialog, **WHEN** the user presses `Cmd+W`, **THEN**
  that dialog closes and the app and backend remain running.
- **GIVEN** a closable file tab is focused and no dismissible overlay is open, **WHEN** the user
  presses `Cmd+W`, **THEN** the file follows its normal close/dirty guard.
- **GIVEN** only a task, chat, session, terminal, or structural panel is focused, **WHEN** the user
  presses `Cmd+W`, **THEN** nothing closes.
- **GIVEN** the macOS app is running, **WHEN** the user presses `Cmd+,`, **THEN** the existing
  Settings page opens in the main window.
- **GIVEN** the user clicks the macOS red close control, **WHEN** shutdown completes, **THEN** both
  the app and its owned backend exit.
- **GIVEN** a signed newer desktop version exists, **WHEN** an automatic check finds it, **THEN**
  Kandev prompts in the existing Updates page before any download, install, or restart.
- **GIVEN** a Linux AppImage installation, **WHEN** the user confirms a valid update, **THEN** the
  signed AppImage updater artifact installs and Kandev relaunches after clean backend shutdown.
- **GIVEN** a session waits for input while Kandev is not focused, **WHEN** its native notification
  is delivered, **THEN** Kandev sends no duplicate browser or shell notification and generic app
  focus or activation does not navigate to an inferred task.
- **GIVEN** a saved window position belongs to a disconnected display, **WHEN** Kandev reopens,
  **THEN** the main window appears within a currently visible display.
- **GIVEN** the same settings page is opened in a browser or narrow/mobile viewport, **WHEN** it
  renders, **THEN** the existing service-update experience and responsive layout remain intact;
  desktop-only native actions are not shown.

## Out of scope

- Tray/background operation after the last window closes.
- Launch at login or OS autostart.
- Global shortcuts that operate while another application is focused.
- Silent, forced, or downgrade updates; App Store/Microsoft Store/Snap distribution.
- Auto-updating Linux `.deb` or `.rpm` packages outside their package managers.
- Notifications beyond waiting-for-input and session failure in this increment.
- A separate native settings window, multiple native windows, or a rewritten native UI.
- Mobile Tauri targets. Native shortcuts have no mobile analogue; the shared settings page must
  remain responsive and preserve browser/mobile behavior.
- Replacing npm, Homebrew, Docker, runtime tarball, or existing package-manager update channels.
- Rewriting the shared frontend or backend in Rust.

## Implementation plans and decisions

- [Initial Desktop Tauri App plan](../../plans/desktop-tauri-app/plan.md)
- [Desktop Native Integration plan](../../plans/desktop-native-integration/plan.md)
- [ADR-0026: Tauri desktop shell over native runtime](../../decisions/0026-tauri-desktop-shell.md)
- [ADR-0039: Native desktop integration boundary](../../decisions/0039-native-desktop-integration-boundary.md)
