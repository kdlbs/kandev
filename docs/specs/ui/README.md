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

The user interface system owns presentation and interaction behavior shared by
Kandev's web surfaces, including responsive composition, accessible controls,
and rendered-content behavior.

## Ownership

This system owns user-visible component behavior, viewport and input-modality
adaptation, local interaction state, visual containment, and accessibility
semantics. Backend data, permissions, and persistence remain owned by their
domain systems.

## Exclusions

- Durable task, plan, and workflow data belongs to the [task and workflow
  system](../tasks/).
- Repository files and worktrees belong to the workspace system.
- Agent execution and runtime state belong to the agent system.

## Specification map

### Requirements

- [Resizable Markdown tables](requirements/resizable-markdown-tables.md)

### System design

- [Resizable Markdown tables](system-design/resizable-markdown-tables.md)

## Migration status

UI specifications are being migrated capability by capability. Documents named
above are authoritative. Unmigrated flat UI specifications remain authoritative
through [`../INDEX.md`](../INDEX.md) until replaced.

## Related systems

- [Tasks](../tasks/): supplies task plans, sessions, and task-scoped documents
  rendered by UI surfaces.
