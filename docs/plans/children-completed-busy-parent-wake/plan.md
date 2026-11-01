---
created: 2026-10-20
status: implemented
requirements:
  - REQ-TASKS-SUBTASK-COMPLETION-TRIGGER-001
system_design:
  - ../../specs/tasks/system-design/subtask-completion-busy-parent-wake.md
legacy_specs: []
---

# Implementation Plan: Children-Completed Busy Parent Wake

## Overview

Deliver the parent's `on_children_completed` auto-start when the last child
completes while the parent is still in a turn
([issue #4610](https://github.com/kdlbs/kandev/issues/4610)), following the
[owning design](../../specs/tasks/system-design/subtask-completion-busy-parent-wake.md).

## Evidence

On `main` at `77247fa20`, `autoStartAgentCallback` ignores
`AutoStartAgentAction.QueueIfBusy` and returns the `LaunchSession` error for a
running parent. `evaluateChildrenCompleted` then reports failure, the operation
is not marked applied, and no later event re-evaluates it, so the parent is
never started again.

## Scope

### In scope

- The busy parent failure mode and scenario under
  `AC-TASKS-SUBTASK-COMPLETION-TRIGGER-001.1`.
- `autoStartAgentCallback`, its pre-acceptance busy check, and the drain after
  a turn that ends while the prompt is queued.

### Out of scope

- The `on_enter` auto-start path and the Office parent wake reconciler.
- A level-triggered reconciler for Kanban parents.

## Work orders

- [x] [Task 01: Queue the auto-start for a busy parent](task-01-queue-busy-parent-auto-start.md)

## Verification

```bash
(cd apps/backend && go test -count=1 ./internal/orchestrator/... ./internal/workflow/...)
bash scripts/lint-go-changed <changed Go files>
python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all
```

## Verification results

Author-reported results and contributor-review consumer validation are
recorded in the work order.

## Risks

- Any engine-run generic `auto_start_agent` (not only `on_children_completed`)
  now queues instead of failing when the session is mid-turn. That matches the
  compiled `QueueIfBusy` flag and the `on_enter` behavior.
