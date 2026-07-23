---
id: "01-workspace-repository-marker"
title: "Mark selected workspace repositories"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/multi-branch/spec.md"
---

# Task 01: Mark selected workspace repositories

## Acceptance

- A workspace or discovered-on-disk repository selected in another task row remains present and displays `Already added` in the current row's picker.
- A row's own selection is not marked unless another row also selects the same normalized repository identity; changing or removing the other row clears the marker on rerender.
- Quick chat continues to hide repositories selected by another context row, and marked task-dialog options remain selectable for intentional multi-branch use.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- components/task-create-dialog-repo-chips.test.tsx`
- `cd apps && pnpm --filter @kandev/web e2e -- e2e/tests/task/mobile-create-task-repository-selection.spec.ts --project=mobile-chrome`

## Files likely touched

- `apps/web/components/task-create-dialog-workspace-repo-chips.tsx`
- `apps/web/components/task-create-dialog-repo-chips.tsx`
- `apps/web/components/task-create-dialog-repo-chips.test.tsx`
- `apps/web/e2e/tests/task/mobile-create-task-repository-selection.spec.ts`
- `docs/plans/mark-selected-task-repositories/task-01-workspace-repository-marker.md`

## Inputs

- `docs/specs/tasks/multi-branch/spec.md`, Frontend and Task-creation scenarios.
- `docs/plans/mark-selected-task-repositories/plan.md`, Workspace and discovered repositories plus Mobile design contract.
- Existing `WorkspaceRepoChips` duplicate policy used by quick chat.

## Dependencies

None.

## Output contract

Update only this task file's status, not `plan.md`. Report the behavior implemented, files changed, tests and E2E commands run, blockers, and remaining risks. Follow `apps/web/AGENTS.md`, `/tdd`, `/e2e`, and `/mobile-parity`; do not spawn subagents or broaden scope.
