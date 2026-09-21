---
id: "01-validate-inherited-repository-selections"
title: "Validate inherited repository selections"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-MCP-WORKSPACE-MODE-004
acceptance_criteria:
  - AC-TASKS-MCP-WORKSPACE-MODE-004.5
system_design:
  - ../../specs/tasks/system-design/mcp-workspace-mode.md
---

# Task 01: Validate Inherited Repository Selections

## Summary

Add creation-time admission for explicit repository selections combined with
`inherit_parent`. Reject selections that cannot map to exactly one active
repository/branch slot in the parent's materialized environment before any task
or session is created.

## In scope

- Add task-service validation over parent environment repository inventory.
- Invoke it from MCP task creation before mutation.
- Return `new_workspace` guidance for incompatible selections.
- Add mismatch and no-side-effect regression coverage.

## Out of scope

- Automatic workspace-mode conversion.
- Existing task repair or migration.
- Launch-time inventory matching changes.
- Frontend changes.

## Acceptance

- An explicit `repository@main` selection is rejected when the inherited
  environment contains that repository only on feature-branch slots.
- One active exact slot is accepted; missing, failed, deleted, or multiple
  matches are rejected.
- Rejection occurs before task and session creation and contains no filesystem
  path or worktree identity.

## Verification

```bash
(cd apps/backend && go test ./internal/task/service -run '^TestValidateInheritedWorkspaceRepositorySelection' -count=1)
(cd apps/backend && go test ./internal/mcp/handlers -run '^TestHandleCreateTask_InheritParent.*Repository' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- apps/backend/internal/task/service apps/backend/internal/mcp/handlers docs/specs/tasks docs/plans/inherit-parent-repository-admission
```

## Files touched

- `apps/backend/internal/task/service/inherited_workspace_admission.go`
- `apps/backend/internal/task/service/inherit_parent_repository_admission_test.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/handlers_test.go`
- `docs/specs/tasks/requirements/mcp-workspace-mode.md`
- `docs/specs/tasks/system-design/mcp-workspace-mode.md`

## Dependencies

None.

## Risks

- The task-service matcher must stay aligned with launch branch identity rules.

## Parallelism

`sequential`

## Inputs

- MCP workspace-mode requirements and system design.
- Canonical inventory matching in the orchestrator executor.
- Existing MCP subtask repository-resolution tests.

## Results

- RED: the service regression failed to compile because the admission method
  did not exist.
- GREEN: service and MCP handler regressions pass after the guard was added.
