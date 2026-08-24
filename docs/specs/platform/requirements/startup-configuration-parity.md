---
status: active
system: platform
created: 2026-08-20
owners:
  - kandev
---
# Startup Configuration Parity Requirements

## Overview

Kandev has two startup configuration paths. Some settings use typed YAML and environment variables. Other settings read only environment variables at their consumer. Operators must then maintain service-specific environment setup even when they already use `config.yaml`.

## Requirements

### REQ-PLATFORM-STARTUP-CONFIGURATION-PARITY-001: Startup Configuration Parity

**Intent:** Kandev has two startup configuration paths. Some settings use typed YAML and environment variables. Other settings read only environment variables at their consumer. Operators must then maintain service-specific environment setup even when they already use `config.yaml`.

#### Acceptance criteria

- **AC-PLATFORM-STARTUP-CONFIGURATION-PARITY-001.1:** **GIVEN** `~/.kandev/config.yaml` contains `server.trustedProxies`, **WHEN** Kandev starts behind the listed proxy, **THEN** it trusts forwarded headers from that proxy without an environment variable.
- **AC-PLATFORM-STARTUP-CONFIGURATION-PARITY-001.2:** **GIVEN** YAML and an environment variable set the same option, **WHEN** Kandev starts, **THEN** the environment value wins.
- **AC-PLATFORM-STARTUP-CONFIGURATION-PARITY-001.3:** **GIVEN** a launch flag, environment variable, and YAML set the backend port, **WHEN** Kandev starts, **THEN** the launch flag wins.
- **AC-PLATFORM-STARTUP-CONFIGURATION-PARITY-001.4:** **GIVEN** working-directory, home, and system configuration files exist, **WHEN** Kandev starts, **THEN** it reads only the working-directory file.
- **AC-PLATFORM-STARTUP-CONFIGURATION-PARITY-001.5:** **GIVEN** the working-directory file is absent and the home file exists, **WHEN** Kandev starts, **THEN** it reads only the home file.
- **AC-PLATFORM-STARTUP-CONFIGURATION-PARITY-001.6:** **GIVEN** the first existing file is invalid, **WHEN** Kandev starts, **THEN** startup fails without reading a lower-priority file.
- **AC-PLATFORM-STARTUP-CONFIGURATION-PARITY-001.7:** **GIVEN** `database.path` is set in the selected file, **WHEN** the launcher starts the backend, **THEN** the backend uses that path.
- **AC-PLATFORM-STARTUP-CONFIGURATION-PARITY-001.8:** **GIVEN** a stable startup secret is set in YAML, **WHEN** Kandev starts, **THEN** the owning component receives it and logs do not expose it.

## Migrated source detail

## Why

Kandev has two startup configuration paths. Some settings use typed YAML and
environment variables. Other settings read only environment variables at their
consumer. Operators must then maintain service-specific environment setup even
when they already use `config.yaml`.

This split also causes inconsistent launcher behavior. For example, the
launcher can replace a database path that the backend would otherwise read from
YAML. A stable configuration contract must apply before the launcher starts the
backend and continue through every managed process.

## What

Kandev gives every stable, operator-facing startup setting a canonical YAML key.
Each setting keeps its current environment variable as a compatible override.

The effective startup precedence is:

1. A command-line launch flag, when that setting has one.
2. An explicit environment variable.
3. The selected `config.yaml` value.
4. The existing built-in or profile default.

For settings that also have a database override, the environment variable and
YAML value both take precedence over the database value. The full order is
environment variable, YAML, database override, then profile or built-in default.

Kandev selects the first existing configuration file in this order:

1. `config.yaml` in the process working directory.
2. `<KANDEV_HOME_DIR>/config.yaml`, or `~/.kandev/config.yaml` when the home
   directory has no explicit override.
3. `/etc/kandev/config.yaml`.

Kandev does not merge these files. If the first existing candidate is invalid or
unreadable, startup fails and reports that file. Kandev does not continue to a
lower-priority candidate.

