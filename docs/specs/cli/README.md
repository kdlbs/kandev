---
status: draft
system: cli
specification_version: 1
migration: in_progress
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

## Migration record

Migration remains in progress while legacy source detail is extracted from the
canonical requirement and system-design documents. Use the catalog command to find them.

## Related systems

- [Platform](../platform/README.md): owns shared process startup.
- [Desktop](../desktop/README.md): embeds the native runtime.
