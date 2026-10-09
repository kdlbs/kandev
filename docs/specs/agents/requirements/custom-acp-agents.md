---
status: draft
system: agents
created: 2026-09-22
updated: 2026-09-28
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

### REQ-AGENTS-CUSTOM-ACP-002: Operator-Registered ACP Agents Keep Their Conversation Across Reconnects

**Intent:** When the process of an operator-registered ACP agent is replaced (backend restart, agent
crash, runtime recovery, or Resume), the agent continues the provider conversation it already had,
exactly as a built-in ACP agent does, instead of silently starting a new one under the same Kandev
session.

As an operator running my own ACP agent, I want a reconnect to reopen the agent's own session, so
that the agent still remembers the conversation after its process is replaced.

#### Acceptance criteria

- **AC-AGENTS-CUSTOM-ACP-002.1:** When an execution of an operator-registered ACP agent starts with a
  stored provider session ID, and the agent's `initialize` response advertises
  `agentCapabilities.loadSession` or `agentCapabilities.sessionCapabilities.resume`, the system shall
  restore that exact session ID with `session/resume` when advertised, otherwise with
  `session/load`, and shall not send `session/new`.
- **AC-AGENTS-CUSTOM-ACP-002.2:** When the agent advertises neither capability, the system shall start
  the execution with `session/new` and shall not fail the launch, so a definition whose CLI cannot
  restore keeps working as it does today.
- **AC-AGENTS-CUSTOM-ACP-002.3:** When a restore request fails, the system shall classify the failure
  exactly as it does for built-in ACP agents: a recognized compatibility failure creates a
  replacement session, and any other failure keeps the stored session ID for a later retry.
- **AC-AGENTS-CUSTOM-ACP-002.4:** The behavior shall apply to every definition that declares the ACP
  protocol, including definitions stored before this requirement, without a migration or a new
  operator setting.

#### Exclusions

- Terminal (passthrough) definitions. Their CLI owns its own resume flags.
- Recognizing additional provider-specific "unknown session" error shapes. A provider whose
  unknown-session error is not already recognized keeps its stored session ID and surfaces the
  recovery failure (AC-AGENTS-CUSTOM-ACP-002.3), which is the existing contract of
  REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.
- Any change to the working directory sent with the restore request.
