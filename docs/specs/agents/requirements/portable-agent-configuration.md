---
status: active
system: agents
created: 2026-08-15
owners:
  - kandev
---
# Copy agent configuration to isolated executors Requirements

## Overview

An agent can use different configuration on the host and in an isolated executor. This difference can change the models, hooks, permissions, MCP servers, and provider behavior.

## Requirements

### REQ-AGENTS-PORTABLE-AGENT-CONFIGURATION-001: Copy agent configuration to isolated executors

**Intent:** An agent can use different configuration on the host and in an isolated executor. This difference can change the models, hooks, permissions, MCP servers, and provider behavior.

#### Acceptance criteria

- **AC-AGENTS-PORTABLE-AGENT-CONFIGURATION-001.1:** **GIVEN** a Codex agent row, **WHEN** the user selects an authentication method and the Codex configuration bundle, **THEN** both choices remain selected independently.
- **AC-AGENTS-PORTABLE-AGENT-CONFIGURATION-001.2:** **GIVEN** a selected Claude bundle, **WHEN** Kandev creates a fresh Docker environment, **THEN** `.claude/settings.json` exists in the agent home.
- **AC-AGENTS-PORTABLE-AGENT-CONFIGURATION-001.3:** **GIVEN** a selected OpenCode bundle with JSON configuration, **WHEN** Kandev creates an SSH environment, **THEN** Kandev copies it to the remote user home.
- **AC-AGENTS-PORTABLE-AGENT-CONFIGURATION-001.4:** **GIVEN** a selected bundle, **WHEN** the task uses a warm resume, **THEN** Kandev does not copy the host file again.
- **AC-AGENTS-PORTABLE-AGENT-CONFIGURATION-001.5:** **GIVEN** a selected bundle, **WHEN** the user resets the environment, **THEN** Kandev copies the current host file.
- **AC-AGENTS-PORTABLE-AGENT-CONFIGURATION-001.6:** **GIVEN** a missing selected source, **WHEN** Kandev prepares the environment, **THEN** Kandev warns and continues the launch.
- **AC-AGENTS-PORTABLE-AGENT-CONFIGURATION-001.7:** **GIVEN** a phone with a coarse pointer, **WHEN** the user taps the warning icon, **THEN** a drawer shows the full warning.
- **AC-AGENTS-PORTABLE-AGENT-CONFIGURATION-001.8:** **GIVEN** a desktop with a fine pointer, **WHEN** the user focuses the warning icon, **THEN** a tooltip shows the full warning.

## Migrated source detail

## Why

An agent can use different configuration on the host and in an isolated executor.
This difference can change the models, hooks, permissions, MCP servers, and provider behavior.

Users need an optional copy function for small and known configuration files.
Kandev must show the risk before the user enables this function.

## Outcome

An executor profile can select portable configuration bundles for supported agents.
Kandev copies the selected files during fresh provisioning.

The user sees a warning icon and clear risk information.
Authentication choices remain separate from configuration choices.

## Scope

This feature applies to these executor types:

- Local Docker
- SSH
- Sprites

Local and Worktree use the host agent configuration directly.
Remote Docker is not available.

The first release supports these agent files:

| Agent | Source file | Target file |
|---|---|---|
| Claude | `~/.claude/settings.json` | `~/.claude/settings.json` |
| Codex | `~/.codex/config.toml` | `~/.codex/config.toml` |
| OpenCode | `~/.config/opencode/opencode.json` | `~/.config/opencode/opencode.json` |
| OpenCode | `~/.config/opencode/opencode.jsonc` | `~/.config/opencode/opencode.jsonc` |

The tilde in the target column means the agent home in the executor.
Kandev copies each OpenCode file that exists.

## User interface

The executor profile editor shows each supported agent as an expandable row.
The row appears for Local Docker, SSH, and Sprites profiles and contains both
the agent's authentication choices and its configuration bundle choices.
Authentication and configuration remain independent selections. There is no
separate global agent-configuration section.

The configuration choices give this information:

- Kandev copies the selected files without changes.
- A copied file can contain secrets, environment values, hooks, or commands.
- A copied file can change models, permissions, MCP servers, and network endpoints.
- A copied host path can be invalid in the executor.

A warning icon appears beside the configuration choices.
The icon has a translated accessible name.

On desktop, hover or keyboard focus opens a tooltip.
On a coarse pointer, a tap opens a bottom drawer with the same information.

The warning control has a minimum touch target of 44 by 44 CSS pixels.
The control does not depend on hover.

Each supported agent has one checkbox for each declared bundle.
The checkbox is independent from the agent authentication controls.

If no source file exists, the checkbox is disabled for a new selection.
The row shows that Kandev did not find the file on this machine.

If a saved source later disappears, the saved checkbox stays selected.
The user can remove the selection and save the profile.

The existing route-level **Save changes** action stores the selection.
The page shows dirty state before the save.

Desktop and mobile show the same choices and outcomes.
The existing settings page remains the only vertical scroll owner.

## Architecture and communication

The feature uses this data flow:

```text
Executor profile editor
  -> GET /api/v1/agent-config-bundles
  <- bundle metadata and local file availability
  -> save ExecutorProfile.Config.agent_config_bundles

Task launch
  -> orchestrator copies selected bundle IDs into launch metadata
  -> lifecycle resolves IDs against the backend bundle catalog
  -> lifecycle reads allowlisted files from the Kandev host home
  -> Docker, SSH, or Sprites writes files into the executor agent home
  -> agent process reads its normal configuration path
```