The home directory is bootstrap configuration. A launch flag or the explicit
`KANDEV_HOME_DIR` environment variable selects it before Kandev searches for the
home configuration file. A configuration file in the working directory or in
`/etc/kandev` can set `homeDir`. The selected home configuration file cannot use
`homeDir` to relocate itself.

Configuration changes require a restart. Kandev does not watch configuration
files.

## Canonical additions

The existing typed configuration keys remain canonical. The following stable
environment-only settings gain YAML keys:

| YAML key | Type and valid YAML value | Default | Compatible environment variable |
|---|---|---|---|
| `server.trustedProxies` | List of valid IP or CIDR strings | Empty list | `KANDEV_TRUSTED_PROXIES` |
| `tasks.preparationTimeout` | Positive Go duration string | `10m` | `KANDEV_TASK_PREPARATION_TIMEOUT` |
| `credentials.file` | File path string. Empty disables the provider. | Empty | `KANDEV_CREDENTIALS_FILE` |
| `limits.ghMaxConcurrent` | Positive integer | `8` | `KANDEV_GH_MAX_CONCURRENT` |
| `limits.gitMaxConcurrent` | Positive integer | `12` | `KANDEV_GIT_MAX_CONCURRENT` |
| `limits.lspMaxConnections` | Positive integer | `8` | `KANDEV_LSP_MAX_CONNECTIONS` |
| `messageQueue.maxPerSession` | Integer of zero or greater. Zero disables the cap. | `10` | `KANDEV_QUEUE_MAX_PER_SESSION` |
| `agentctl.idleTimeout` | Nonnegative Go duration string. Zero disables reaping. | `1h` | `KANDEV_ACP_IDLE_TIMEOUT` |
| `agentctl.idleReaperInterval` | Positive Go duration string | `1m` | `KANDEV_ACP_IDLE_REAPER_INTERVAL` |
| `agentctl.notificationQueueCapacity` | Integer from `1024` through `131072` | `131072` | `KANDEV_ACP_NOTIF_QUEUE` |
| `planning.coalesceWindowMs` | Integer of zero or greater | `300000` | `KANDEV_PLAN_COALESCE_WINDOW_MS` |
| `office.schedulerTickMs` | Positive integer | `5000` | `KANDEV_OFFICE_SCHEDULER_TICK_MS` |
| `observability.otlpEndpoint` | Endpoint string. Empty disables tracing. | Empty | `OTEL_EXPORTER_OTLP_ENDPOINT` |
| `launcher.webPort` | Integer from `1` through `65535`, or absent for automatic selection | Automatic | `KANDEV_WEB_PORT` |
| `launcher.healthTimeoutMs` | Positive integer | `45000` in release, `600000` in dev | `KANDEV_HEALTH_TIMEOUT_MS` |
| `launcher.noBrowser` | Boolean | `false` | `KANDEV_NO_BROWSER` |

YAML uses native lists, integers, and booleans as shown above. Duration values
use strings so their units remain visible. The trusted proxy environment value
keeps its comma-separated syntax. Numeric and boolean environment values keep
their current string syntax and compatibility parsing.

The launcher also honors the existing YAML keys `homeDir`, `server.host`,
`server.port`, `database.path`, and `logging.level`. `server.port` is the YAML
equivalent for `KANDEV_BACKEND_PORT`, compatibility variable `KANDEV_PORT`, and
the internal backend port handoff.

The published configuration reference lists every supported stable startup
environment variable. Each row names its YAML equivalent or a justified
exclusion.

## Stable secrets

Stable startup secrets get the same YAML parity as other stable startup
settings. This includes existing typed database, authentication, and office
secrets. It does not include credentials that users manage in the product UI or
database.

Kandev never logs secret values. On Unix systems, Kandev warns when the selected
configuration file contains a secret and group or other users can read it. The
warning does not block startup. Public guidance recommends owner-only mode
`0600` and an environment-based secret manager where available.

## Configuration ownership

