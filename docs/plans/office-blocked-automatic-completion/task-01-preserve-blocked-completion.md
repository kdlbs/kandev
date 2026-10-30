---
id: "01-preserve-blocked-completion"
title: "Preserve blocked outcomes during automatic completion"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-TASKS-001
acceptance_criteria:
  - AC-OFFICE-TASKS-001.1
  - AC-OFFICE-TASKS-001.9
system_design:
  - ../../specs/office/system-design/tasks-02.md
---

# Task 01: Preserve blocked outcomes during automatic completion

## Summary

The automatic terminal-step path read a task, treated only terminal states as
final, and then wrote COMPLETED. A task marked BLOCKED could therefore be
overwritten by the automatic completion that follows a turn end or completed
children, and a concurrent state or workflow-step change could land between the
read and the write.

The repair keeps BLOCKED Office tasks waiting and makes the automatic terminal
write a compare-and-set on the observed state and workflow step. A stale write
returns `ErrAutomaticCompletionStale` and publishes no completion side effects.
Manual status changes, approval redirects, and non-Office completion keep their
existing paths.

## In scope

- `apps/backend/internal/office/dashboard/task_status_persistence.go`: split
  automatic terminal persistence from the existing manual and step-guarded
  paths.
- `apps/backend/internal/office/repository/sqlite/tasks.go`: add the
  state-and-step compare-and-set update used only by the automatic path.
- `apps/backend/internal/orchestrator/event_handlers_children_completed.go` and
  `event_handlers_workflow.go`: stop the automatic terminal and turn-complete
  transitions for a BLOCKED Office task and treat a stale automatic write as a
  no-op instead of a failure.
- `apps/backend/internal/office/dashboard/service.go` and `service_tasks.go`:
  carry the expected state and workflow step on the status request.

## Out of scope

- Reopening completed tasks, replaying history, or migrating stored rows.
- Changing manual completion, approval redirects, or non-Office tasks.
- Review-thread changes or any public PR update.

## Verification

- The PR head's existing checks passed for the touched packages; the failing
  job on this head was only `Publish PR documentation coverage status`.
- This delivery package adds documentation only. Coverage is proven by the
  repository's `.github/scripts/pr-docs.cjs` validator reading this work order,
  its sibling plan, `REQ-OFFICE-TASKS-001`, and the referenced design at the PR
  head.
- No new permanent test is introduced by this delta. Temporary integration
  scenarios used while preparing the head confirmed blocked turn-end/terminal
  handling and the state-plus-step compare-and-set; they are not part of the
  repository.

## Changed files

- `apps/backend/internal/office/dashboard/service.go`
- `apps/backend/internal/office/dashboard/service_tasks.go`
- `apps/backend/internal/office/dashboard/task_status_persistence.go`
- `apps/backend/internal/office/repository/sqlite/tasks.go`
- `apps/backend/internal/orchestrator/event_handlers_children_completed.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
