---
status: draft
system: plugins
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# Plugin system

## Purpose

The plugin system owns plugin packages, registration, lifecycle, host APIs,
permissions, event delivery, and plugin-provided capabilities.

## Ownership

This system owns plugin manifests, marketplace behavior, plugin state,
installation and health, host data and tool APIs, contribution points, and
plugin security boundaries.

## Exclusions

- Core UI behavior belongs to the [UI system](../ui/README.md).
- External service credentials belong to the [integration system](../integrations/README.md).
- Desktop packaging belongs to the [desktop system](../desktop/README.md).

## Migration record

Migration remains in progress while legacy source detail is extracted from the
canonical requirement and system-design documents. Use the catalog command to find them.

## Related systems

- [UI](../ui/README.md): renders plugin contributions.
- [Canvases](../canvases/README.md): binds isolated web applications to task
  and workspace canvas lifecycles.
- [Canvas distribution](../canvases/system-design/marketplace-sharing.md): owns
  canvas export/import and the canvas-specific marketplace flow; reuses plugin
  manifests, static validation, catalog sources, and runtime isolation.
- [Integrations](../integrations/README.md): supplies external connections.