One typed configuration catalog maps stable operator settings to YAML keys,
environment variables, defaults, sensitivity, and owning runtime component.
Completeness tests prevent a stable setting from bypassing this catalog.

Kandev does not copy YAML values into public process environment variables.
The launcher reads the bootstrap subset from the same catalog. The backend
loads the full typed configuration. Managed agentctl processes receive the
resolved settings that they own through explicit startup configuration.

## Message queue source

When `messageQueue.maxPerSession` comes from YAML, the Settings UI treats it as
an operator lock. The API reports `configuration` as the source. The UI names
the configuration file as the source and does not claim that an environment
variable set the value.

The existing responsive Settings layout remains unchanged. Desktop and mobile
users receive the same source and lock behavior.

## Failure modes

| Condition | Behavior |
|---|---|
| No configuration candidate exists | Kandev starts with environment, profile, and built-in values. |
| The first existing candidate is unreadable | Startup fails and names the selected file. |
| The first existing candidate has invalid YAML or an invalid typed value | Startup fails and names the setting and file. |
| A legacy environment value is invalid | Kandev keeps that setting's current compatibility behavior. |
| A home configuration file contains `homeDir` | Startup fails with a relocation error. |
| A configuration file contains a secret and has broad Unix read permissions | Kandev warns without revealing the value, then continues. |
| A managed agentctl process cannot receive valid resolved configuration | Its launch fails instead of silently using a different value. |

## Scenarios

- **GIVEN** `~/.kandev/config.yaml` contains
  `server.trustedProxies`, **WHEN** Kandev starts behind the listed proxy,
  **THEN** it trusts forwarded headers from that proxy without an environment
  variable.
- **GIVEN** YAML and an environment variable set the same option, **WHEN**
  Kandev starts, **THEN** the environment value wins.
- **GIVEN** a launch flag, environment variable, and YAML set the backend port,
  **WHEN** Kandev starts, **THEN** the launch flag wins.
- **GIVEN** working-directory, home, and system configuration files exist,
  **WHEN** Kandev starts, **THEN** it reads only the working-directory file.
- **GIVEN** the working-directory file is absent and the home file exists,
  **WHEN** Kandev starts, **THEN** it reads only the home file.
- **GIVEN** the first existing file is invalid, **WHEN** Kandev starts, **THEN**
  startup fails without reading a lower-priority file.
- **GIVEN** `database.path` is set in the selected file, **WHEN** the launcher
  starts the backend, **THEN** the backend uses that path.
- **GIVEN** a stable startup secret is set in YAML, **WHEN** Kandev starts,
  **THEN** the owning component receives it and logs do not expose it.
- **GIVEN** a secret-bearing file is readable by other users on Unix, **WHEN**
  Kandev loads it, **THEN** the logs contain a permission warning without the
  secret value.
- **GIVEN** `messageQueue.maxPerSession` is set in YAML, **WHEN** a desktop or
  mobile user opens Message Queue settings, **THEN** the field is locked and
  identifies configuration as its source.
- **GIVEN** agentctl timeout or tracing settings are set in YAML, **WHEN**
  Kandev launches agentctl through a supported executor, **THEN** that process
  uses the resolved values.
- **GIVEN** an internal, generated, test, or debug-only environment variable,
  **WHEN** the configuration completeness test runs, **THEN** its recorded
  exclusion satisfies the catalog without creating a public YAML key.

## Out of scope

- A `.env` file loader.
- A public `--config` flag.
- Merging multiple configuration files.
- Live reload or a file watcher.
- A Settings UI editor for startup configuration.
- YAML keys for profile selectors, mocks, E2E controls, or debug-only controls.
- YAML keys for generated tokens, launcher-to-child wiring, packaging metadata,
  bundle discovery, workspace or session injection, or deprecated no-op values.
- YAML keys for credentials managed in the product UI or database.
- Replacing standard platform detection when an existing typed setting already
  controls the result, such as `DOCKER_HOST` with `docker.host`.

Decision:
[Startup configuration uses one typed source model](../../../decisions/2026-08-20-startup-configuration-source-parity.md).
