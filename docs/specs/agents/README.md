---
status: draft
system: agents
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# Agent system

## Purpose

The agent system owns configured agent identities, profiles, roles, permissions,
provider capabilities, and agent-facing runtime contracts.

## Ownership

This system owns agent profile data, role governance, profile-backed utility
agents, provider model options, agent permissions, and the agent capability
surface shared by task and Office consumers.

## Exclusions

- Durable work items and workflow transitions belong to the [task and workflow
  system](../tasks/README.md).
- Autonomous Office identities and dashboards belong to the [Office
  system](../office/README.md).
- Presentation-only behavior belongs to the [UI system](../ui/README.md).

## Migration record

Migration remains in progress while legacy source detail is extracted from the
canonical requirement and system-design documents. Use the catalog command to
find current sources.

## Related systems

- [Settings parity](../platform/requirements/agent-settings-parity.md): owns the
  shared discovery and interface contract. Agents retain profile data ownership.
- [Tasks](../tasks/README.md): consumes agent profiles for task execution.
- [Office](../office/README.md): consumes agent identities for autonomous work.
- [Platform](../platform/README.md): owns shared process and runtime services.
