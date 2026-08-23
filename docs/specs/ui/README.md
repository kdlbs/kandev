---
status: active
system: ui
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# User interface system

## Purpose

The UI system presents Kandev state and actions in the web and desktop user
interfaces. It gives desktop and mobile users equivalent access to product
capabilities.

## Ownership

This system owns presentation behavior, responsive composition, interaction
patterns, accessible UI semantics, and browser rendering performance.

## Exclusions

- Task identity, workflow state, and runtime state publication belong to the
  [task and workflow system](../tasks/).
- Browser diagnostic history and bundle data belong to the
  [platform system](../platform/).
- Office agent lifecycle behavior belongs to the Office specifications in this
  directory until that system is migrated.

## Specification map

### Requirements

- [Persistent status motion](requirements/persistent-status-motion.md)

### System design

- [Persistent status motion](system-design/persistent-status-motion.md)

## Migration status

Migration is in progress. The requirement and design documents named above are
authoritative for persistent status motion. Existing flat UI specifications
remain authoritative for capabilities that this index does not map.

## Related systems

- [Tasks](../tasks/): supplies task, session, and workflow state for UI
  presentation.
- [Platform](../platform/): supplies browser diagnostics and runtime services.
