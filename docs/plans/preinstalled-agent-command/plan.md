---
created: 2026-11-01
status: complete
requirements:
  - REQ-EXECUTORS-PREINSTALLED-COMMAND-001
system_design:
  - ../../specs/executors/system-design/preinstalled-agent-command.md
legacy_specs: []
---

# Implementation plan: Preinstalled agent command override

## Overview

One work order adds an optional per-profile `command_override` that replaces
the `npx` launch of managed npm agents with a validated absolute path, reusing
the profile environment variables for the node's environment.

## Scope

### In scope

- The profile field, its storage, API and validation.
- `CommandOptions.CommandOverride` and `ManagedNPMRuntimeSpec.CommandFor`.
- Launch and remote preflight wiring, with tests.

### Out of scope

- OpenCode and agents without a managed npm runtime.
- Installing or verifying the executable's contents on the node.
- A user interface field.

## Technical approach

See [the system design](../../specs/executors/system-design/preinstalled-agent-command.md).

## Parallelism

`sequential`

## Work orders

- [x] [Task 01: Launch managed npm agents from a preinstalled command](task-01-command-override.md)
