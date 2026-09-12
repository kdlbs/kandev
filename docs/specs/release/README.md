---
status: draft
system: release
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# Release system

## Purpose

The release system owns version channels, packaging, publication, and release
artifact verification across Kandev distributions.

## Ownership

This system owns stable and nightly channel semantics, npm, Homebrew, Scoop,
container, and desktop release publication contracts.

## Exclusions

- Desktop runtime lifecycle belongs to the [desktop system](../desktop/README.md).
- Agent runtime update behavior belongs to the [agent system](../agents/README.md).

## Migration record

Migration remains in progress while legacy source detail is extracted from the
canonical requirement and system-design documents. Use the catalog command to find them.

## Related systems

- [Desktop](../desktop/README.md): consumes published desktop artifacts.
- [Platform](../platform/README.md): exposes release update notifications.
