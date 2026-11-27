---
status: active
system: launcher
created: 2026-11-26
owners:
  - kandev
---

# Local-only CLI startup requirements

## Overview

Standalone Kandev listens on `127.0.0.1` when no bind address is configured.
This limits network exposure while authentication is off by default.
The launcher owns the startup contract and uses the shared configuration
system to resolve the effective bind.

## Requirements

### REQ-LAUNCHER-LOCAL-ONLY-001: Default to local access

**Intent:** Require a deliberate network configuration before exposing a
standalone instance to other computers.

#### Acceptance criteria

- **AC-LAUNCHER-LOCAL-ONLY-001.1:** A standalone Kandev instance with no bind
  configured shall listen only on `127.0.0.1`. This shall apply to bare
  `kandev`, `run`, `start`, `dev`, and headless startup. Local HTTP and
  WebSocket clients shall use the same listener.
- **AC-LAUNCHER-LOCAL-ONLY-001.2:** The implicit local bind shall keep
  `http://localhost:<port>` as the launcher access URL. An upgrade shall apply
  the new bind on restart without changing saved settings or the browser origin.
- **AC-LAUNCHER-LOCAL-ONLY-001.3:** Existing explicit IPv4, IPv6, hostname,
  multi-address, and wildcard binds shall retain their meaning and precedence.
  An invalid bind shall still produce a startup error.
- **AC-LAUNCHER-LOCAL-ONLY-001.4:** The official container and existing Kandev
  systemd and launchd units shall retain their previous fallback when no address
  is configured. Their explicit binds shall still win. Desktop shall retain
  its explicitly selected loopback address.
- **AC-LAUNCHER-LOCAL-ONLY-001.5:** The quickstart shall use the default
  standalone command. Upgrade guidance shall explain how to restore intentional
  network access using the existing bind settings and protected access guidance.

## Out of scope

- Changes to authentication, TLS, proxy trust, published Docker ports, or
  service-unit migration.
- Persisting or inferring intent from an existing data directory.
