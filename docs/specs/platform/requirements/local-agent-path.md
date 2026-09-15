---
status: active
system: platform
created: 2026-09-15
owners:
  - kandev
---

# Local agent executable discovery

## Overview

Native agent installers place executables in the runtime user's `~/.local/bin`.
Kandev's shared startup environment must make those executables available to
discovery and local execution, including when installation occurs after startup.
Platform owns the shared process environment; agent identity and authentication
remain owned by the agent system.

## Requirements

### REQ-PLATFORM-LOCAL-AGENT-PATH-001: Local agent executable availability

**Intent:** An agent installed through Kandev is discoverable without restarting
solely to load shell configuration.

#### Acceptance criteria

- **AC-PLATFORM-LOCAL-AGENT-PATH-001.1:** On macOS and Linux, regular native/npm
  launches and service launches shall discover executable agent CLIs in the
  runtime user's `~/.local/bin` even when the launcher's inherited PATH omits it,
  provided the runtime home resolves to a usable absolute path.
- **AC-PLATFORM-LOCAL-AGENT-PATH-001.2:** If that directory or executable is
  created after startup, the existing post-install refresh or manual rescan
  shall discover it without a PATH-related restart.
- **AC-PLATFORM-LOCAL-AGENT-PATH-001.3:** Local capability probes and newly
  launched local agent processes shall resolve the installed executable using
  the same startup environment; discovery alone shall not be the only repaired
  consumer.
- **AC-PLATFORM-LOCAL-AGENT-PATH-001.4:** Existing PATH entries shall retain their
  order and precedence. Repeated startup normalization shall not duplicate the
  added directory. The directory shall be derived from the runtime user's home,
  independently of Kandev's data directory. System services shall use the effective
  service account's home even when inherited HOME is missing or names another user.
- **AC-PLATFORM-LOCAL-AGENT-PATH-001.5:** Missing or non-executable agent files
  shall remain unavailable. When the runtime home cannot be resolved to a usable
  absolute path, inherited PATH shall remain unchanged. Windows startup and remote executor environments
  shall retain their existing behavior.

## Out of scope

Cursor authentication, new executable aliases, custom install directories,
shell-profile editing, remote installation changes, and UI layout changes.

## Implementation plans

- [Cursor install discovery](../../../plans/cursor-install-discovery/plan.md)
