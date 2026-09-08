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
task-owned scratch workspace. A first-workspace-repository fallback is also
ambiguous: it does not record user intent, cannot record a base branch, and
changes behavior when repository ordering changes.

The hidden task contract and the normal task contract have different authority
and cleanup rules. A visible task must not receive the coordinator MCP profile,
and deleting its automation must not delete the task that a person expects to
find in the sidebar.

## Decision

1. Persist `task_mode` with `automation_run` as the compatibility default and
   `normal_task` as the explicit visible target.
2. Persist an ordered list of explicit repository and base-branch pairs. An
   empty list means intentional repository-free execution. There is no
   workspace-default or first-repository behavior. Compatibility ID-only
   requests use each named repository's configured default branch; an empty
   compatibility request remains empty.
3. A repository-free automation uses a task-owned scratch workspace. Worktree
   and Local-compatible executor profiles remain valid choices. A Worktree
   profile does not create a Git worktree when no repository is attached.
4. Hidden target mode keeps the fixed `SurfaceAutomation` profile and hidden
   `automation_run` origin. Normal-task mode requires a workflow, uses a
   visible `automation_task` origin, and receives the normal task profile and
   lifecycle. The automation editor does not choose a workflow step; normal
   task creation resolves the workflow's configured start step.
5. The existing `new_task` and `reuse_thread` policy applies to both target
   modes. A reusable visible target continues one visible task and primary
   session, while an isolated visible firing creates a separate visible task.
   Every firing remains an exact `AutomationRun`.
6. Hidden tasks remain owned by automation cleanup. Visible normal tasks remain
   ordinary task records when an automation is disabled or deleted. Open run
   records are terminalized without deleting the visible task.
7. The automation editor composes the repository/base-branch, workflow,
   agent-profile, and executor-profile selectors from task creation. Search,
   logos, workflow previews, availability rules, and mobile behavior therefore
   have one implementation across both surfaces.

## Consequences

- Users can run reports and coordination prompts without a repository or
  workflow, even when Worktree is the only available executor profile.
- Users can choose whether a scheduled firing becomes ordinary Kanban work or
  remains a background automation run.
- Target mode, repository mode, continuation compatibility, and exact-run
  finalization become persisted contracts that require migrations and tests.
- The run dispatcher must keep hidden and visible task origins separate while
  sharing admission, continuation, and exact identity logic.
- Existing empty repository rows become explicit no-repository automations.
  Existing selected repositories keep their order and receive a concrete base
  branch during migration.
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
5. **Require Local for repository-free execution.** Rejected because the
   lifecycle already gives repository-free sessions a task-owned scratch
   directory. Requiring another configured executor would block valid
   automations without improving isolation.
