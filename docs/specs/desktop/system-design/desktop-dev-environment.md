---
status: draft
system: desktop
requirements:
  - REQ-DESKTOP-DEV-ENV-001
created: 2026-09-26
owners:
  - kandev
---

# Desktop Development Environment System Design

## Purpose and boundaries

`make desktop-dev` is a source-checkout entry point for the Tauri shell. Its
runtime defaults should match the state root and profile selected by
[`make dev`](../../platform/requirements/go-dev-launcher.md) while preserving
the existing desktop process, health-token, and WebView origin boundaries in
[ADR 0026](../../../decisions/0026-tauri-desktop-shell.md). This design does
not change the installed app or the shared platform launcher's database
selection policy.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-DESKTOP-DEV-ENV-001` | [Launch flow](#launch-flow), [Scope and safety](#scope-and-safety), and [Verification](#verification) |

## Launch flow

The root `Makefile` continues to run `desktop-runtime` first, building a local
runtime bundle. Only the `desktop-dev` recipe then supplies these values to the
Tauri dev process:

- `KANDEV_HOME_DIR=$(CURDIR)/.kandev-dev`;
- `KANDEV_DATABASE_PATH=$(CURDIR)/.kandev-dev/data/kandev.db`;
- `KANDEV_DATABASE_DRIVER=sqlite`;
- `KANDEV_E2E_MOCK=false`;
- `KANDEV_DEBUG_DEV_MODE=true`.

The recipe sets these values for the launched process itself, so ambient values
with the same names cannot redirect the state root, select another database
driver, or select the e2e profile. `$(CURDIR)` is the absolute checkout path
when the root Makefile runs; the home and database path describe one state
root. The backend reads dev-profile settings from embedded `profiles.yaml` as
usual. The recipe disables the higher-priority E2E selector rather than copying
profile defaults into Make or Rust.

`apps/desktop/src-tauri/src/backend.rs` already copies the Tauri process
environment into its child launcher command and only replaces desktop-owned
host, bundle, and notification variables. The existing `kandev --headless
--port <desktop-port>` launch, desktop health token, loopback bind, readiness
checks, and owned-origin validation remain in place. This avoids a second
database resolver in the Tauri shell.

## Scope and safety

The recipe-level values are confined to `make desktop-dev`; they do not become
global Make exports or inputs to `desktop-build`, `desktop-open`, or installed
desktop launches. Both `KANDEV_HOME_DIR` and `KANDEV_DATABASE_PATH` are pinned
because a separate inherited database path would otherwise override the
repo-local home. The SQLite driver is also explicit because the database path
does not select the driver. The E2E selector is set false because the profile
detector gives it precedence over the dev selector. Config-file values for the
home and database settings remain lower priority than the explicit process
environment.

The backend's existing single-owner check handles a simultaneous `make dev`
against the same `.kandev-dev` home. A collision is an error, not a reason to
choose another state root. The normal desktop shutdown and parent-watch
behavior remain unchanged.

## Verification

Test the Make target's effective child environment with clean and inherited
production paths, a PostgreSQL driver, and an enabled E2E selector. A macOS
launch smoke should then prove that the actual backend selected the repo-local
SQLite database, wrote its log under the same home, and used the dev profile.
Confirm `desktop-build` remains unaffected.

## Related designs

- [Platform Go dev launcher](../../platform/system-design/go-dev-launcher.md)
- [Desktop Tauri app](desktop-tauri-app.md)
