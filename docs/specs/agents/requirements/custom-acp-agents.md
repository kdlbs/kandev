---
status: draft
system: agents
created: 2026-09-22
updated: 2026-09-22
owners:
  - jnmanso
---
# Operator-Registered Agent Requirements

## Overview

An operator can register a CLI as an agent from Settings without changing Kandev source. That
definition names the protocol Kandev drives the CLI with, and the registry, the discovery sweep, and
the capability probe all honour it for the lifetime of the backend process rather than only at boot.

## Requirements

### REQ-AGENTS-CUSTOM-ACP-001: Operator-Registered Agents Run In Their Declared Protocol

**Intent:** Let an operator register a CLI that speaks the Agent Client Protocol and have Kandev drive
it as a structured agent, while keeping every existing terminal definition working untouched and
keeping registry changes visible without a restart.

#### Acceptance criteria

- **AC-AGENTS-CUSTOM-ACP-001.1:** When an agent is registered, replaced, or unregistered after
  startup, the discovery sweep shall resolve the agent list from the agent registry rather than from a
  list captured at boot, so a created agent appears and a deleted agent disappears without restarting
  the backend.
- **AC-AGENTS-CUSTOM-ACP-001.2:** When a registry membership change is committed, the cached sweep
  results shall be dropped so the change is observable before the cache TTL expires, and a sweep that
  began before that invalidation shall not publish its superseded results.
- **AC-AGENTS-CUSTOM-ACP-001.3:** When an operator-registered definition carries no protocol, the
  system shall continue to run it as terminal passthrough, so definitions stored before the field
  existed keep their behaviour with no migration.
- **AC-AGENTS-CUSTOM-ACP-001.4:** When a definition declares the ACP protocol, the system shall
  register an agent whose runtime protocol is ACP, which advertises inference so the capability probe
  covers it, and which is not classified as passthrough-only.
- **AC-AGENTS-CUSTOM-ACP-001.5:** When an ACP definition is created, the system shall seed its default
  profile as non-passthrough with no model and shall start a capability probe, so the profile editor
  offers the agent's probed models without a manual refresh.
- **AC-AGENTS-CUSTOM-ACP-001.6:** When a definition names an unknown protocol, or pairs the ACP
  protocol with a passthrough MCP strategy, the system shall reject it with a client error rather than
  registering a downgraded agent.
- **AC-AGENTS-CUSTOM-ACP-001.7:** When the capability probe spawns an operator-registered command, it
  shall accept that command even though no compiled-in literal can cover it, while a command that
  claims to be built-in shall still resolve against the allow-list.
