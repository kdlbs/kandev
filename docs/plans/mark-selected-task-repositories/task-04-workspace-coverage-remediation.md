---
id: "04-workspace-coverage-remediation"
title: "Prove workspace marker exclusion"
status: done
wave: 2
depends_on: ["01-workspace-repository-marker"]
plan: "plan.md"
spec: "../../specs/tasks/multi-branch/spec.md"
---

# Task 04: Prove workspace marker exclusion

## Acceptance

- A component regression test reopens the only selected workspace row and proves its own option is not marked `Already added`.
- Rerender coverage proves a marker disappears when the selecting sibling changes or is removed, including normalized discovered-path identity where practical.
- Existing quick-chat hide and marked-but-selectable task-dialog tests remain green.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- components/task-create-dialog-repo-chips.test.tsx`

## Files likely touched

- `apps/web/components/task-create-dialog-repo-chips.test.tsx`
- `docs/plans/mark-selected-task-repositories/task-04-workspace-coverage-remediation.md`

## Inputs

- Code-review blocker 3 and QA coverage report.
- Existing `WorkspaceRepoChips` component tests.

## Dependencies

Task 01.

## Output contract

Update only this task file's status, not `plan.md`. Report the scenarios covered, files changed, commands/results, blockers, and remaining gaps. Follow `apps/web/AGENTS.md` and `/tdd`; do not spawn subagents or broaden scope.

## Completion notes

- Added workspace-selector regressions for reopening the only selected row without a self-marker, and for clearing a workspace marker when the selecting sibling changes and when that sibling is removed.
- Extended the existing discovered-repository regression to preserve normalized trailing-slash identity, then clear the marker after a sibling changes paths and again after that sibling is removed.
- Preserved the existing quick-chat hide-selected and task-dialog marked-but-selectable coverage.
- Prove-It note: the prerequisite implementation was already integrated when this test packet began, so the behavioral regressions were green on their first runnable assertion. The initial test invocation did fail only because this suite lacks the Jest-DOM `toHaveAccessibleName` matcher; it was immediately replaced with a plain Vitest `textContent` assertion. No production code was changed.
- Verification: `cd apps && pnpm --filter @kandev/web test -- components/task-create-dialog-repo-chips.test.tsx` — passed (1 file, 20 tests).
- Blockers: none. Remaining gap: browser/mobile rendering remains owned by the separate E2E task.
