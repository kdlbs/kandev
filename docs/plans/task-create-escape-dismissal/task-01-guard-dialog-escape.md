---
id: "01-guard-dialog-escape"
title: "Guard Create Task Escape dismissal"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/task-create-escape-dismissal.md"
---

# Task 01: Guard Create Task Escape Dismissal

## Intent

Keep the Create Task dialog open for every Escape press.
Keep nested autocomplete dismissal and non-create dialog behavior unchanged.

## Inputs

- `docs/specs/tasks/requirements/task-create-escape-dismissal.md`
- `docs/plans/task-create-escape-dismissal/plan.md`
- `apps/web/components/task-create-dialog.tsx`
- `apps/web/components/task-create-dialog.test.tsx`
- `apps/web/e2e/tests/task/task-create-prompt-autocomplete-qa.spec.ts`
- `apps/web/e2e/tests/task/mobile-create-task-webkit-rendering.spec.ts`

## Acceptance

- Escape never closes `TaskCreateDialog` in create mode.
- Escape closes an open prompt autocomplete menu and preserves the draft.
- Edit Task and New Agent keep their current Escape behavior.
- Desktop and mobile E2E tests prove the create-mode contract.

## TDD sequence

1. Add the component and E2E regression assertions.
2. Run the focused tests and record the expected failures.
3. Add the create-mode `onEscapeKeyDown` guard.
4. Run the same tests and record their successful results.

## Files likely touched

- `apps/web/components/task-create-dialog.tsx`
- `apps/web/components/task-create-dialog.test.tsx`
- `apps/web/e2e/tests/task/task-create-prompt-autocomplete-qa.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-create-escape.spec.ts`
- `docs/plans/task-create-escape-dismissal/plan.md`
- `docs/plans/task-create-escape-dismissal/task-01-guard-dialog-escape.md`

## Dependencies

None.

## Parallelism

`sequential`. The test and production changes share the dialog contract.

## Verification

If workspace dependencies are absent, run this bootstrap once:

```bash
cd apps && pnpm install --frozen-lockfile
```

Run the component test:

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/task-create-dialog.test.tsx
```

Run the desktop E2E scenarios with a fresh production build:

```bash
cd apps/web && pnpm e2e:run tests/task/task-create-prompt-autocomplete-qa.spec.ts -- --grep "Escape"
```

Run the mobile E2E scenario with a fresh production build:

```bash
cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-create-escape.spec.ts
```

Run the frontend type and lint checks:

```bash
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
```

## Risks

- An unconditional shared-dialog guard can change Edit Task and New Agent behavior.
- Stopping propagation can keep the autocomplete menu open.
- A visibility-only E2E assertion can pass during the close animation.

## Output contract

Report the changed files, RED and GREEN results, blockers, and remaining risks.
Update this task status and the plan checkbox in the same conversation.

## Results

- RED: `task-create-dialog.test.tsx` failed 3 Escape contract tests as expected.
- RED: the focused desktop E2E run failed both Escape scenarios with
  `data-state="closed"`.
- GREEN: `task-create-dialog.test.tsx` passed 7 tests.
- GREEN: the focused desktop E2E run passed 2 tests.
- GREEN: `mobile-task-create-escape.spec.ts` passed 1 test in `mobile-chrome`.
- GREEN: frontend typecheck passed.
- GREEN: frontend lint passed.
- Fresh desktop and mobile PR screenshots were captured with synthetic E2E data.
