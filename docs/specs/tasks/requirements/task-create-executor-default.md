---
status: active
system: tasks
created: 2026-08-01
updated: 2026-08-18
owners:
  - Kandev
---
# Task Create Executor Default Requirements

## Overview

This document is the migrated task-system source for the capability. The source detail below remains authoritative while the system is migrated into separate requirement and design records.

## Requirements

### REQ-TASKS-TASK-CREATE-EXECUTOR-DEFAULT-001: Task Create Executor Default

**Intent:** Preserve the observable task or workflow behavior recorded by the legacy specification.

#### Acceptance criteria

- **AC-TASKS-TASK-CREATE-EXECUTOR-DEFAULT-001.1:** When a consumer uses this capability, the system shall provide the observable behavior and exclusions documented below.

## Migrated source detail

## Why

Repository-backed tasks must start in isolated workspaces unless the user or workspace makes an
explicit contrary choice. A portable last-used Local executor profile must not change a later
ordinary task to an in-place run. This rule also applies after an update and in another browser.

## Broken behavior

The task-create dialog currently restores a valid backend-owned last-used executor profile before
it resolves the task source and workspace default. After any Local task, that portable preference
can therefore select Local for the next ordinary repository-backed task even when the workspace
has no Local default.

The same stale selection can occur within one open dialog. Repository-less mode requires Local and
clears the previous executor when entered. Repo mode can then retain Local instead of resolving the
repository-backed default again.

## What

- The task-create dialog resolves the default executor before it considers the last-used executor
  profile.
- An ordinary repository-backed task uses the workspace's valid configured default executor.
  When the workspace has no valid default, it uses Worktree when Worktree is available.
- A task with no repository, or a task created from an explicit unmanaged local path, continues to
  prefer a direct Local executor.
- The backend-owned last-used executor profile may refine the profile choice only when that profile
  belongs to the already-resolved default executor. It cannot switch the executor.
- Changing between repository-backed and repository-less source modes resolves the executor policy
  for the destination mode. Returning to Repo does not retain Local only because repository-less
  mode required it.
- A valid explicit workspace default, including Local, remains authoritative.
- A user may manually select a different executor profile for the task being created. Successful
  task creation continues to record that profile in backend user settings, but the recorded value
  does not override the next task's executor policy.
- If the preferred executor or its profiles are unavailable, the dialog uses the existing eligible
  fallback so task creation remains usable.
- Desktop and mobile use the same selection policy. This change does not alter dialog composition,
  touch behavior, or responsive layout.

## Persistence guarantees

`users.settings.task_create_last_used.executor_profile_id` remains a portable backend-owned user
preference as defined by [ADR 0028](../../../decisions/0028-task-create-last-used-source-of-truth.md)
and [ADR 0041](../../../decisions/0041-backend-owned-portable-user-settings.md). No browser storage,
schema migration, or new persistence field is introduced. The stored profile is a convenience
within the executor selected by policy, not a portable override of that policy.

## Scenarios

- **GIVEN** an ordinary repository-backed task, no workspace executor default, and a last-used
  Local profile, **WHEN** the user opens Create Task, **THEN** a Worktree profile is selected.
- **GIVEN** the same backend user settings are opened in a different browser or after an update,
  **WHEN** the user opens an ordinary repository-backed task dialog, **THEN** the same Worktree
  safety default is selected.
- **GIVEN** an ordinary repository-backed task and an explicit Local workspace default, **WHEN**
  the user opens Create Task, **THEN** a Local profile is selected even if the last-used profile is
  Worktree.
- **GIVEN** an explicit unmanaged local path or no repository, **WHEN** the user opens Create Task,
  **THEN** a direct Local profile is preferred.
- **GIVEN** Worktree is the resolved executor and the last-used profile is another valid Worktree
  profile, **WHEN** the dialog restores selections, **THEN** that Worktree profile is selected.
- **GIVEN** Local was selected manually for one ordinary task and the workspace still has no
  executor default, **WHEN** the next ordinary repository-backed task dialog opens, **THEN** it
  returns to Worktree.
- **GIVEN** Repo with no executor default, **WHEN** the user selects None and Repo, **THEN** the
  executor changes from Worktree to Local to Worktree.
- **GIVEN** the preferred executor has no usable profile, **WHEN** the dialog resolves defaults,
  **THEN** it selects the first existing eligible fallback rather than leaving an invalid profile.

## Out of scope

- Removing backend persistence for task-create selections.
- Adding per-executor last-used profile history.
- Preventing users from manually choosing Local for an ordinary task.
- Changing workspace executor settings or their explicit precedence.
- Changing task-create dialog layout or mobile interaction patterns.