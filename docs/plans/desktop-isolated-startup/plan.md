---
created: 2026-09-25
status: completed
requirements:
  - REQ-DESKTOP-DESKTOP-TAURI-APP-001
  - REQ-DESKTOP-ISOLATED-STARTUP-001
  - REQ-DESKTOP-ISOLATED-STARTUP-002
  - REQ-DESKTOP-ISOLATED-STARTUP-003
  - REQ-DESKTOP-ISOLATED-STARTUP-004
system_design:
  - ../../specs/desktop/system-design/desktop-tauri-app.md
  - ../../specs/desktop/system-design/isolated-startup.md
legacy_specs: []
---

# Implementation Plan: Temporary desktop test windows

## Overview

When a terminal backend owns the desktop's selected data, desktop startup
should explain the conflict and allow repeated opening of independent,
disposable test windows. Normal GUI launches remain single-instance. The
[requirements](../../specs/desktop/requirements/isolated-startup.md),
[system design](../../specs/desktop/system-design/isolated-startup.md),
[desktop app design](../../specs/desktop/system-design/desktop-tauri-app.md),
and [process decision](../../decisions/2026-09-25-temporary-desktop-test-processes.md)
define the result. Implement the conflict signal, per-process temporary launch,
then the visible launcher and multi-window smoke proof. The
[macOS overlay decision](../../decisions/2026-09-25-macos-overlay-titlebar.md)
adds a single-row native-controls layout after that startup flow is in place.

## Scope

### In scope

- Classify an active home/database ownership conflict from typed backend
  evidence, not lock-file existence or arbitrary stderr text.
- Keep the normal desktop process as a conflict launcher that may open several
  independent temporary test processes.
- Allocate one private OS-temporary home and in-home SQLite database per test
  process; remove it only after confirmed clean backend stop.
- Explain the one-instance-per-database rule, show the conflict and owner,
  then offer a clearly separated bottom fallback with the data-loss warning.
- Match Kandev's light/dark background and grid spinner throughout desktop
  loading, including the first frame before the bundled CSS arrives.
- On macOS, let Kandev fill the former title-strip area while retaining native
  traffic-light controls and a safe drag region through startup and app use.
- Document the temporary flow and a separate reusable developer home.

### Out of scope

- Two backends sharing one home/database, attachment to a terminal backend,
  multiple backend windows inside one Tauri process, and deletion after an
  uncertain shutdown or crash.

## Technical approach

`apps/backend/internal/backendapp/main.go` emits a versioned marker only for
`ownershiplock.ConflictError`, preserving terminal diagnostics. Rust in
`apps/desktop/src-tauri/src/backend.rs` parses complete bounded marker lines
from the owned launcher. The conflict startup page can invoke a narrow native
command to spawn `current_exe` in temporary-test mode; it stays open after each
spawn.

`apps/desktop/src-tauri/src/main.rs` installs the single-instance plugin only
for normal mode. A test process creates a random new directory directly under
the OS temp root, pins an empty config plus in-home SQLite path, and uses its
own launcher, health token, port, and origin. It does not persist normal window
geometry or run automatic updates. After a clean backend stop it removes only
its own directory; uncertain stops retain the data for inspection.

The startup page renders localized status and the repeatable launch action.
The existing System storage page exposes the test home after readiness. Update
`docs/public/desktop-app.md` with temporary-window behavior and the reusable
`make desktop-dev` command.

The macOS Tauri window uses an overlay title bar with the native title hidden,
but retains decorations and traffic lights. Startup and web-app headers reserve
their upper-left hit area, including with a collapsed sidebar. Windows and
Linux keep their current native decorations.

## ASCII UI preview

`UI-00: Loading in normal or temporary desktop windows`

```text
  SYSTEM LIGHT (white)                    SYSTEM DARK (#181818)
  +------------------------------+          +------------------------------+
  |                              |          |                              |
  |          [ ][ ][ ]           |          |          [ ][ ][ ]           |
  |          [ ][ ][ ]           |          |          [ ][ ][ ]           |
  |          [ ][ ][ ]           |          |          [ ][ ][ ]           |
  |       Kandev is starting     |          |       Kandev is starting     |
  |                              |          |                              |
  +------------------------------+          +------------------------------+
       primary purple grid                    primary purple grid
```

