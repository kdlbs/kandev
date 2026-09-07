---
status: draft
system: platform
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# Platform system

## Purpose

The platform system owns cross-cutting runtime services, configuration,
observability, notifications, localization, lifecycle safety, and shared
operational guarantees.

## Ownership

This system owns startup and shutdown contracts, process and port-independent
runtime safety, configuration precedence, diagnostics, notifications,
localization, feature toggles, health, and shared session recovery services.

## Exclusions

- Executor-specific runtime environments belong to the [executor
  system](../executors/README.md).
- Agent identity belongs to the [agent system](../agents/README.md).
- Desktop shell behavior belongs to the [desktop system](../desktop/README.md).

## Migration record

Migration remains in progress while legacy source detail is extracted from the
canonical requirement and system-design documents. Use the catalog command to find them.

## Related systems

- [Agents](../agents/README.md): consumes shared runtime services.
- [Executors](../executors/README.md): owns execution-environment details.
- [Desktop](../desktop/README.md): embeds platform startup and shutdown.
