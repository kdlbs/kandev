---
status: draft
system: integrations
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# Integration system

## Purpose

The integration system owns connections to external services and the
provider-specific contracts that synchronize or act on external work.

## Ownership

This system owns provider credentials, provider identity, external issue and
pull-request synchronization, integration settings, provider-aware review
automation, external question or answer flows, and provider-specific UI
outcomes that expose those contracts.

## Exclusions

- Generic authentication belongs to the [auth system](../auth/README.md).
- Durable Kandev tasks belong to the [task system](../tasks/README.md).
- Plugin-owned services belong to the [plugin system](../plugins/README.md).
- Reusable presentation contracts without provider state belong to the
  [UI system](../ui/README.md).

## Migration record

Migration remains in progress while legacy source detail is extracted from the
canonical requirement and system-design documents. Use the catalog command to
find current sources.

## Related systems

- [Auth](../auth/README.md): authenticates users and service requests.
- [Tasks](../tasks/README.md): owns the Kandev task receiving external work.
- [Plugins](../plugins/README.md): owns plugin-host integration contracts.