The squares pulse in Kandev's staggered grid pattern. With reduced motion,
they remain visible and still. The grid disappears if startup fails; the
failure message takes its place.

`UI-00a: macOS window edge, with no separate title strip`

```text
  LOADING / CONFLICT               EXPANDED SIDEBAR (320px minimum)
  +-------------------------+      +----------------------------------------+
  | (red)(yellow)(green)     |      | (red)(yellow)(green) Kandev / Sky v  [<] |
  |                         |      |----------------------------------------|
  |      [ ][ ][ ]          |      | Home                                   |
  |      [ ][ ][ ]          |      | New Task                               |
  |      [ ][ ][ ]          |      +----------------------------------------+
  +-------------------------+

  COLLAPSED RAIL (56px)            ADJACENT PAGE HEADER
  +----------+---------------------+---------------------------+
  | (red)(yellow)(green)           |  Home            Search  |
  |----------+---------------------+---------------------------|
  |    K     |                                                 |
  |   [>]    |                                                 |
  +----------+-------------------------------------------------+
```

The colored circles are macOS native window controls, not HTML buttons. The
controls occupy reserved space in the existing app header row; empty header
space remains draggable. `[<]` collapses the sidebar and `[>]` expands it.
The workspace switcher stays in the expanded header; long names truncate
without hiding its chevron or the collapse button. At 56px, the rail's brand
and expand button move below the native controls, and the adjacent page
header clears their hit area. The switcher reappears after expansion. The
centered native “Kandev” title is hidden.

`UI-01: Normal desktop window, confirmed data conflict`

```text
+--------------------------------------------------------------------+
| (red)(yellow)(green)                                                |
| KANDEV                                      Desktop startup blocked |
|                                                                    |
| Only one Kandev instance can use this database at a time.         |
| Another instance is already using your main Kandev data.          |
|                                                                    |
| Database: /Users/.../.kandev/data/kandev.db                        |
| Running process: 6804 (.../bin/kandev)                             |
| [ Technical details v ]                                            |
|--------------------------------------------------------------------|
| Start a temporary instance                                         |
| Warning: This starts with empty data, separate from your main    |
| instance. You will lose anything created there when its window   |
| closes normally. You can open more than one temporary instance.  |
|                                                                    |
| [ Start isolated temporary instance ]                              |
+--------------------------------------------------------------------+
```

`UI-02: First test window is opening; launcher remains available`

```text
  NORMAL WINDOW (still open)                TEST WINDOW A
  +-------------------------------+         +------------------------------+
  | Main database is in use       |         | Temporary test instance      |
  | [ Start isolated instance ]   |         | Starting backend...          |
  | Test window opening           |         | Data: /tmp/kandev-...-A      |
  +-------------------------------+         | Removed after clean quit     |
                                            +------------------------------+
```

`UI-03: Second click opens another independent test window`

```text
  NORMAL LAUNCHER             TEST WINDOW A           TEST WINDOW B
  +--------------------+      +------------------+     +------------------+
  | Main DB is in use  |      | Kandev, data A   |     | Kandev, data B   |
  | [ Start another ]  |      | backend A        |     | backend B        |
  +--------------------+      +------------------+     +------------------+
```

The repeatable action, independent windows, path, and clean-quit rule are
structural requirements for `AC-DESKTOP-ISOLATED-STARTUP-001.1` through `.4`
and `AC-DESKTOP-ISOLATED-STARTUP-002.1` through `.4`. `UI-00` illustrates
`AC-DESKTOP-ISOLATED-STARTUP-004.1` and `.2`. Copy and spacing are
illustrative; implementation must localize copy and use desktop design tokens.
`UI-01` shows the reported default SQLite case. For a home-only conflict
without that SQLite database, the main line names the exclusive data folder
instead of claiming a database conflict.
The Tauri window's configured minimum width is 960px, so no phone-width
desktop composition exists. This native startup screen has no mobile web route;
mobile parity requires no separate phone flow here. At minimum desktop size
and short window height, actions remain visible or reachable in one internal
scroll region.

