---
status: draft
system: auth
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# Authorization and identity system

## Purpose

The authorization and identity system owns authentication state, trust
boundaries, session credentials, and authorization checks shared by Kandev
surfaces.

## Ownership

This system owns authenticated identity, trusted proxy handling, session and
cookie boundaries, self-action guards, share-link access, and security checks
for user and service requests.

Executor/profile ownership and private-compute admission belong to the
[executor system](../executors/README.md), consuming this system's authenticated
identity and scopes without granting workspace reach.

## Exclusions

- Provider credentials and external service connections belong to the
  [integration system](../integrations/README.md).
- Agent permission policy belongs to the [agent system](../agents/README.md).

## Migration record

Migration remains in progress while legacy source detail is extracted from the
canonical requirement and system-design documents. Use the catalog command to find them.

## Related systems

- [Integrations](../integrations/README.md): owns external provider auth.
- [Platform](../platform/README.md): owns process-level security boundaries.
