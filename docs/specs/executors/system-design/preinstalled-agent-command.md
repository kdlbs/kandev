---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-PREINSTALLED-COMMAND-001
---

# Preinstalled agent command override

## Purpose and boundaries

This design adds one optional field, `command_override`, to the agent profile
and threads it into command construction. It reuses the profile `env_vars`,
the `CommandOptions` plumbing and `ManagedNPMRuntimeAgent`. It adds no
executor, no setting outside the profile, and no shell handling.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-EXECUTORS-PREINSTALLED-COMMAND-001 | Configuration, Validation, Command construction, Environment, Verification |

## Configuration

An operator sets `command_override` on an agent profile through the profile
API (`POST`/`PATCH` profile bodies, and agent creation profiles) and sets the
node's environment through the profile's existing `env_vars`, for example
`CODEX_HOME`. The value is stored in `agent_profiles.command_override`; empty
means no override.

## Validation

`agents.ValidateCommandOverride` accepts the empty string or one clean absolute
path. It rejects relative paths, uncleaned paths, whitespace, control
characters and shell metacharacters. Values are never trimmed. The settings
controller applies it on create and update and answers HTTP 400. The lifecycle
applies it again at launch, so a stored bad value fails the launch.

## Command construction

`ManagedNPMRuntimeSpec.CommandFor` returns the override path plus the spec's
runtime arguments when `CommandOptions.CommandOverride` is set, and otherwise
the unchanged `npx` command. Every managed npm agent's `BuildCommand` calls it.
`applyCommandOverride` in `lifecycle/manager_launch.go` copies the profile
value into the options, clears the native binary preference, and fails closed
for agents without a managed npm runtime and for OpenCode, which selects its own
runtime family. The result is argv; the command builder appends profile CLI
flags as before. Remote preflight receives the validated value through
`ExecutorCreateRequest.CommandOverride`.

## Environment

The override changes only argv. Profile `env_vars` follow the existing merge
and are forwarded to plugin-launched nodes as the existing agent environment.

## Verification

Table tests cover validation, `CommandFor`, launch command construction for
standalone, plugin remote, SSH and Docker runtimes, the unchanged default,
rejection cases, environment merge, store round trip and controller validation.
