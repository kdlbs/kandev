# ADR-2026-08-23: Give Automations Explicit Hidden and Visible Task Targets

**Status:** accepted
**Date:** 2026-08-23
**Area:** backend, frontend, workflow, protocol
**Related ADRs:** [User-configured automation continuity](2026-08-22-user-configured-automation-continuity.md), [Task-owned worktree lifetime](2026-08-08-task-owned-worktree-lifetime.md)
**Related specs:** [Automation target modes](../specs/office/requirements/automation-target-modes.md), [Automation target modes design](../specs/office/system-design/automation-target-modes.md)

## Context

The current automation contract treats every firing as hidden work. That is
appropriate for coordinator agents, but it prevents a scheduled action from
entering a selected workflow as ordinary Kanban work. It also rejects a valid
repository-free automation even though the agent lifecycle already supports a
task-owned scratch workspace. The existing empty repository selection means
"use the workspace's first repository", so it cannot express an intentional
no-repository run.

The hidden task contract and the normal task contract have different authority
and cleanup rules. A visible task must not receive the coordinator MCP profile,
and deleting its automation must not delete the task that a person expects to
find in the sidebar.

## Decision

1. Persist `task_mode` with `automation_run` as the compatibility default and
   `normal_task` as the explicit visible target.
2. Persist `repository_mode` separately from `repository_ids`. `none` means
   intentional repository-free execution, `selected` uses the listed
   repositories, and `workspace_default` preserves legacy empty-selection
   behavior for existing rows and omitted old-client fields. A provider
   trigger that requires a repository cannot use explicit `none`.
3. A repository-free automation uses the Local executor and a task-owned
   scratch workspace. Worktree is not a valid repository-free choice. The
   backend validates this even when the request bypasses the editor.
4. Hidden target mode keeps the fixed `SurfaceAutomation` profile and hidden
   `automation_run` origin. Normal-task mode requires a workflow, uses a
   visible `automation_task` origin, and receives the normal task profile and
   lifecycle. The workflow step remains optional and resolves through the
   workflow's normal starting-step rule.
5. The existing `new_task` and `reuse_thread` policy applies to both target
   modes. A reusable visible target continues one visible task and primary
   session, while an isolated visible firing creates a separate visible task.
   Every firing remains an exact `AutomationRun`.
6. Hidden tasks remain owned by automation cleanup. Visible normal tasks remain
   ordinary task records when an automation is disabled or deleted. Open run
   records are terminalized without deleting the visible task.

## Consequences

- Users can run reports and coordination prompts without a repository or
  workflow, with a clear Local scratch execution path.
- Users can choose whether a scheduled firing becomes ordinary Kanban work or
  remains a background automation run.
- Target mode, repository mode, continuation compatibility, and exact-run
  finalization become persisted contracts that require migrations and tests.
- The run dispatcher must keep hidden and visible task origins separate while
  sharing admission, continuation, and exact identity logic.
- Existing empty repository rows preserve their historical workspace-default
  behavior. New explicit no-repository saves do not silently attach a repo.
- Visible tasks remain after automation deletion, so the automation UI must not
  promise that deleting an automation deletes all work it created.

## Alternatives Considered

1. **Reuse the withdrawn `execution_mode` column.** Rejected because that
   column describes a retired behavior and is intentionally excluded from the
   current Go model. A new typed target makes the contract explicit and keeps
   migration-only data separate.
2. **Treat an empty `repository_ids` list as no repository.** Rejected because
   existing automations use an empty list for the workspace-first fallback.
   `repository_mode` preserves both meanings without guessing from history.
3. **Keep visible tasks on the hidden `automation_run` origin.** Rejected
   because board/sidebar queries and coordinator authorization use that origin
   as a hidden boundary. A distinct visible provenance keeps the security and
   visibility rules auditable.
4. **Force visible tasks to use `new_task`.** Rejected because a user may want
   a visible recurring task that keeps one conversation and task environment.
5. **Allow Worktree to create a worktree without a repository.** Rejected
   because Worktree semantics require a source repository. Local scratch is
   already supported by the lifecycle and gives the user an honest executor
   label.
