---
status: active
system: office
specification_version: 1
migration: in_progress
owners:
  - Kandev
---

# Office System

## Purpose

The Office system coordinates autonomous workspaces, agents, tasks, and
automation runs. It owns the observable lifecycle of scheduled automation
work and the settings that control whether firings start new work or continue
an existing conversation.

## Ownership

- Automation settings and trigger admission.
- Automation target mode, repository selection, and visible task ownership.
- Automation run identity, continuity, and recovery.
- The fixed coordinator MCP authority for automation sessions.
- The automation run transcript and its relationship to hidden tasks.

## Exclusions

- Human task authoring remains in the task system.
- Provider-native context compaction remains in the agent runtime.
- Public user documentation remains under [`docs/public`](../../public/).

## Specification map

### Requirements

- [Automation continuity](requirements/automation-continuity.md)
- [Automation target modes](requirements/automation-target-modes.md)

### System designs

- [Automation target modes](system-design/automation-target-modes.md)

### Legacy references

- [Automation settings](automations-settings.md)
- [Automation runs](automation-runs.md)

The legacy documents remain in the migration catalog while this system
specification becomes the source for continuity behavior.
