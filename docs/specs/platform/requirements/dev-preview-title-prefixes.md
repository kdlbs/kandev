---
status: active
system: platform
created: 2026-08-09
owners:
  - Kandev
---
# Environment-specific browser tab title prefixes Requirements

## Overview

Developers often keep a local development instance and one or more pull-request preview instances open at the same time. Identical `Kandev` browser tabs make it easy to use the wrong instance.

## Requirements

### REQ-PLATFORM-DEV-PREVIEW-TITLE-PREFIXES-001: Environment-specific browser tab title prefixes

**Intent:** Developers often keep a local development instance and one or more pull-request preview instances open at the same time. Identical `Kandev` browser tabs make it easy to use the wrong instance.

#### Acceptance criteria

- **AC-PLATFORM-DEV-PREVIEW-TITLE-PREFIXES-001.1:** A Kandev browser tab title uses the existing `<prefix> Kandev` composition.
- **AC-PLATFORM-DEV-PREVIEW-TITLE-PREFIXES-001.2:** A backend started in the development profile, including `make dev`, uses the default prefix `Dev`, so its tab title is `Dev Kandev`.
- **AC-PLATFORM-DEV-PREVIEW-TITLE-PREFIXES-001.3:** A PR preview environment uses the prefix `Preview`, so its tab title is `Preview Kandev`.
- **AC-PLATFORM-DEV-PREVIEW-TITLE-PREFIXES-001.4:** An explicitly supplied `KANDEV_WEB_TITLE_PREFIX` continues to take precedence over a profile default. Its value is trimmed by the existing title-prefix contract.
- **AC-PLATFORM-DEV-PREVIEW-TITLE-PREFIXES-001.5:** A normal production/start launch with no explicit prefix keeps the plain `Kandev` title.
- **AC-PLATFORM-DEV-PREVIEW-TITLE-PREFIXES-001.6:** A debug start (`make start-debug`) can enable pprof and debug logging without selecting the development profile. It uses the explicit prefix `Debug` by default, so its title is `Debug Kandev`.
- **AC-PLATFORM-DEV-PREVIEW-TITLE-PREFIXES-001.7:** The prefix is present both on the initial server-rendered page and after a client boot from `/api/v1/app-state`, using the title behavior delivered by PR #2459.
- **AC-PLATFORM-DEV-PREVIEW-TITLE-PREFIXES-001.8:** A supervised backend restart keeps the configured prefix.

## Migrated source detail

The profile-selection boundary follows [ADR-2026-08-10-debug-launcher-profile-selection](../../../decisions/2026-08-10-debug-launcher-profile-selection.md).

## Why

Developers often keep a local development instance and one or more pull-request
preview instances open at the same time. Identical `Kandev` browser tabs make it
easy to use the wrong instance.

## What

- A Kandev browser tab title uses the existing `<prefix> Kandev` composition.
- A backend started in the development profile, including `make dev`, uses the
  default prefix `Dev`, so its tab title is `Dev Kandev`.
- A PR preview environment uses the prefix `Preview`, so its tab title is
  `Preview Kandev`.
- An explicitly supplied `KANDEV_WEB_TITLE_PREFIX` continues to take precedence
  over a profile default. Its value is trimmed by the existing title-prefix
  contract.
- A normal production/start launch with no explicit prefix keeps the plain
  `Kandev` title.
- A debug start (`make start-debug`) can enable pprof and debug logging without
  selecting the development profile. It uses the explicit prefix `Debug` by
  default, so its title is `Debug Kandev`.
- The prefix is present both on the initial server-rendered page and after a
  client boot from `/api/v1/app-state`, using the title behavior delivered by
  PR #2459.
- A supervised backend restart keeps the configured prefix.

## API surface

The existing environment contract remains the public configuration surface:

| Environment | `KANDEV_WEB_TITLE_PREFIX` | Result |
|---|---|---|
| Development profile (`make dev`) | `Dev` by default | `Dev Kandev` |
| PR preview launcher | `Preview` | `Preview Kandev` |
| Debug start (`make start-debug`) | `Debug` by default | `Debug Kandev` |
| Normal start with no override | unset | `Kandev` |
| Any launch with an explicit override | caller value | `<value> Kandev` |

The prefix is process configuration. It is not stored in the database and is
not changed from the Kandev UI.

## Failure modes

- If the prefix is unset or blank, the title remains `Kandev`.
- `KANDEV_DEBUG_PPROF_ENABLED=true` enables legacy pprof behavior but does not
  select the development profile or add the `Dev` prefix. The debug launcher
  supplies the `Debug` prefix unless the caller supplies another prefix.
- If the preview launcher cannot set the environment variable, the preview
  falls back to the existing plain-title behavior rather than failing startup.
- If a supervised restart occurs, the launcher preserves the allowlisted prefix
  in the restart manifest so the restarted backend does not silently change its
  title.

## Persistence guarantees

The title prefix lasts for the lifetime of the launched environment and its
supervised restarts. It is not persisted across separate launches unless the
caller supplies the same environment or profile again.

## Scenarios

- **GIVEN** a clean checkout with no title-prefix override, **WHEN** the user
  runs `make dev` and opens the Kandev URL, **THEN** the browser title is
  `Dev Kandev`.
- **GIVEN** a PR preview deployment, **WHEN** its startup service launches
  Kandev, **THEN** the browser title is `Preview Kandev`.
- **GIVEN** a development profile and `KANDEV_WEB_TITLE_PREFIX=Custom`, **WHEN**
  the backend starts, **THEN** the browser title is `Custom Kandev`.
- **GIVEN** a normal start with no title-prefix environment variable, **WHEN**
  the user opens Kandev, **THEN** the browser title is `Kandev`.
- **GIVEN** a debug start with pprof and debug logging enabled but without the
  development-profile selector, **WHEN** the user opens Kandev, **THEN** the
  browser title is `Debug Kandev`.
- **GIVEN** a preview or development backend with a configured prefix, **WHEN**
  its supervisor restarts the backend, **THEN** the browser title retains the
  same prefix.
- **GIVEN** a phone-sized browser viewport, **WHEN** either environment is
  opened, **THEN** the same title is shown. No viewport-specific interaction is
  required.

## Out of scope

- Changing the title dynamically from the Kandev UI.
- Changing desktop window titles, CLI output, logs, or task titles.
- Automatically labeling arbitrary self-hosted instances beyond the existing
  explicit environment variable.
