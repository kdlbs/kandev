---
status: draft
system: agents
created: 2026-10-25
owners:
  - kandev
---

# Pi remote credentials requirements

## Overview

Remote executor profiles let a user pick, per agent, which credentials and
configuration reach the remote environment. Pi declared none, so a Pi session
on a remote executor started unauthenticated unless someone logged in inside
that environment, which writes a real credential into it. Pi stores
`/login` credentials, one entry per provider, in `~/.pi/agent/auth.json`, its
settings in `settings.json`, and custom providers and models in `models.json`,
all under `~/.pi/agent`.

## Requirements

### REQ-AGENTS-PI-REMOTE-AUTH-001: Pi credentials on remote executors

**Intent:** Authenticate Pi in a remote environment with the credentials the
user already has on the Kandev host, without discarding credentials that exist
only remotely.

#### Acceptance criteria

- **AC-AGENTS-PI-REMOTE-AUTH-001.1:** Remote authentication shall offer copying
  `~/.pi/agent/auth.json` into `~/.pi/agent` on the remote, merging top-level
  provider entries into an existing remote file so that remote-only providers
  remain.
- **AC-AGENTS-PI-REMOTE-AUTH-001.2:** Remote authentication shall offer
  `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY` and
  `OPENROUTER_API_KEY` from stored secrets, and every remote executor that
  forwards an allowlist of agent credential variables, including SSH, shall
  forward each of them.
- **AC-AGENTS-PI-REMOTE-AUTH-001.3:** The auth file option shall be labelled by
  the card's translated default rather than an English label from the agent.

### REQ-AGENTS-PI-REMOTE-AUTH-002: Pi configuration bundle

**Intent:** Let a remote Pi use the same settings, providers and models as the
host, separately from credentials.

#### Acceptance criteria

- **AC-AGENTS-PI-REMOTE-AUTH-002.1:** A `pi.config` bundle shall copy
  `~/.pi/agent/settings.json` and `~/.pi/agent/models.json` to the same paths
  on the remote, and shall not contain `auth.json`.
- **AC-AGENTS-PI-REMOTE-AUTH-002.2:** The bundle label shall be localized in
  every shipped locale.
