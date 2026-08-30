---
status: current
system: tasks
requirements:
  - REQ-TASKS-ARCHIVE-CONFIRMATION-001
created: 2026-08-24
owners:
  - kandev
---

# Task Archive Confirmation System Design

## Context and boundaries

The task system owns the user-level archive-confirmation preference and the
post-archive task selection contract. Archive execution, deletion, and
programmatic archive callers remain separate contracts.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-TASKS-ARCHIVE-CONFIRMATION-001 | Preference, archive flow, navigation, failure behavior |

## Components and responsibilities

- The user-settings service reads and writes the preference.
- Task archive actions ask the settings state whether confirmation is required.
- The archive dialog remains responsible for cleanup summary and optional
  cascade selection.
- The task-navigation coordinator selects a surviving task or the task
  overview after a successful archive.
- Desktop and mobile archive surfaces use the same preference and navigation
  coordinator.

## Data and API contracts

The existing user settings JSON stores `confirm_task_archive`. A missing value
means true for backward compatibility. The existing user-settings GET, PATCH,
and `user.settings.updated` contracts carry the boolean. No archive endpoint
changes.

## Control flow

When confirmation is enabled, user archive actions open the existing dialog.
The dialog may request a cascade archive. When confirmation is disabled, the
action archives only the requested task immediately; it never silently adds
subtasks.

After an active task is archived, navigation and the URL update as one
transaction from the user's perspective. A cascade archive excludes the
archived tree from replacement-task selection. If no safe task remains, the
overview is the destination.

## Failure and recovery

The settings client keeps the previous value when persistence fails. Until
settings load successfully, the client requires confirmation. If an archive
fails after temporary navigation, the coordinator restores the original task
and URL. Existing archive-surface error handling remains authoritative.

## Persistence and compatibility

The preference survives restarts in the existing per-user settings record and
applies across workspaces for that user. Delete confirmations, API/CLI/MCP
archive callers, and programmatic archive operations do not consult this
preference.

## Verification

- Unit-test missing, true, false, and failed settings mutations.
- Test archive navigation with unrelated tasks, cascade trees, and no safe
  destination.
- Cover desktop and mobile archive entry points with the existing task-surface
  integration tests.

## Related records

- [Archive Confirmation Preference](../../../plans/archive-confirmation-preference/plan.md)
- [Cascade Archive Navigation](../../../plans/cascade-archive-navigation/plan.md)
