---
status: draft
system: cli
specification_version: 1
migration: complete
owners:
  - kandev
---

# CLI system

## Purpose

The CLI system owns native command-line launch modes, process contracts, and
parity between CLI and other Kandev launch surfaces.

## Ownership

This system owns the native Kandev CLI, command routing, CLI configuration,
and CLI-specific compatibility behavior.

## Exclusions

- Shared launcher behavior belongs to the [platform system](../platform/README.md).
- Desktop shell behavior belongs to the [desktop system](../desktop/README.md).

## Specification map

### Requirements



- [CLI-Mode Task Parity (Kanban)](requirements/cli-mode-parity.md)
- [Native Kandev CLI](requirements/native-kandev-cli.md)

### System design



- None.

## Migration record

All legacy sources assigned to this system are now represented by the canonical requirement and system-design documents above. Source detail is retained in those documents or in their linked design parts.

## Related systems

- [Platform](../platform/README.md): owns shared process startup.
- [Desktop](../desktop/README.md): embeds the native runtime.
