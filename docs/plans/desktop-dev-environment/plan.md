---
created: 2026-09-26
status: in_progress
requirements:
  - REQ-DESKTOP-DEV-ENV-001
system_design:
  - ../../specs/desktop/system-design/desktop-dev-environment.md
legacy_specs: []
---

# Implementation Plan: Desktop Dev Environment

## Overview

Make `make desktop-dev` select the repo-local development home, SQLite
database, and runtime profile automatically. Keep the existing Tauri shell and
bundled backend launch path. One work order owns the Make recipe, its launch
evidence, and the contributor documentation.

## Scope

### In scope

- Default `desktop-dev` to `<checkout>/.kandev-dev`, its SQLite database, and
  the dev profile.
- Override ambient home and database paths for this target so a task shell
  cannot redirect desktop development into production state.
- Keep `desktop-build`, `desktop-open`, and installed desktop behavior unchanged.
- Document the desktop development command and its state location.

### Out of scope

- Product UI and Go backend live reload inside the Tauri window.
- A separate desktop-only development database. Sharing `.kandev-dev` with
  `make dev` is intentional; run the two commands one at a time.
- Changes to the platform `make dev` database override or backup policy.

## Technical approach

In the root `Makefile`, set `KANDEV_HOME_DIR`, `KANDEV_DATABASE_PATH`, and
`KANDEV_DEBUG_DEV_MODE` on the `desktop-dev` recipe's Tauri invocation. Use
`$(CURDIR)` to identify the checkout. Keep those values scoped to that recipe,
after `desktop-runtime` has prepared the local bundle. The existing Tauri
`desktop_environment` function forwards them to the bundled native launcher,
and the backend configuration gives those environment values precedence over
YAML defaults.

Add a targeted Make command test for a clean environment and inherited
production paths. Extend contributor documentation with the default home,
database, profile, log location, and single-owner behavior. Use a macOS launch
smoke to confirm the actual backend path and profile; the Make command test
alone cannot prove the full process handoff.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| AC-DESKTOP-DEV-ENV-001.1 to 001.3 | `scripts/desktop-dev-env.test.sh` checks the effective Tauri child environment; macOS launch smoke checks the backend's selected database, log location, and dev profile. |
| AC-DESKTOP-DEV-ENV-001.4 | The targeted Make test checks `desktop-build` has no development environment assignment; source inspection confirms installed launches do not use the Make target. |
| AC-DESKTOP-DEV-ENV-001.5 | macOS smoke starts `make dev` first, then confirms a concurrent `make desktop-dev` reports the existing home-owner failure without opening another database. |

## E2E tests

- The macOS smoke for `AC-DESKTOP-DEV-ENV-001.1` to `001.5` launches the real
  Tauri app and bundled Go backend. It records the resolved database, log
  path, profile, and contention result. It does not use the Linux desktop
  release smoke, which does not exercise the `desktop-dev` Make target.

## Work orders

- [ ] [Task 01: Default desktop dev to repo-local state](task-01-default-desktop-dev-state.md)

## Verification results

- `bash scripts/desktop-dev-env.test.sh` — passed for clean and inherited
  environment values; `desktop-build` remains free of the dev overrides.
- `make test-scripts` — passed, including the desktop environment test.
- `node scripts/validate-public-docs.mjs` — passed (47 published pages).
- `make build` — passed for the web app, backend, and bundled runtime helpers.
- The macOS app launch and shared-home contention smoke could not run because
  this implementation host is Linux x86_64. Record that smoke in the work order
  when a macOS development host is available.

## Risks

- A developer already running `make dev` cannot concurrently open
  `make desktop-dev` against the same `.kandev-dev` home. The existing
  single-owner error must remain visible.
- `make desktop-dev` still builds an embedded runtime before launch. Source
  edits to the Go backend or product UI require restarting the target.
- This checkout runs on Linux, so the macOS launch smoke must execute on a
  macOS host during implementation.
