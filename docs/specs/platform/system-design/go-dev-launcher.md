---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-GO-DEV-LAUNCHER-001
created: 2026-08-26
owners:
  - kandev
---
# Go dev launcher and startup version System Design

## Purpose and boundaries

The platform launcher owns the startup banner shown by `dev`, `start`, and
`run`. This design adds build identity to the existing banner without changing
the backend health contract, service installation metadata, or network binding
behavior documented by the platform launcher requirement.

The launcher consumes build metadata from the native binary entrypoint. It does
not resolve a version from the database, the selected bundle directory, or an
environment variable used only for release display metadata.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-GO-DEV-LAUNCHER-001` | [Build identity](#build-identity) and [Startup flow](#startup-flow) |

## Components and responsibilities

- `apps/backend/cmd/kandev` owns the build-time `Version` variable and passes it
  into `launcher.BuildInfo`.
- `internal/launcher.Run` selects the command and forwards the build version to
  the selected launch path.
- `internal/launcher.runStart` and `internal/launcher.runInstalled` carry the
  version in `managedAppConfig`.
- `internal/launcher.runDev` carries the version through the dev launch path.
- `internal/launcher.logStartup` renders the shared stdout banner.

## Build identity

`launcher.BuildInfo.Version` is the canonical value for the launcher banner. It
is the same value used by the native `--version` path and by the backend build
metadata when the binary is dispatched with `__backend`. The compiled default
is `dev`, so a local or otherwise unstamped binary never prints an empty version.

The existing `KANDEV_VERSION` value remains optional release display metadata for
the installed-release header. It does not override the binary build identity
shown on the `version:` line.

## Startup flow

Each shared launch mode resolves ports, endpoints, database path, and log level
before it calls the banner. The banner prints the existing header first, then a
single line in this shape:

```text
[kandev] version: <build version>
```

The line appears before the URL, network, MCP, database, and log-level lines.
Those existing lines retain their current order and values. The version line is
printed before child startup so it remains available when backend readiness
fails.

## Failure and recovery

If build metadata is empty at an internal call boundary, the launcher uses the
same `dev` fallback as the native binary defaults. Version rendering cannot
prevent launch, change endpoint selection, or change backend readiness behavior.

## Observability

The startup banner exposes the build identity on launcher stdout. `--version`
remains the machine-readable single-value command. The backend `/health` and
`/api/v1/system/info` version contracts remain independent but use the same
build metadata for the dispatched backend process.