## Tests

| Acceptance | Evidence |
| --- | --- |
| `AC-DESKTOP-ISOLATED-STARTUP-001.1`, `.3` | Go conflict-marker/storage-kind tests and Rust parser/state tests |
| `AC-DESKTOP-ISOLATED-STARTUP-002.1`, `.2`, `.4` | Rust process-mode, path, environment, and independent-child tests |
| `AC-DESKTOP-ISOLATED-STARTUP-002.3` | Rust clean-stop and uncertain-stop cleanup tests |
| `AC-DESKTOP-ISOLATED-STARTUP-002.5` | Normal-launch single-instance regression |
| `AC-DESKTOP-ISOLATED-STARTUP-003.1` | Desktop guide command and docs validation |
| `AC-DESKTOP-ISOLATED-STARTUP-004.1`, `.2` | Rendered first-frame and loaded-state checks in OS light/dark and reduced-motion modes; failed-state check |
| `AC-DESKTOP-DESKTOP-TAURI-APP-001.10` | macOS native window smoke for traffic lights, drag, startup/conflict/ready, collapsed sidebar, full screen, and minimum size; Linux/Windows decoration regression |
| `AC-DESKTOP-DESKTOP-TAURI-APP-001.11` | Rendered expanded/collapsed/hover-reveal checks at 320px sidebar and 960px window; long workspace names, toggle hit targets, and switcher menu |

## E2E tests

Extend `apps/desktop/e2e/desktop-launch-smoke.mjs` to prove
`AC-DESKTOP-ISOLATED-STARTUP-001.2` and
`AC-DESKTOP-ISOLATED-STARTUP-002.1` through `.5`: one normal conflict launcher
opens two temporary GUI children, each fake runtime records distinct home,
database, port, and root WebView request after token-matched `/health` and
`/ready`. Closing one child leaves the other and the launcher running. The
existing happy path proves normal launch. A focused rendered check at the
minimum desktop size verifies action reachability and technical-detail scroll.
The rendered check also inspects the boot HTML before the bundled CSS loads,
then after it loads, so theme or spinner flashes are caught in both appearances.

## Work orders

- [x] [Task 01: Emit typed ownership conflict](task-01-conflict-diagnostic.md)
- [x] [Task 02: Start independent temporary windows](task-02-isolated-backend.md)
- [x] [Task 03: Present and verify multi-window recovery](task-03-conflict-recovery-ui.md)
- [x] [Task 04: Integrate macOS window controls](task-04-macos-window-chrome.md)

## Verification results

Task 01 passed `TMPDIR=/root/.cache/kandev-desktop-task-tmp go test
./internal/backendapp ./internal/backendapp/ownershiplock -count=1` from
`apps/backend`.

Task 02 passed `cargo test --features desktop-runtime --lib` (81 tests),
`cargo check --features desktop-runtime --bin kandev-desktop`, and
`cargo fmt --all -- --check` from `apps/desktop/src-tauri`.

Task 03 passed `TMPDIR=/root/.cache/kandev-desktop-task-tmp pnpm --filter
@kandev/desktop e2e`, including the release bundle and two-window isolated-home
smoke. `node --test apps/desktop/e2e/desktop-launch-smoke.test.mjs` passed 13
tests. Public-doc validation passed 62 validator tests and checked 47 pages.

Task 04 passed desktop and web typechecks, focused web tests, web lint and i18n
checks, and desktop and mobile Playwright layout checks. The desktop layout
test covers expanded, collapsed, and hover-revealed sidebar states. Native
traffic-light placement, dragging, and full-screen behavior still require a
macOS host; this implementation was built and tested on Linux.

## Risks

- The nested launcher may split or truncate stderr; marker parsing must be
  bounded and reject malformed output.
- Shared Tauri app data currently stores window geometry. Test processes must
  avoid concurrent writes there and avoid competing automatic updates.
- Forced backend shutdown may leave live worktrees. Cleanup must require a
  confirmed clean stop and exact ownership of the temporary directory.
- The Linux fake-runtime smoke drives startup controls through X11. Native
  AppKit controls, traffic-light placement, and drag behavior still require a
  macOS run.
