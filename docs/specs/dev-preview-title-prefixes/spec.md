---
status: shipped
created: 2026-08-09
owner: Kandev
---

# Environment-specific browser tab title prefixes

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
| Normal start with no override | unset | `Kandev` |
| Any launch with an explicit override | caller value | `<value> Kandev` |

The prefix is process configuration. It is not stored in the database and is
not changed from the Kandev UI.

## Failure modes

- If the prefix is unset or blank, the title remains `Kandev`.
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
- **GIVEN** a preview or development backend with a configured prefix, **WHEN**
  its supervisor restarts the backend, **THEN** the browser title retains the
  same prefix.
- **GIVEN** a phone-sized browser viewport, **WHEN** either environment is
  opened, **THEN** the same title is shown; no viewport-specific interaction is
  required.

## Out of scope

- Changing the title dynamically from the Kandev UI.
- Changing desktop window titles, CLI output, logs, or task titles.
- Automatically labeling arbitrary self-hosted instances beyond the existing
  explicit environment variable.
