---
status: draft
system: desktop
requirements:
  - REQ-DESKTOP-ISOLATED-STARTUP-001
  - REQ-DESKTOP-ISOLATED-STARTUP-002
  - REQ-DESKTOP-ISOLATED-STARTUP-003
  - REQ-DESKTOP-ISOLATED-STARTUP-004
created: 2026-09-25
updated: 2026-09-25
owners:
  - kandev
---

# Isolated Desktop Startup System Design

## Purpose and boundaries

The Tauri shell owns the conflict screen and the processes it launches. The
backend continues to enforce [exclusive runtime-state ownership](../../executors/requirements/port-collision-safety.md).
Normal desktop launch keeps its shared-data and single-instance behavior.
Temporary test launch uses a separate GUI process and an operating-system
temporary home per window. This follows
[ADR-2026-09-25](../../../decisions/2026-09-25-temporary-desktop-test-processes.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-DESKTOP-ISOLATED-STARTUP-001` | [Conflict evidence and presentation](#conflict-evidence-and-presentation) |
| `REQ-DESKTOP-ISOLATED-STARTUP-002` | [Process modes](#process-modes), [Temporary home](#temporary-home), and [Shutdown](#shutdown) |
| `REQ-DESKTOP-ISOLATED-STARTUP-003` | [Developer entry point](#developer-entry-point) |
| `REQ-DESKTOP-ISOLATED-STARTUP-004` | [Startup appearance](#startup-appearance) |

## Startup appearance

`apps/desktop/index.html` paints before the bundled stylesheet is ready.
Its critical inline CSS and `apps/desktop/src/styles.css` use the same
OS-appearance media query and background values as the web application's
`--background` token: white in light mode and `#181818` in dark mode. The
startup shell, body, and failure panel use neutral Kandev surfaces so a
green-tinted frame does not appear during handoff. The OS appearance is the
available source before backend settings load; the web app may apply a saved
theme preference once it becomes available.

The plain Tauri startup HTML renders nine square cells, adapting the grid
spinner CSS and animation from `apps/web/app/globals.css`; it does not load the
React `GridSpinner` component. Its `currentColor` follows the web `--primary`
values for light and dark modes (`oklch(0.51 0.23 277)` and
`oklch(0.59 0.2 277)`). The nine cells and their staggered scale animation
appear in the critical CSS as well as the loaded stylesheet, preventing a
ring or unstyled intermediate frame. Reduced-motion mode leaves a visible
static grid. On a terminal startup failure, including a confirmed conflict,
the loading grid is hidden so the failure message is not mistaken for an
ongoing load. Loading uses a polite live region. Conflict and failure panels
use assertive live regions, as do errors from opening another temporary test
window. The decorative grid is hidden from assistive technology.

## macOS window chrome

The desktop window keeps macOS decorations and native traffic lights while
using Tauri's overlay title-bar style and a hidden native title. The startup
HTML and CSS reserve an upper-left controls area from the first frame. The
loading grid and any conflict or failure panel stay clear of it. A small
unobstructed top area permits dragging the window without taking pointer
events from the controls or fallback action. Temporary test windows use the
same window chrome as the normal launcher. The web app continues the overlay
layout after navigation, as defined by the [desktop app design](desktop-tauri-app.md#macos-window-chrome).

## Conflict evidence and presentation

`ownershiplock.Acquire` returns a typed `ConflictError` with the target kind,
canonical path, and optional advisory owner record. In
`apps/backend/internal/backendapp/main.go`, an `errors.As` check emits one
versioned machine-readable startup line for this error while retaining the
current terminal diagnostic. Include a closed storage classification so the
desktop can distinguish the default in-home SQLite case from a home-only or
external-database conflict; include the effective SQLite path for the default
case. Do not emit the line for lock-file existence,
permission errors, or other acquisition failures. Do not include secrets.

`apps/desktop/src-tauri/src/backend.rs` keeps bounded raw output for parsing
complete marker lines alongside bounded display output. Only a valid marker
from the owned launcher exiting before readiness puts the normal window into
the conflict state. The shell never reads advisory lock metadata to decide
whether another process is active. For a default in-home SQLite conflict, the
primary copy states that only one Kandev instance can use the database at a
time. For a home-only conflict that does not establish use of the same SQLite
database, it instead names the exclusive data-folder rule. The startup page
shows the affected path and available owner details, with raw output behind a
technical-details control.

The conflict page keeps cause and owner details above a distinct bottom
fallback section. That section warns that an isolated instance starts empty,
cannot see changes in the main instance, and deletes its own data after clean
quit. Its **Start isolated temporary instance** button calls a narrow Tauri
command that accepts no caller path or executable. The command is available
only to the startup WebView in a verified conflict state. The page remains open
after a test-window launch; it briefly disables the button while spawning,
then allows another click. A spawn failure appears on the launcher page
without changing its conflict state. Every successful click starts one
independent window.

`apps/desktop/src/main.ts` renders startup copy through `getStartupText` and
the six catalogs under `apps/desktop/src/locales/`. It uses
`navigator.languages` while backend settings are unavailable. The shipped
Tauri window has a 960px minimum width; this startup screen has no phone route.
At supported desktop widths and short heights, the panel has one scroll owner,
accessible controls, and no clipped action.

## Process modes

`apps/desktop/src-tauri/src/main.rs` distinguishes normal and temporary-test
mode with a narrow internal launch argument. The normal process installs
`tauri_plugin_single_instance` as today, so another ordinary launch focuses
it. A temporary-test process omits that plugin. The conflict launcher spawns
its own `current_exe` with the internal argument and the same packaged runtime
location; it does not set a parent-lifetime watchdog for the GUI child. Each
test GUI independently owns its launcher/backend process tree and loopback
origin. Closing the conflict launcher does not close already opened test
windows.

The test process creates its home before backend launch, skips normal desktop
window-geometry persistence, and does not start automatic update checks or
permit update installation. Those operations otherwise use shared Tauri app
data or affect the whole installed application. Its startup page identifies
the temporary home and clean-quit deletion rule. If startup fails after home
allocation, the failure panel keeps that path visible. After readiness, the
existing System storage page's `HomeDirRow` shows the active home.

## Temporary home

Every test process creates a cryptographically random, nonexisting directory
directly beneath its operating system temporary directory, using owner-only
permissions where supported. It never accepts a path from the startup WebView
or shares a name with another process. It writes an empty `config.yaml` in the
new directory and starts the launcher with:

```text
KANDEV_HOME_DIR=<new temporary home>
KANDEV_DATABASE_DRIVER=sqlite
KANDEV_DATABASE_PATH=<new temporary home>/data/kandev.db
KANDEV_INTERNAL_CONFIG_FILE=<new temporary home>/config.yaml
```

The test child removes inherited `KANDEV_*` data/configuration overrides that
could redirect it to shared or external storage, while preserving the explicit
runtime profile selector (`KANDEV_DEBUG_DEV_MODE` or `KANDEV_E2E_MOCK`). The
normal desktop environment still supplies executable discovery, bundled
runtime, loopback host, native notification flag, per-launch health token, and
the desktop-to-launcher parent PID. Backend startup still acquires its home
lock and passes token-verified `/health` followed by `/ready` before WebView
navigation. Temporary windows use `pick_loopback_port()` for a kernel-assigned
port. Normal launches use `pick_desktop_port()`, which prefers the configured
desktop port and falls back to another available loopback port.

## Shutdown

Each test process owns only its own backend and home. On normal quit, it first
stops and reaps the launcher/backend tree. The launcher exits with status zero
only when every supervised process exited gracefully with status zero and no
forced cleanup, failure, or uncertain process status. The desktop removes its
recorded temporary directory only after that confirmed launcher result and
backend readiness. Startup or initial-navigation failure retains the home for
inspection even if the backend reached readiness. A forced kill, uncertain
stop, unexpected process exit, or crash also leaves the directory intact; a
later launch does not assume a leftover directory is active or delete it
opportunistically. The operating system or the user may clean such leftovers
later. Directory removal verifies the stored canonical path is the exact
random directory the process created under its temp root; it never follows a
replaced root symlink.

The conflict launcher never stops the terminal backend, removes a lock
sidecar, copies production data, or releases another process's lock. The
existing origin checks on privileged commands remain unchanged once a test
WebView navigates to its owned backend.

## Developer entry point

`make desktop-dev` already passes its shell environment into `tauri dev`.
Public guidance shows a reusable isolated home with explicit
`KANDEV_HOME_DIR`, in-home `KANDEV_DATABASE_PATH`, SQLite driver, and
`KANDEV_DEBUG_DEV_MODE=true`. The conflict launcher can instead open several
disposable test windows. The repository has no `./kandev-dev` script; an
external environment wrapper can set the same variables for a reusable home.

## Verification boundaries

- Go tests prove only `ConflictError` emits the marker and the terminal
  diagnostic remains readable.
- Rust tests prove the normal-versus-test process mode, marker parsing,
  temporary path allocation, environment filtering, independent child
  ownership, and cleanup only after confirmed stop.
- Desktop smoke tests open two test windows from one conflict launcher and
  prove distinct homes, databases, ports, and owned WebView origins. A normal
  second launch still focuses the first normal window.

## Implementation plan

- [Isolated desktop startup](../../../plans/desktop-isolated-startup/plan.md)
