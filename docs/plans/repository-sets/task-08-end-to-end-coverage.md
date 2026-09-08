---
id: "08-end-to-end-coverage"
title: "End-to-end coverage for repository sets"
status: done
wave: 7
depends_on: ["05-apply-set-in-picker", "06-save-selection-as-set", "07-settings-management"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/repository-sets.md"
---

# Task 08: End-To-End Coverage For Repository Sets

## Acceptance

- A desktop spec seeds a workspace with two repositories and a set containing both, then: applies the
  set from the create dialog and asserts two `repo-chip` rows appear with branch pills defaulted;
  submits and asserts the created task carries both repository ids; applies the same set again and
  asserts the row count is unchanged; and saves the current selection as a new set and asserts it
  appears in the Sets list.
- A mobile spec covers applying a set through the phone sheet and asserts the same resulting rows.
- A settings spec covers create, edit-members, reorder, and delete of a set, and asserts a deleted
  repository disappears from its set without removing the set.
- Every wait uses a `causal-waits.ts` primitive armed before the action, or `expect.poll` against
  `e2e/helpers/api-client.ts`; no raw `page.waitForTimeout` or hand-picked assertion timeout.

## Verification

```bash
cd apps/web && pnpm e2e:raw --grep "repository set"
```

## Files likely touched

- `apps/web/e2e/tests/task/create-task-repository-sets.spec.ts` (new)
- `apps/web/e2e/tests/task/mobile-create-task-repository-sets.spec.ts` (new)
- `apps/web/e2e/tests/settings/workspace-repository-sets.spec.ts` (new)
- `apps/web/e2e/helpers/api-client.ts` (seeding helpers for sets, if absent)

## Dependencies

Tasks 05, 06, and 07 for the surfaces under test.

## Inputs

- Spec: Applying a set, Defining a set, Failure modes.
- Patterns: `e2e/tests/task/subtask.spec.ts:929` for the multi-repository creation flow and the
  "created task carries both repository ids" assertion; `mobile-create-task-repository-selection.spec.ts`
  for the phone chip flow; `e2e/README.md` and `e2e/helpers/causal-waits.ts` for the wait rules.
- Reuse existing test ids (`create-task-dialog`, `repo-chips-row`, `repo-chip`, `repo-chip-trigger`,
  `branch-chip-trigger`, `add-repository`) plus the ids added in tasks 05 to 07.

## Risks

- `watchWs(page)` only observes sockets opened after it is called, so it must precede the first
  `page.goto()`.
- Applying a set writes nothing, so there is no backend event to wait on for the apply itself; wait on
  the set-list fetch or the seeded state instead, and assert the rows with default timeouts.

## Output contract

Summary, files changed, tests run with results, blockers, risks, divergence from the plan, and
task/plan status updates.
