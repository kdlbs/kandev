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

## Specification map

### Requirements



- [Plugin-Contributed Agent Tools](requirements/agent-tools.md)
- [Plugin-Initiated Workflow Step Transitions](requirements/plugin-initiated-step-transitions.md)
- [Plugin Authoring Experience](requirements/authoring-experience.md)
- [Plugin Marketplace](requirements/marketplace.md)
- [Plugin nav items in the sidebar footer icon row](requirements/plugin-nav-sidebar-footer.md)
- [Plugin System](requirements/plugins.md)
- [Plugin Repository Task Creation](requirements/repository-provider-task-creation.md)
- [Voice Plugin Host Prerequisites](requirements/voice-extraction-host.md)
- [Voice Mode Leaves Core](requirements/voice-extraction.md)

### System design



- [Plugin-Initiated Workflow Step Transitions](system-design/plugin-initiated-step-transitions.md)
- [Plugin Marketplace](system-design/marketplace.md)
- [Plugin nav items in the sidebar footer icon row System Design Part 1](system-design/plugin-nav-sidebar-footer-01.md)
- [Plugin nav items in the sidebar footer icon row System Design Part 2](system-design/plugin-nav-sidebar-footer-02.md)
- [Plugin nav items in the sidebar footer icon row System Design Part 3](system-design/plugin-nav-sidebar-footer-03.md)
- [Plugin nav items in the sidebar footer icon row System Design Part 4](system-design/plugin-nav-sidebar-footer-04.md)
- [Plugin System System Design Part 1](system-design/plugins-01.md)
- [Plugin System System Design Part 2](system-design/plugins-02.md)
- [Plugin System System Design Part 3](system-design/plugins-03.md)
- [Plugin System System Design Part 4](system-design/plugins-04.md)
- [Plugin Repository Task Creation](system-design/repository-provider-task-creation.md)

## Migration record

Migration remains in progress while legacy source detail is extracted from the
canonical requirement and system-design documents above.

## Related systems

- [UI](../ui/README.md): renders plugin contributions.
- [Integrations](../integrations/README.md): supplies external connections.
