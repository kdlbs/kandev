# ADR-2026-08-18-plugin-task-row-metadata: Keep Task Row Plugin Metadata Generic

**Status:** accepted
**Date:** 2026-08-18
**Area:** frontend

## Context

Plugins need compact, read-only content on sidebar and `/tasks` rows. The first
consumer was a tags plugin, but naming the public slot after tags would make one
plugin's data model part of the host contract.

## Decision

The host exposes `task-row-metadata` with `TaskRowMetadataSlotProps`. The slot
accepts contributions from any plugin and describes only the host surface:
task, workspace, workflow step, and `sidebar` or `task-list` presentation. The
host owns the conditional wrapper and emits no DOM when the slot is empty.

Primary task-menu actions use the same registration on kanban, desktop sidebar,
and phone task-sheet menus. Edit-group actions remain card-only.

## Consequences

Plugins can show tags, ownership, provider state, or other compact metadata
without new host hooks. Host layouts stay stable when no plugin contributes.
The generic contract cannot promise tags-specific styling or behavior.

## Alternatives Considered

`task-row-tags` and `TaskRowTagsSlotProps` were rejected because they couple the
host API to one plugin. Reusing `task-card-tags` was rejected because dense rows
need a surface discriminator and different layout ownership.
