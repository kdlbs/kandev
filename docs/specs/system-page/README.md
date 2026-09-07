---
status: draft
system: system-page
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# System page

## Purpose

The system-page system owns operational diagnostics and maintenance surfaces
for inspecting Kandev health, storage, logs, backups, and cleanup.

## Ownership

This system owns system-page information architecture, storage overview and
maintenance operations, diagnostic bundles, and operator-facing maintenance
feedback.

## Exclusions

- Backend diagnostic events belong to the [platform system](../platform/README.md).
- General settings presentation belongs to the [UI system](../ui/README.md).

## Migration record

Migration remains in progress while legacy source detail is extracted from the
canonical requirement and system-design documents. Use the catalog command to find them.

## Related systems

- [Platform](../platform/README.md): supplies health and diagnostics.
- [UI](../ui/README.md): renders the system page.
