---
id: "01-preserve-routine-context-and-assignments"
title: "Preserve routine context and required assignments"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-SCHEDULER-001
acceptance_criteria:
  - AC-OFFICE-SCHEDULER-001.8
  - AC-OFFICE-SCHEDULER-001.16
  - AC-OFFICE-SCHEDULER-001.17
  - AC-OFFICE-SCHEDULER-001.18
system_design:
  - ../../specs/office/system-design/scheduler-01.md
---

# Task 01: Preserve routine context and required assignments

## Summary

A taskless routine wake built its prompt without the registered routine's
identity or work description, so an agent could mistake recurring routine work
for generic monitoring. A task created in an Office workspace's system-owned
builtin Routine workflow did not project as Office. A routine-source wake could
coalesce into another routine's in-flight run. On the earlier head, the trusted
assignment minimum used the generic `required_subtask_count` key and not every
completion write passed through the guard.

The current head resolves the routine ID, name, and description into the prompt
for the routine-dispatch reasons, grants Office identity only to the
same-workspace system Routine workflow, and coalesces a routine-source wake only
into an in-flight run from the same routine. Trusted creation persists the
template's assignment minimum under the server-owned namespaced metadata key
`kandev.office_required_subtask_count` before auto-start; the legacy generic
metadata key `required_subtask_count` remains inert and cannot forge or clear
the minimum. Every task-state completion write and Done-step entry uses the task
repository transaction and is blocked until enough same-workspace,
non-ephemeral, durable child tasks have a runner seat on that child. The
workflow flag heal applies only to the Office Default template.

## In scope

- `apps/backend/internal/office/routines/service.go`,
  `task_context.go`: trusted routine task options and template parsing for
  `project_id`, `blocked_by`, and the assignment minimum.
- `apps/backend/internal/backendapp/adapters_office.go` and `main.go`: pass the
  options into trusted task creation without letting generic callers forge them.
- `apps/backend/internal/office/service/prompt_builder.go`,
  `prompt_routine_context.go`, `scheduler_integration.go`: routine work contract
  in the prompt, scoped to routine dispatch reasons.
- `apps/backend/internal/office/wakeup/dispatcher.go`: coalesce a
  routine-source wake only into an in-flight run from the same routine.
- `apps/backend/internal/task/models/task_metadata_update.go`,
  `task/repository/sqlite/task_metadata_merge.go`: protect the server-owned
  namespaced assignment minimum from generic metadata updates; leave the legacy
  generic key inert.
- `apps/backend/internal/task/repository/sqlite/routine_required_subtasks.go`,
  `completion_gates.go`, `completion_gate_batch.go`, `workflow.go`, `task.go`:
  namespaced minimum read and the guarded completion transaction for every
  task-state write and Done-step entry.
- `apps/backend/internal/office/shared/task_state_writer.go`,
  `office/service/service.go`, `office/service/tree_controls.go`: route state
  writes through the guarded task repository.
- `apps/backend/internal/task/repository/sqlite/defaults.go`: keep configured
  Routine workflow step flags during the builtin heal.
- `docs/specs/office/requirements/scheduler.md`: AC-001.8, AC-001.16,
  AC-001.17, and AC-001.18 at this head.
- `docs/specs/office/system-design/scheduler-01.md` and
  `docs/public/office-routines.md`: current same-routine coalescing and trusted
  minimum wording.

## Out of scope

- Replaying existing runs or reopening completed tasks.
- Changing concurrency policy, provider routing, or attachment behavior.
- Reinterpreting the legacy generic `required_subtask_count` key as the trusted
  minimum, or adding a new contract or metadata key.
- Review-thread changes or any public PR update.

## Verification

- The PR head adds or updates focused tests for the adapter, routine service and
  prompt contract, namespaced requirement protection, completion guards,
  same-routine coalescing, Office identity, and builtin workflow heal.
- This delivery package adds documentation only. Coverage is proven by the
  repository's `.github/scripts/pr-docs.cjs` validator reading this work order,
  its sibling plan, `REQ-OFFICE-SCHEDULER-001`, and the referenced design at the
  PR head. The docs delta does not rerun product tests or CI, and a local pass is
  not a claim of live CI success.

## Changed files

The PR head changes 50 paths. The behavior areas are:

- `apps/backend/internal/backendapp/` (adapter and wiring)
- `apps/backend/internal/office/` (dashboard, repository, routines, service,
  shared state writer, wakeup dispatcher, AGENTS.md)
- `apps/backend/internal/task/` (models, repository, service)
- `docs/public/office-routines.md`
- `docs/specs/office/requirements/scheduler.md`
- `docs/specs/office/system-design/scheduler-01.md`
