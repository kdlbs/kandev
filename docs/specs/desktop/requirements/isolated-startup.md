---
status: draft
system: desktop
created: 2026-09-25
updated: 2026-09-25
owners:
  - kandev
---

# Isolated Desktop Startup Requirements

## Overview

The desktop shell normally opens the same Kandev data as a terminal launch.
When another backend owns that data, the desktop should explain the conflict
and let the user open several independent temporary test windows. Desktop owns
this choice because it starts its backend and renders the startup surface
before the web application is available.

## Terminology

- **Normal desktop instance:** The existing single-instance GUI process using
  the selected Kandev home and database.
- **Temporary test instance:** A separate GUI process with its own new
  operating-system temporary directory, Kandev home, and SQLite database.
- **Ownership conflict:** Another live backend holds the operating-system lock
  for the selected home or database. A stale lock file alone is not a conflict.

## Requirements

### REQ-DESKTOP-ISOLATED-STARTUP-001: Explain a conflicting instance

**Intent:** Let the user understand why normal desktop startup stopped and
start a test window without risking the running instance.

#### Acceptance criteria

- **AC-DESKTOP-ISOLATED-STARTUP-001.1:** If another backend owns the selected
  home or SQLite database, the startup screen shall explain that only one
  Kandev instance can use that database or data folder at a time. For the
  default in-home SQLite database, it shall explicitly say that only one
  Kandev instance can use the database at a time. It shall identify the
  affected data path and show owner process details when available.
- **AC-DESKTOP-ISOLATED-STARTUP-001.2:** The conflict screen shall offer
  **Start isolated temporary instance** in a fallback section at the bottom.
  Before that action, it shall warn that the instance uses empty data separate
  from the main instance and that data created there is deleted after a clean
  quit. The conflict screen shall remain available to start another isolated
  instance.
- **AC-DESKTOP-ISOLATED-STARTUP-001.3:** An unrelated startup failure,
  including permission denial, invalid configuration, missing runtime files,
  or a stale lock file with no active owner, shall not be presented as an
  ownership conflict or offer the temporary-window action.
- **AC-DESKTOP-ISOLATED-STARTUP-001.4:** The conflict explanation and action
  shall use localized copy, with the operating system's language as the
  fallback when app settings are unavailable.

### REQ-DESKTOP-ISOLATED-STARTUP-002: Run independent temporary instances

**Intent:** Let a normal terminal or desktop Kandev instance and multiple
desktop test instances run concurrently without sharing mutable runtime state.

#### Acceptance criteria

- **AC-DESKTOP-ISOLATED-STARTUP-002.1:** Each explicit temporary-window action
  shall start a separate desktop process with a previously unused temporary
  Kandev home, an SQLite database inside it, and an available loopback port.
  It shall not open, migrate, overwrite, or stop another instance's data.
- **AC-DESKTOP-ISOLATED-STARTUP-002.2:** Temporary launch shall ignore
  inherited data and configuration overrides that could redirect it to a
  shared home or external database. The test window shall show its own data
  location during startup and on failure; after startup, the existing System
  storage page shall show that home.
- **AC-DESKTOP-ISOLATED-STARTUP-002.3:** Closing one test window shall stop
  only its owned backend. After a confirmed clean stop, it shall remove only
  its own temporary directory. If shutdown is uncertain or the process
  crashes, the directory shall be left intact rather than deleting potentially
  live data. Another test window and the normal instance shall remain running.
- **AC-DESKTOP-ISOLATED-STARTUP-002.4:** A failed test-window start shall show
  its own error and retained directory path without changing the conflict
  launcher or another running instance. The launcher shall allow another
  explicit attempt.
- **AC-DESKTOP-ISOLATED-STARTUP-002.5:** Normal desktop launches shall retain
  the existing shared-data and single-instance focus behavior, including an
  explicit `KANDEV_HOME_DIR`. Temporary data isolation shall not silently
  change the selected runtime profile.

### REQ-DESKTOP-ISOLATED-STARTUP-003: Support deliberate developer isolation

**Intent:** Let a developer choose either disposable test windows or a reusable
development home while a normal Kandev instance continues running.

#### Acceptance criteria

- **AC-DESKTOP-ISOLATED-STARTUP-003.1:** Public desktop development guidance
  shall show a reusable `make desktop-dev` launch with a separate
  `KANDEV_HOME_DIR`, in-home SQLite database, and dev profile. It shall also
  explain how temporary test windows differ and how to open more than one.

### REQ-DESKTOP-ISOLATED-STARTUP-004: Match Kandev during desktop loading

**Intent:** Make the native startup surface feel like Kandev before its web
application is ready, for both normal and temporary test windows.

#### Acceptance criteria

- **AC-DESKTOP-ISOLATED-STARTUP-004.1:** From the first painted frame through
  backend readiness, the startup background shall match the web application's
  background for the operating system's light or dark appearance: white in
  light mode and `#181818` in dark mode. The initial HTML and loaded desktop
  stylesheet shall agree so the background does not flash another color.
- **AC-DESKTOP-ISOLATED-STARTUP-004.2:** The loading indicator shall use
  Kandev's nine-square grid form, motion, and light/dark primary color instead
  of the green-and-yellow ring. It shall remain legible against either
  background, stop animating when reduced motion is requested, and cease to
  indicate loading after startup fails. The visible startup status shall
  remain available to assistive technology.

## Out of scope

- Sharing one database or Kandev home between live backends.
- Attaching a desktop WebView to a terminal-owned backend.
- Automatic deletion of data after a crash or uncertain backend shutdown.
- Changing the terminal launcher's conflict policy.

## Related contracts

- [Desktop app](desktop-tauri-app.md) defines normal single-instance behavior.
- [Exclusive runtime-state ownership](../../executors/requirements/port-collision-safety.md)
  defines the backend safety boundary that this flow preserves.
