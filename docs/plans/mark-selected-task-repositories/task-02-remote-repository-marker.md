---
id: "02-remote-repository-marker"
title: "Mark selected Remote repositories"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/multi-branch/spec.md"
---

# Task 02: Mark selected Remote repositories

## Acceptance

- A provider-backed repository selected in another Remote row remains present and displays `Already added` in the current row's provider picker.
- Matching prefers provider/id metadata and uses normalized repository URL fallback for pasted or incomplete row metadata without marking unrelated repositories.
- A row's own selection is not marked unless another row selects the same repository; marked options remain touch-accessible and selectable.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- components/task-create-dialog-remote-repo-chip.test.tsx components/task-create-dialog-remote-repo-chips.test.tsx`
- `cd apps && pnpm --filter @kandev/web e2e -- e2e/tests/task/mobile-create-task-remote-repo.spec.ts --project=mobile-chrome`

## Files likely touched

- `apps/web/components/task-create-dialog-remote-repo-chips.tsx`
- `apps/web/components/task-create-dialog-remote-repo-chip.tsx`
- `apps/web/components/task-create-dialog-remote-repo-chip.test.tsx`
- `apps/web/components/task-create-dialog-remote-repo-chips.test.tsx`
- `apps/web/e2e/tests/task/mobile-create-task-remote-repo.spec.ts`
- `docs/plans/mark-selected-task-repositories/task-02-remote-repository-marker.md`

## Inputs

- `docs/specs/tasks/multi-branch/spec.md`, Frontend and Task-creation scenarios.
- `docs/plans/mark-selected-task-repositories/plan.md`, Remote provider repositories plus Mobile design contract.
- Existing `TaskRemoteRepoRow` picker metadata and `RemoteRepository` identity fields.

## Dependencies

None.

## Output contract

Update only this task file's status, not `plan.md`. Report the behavior implemented, files changed, tests and E2E commands run, blockers, and remaining risks. Follow `apps/web/AGENTS.md`, `/tdd`, `/e2e`, and `/mobile-parity`; do not spawn subagents or broaden scope.
