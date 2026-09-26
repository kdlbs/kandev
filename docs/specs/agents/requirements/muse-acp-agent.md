---
status: draft
system: agents
created: 2026-09-19
owners:
  - kandev
---

# Muse Code ACP agent requirements

## Overview

Meta's Muse Code CLI speaks the Muse Session Protocol through `muse serve`, not
ACP. Kandev needs it as a built-in agent so a user can select it for a task,
run and resume a structured session, and use MCP servers, without Kandev
implementing MSP itself. The integration reuses the community
`@bex-co/muse-code-acp` MSP-to-ACP adapter as a managed npm runtime and targets
the native `muse` executable for discovery, install, login, and passthrough.

## Requirements

### REQ-AGENTS-MUSE-ACP-001: Built-in Muse agent identity and discovery

**Intent:** Give Muse a stable catalog identity and report it available only
when the executable the adapter drives is present.

#### Acceptance criteria

- **AC-AGENTS-MUSE-ACP-001.1:** The agent identifier shall be `muse-acp`, the
  display name `Muse`, and the catalog name `Muse Code ACP`, with a display
  order unique among built-in agents.
- **AC-AGENTS-MUSE-ACP-001.2:** Discovery shall report the agent available only
  when `muse` is on the backend PATH and answers `--version`; an adapter-only
  host shall report it unavailable.

### REQ-AGENTS-MUSE-ACP-002: Structured and passthrough launch surfaces

**Intent:** Keep structured execution on the pinned adapter and terminal
passthrough on the native CLI.

#### Acceptance criteria

- **AC-AGENTS-MUSE-ACP-002.1:** Structured sessions and one-shot inference shall
  launch `npx --yes --prefer-offline @bex-co/muse-code-acp@<effective-version>`
  with no extra arguments over `agent.ProtocolACP`, with the default version
  pinned in the managed npm runtime catalogue.
- **AC-AGENTS-MUSE-ACP-002.2:** CLI passthrough shall launch `muse`, passing a
  selected model as `--model <model>`.
- **AC-AGENTS-MUSE-ACP-002.3:** MCP servers shall be delivered through ACP
  `session/new`; the agent shall declare no project MCP file strategy.

### REQ-AGENTS-MUSE-ACP-003: Credentials, install, and resume

**Intent:** Use Muse's own credential, install, and session-storage mechanisms.

#### Acceptance criteria

- **AC-AGENTS-MUSE-ACP-003.1:** Remote authentication shall offer copying
  `~/.config/muse/auth.json` and setting `META_API_KEY`; login shall run
  `muse login`.
- **AC-AGENTS-MUSE-ACP-003.2:** On POSIX hosts, the install script shall run the
  official `https://dev.meta.ai/install.sh` into the first writable absolute
  PATH directory, fail when the download fails, and verify `muse --version`.
  Windows shall expose no automated install action because the official
  installer is a POSIX shell script.
- **AC-AGENTS-MUSE-ACP-003.3:** Native session resume shall use Muse's default
  `~/.local/share/muse` path. Kandev shall remove `XDG_CONFIG_HOME` and
  `XDG_DATA_HOME` overrides from Muse processes and mount an isolated executor
  home so both `~/.config/muse` credentials and `~/.local/share/muse` sessions
  are available without mounting the real host home.
