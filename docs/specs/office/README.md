---
status: draft
system: office
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# Office system

## Purpose

The Office system owns autonomous agent workspaces, coordination, scheduling,
automation runs, dashboards, inboxes, and Office-specific live state.

## Ownership

This system owns Office agents and roles, autonomous task assignment,
scheduler and wakeup policy, Office routing, automation runs, inbox activity,
dashboard projections, and Office testing contracts.

## Exclusions

- Durable task and workflow primitives belong to the [task system](../tasks/README.md).
- Shared agent profiles and permissions belong to the [agent system](../agents/README.md).
- External provider connections belong to the [integration system](../integrations/README.md).

## Migration record

Migration remains in progress while legacy source detail is extracted from the
canonical requirement and system-design documents. Use the catalog command to find them.

## Related systems

- [Tasks](../tasks/README.md): supplies durable work and workflow primitives.
- [Agents](../agents/README.md): supplies agent profiles and permission policy.
- [Integrations](../integrations/README.md): supplies provider connections.