The frontend never reads the configuration file data.
The browser receives file paths, labels, bundle IDs, and availability only.

The backend is the authority for the bundle definitions.
Agent integrations declare the supported files and target paths.

The task handler exposes the catalog through HTTP.
The executor profile APIs already store the profile configuration map.

The orchestrator passes selected bundle IDs to the lifecycle manager.
The lifecycle manager owns file checks and transfer rules.

## State and data storage

The executor profile stores selected bundle IDs in `config.agent_config_bundles`.
This value is a JSON array of strings in the existing profile record.

The backend catalog exists in process memory.
Kandev builds it from enabled agent integrations and the current host operating system.

Local file availability is transient.
The backend calculates it for each catalog request.

The copied file data is transient.
Kandev reads it during fresh provisioning and sends it to the selected executor.

Kandev does not store copied file data in SQLite.
Kandev does not send copied file data to the browser.

Docker stores the copy in the Kandev-managed session directory for that environment.
Sprites stores the copy in the sandbox home.
SSH stores the copy in the configured remote user home.

## Lifecycle

The profile selection has these states:

1. **Off:** The profile has no bundle ID. Kandev does not copy the bundle.
2. **Selected:** The profile contains the bundle ID. Kandev copies it during fresh provisioning.
3. **Unavailable:** A selected source does not exist or cannot be read. Kandev warns and continues.
4. **Copied:** The executor contains the file with mode `0600`.

An environment reset starts fresh provisioning and copies current host data.
A warm resume does not copy the files again.

## Transfer and security rules

- Kandev accepts bundle IDs only.
- Kandev does not accept arbitrary source or target paths from the browser.
- Kandev copies regular files only.
- Kandev does not follow symbolic links.
- Kandev does not copy directories, sessions, databases, caches, or lock files.
- One source file has a maximum size of 1 MiB.
- One launch has a maximum combined size of 4 MiB.
- Every target must remain under the executor agent home.
- Kandev writes each target with mode `0600`.
- Fresh provisioning replaces an existing selected target file.

The SSH copy can replace configuration for other processes on the same remote account.
The warning text must include this effect for SSH profiles.

## Failure behavior

If a bundle ID is unknown, Kandev skips the bundle and writes a preparation warning.
The launch continues with the configuration that is already in the executor.

If a source is missing, unreadable, a symbolic link, or too large, Kandev skips it.
Kandev writes a preparation warning without file data.

If a target path is unsafe, Kandev skips it and records an internal configuration error.
The launch continues because the bundle is optional.

If a target write fails, Kandev writes a preparation warning.
The launch continues and the agent uses its remaining configuration.

Warnings can contain the agent name, bundle label, and relative path.
Warnings must not contain file data or secret values.

## Scenarios

- **GIVEN** a Codex agent row, **WHEN** the user selects an authentication method and the Codex configuration bundle, **THEN** both choices remain selected independently.
- **GIVEN** a selected Claude bundle, **WHEN** Kandev creates a fresh Docker environment, **THEN** `.claude/settings.json` exists in the agent home.
- **GIVEN** a selected OpenCode bundle with JSON configuration, **WHEN** Kandev creates an SSH environment, **THEN** Kandev copies it to the remote user home.
- **GIVEN** a selected bundle, **WHEN** the task uses a warm resume, **THEN** Kandev does not copy the host file again.
- **GIVEN** a selected bundle, **WHEN** the user resets the environment, **THEN** Kandev copies the current host file.
- **GIVEN** a missing selected source, **WHEN** Kandev prepares the environment, **THEN** Kandev warns and continues the launch.
- **GIVEN** a phone with a coarse pointer, **WHEN** the user taps the warning icon, **THEN** a drawer shows the full warning.
- **GIVEN** a desktop with a fine pointer, **WHEN** the user focuses the warning icon, **THEN** a tooltip shows the full warning.
- **GIVEN** a saved selection, **WHEN** the user reloads the profile page, **THEN** the same checkbox stays selected.

## Compatibility

Codex `auth.json` remains available as an authentication file.
Codex `config.toml` moves to the independent configuration bundle.

Kandev does not migrate old profiles to the new configuration selection.
Users must enable the copy when they want `config.toml` in a new environment.

This change removes the implicit Local Docker copy of provider configuration.
It keeps explicit authentication choices and secret references unchanged.

## Permissions

The existing executor profile permissions control catalog access and profile changes.
This feature does not add a new user role or permission.

## Required companion behavior

Configuration copying can improve model parity, but it cannot guarantee parity.
The executor can still advertise fewer models because credentials, versions, accounts, or network access differ.

The [no-silent-model-fallback specification](no-silent-model-fallback.md) handles that case.
If the executor omits the profile model, Kandev starts the agent default and persists a chat warning.

The companion behavior works when copying is off, unavailable, incomplete, or stale.
Portable configuration is not a launch requirement.

## Out of scope

- Arbitrary user-defined file paths.
- Complete agent-home copies.
- Project-level configuration inside a repository.
- File merge, template, edit, or content preview functions.
- Secret detection or content sanitization.
- The model-selection implementation in the companion specification.
- Configuration synchronization after a warm resume.
