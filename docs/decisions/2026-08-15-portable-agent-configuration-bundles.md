# ADR-2026-08-15: Use Explicit Portable Agent Configuration Bundles

**Status:** accepted
**Date:** 2026-08-15
**Area:** backend, frontend, protocol, security

## Context

Agent tools read user configuration from files in the user home directory.
An isolated executor does not share these files with the Kandev host.

This difference can change models, hooks, MCP servers, permissions, and provider behavior.
The difference applies to Claude, Codex, OpenCode, and other agent providers.

Kandev already transfers selected authentication files to isolated executors.
Authentication and configuration have different risks and different user choices.

Some configuration files can contain secrets, environment values, or commands.
Some files also contain host paths that do not work in another environment.

Kandev needs an optional transfer function for simple configuration files.
This function must not copy a complete agent home or accept arbitrary host paths.

## Decision

### Agent integrations declare portable bundles

Each agent integration can declare zero or more portable configuration bundles.
A bundle has a stable ID, a label, and one or more file mappings.

Each file mapping contains these values:

- An operating-system-specific source path under the Kandev host home.
- A target path under the agent home in the executor.
- A file size limit.

Kandev controls this allowlist in source code.
The profile API does not accept user-defined source paths or target paths.

The first release contains these bundles:

| Agent | Host source | Executor target |
|---|---|---|
| Claude | `.claude/settings.json` | `.claude/settings.json` |
| Codex | `.codex/config.toml` | `.codex/config.toml` |
| OpenCode | `.config/opencode/opencode.json` | `.config/opencode/opencode.json` |
| OpenCode | `.config/opencode/opencode.jsonc` | `.config/opencode/opencode.jsonc` |

The OpenCode bundle copies each listed file that exists.
At least one listed OpenCode file must exist before a new selection is available.

An agent without a safe bundle does not show a configuration option.
The agent integration can add a bundle after its file contract is known.

### Configuration selection is independent from authentication

An executor profile stores selected bundle IDs in `config.agent_config_bundles`.
The value is a JSON array of stable bundle IDs.

The profile can select authentication and configuration independently.
For example, a Codex profile can use `OPENAI_API_KEY` and copy `config.toml`.

The Codex authentication bundle will contain `auth.json` only.
Kandev will stop treating `config.toml` as an authentication file.

Existing profiles do not get a configuration selection automatically.
This rule removes the current implicit Docker copy of Codex `config.toml`.

### The backend owns discovery and transfer

The backend builds the bundle catalog from enabled agent integrations.
It also checks which declared host files exist.

The frontend reads this catalog from `GET /api/v1/agent-config-bundles`.
The response contains metadata and availability only.
The response never contains file data.

The executor profile save API stores bundle IDs in the existing profile configuration map.
No new database table or migration is necessary.

During fresh provisioning, the lifecycle manager resolves the selected IDs again.
Then it reads the current host files and copies them to the executor.

The file data exists in memory during transfer.
Kandev does not store the file data in SQLite or in the executor profile.

### Transfer rules

The transfer applies to Local Docker, SSH, and Sprites executors.
Local and Worktree executors already use the host agent configuration.
Remote Docker remains unavailable.

Kandev copies configuration only during fresh environment provisioning.
An environment reset also causes a new copy.
A warm resume uses the files that already exist in the executor.

Kandev overwrites a selected target file during fresh provisioning.
For SSH, this target is under the configured remote user home.
This overwrite can affect other processes that use the same remote account.

Kandev copies only regular files.
It does not follow symbolic links or copy directories.

Each source file has a limit of 1 MiB.
All selected files for one launch have a combined limit of 4 MiB.

The transfer rejects absolute targets and path traversal.
The transfer writes target files with mode `0600`.

Kandev does not parse or remove parts of a selected file.
The agent provider remains the authority for the file format and behavior.

If a selected source disappears, Kandev skips that source and writes a preparation warning.
If a target write fails, Kandev writes a preparation warning and continues the launch.
The agent then uses the configuration that remains in the executor.

### The user interface shows the risk

The executor profile editor shows each supported agent as an expandable row.
Each expanded row contains the agent's authentication controls and independent
configuration checkboxes. There is no separate global configuration section.

The configuration controls show a warning icon beside their title.
The visible description states that Kandev copies selected files without changes.

On a fine pointer, hover or keyboard focus opens the warning tooltip.
On a coarse pointer, the same control opens a bottom drawer.

The warning explains these risks:

- A configuration file can contain secrets or environment values.
- A configuration file can add hooks or commands in the executor.
- A configuration file can change permissions, MCP servers, models, and network endpoints.
- A host path can be invalid in the executor.
- Fresh provisioning overwrites the selected target file.

### Configuration copying does not guarantee model parity

A copied file can improve parity between the host and an isolated executor.
It cannot guarantee an equal model catalog.

Credentials, agent versions, provider accounts, and network access can still differ.
The executor ACP catalog remains authoritative when Kandev starts an agent.

If the executor omits the profile model, Kandev follows the
[executor-authoritative model-selection decision](2026-08-15-executor-authoritative-model-selection.md).
Kandev does not send the missing model, starts the agent default, and persists a chat warning.

## Consequences

- Agent providers use one shared extension contract for portable configuration.
- Users can reproduce simple host configuration without copying opaque runtime state.
- Authentication does not imply consent to copy configuration.
- Executor profiles persist intent, but they do not persist file data.
- File changes on the host apply after fresh provisioning or an environment reset.
- SSH users must understand that the copy can replace files in a shared remote home.
- Provider configuration can run extra hooks with the authority of the agent process.
- Task launch remains safe when configuration copying is off or does not create model parity.

## Alternatives Considered

1. **Copy the complete agent home.** Rejected because it includes credentials, sessions, databases, caches, and host-specific state.
2. **Accept arbitrary host paths.** Rejected because it creates a broad host-file export function.
3. **Keep configuration inside authentication bundles.** Rejected because users need independent consent and independent secret choices.
4. **Store file data in the profile.** Rejected because profiles are database records and file data can contain secrets.
5. **Copy configuration on every resume.** Rejected because a resume must preserve the existing executor state.
6. **Parse and sanitize provider files.** Rejected because Kandev does not own each provider format or its full semantics.
