---
id: "02-prove-responsive-behavior"
title: "Prove responsive behavior"
status: done
wave: 2
depends_on: ["01-correct-branch-messaging"]
plan: "plan.md"
spec: "../../specs/office/requirements/tasks.md"
---

# Task 02: Prove Responsive Behavior

## Intent

Prove the corrected branch messaging through the shipped desktop and mobile
subtask creation entry points.

## Acceptance

- Desktop E2E shows the parent branch indicator for inherited mode, removes it
  after selecting a new workspace, and keeps the repository picker visible.
- Mobile E2E reaches the same dialog from the Tasks sheet and proves the same
  outcome without changing the existing mobile composition.
- Both scenarios run against a rebuilt production web bundle.

## Files likely touched

- `apps/web/e2e/tests/task/subtask.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts`

## Dependencies

- Task 01.

## Parallelism

`sequential`

## Inputs

- Task 01's completed UI behavior.
- Existing desktop subtask creation coverage in `subtask.spec.ts`.
- Existing mobile New Subtask flow in `mobile-sidebar-task-actions.spec.ts`.
- Mobile parity contract in the implementation plan.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
make build-web
(cd apps/web && pnpm e2e:raw tests/task/subtask.spec.ts --project=chromium --grep "create subtask from the sidebar task context menu")
(cd apps/web && pnpm e2e:raw tests/task/mobile-sidebar-task-actions.spec.ts --project=mobile-chrome --grep "opens create subtask from the mobile task actions menu")
```

## Output contract

Report exact desktop/mobile commands and outcomes, rebuilt artifact evidence,
changed files, cleanup or teardown evidence, blockers, risks, and the
synchronized task/plan status.

## Results

- Production bundle: `make build-web` passed. The unchanged backend binary and E2E fixture plugin package were also generated because this fresh worktree did not have them.
- Desktop: `cd apps/web && pnpm e2e:raw tests/task/subtask.spec.ts --project=chromium --grep "create subtask from the sidebar task context menu"` passed (1 test).
- Mobile: `cd apps/web && pnpm e2e:raw tests/task/mobile-sidebar-task-actions.spec.ts --project=mobile-chrome --grep "opens create subtask from the mobile task actions menu"` passed (1 test).
- The mobile scenario uses the existing Tasks drawer and touch toggle path. No responsive composition, scrolling, safe-area, or touch-target changes were required.
- The focused runs used isolated fixture backends and completed their normal teardown.
