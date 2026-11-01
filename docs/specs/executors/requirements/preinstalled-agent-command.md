---
status: active
system: executors
created: 2026-11-01
owners:
  - kandev
---

# Preinstalled agent command override

## Overview

Managed npm agents, such as the Codex app server, start through `npx` with an
exact package version. A node with an empty home directory, no network and a
read-only toolchain cannot run that launch. An operator can instead name an
executable that is already installed on the node and give the agent profile the
environment it needs.

## Terminology

- **Command override:** An operator-configured absolute path to a preinstalled
  agent executable on an agent profile.
- **Runtime arguments:** The arguments the managed npm package's executable
  receives, for example `app-server`.

## Requirements

### REQ-EXECUTORS-PREINSTALLED-COMMAND-001: Preinstalled command launch

**Intent:** An operator can launch a managed npm agent from a preinstalled
executable without `npx`, a package registry or a shell.

#### Acceptance criteria

- **AC-EXECUTORS-PREINSTALLED-COMMAND-001.1:** Where an agent profile sets a
  command override and its agent runs from a managed npm runtime, the system
  shall launch the override path followed by the agent's runtime arguments and
  shall not run `npx`.
- **AC-EXECUTORS-PREINSTALLED-COMMAND-001.2:** When a profile sets no command
  override, the system shall launch the agent exactly as before.
- **AC-EXECUTORS-PREINSTALLED-COMMAND-001.3:** If a command override is not a
  clean absolute path, or contains whitespace, control characters or shell
  syntax, then the system shall reject it when the profile is saved and when
  the agent launches, and shall not fall back to `npx`.
- **AC-EXECUTORS-PREINSTALLED-COMMAND-001.4:** If the agent does not run from a
  managed npm runtime, then the system shall refuse a launch whose profile sets
  a command override.
- **AC-EXECUTORS-PREINSTALLED-COMMAND-001.5:** The profile's environment
  variables shall reach the launched agent with or without a command override.
- **AC-EXECUTORS-PREINSTALLED-COMMAND-001.6:** Remote preflight shall check the
  command override instead of `npx` when the profile sets a valid one.
