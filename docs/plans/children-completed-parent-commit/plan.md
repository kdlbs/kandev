---
created: 2026-10-19
status: implemented
requirements:
  - REQ-TASKS-SUBTASK-COMPLETION-TRIGGER-002
system_design:
  - ../../specs/tasks/system-design/subtask-completion-parent-commit.md
legacy_specs: []
---

# Implementation Plan: Children-Completed Parent Commit

## Overview

Persist the parent's `on_children_completed` move when the last child completes
in its own agent turn ([issue #4553](https://github.com/kdlbs/kandev/issues/4553)),
following the [owning design](../../specs/tasks/system-design/subtask-completion-parent-commit.md).

## Evidence

On `main` at `b544f15d8`, `processOnChildrenCompleted` runs with the child's
turn context. `workflowStore.updateTransitionTask` commits the parent's move with
the child's workflow delivery effect, whose key the child's own move already
inserted, so the repository returns `applied=false`. `applyTransition` returns
`(0, nil)`, `ApplyTransition` drops the flag, and the lifecycle records history,
runs the target step's `on_enter`, and marks the operation applied while
`tasks.workflow_step_id` stays on the waiting step.

## Scope

### In scope

- `REQ-TASKS-SUBTASK-COMPLETION-TRIGGER-002` with both criteria.
- `processOnChildrenCompleted`, `applyEngineTransitionWithMode`, and the
  `workflowStore` transition commit.

### Out of scope

- The engine `Store.ApplyTransition` signature and deferred-move commits.
- Trigger detection and step actions.

## Work orders

- [x] [Task 01: Persist the parent move after an in-turn child completion](task-01-persist-parent-move.md)

## Verification

```bash
(cd apps/backend && go test -count=1 ./internal/orchestrator/ ./internal/workflow/... ./internal/task/repository/...)
bash scripts/lint-go-changed <changed Go files>
python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all
```

## Verification results

Author-reported results are recorded in the work order.

## Risks

- An engine transition whose effect-backed commit is refused now stops before
  `on_enter` instead of running it. Turn replays are already filtered by
  `workflowEffectAlreadyApplied` before evaluation, so this only changes the
  outcome for a commit that wrote nothing.
