---
status: active
system: platform
created: 2026-08-12
owners:
  - kandev
---
# Setup and Launch Timeout Requirements

## Overview

Environment setup can install dependencies or build a large project. A valid setup that runs for more than one minute must not fail because a shorter, hidden session-launch deadline expires first.

## Requirements

### REQ-PLATFORM-SETUP-LAUNCH-TIMEOUT-001: Setup and Launch Timeout

**Intent:** Environment setup can install dependencies or build a large project. A valid setup that runs for more than one minute must not fail because a shorter, hidden session-launch deadline expires first.

#### Acceptance criteria

- **AC-PLATFORM-SETUP-LAUNCH-TIMEOUT-001.1:** Kandev allows setup and prepare scripts to run for 10 minutes by default.
- **AC-PLATFORM-SETUP-LAUNCH-TIMEOUT-001.2:** Operators can set `tasks.preparationTimeout` in startup YAML or use `KANDEV_TASK_PREPARATION_TIMEOUT` as its environment override. The value uses Go duration syntax, such as `90s`, `10m`, or `1h`.
- **AC-PLATFORM-SETUP-LAUNCH-TIMEOUT-001.3:** The environment value overrides YAML when both are present.
- **AC-PLATFORM-SETUP-LAUNCH-TIMEOUT-001.4:** The configured value must be greater than zero. An unset, invalid, zero, or negative value uses the 10-minute default.
- **AC-PLATFORM-SETUP-LAUNCH-TIMEOUT-001.5:** Kandev reads the resolved startup value when the backend process starts. A change requires a restart.
- **AC-PLATFORM-SETUP-LAUNCH-TIMEOUT-001.6:** The timeout applies to repository setup scripts and executor-profile prepare scripts on Local, Worktree, Docker, Sprite, and SSH launches.
- **AC-PLATFORM-SETUP-LAUNCH-TIMEOUT-001.7:** Runtime launch phases use the setup timeout plus a fixed five-minute allowance. Each launch-phase context starts when that phase begins.
- **AC-PLATFORM-SETUP-LAUNCH-TIMEOUT-001.8:** Preparation scripts use their own setup context. Runtime setup work such as Sprite uploads does not reduce the preparation-script budget.

## Migrated source detail

## Why

Environment setup can install dependencies or build a large project. A valid
setup that runs for more than one minute must not fail because a shorter,
hidden session-launch deadline expires first.

## What

- Kandev allows setup and prepare scripts to run for 10 minutes by default.
- Operators can set `tasks.preparationTimeout` in startup YAML or use
  `KANDEV_TASK_PREPARATION_TIMEOUT` as its environment override. The value uses
  Go duration syntax, such as `90s`, `10m`, or `1h`.
- The environment value overrides YAML when both are present.
- The configured value must be greater than zero. An unset, invalid, zero, or
  negative value uses the 10-minute default.
- Kandev reads the resolved startup value when the backend process starts. A
  change requires a restart.
- The timeout applies to repository setup scripts and executor-profile prepare
  scripts on Local, Worktree, Docker, Sprite, and SSH launches.
- Runtime launch phases use the setup timeout plus a fixed five-minute
  allowance. Each launch-phase context starts when that phase begins.
- Preparation scripts use their own setup context. Runtime setup work such as
  Sprite uploads does not reduce the preparation-script budget.
- Manager shutdown still cancels a shared launch, and one caller leaving does
  not cancel work that another caller still needs.
- A setup-script failure keeps the runtime-specific behavior documented in
  [Executors](../../../public/executors.md). This change does not make a non-fatal
  setup failure fatal.

Decision: [ADR-2026-08-12-setup-timeout-owns-launch-budget](../../../decisions/2026-08-12-setup-timeout-owns-launch-budget.md).

## Failure modes

| Condition | Behavior |
|---|---|
| `tasks.preparationTimeout` and `KANDEV_TASK_PREPARATION_TIMEOUT` are absent | Kandev uses 10 minutes. |
| The value is invalid, zero, or negative | Kandev uses 10 minutes. |
| A setup script reaches the configured limit | Kandev stops the script and reports the existing runtime-specific setup failure. |
| A runtime launch phase does not finish within its derived launch limit | The launch fails with a deadline error and releases its activity lease. |
| The backend stops during launch | Manager cancellation stops the shared launch. |

## Scenarios

- **GIVEN** no timeout override and a repository setup script that takes 90
  seconds, **WHEN** a user launches a session, **THEN** setup completes without
  a one-minute session-launch deadline error.
- **GIVEN** `tasks.preparationTimeout: 15m` at process start and a prepare
  script that takes more than 10 minutes but less than 15 minutes, **WHEN** a
  user launches a session, **THEN** Kandev allows the script to complete.
- **GIVEN** an invalid, zero, or negative timeout value, **WHEN** Kandev starts,
  **THEN** launches use the 10-minute setup limit and the 15-minute launch
  limit.
- **GIVEN** a setup script that exceeds the configured limit, **WHEN** the limit
  expires, **THEN** Kandev stops the script and preserves that runtime's current
  fatal or non-fatal setup-failure behavior.
- **GIVEN** a runtime launch phase blocks after setup, **WHEN** its derived
  launch limit expires, **THEN** the launch returns a deadline error and
  releases the manager activity lease.
- **GIVEN** two callers wait for one shared launch, **WHEN** one caller cancels,
  **THEN** the launch continues for the remaining caller until it completes,
  the manager stops, or the derived launch limit expires.

## Out of scope

- Separate timeout settings for each executor or hook.
- A configurable cleanup-script timeout.
- Database, API, or Settings UI configuration.
- Changing whether a setup-script failure is fatal for a runtime.
