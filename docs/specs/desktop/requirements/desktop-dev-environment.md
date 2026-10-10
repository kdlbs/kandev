---
status: draft
system: desktop
created: 2026-09-26
owners:
  - kandev
---

# Desktop Development Environment Requirements

## Overview

Developers need `make desktop-dev` to open the native macOS application with
the same default development state and runtime profile as `make dev`. Today the
Tauri shell runs in development mode, but its backend can use the normal
`~/.kandev` home and database. The desktop system owns this entry point; the
[platform dev launcher](../../platform/requirements/go-dev-launcher.md) owns
the corresponding browser development contract.

## Requirements

### REQ-DESKTOP-DEV-ENV-001: Isolated desktop development launch

**Intent:** A developer can run the native macOS application from a checkout
without selecting the production Kandev state or setting environment variables
by hand.

#### Acceptance criteria

- **AC-DESKTOP-DEV-ENV-001.1:** When a developer runs `make desktop-dev` from a
  checkout with no explicit development configuration, the backend shall use
  that checkout's `.kandev-dev` home, including its default
  `.kandev-dev/data/kandev.db` database and `.kandev-dev/logs` directory.
- **AC-DESKTOP-DEV-ENV-001.2:** The same launch shall select the dev runtime
  profile automatically, without requiring the developer to set profile or
  path variables. An inherited E2E profile selector shall not change the
  desktop launch to the e2e profile.
- **AC-DESKTOP-DEV-ENV-001.3:** If the invoking shell contains inherited
  `KANDEV_HOME_DIR`, `KANDEV_DATABASE_PATH`, or a non-SQLite database-driver
  selection, `make desktop-dev` shall still select the checkout's
  `.kandev-dev` home and local SQLite database. It shall not open another
  database because of those inherited values.
- **AC-DESKTOP-DEV-ENV-001.4:** The installed desktop application and
  `make desktop-build` shall retain their existing configuration and default
  user home. The development defaults shall apply only to `make desktop-dev`.
- **AC-DESKTOP-DEV-ENV-001.5:** If another Kandev process already owns the
  checkout's `.kandev-dev` home, `make desktop-dev` shall surface the existing
  startup failure rather than switch to another home or database.

## Out of scope

- Live rebuilding of the Go backend or product web application while the
  Tauri window is open. The existing `desktop-dev` target builds a runtime
  bundle before launch; this requirement changes its default runtime state.
- Changing how `make dev` handles explicit external database targets or
  development database backups.
- Changing the installed desktop app's shared data layout.
