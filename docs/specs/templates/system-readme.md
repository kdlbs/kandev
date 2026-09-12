---
status: draft
system: <system-slug>
specification_version: 1
migration: <in_progress|complete>
owners:
  - <owner>
---

# <System Name>

## Purpose

Describe the capability and the actors or consumers that use it.

## Ownership

List the concepts, behavior, data, and contracts that this system owns.

## Exclusions

List adjacent concepts that another system owns. Link to that system.

## Find specifications

Use the catalog command to list this system's current documents:

    python3 scripts/list-docs.py specs --system <system-slug> --format markdown
    python3 scripts/list-docs.py specs --system <system-slug> --kind requirement --format paths
    python3 scripts/list-docs.py specs --system <system-slug> --kind system-design --format paths

Do not copy the command output into this README. Keep this file focused on the
system boundary, migration record, and related systems.

## Related systems

- [System](../other-system/README.md): Describe the dependency direction.
