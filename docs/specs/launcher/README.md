---
status: active
system: launcher
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# Launcher system

## Purpose

The launcher system starts and supervises Kandev processes for the `dev`,
`start`, `run`, and service entrypoints. It gives operators one startup result,
one reachable access URL, and useful failure evidence.

## Ownership

This system owns launch-mode dispatch, managed-process startup and shutdown,
port selection, backend readiness probes, startup output, and supervisor
handoffs.

## Exclusions

- The backend owns HTTP routes and the `/health` response.
- The common configuration system owns configuration discovery, precedence,
  parsing, and validation.
- Authentication owns forwarded-header trust after the backend accepts a
  request.
- Agent runtime launch and ACP probes are not backend startup readiness.

## Specification map

### Requirements

- [Startup recovery](requirements/startup-recovery.md)

### System design

- [Startup recovery](system-design/startup-recovery.md)

## Migration status

The startup-recovery documents are authoritative for bind-aware readiness and
launcher failure output. Configuration discovery and precedence are defined by
[startup configuration parity](../platform/requirements/startup-configuration-parity.md).
The native launcher migration also incorporates the
[Go development launcher](../platform/requirements/go-dev-launcher.md)
requirement. No removed legacy path remains authoritative.

## Related systems

- [Platform specifications](../platform/): define cross-cutting startup
  configuration and diagnostic-log behavior.
- [Authentication specifications](../auth/): define trusted-proxy behavior.
