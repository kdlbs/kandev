---
status: current
system: agents
requirements:
  - REQ-AGENTS-MUSE-ACP-001
  - REQ-AGENTS-MUSE-ACP-002
  - REQ-AGENTS-MUSE-ACP-003
---

# Muse Code ACP agent system design

## Purpose and boundaries

`agents.MuseACP` declares Muse's identity, discovery, launch, credential, and
session-storage contract. MSP-to-ACP translation belongs to the external
`@bex-co/muse-code-acp` adapter; model access, sandboxing, approvals, and
persistence belong to the native `muse` executable the adapter spawns through
`muse serve`. Kandev adds no MSP code.

## Components

| Surface | Command | Owner |
|---|---|---|
| Structured session, inference, capability probe | `npx --yes --prefer-offline @bex-co/muse-code-acp@<version>` | Managed npm runtime |
| Passthrough, login, discovery | `muse`, `muse login`, `muse --version` | Native Muse CLI |
| Install | official `install.sh` with `MUSE_INSTALL_DIR` | Muse installer |

The adapter is registered in `managed_npm_runtime_versions.json`, so the
existing pin updater, cache repair, and operator version selection apply to it
without new code. The `npx` probe command is already allow-listed.

## Data and contracts

- MCP: ACP `session/new` servers (stdio and HTTP) are materialized by the
  adapter into a temporary Muse config overlay; no Kandev project file.
- Credentials: `$XDG_CONFIG_HOME/muse/auth.json` (default `~/.config`) or
  `META_API_KEY`, which takes priority.
- Sessions: `$XDG_DATA_HOME/muse/sessions` (default `~/.local/share`),
  resumed natively by the adapter and bind-mounted from
  `{home}/.local/share/muse` to `/root/.local/share/muse`.

## Failure and recovery

A missing `muse` executable makes discovery report unavailable before launch.
An unauthenticated host fails the ACP session with the adapter's auth error;
the user runs `muse login` or sets `META_API_KEY`. A Muse host newer than the
adapter's verified schema is reported by the adapter as an advisory
`hostCompatibility` session-info entry and does not block the session.

## Security

The install script downloads to a temporary file so a failed download fails
the job, installs only into an existing writable absolute PATH directory, and
embeds no credentials. Remote auth copies only `auth.json`.
