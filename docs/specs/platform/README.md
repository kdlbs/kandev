---
status: active
system: platform
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# Platform system

## Purpose

The platform system owns install-wide runtime services that support product
features. These services include diagnostics, resource control, startup
configuration, notifications, and browser-local platform data.

## Ownership

This system owns browser diagnostic history, diagnostic bundle boundaries,
platform resource limits, and cross-feature runtime support contracts.

## Exclusions

- User-interface presentation belongs to the [UI system](../ui/).
- Task and workflow state belongs to the [task and workflow system](../tasks/).
- Agent runtime behavior belongs to the agent specifications in this directory
  until that system is migrated.

## Specification map

### Requirements

- [Browser console retention](requirements/browser-console-retention.md)

### System design

- [Browser console retention](system-design/browser-console-retention.md)

## Migration status

Migration is in progress. The documents above are authoritative for browser
console retention. Existing flat platform specifications remain authoritative
for capabilities that this index does not map.

## Related systems

- [UI](../ui/): presents platform diagnostics and performance state.
- [Tasks](../tasks/): supplies task identity for diagnostic partitioning.
