# ADR-2026-08-01-repository-task-executor-defaults: Resolve Task Executor Policy Before Last-Used Profile

**Status:** accepted
**Date:** 2026-08-01
**Area:** frontend

## Context

ADR 0028 made `task_create_last_used.executor_profile_id` portable and backend-owned. The task
dialog consumed that profile before resolving the task source and workspace executor default, so a
single Local task could make later ordinary repository-backed tasks default to Local across
browsers and upgrades. That portable convenience preference conflicts with Worktree's role as the
isolation-first fallback.

## Decision

Task creation separates executor policy from profile convenience. The dialog first resolves an
eligible executor from the task source and explicit workspace default: unmanaged local paths and
repository-less tasks prefer direct Local, a valid workspace default wins for ordinary repository
tasks, and otherwise Worktree is preferred. The backend-owned last-used profile is restored only
when it belongs to that resolved executor; it never switches executors.

Manual profile selection still applies to the task being created and successful creation still
records the selected profile under `users.settings.task_create_last_used`. No persistence schema or
API contract changes.

## Consequences

- Ordinary repository tasks return to an isolation-first Worktree default after a one-off Local
  task, including in another browser or after an update.
- Explicit Local workspace defaults and explicit local-path/repository-less sources keep their
  current behavior.
- A single global last-used profile cannot remember a preferred profile for every executor. After
  crossing executor types, the dialog may use the resolved executor's first profile.
- The policy can be implemented and tested entirely in the frontend selection logic.

## Alternatives Considered

1. **Always force Worktree for every repository task.** Rejected because it would ignore an
   explicit workspace default and remove an intentional configuration surface.
2. **Remove last-used executor persistence.** Rejected because profile restoration remains useful
   when the saved profile belongs to the executor selected by policy.
3. **Persist one last-used profile per executor.** Rejected for this repair because it expands the
   user-settings contract and requires migration and UI semantics beyond the reported regression.
