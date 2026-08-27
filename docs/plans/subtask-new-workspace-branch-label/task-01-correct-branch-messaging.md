---
id: "01-correct-branch-messaging"
title: "Correct branch messaging"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/office/requirements/tasks.md"
---

# Task 01: Correct Branch Messaging

## Intent

Remove the parent-session branch indicator from the isolated subtask workspace
path while preserving it for inherited workspaces.

## Acceptance

- `inherit_parent` continues to identify the active parent worktree branch.
- `new_workspace` never renders the parent branch indicator, including when the
  parent repository remains selected.
- Repository selection, base-branch selection, parent repository seeding,
  executor selection, and task-creation payloads remain unchanged.

## TDD sequence

1. Extend `new-subtask-form-parts.test.tsx` with inherited and isolated mode
   assertions. Run the focused test and confirm the isolated assertion fails
   because the current code renders the legacy badge.
2. Remove the new-workspace badge path and obsolete presentation-only props and
   helper logic.
3. Re-run the focused test, typecheck, and i18n ratchet.

## Files likely touched

- `apps/web/components/task/new-subtask-form-parts.tsx`
- `apps/web/components/task/new-subtask-dialog.tsx`
- `apps/web/components/task/new-subtask-form-parts.test.tsx`

## Dependencies

None.

## Parallelism

`sequential`

## Inputs

- `docs/specs/office/requirements/tasks.md`, section B and Handoff scenarios.
- `docs/plans/subtask-new-workspace-branch-label/plan.md`, Frontend and Tests.
- Existing `WorkspaceSection`, `WorktreeBadge`, and `SubtaskFormBody` rendering
  in `new-subtask-form-parts.tsx`.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm test -- components/task/new-subtask-form-parts.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:ratchet)
```

## Output contract

Report the red and green focused-test results, changed files, typecheck and
i18n outcomes, blockers, risks, and the synchronized task/plan status.

## Results

- RED: `cd apps/web && pnpm test -- components/task/new-subtask-form-parts.test.tsx` failed as expected because isolated mode rendered `Same branch as current session`.
- GREEN: the same focused suite passed with 2 tests after removing the isolated-workspace badge path.
- Typecheck: `cd apps/web && pnpm run typecheck` passed.
- i18n: `cd apps/web && pnpm run i18n:ratchet` passed.
- Changed files: `apps/web/components/task/new-subtask-form-parts.tsx`, `apps/web/components/task/new-subtask-form-parts.test.tsx`, and the presentation-only call-site adjustment in `apps/web/components/task/new-subtask-dialog.tsx`.
- No backend, payload, repository-seeding, base-branch, or worktree-generation behavior changed.
